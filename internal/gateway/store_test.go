package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/scheduler"
)

func openTest(t *testing.T, projectID string) *Gateway {
	t.Helper()
	g, err := Open(Options{
		DataDir:   t.TempDir(),
		Addr:      "127.0.0.1:4201",
		ProjectID: projectID,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestOpenMintsSharedProjectID(t *testing.T) {
	g := openTest(t, "")
	if !id.Is(id.Project, g.Project().ID) {
		t.Fatalf("bound id %q is not a prj-", g.Project().ID)
	}
	if g.Project().Address != "127.0.0.1:4201" {
		t.Fatalf("address = %q", g.Project().Address)
	}
	if g.Addr() != "127.0.0.1:4201" {
		t.Fatalf("Addr = %q", g.Addr())
	}
}

func TestOpenRebindsAddress(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(Options{DataDir: dir, Addr: ":4201"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	prj := first.Project().ID
	first.Close()

	second, err := Open(Options{DataDir: dir, Addr: ":4301", ProjectID: prj})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if second.Project().ID != prj {
		t.Fatalf("project id changed: %q -> %q", prj, second.Project().ID)
	}
	if second.Project().Address != ":4301" {
		t.Fatalf("address not updated: %q", second.Project().Address)
	}
}

func TestOpenRejectsForeignProjectID(t *testing.T) {
	_, err := Open(Options{DataDir: t.TempDir(), ProjectID: "tsk-not-a-project"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenCreatesGatewayDBFile(t *testing.T) {
	dir := t.TempDir()
	g, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	g.Close()
	if _, err := os.Stat(filepath.Join(dir, brand.GatewayDBFile)); err != nil {
		t.Fatal(err)
	}
}

func TestOpenDefaultListenAddr(t *testing.T) {
	g, err := Open(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	if g.Addr() != ":4201" {
		t.Fatalf("Addr = %q", g.Addr())
	}
}

func TestBindProjectRequiresAddress(t *testing.T) {
	g := openTest(t, "")
	_, err := g.Store().BindProject(context.Background(), g.Project().ID, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnqueueKeepsCallerTaskID(t *testing.T) {
	g := openTest(t, "")
	taskID, err := id.New(id.Task)
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ID:        taskID,
		ProjectID: g.Project().ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != taskID {
		t.Fatalf("id = %q, want %q", got.ID, taskID)
	}
}

func TestScanTaskError(t *testing.T) {
	_, err := scanTask(failRow{})
	if err == nil {
		t.Fatal("expected scan error")
	}
}

type failRow struct{}

func (failRow) Scan(dest ...any) error {
	return errors.New("scan failed")
}

func TestUnixTimeRoundTrip(t *testing.T) {
	if unixTime(time.Time{}) != 0 {
		t.Fatal("zero time should encode as 0")
	}
	if !fromUnix(0).IsZero() {
		t.Fatal("0 should decode as zero time")
	}
	ts := time.Unix(1_700_000_000, 0).UTC()
	if got := fromUnix(unixTime(ts)); !got.Equal(ts) {
		t.Fatalf("round trip %v -> %v", ts, got)
	}
}

func TestOpenDataDirNotDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(Options{DataDir: path}); err == nil {
		t.Fatal("expected error")
	}
}

func TestStoreAfterClose(t *testing.T) {
	g := openTest(t, "")
	prj := g.Project().ID
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := g.Store().ListTasks(ctx, prj); err == nil {
		t.Fatal("ListTasks after close")
	}
	if _, err := g.Store().ListQueued(ctx, prj); err == nil {
		t.Fatal("ListQueued after close")
	}
	if _, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: prj}); err == nil {
		t.Fatal("Enqueue after close")
	}
}

func TestServeEmptyAddrUsesBound(t *testing.T) {
	g, err := Open(Options{DataDir: t.TempDir(), Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- g.Serve(ctx, "")
	}()
	deadline := time.Now().Add(2 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		addr := g.Addr()
		if addr != "127.0.0.1:0" {
			resp, getErr := http.Get("http://" + addr + "/health")
			if getErr == nil {
				resp.Body.Close()
				started = resp.StatusCode == http.StatusOK
				if started {
					break
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if !started {
		t.Fatal("did not serve on bound addr")
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve = %v", err)
	}
}

func TestServeListenError(t *testing.T) {
	g := openTest(t, "")
	if err := g.Serve(context.Background(), "127.0.0.1:notaport"); err == nil {
		t.Fatal("expected listen error")
	}
}

func TestServeHealthAndCancel(t *testing.T) {
	g := openTest(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- g.Serve(ctx, "127.0.0.1:0")
	}()

	var url string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		addr := g.Addr()
		if addr != "127.0.0.1:4201" && addr != "" {
			resp, err := http.Get("http://" + addr + "/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					url = "http://" + addr + "/health"
					break
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if url == "" {
		cancel()
		t.Fatal("gateway did not become healthy")
	}

	cancel()
	err := <-errc
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve = %v, want context.Canceled", err)
	}
}

func TestEnqueueAndGetTask(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()

	got, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: g.Project().ID})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if !id.Is(id.Task, got.ID) {
		t.Fatalf("task id %q is not a tsk-", got.ID)
	}
	if got.State != scheduler.TaskQueued {
		t.Fatalf("state = %q, want queued", got.State)
	}

	loaded, err := g.Store().Task(ctx, got.ID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	if loaded.ID != got.ID || loaded.ProjectID != g.Project().ID {
		t.Fatalf("loaded %+v", loaded)
	}
}

func TestEnqueueRequiresBoundProject(t *testing.T) {
	g := openTest(t, "")
	missing, err := id.New(id.Project)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Store().Enqueue(context.Background(), scheduler.Task{ProjectID: missing})
	if err != ErrProjectNotFound {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
}

func TestEnqueueRejectsBadIDs(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	if _, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: "dev-nope"}); err == nil {
		t.Fatal("expected bad project id")
	}
	if _, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: g.Project().ID, ID: "prj-as-task"}); err == nil {
		t.Fatal("expected bad task id")
	}
}

func TestSetStateFollowsMachine(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	task, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: g.Project().ID})
	if err != nil {
		t.Fatal(err)
	}

	if err := g.Store().SetState(ctx, task.ID, scheduler.TaskAssigned, "claimed"); err != nil {
		t.Fatalf("queued -> assigned: %v", err)
	}
	if err := g.Store().SetState(ctx, task.ID, scheduler.TaskRunning, "started"); err != nil {
		t.Fatalf("assigned -> running: %v", err)
	}
	if err := g.Store().SetState(ctx, task.ID, scheduler.TaskDone, "file"); err != nil {
		t.Fatalf("running -> done: %v", err)
	}
	if err := g.Store().SetState(ctx, task.ID, scheduler.TaskQueued, "no"); err == nil {
		t.Fatal("done -> queued should fail")
	}

	got, err := g.Store().Task(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != scheduler.TaskDone {
		t.Fatalf("state = %q", got.State)
	}
	if got.Reason != "file" {
		t.Fatalf("reason = %q", got.Reason)
	}
}

func TestListQueuedOrder(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	first, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: g.Project().ID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: g.Project().ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Store().SetState(ctx, second.ID, scheduler.TaskAssigned, ""); err != nil {
		t.Fatal(err)
	}

	queued, err := g.Store().ListQueued(ctx, g.Project().ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].ID != first.ID {
		t.Fatalf("queued = %+v", queued)
	}

	all, err := g.Store().ListTasks(ctx, g.Project().ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all = %d", len(all))
	}
}

func TestListEmpty(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	all, err := g.Store().ListTasks(ctx, g.Project().ID)
	if err != nil || len(all) != 0 {
		t.Fatalf("ListTasks = %v, %v", all, err)
	}
	queued, err := g.Store().ListQueued(ctx, g.Project().ID)
	if err != nil || len(queued) != 0 {
		t.Fatalf("ListQueued = %v, %v", queued, err)
	}
}

func TestTaskNotFound(t *testing.T) {
	g := openTest(t, "")
	missing, err := id.New(id.Task)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Store().Task(context.Background(), missing)
	if err != scheduler.ErrTaskNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestHealthHandler(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body Health
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.ProjectID != g.Project().ID || body.Addr != g.Addr() {
		t.Fatalf("health = %+v", body)
	}
}

func TestCloseNil(t *testing.T) {
	var g *Gateway
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	var s *Store
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectLookup(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	got, err := g.Store().Project(ctx, g.Project().ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != g.Project().ID {
		t.Fatalf("got %q", got.ID)
	}
	missing, err := id.New(id.Project)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Store().Project(ctx, missing)
	if err != ErrProjectNotFound {
		t.Fatalf("err = %v", err)
	}
	_, err = g.Store().Project(ctx, "not-an-id")
	if err == nil {
		t.Fatal("expected bad id")
	}
}

func TestListRejectsBadProject(t *testing.T) {
	g := openTest(t, "")
	if _, err := g.Store().ListTasks(context.Background(), "nope"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := g.Store().ListQueued(context.Background(), "nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetStateMissingTask(t *testing.T) {
	g := openTest(t, "")
	missing, err := id.New(id.Task)
	if err != nil {
		t.Fatal(err)
	}
	err = g.Store().SetState(context.Background(), missing, scheduler.TaskAssigned, "")
	if err != scheduler.ErrTaskNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestTaskRejectsBadID(t *testing.T) {
	g := openTest(t, "")
	if _, err := g.Store().Task(context.Background(), "prj-wrong"); err == nil {
		t.Fatal("expected error")
	}
}
