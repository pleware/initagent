package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/protocol"
	"github.com/pleware/initagent/internal/scheduler"
)

func mustProject(t *testing.T) string {
	t.Helper()
	prj, err := id.New(id.Project)
	if err != nil {
		t.Fatal(err)
	}
	return prj
}

// asProject builds a hub-facing request naming projectID.
func asProject(method, path, projectID string, body []byte) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	}
	if projectID != "" {
		req.Header.Set(brand.ProjectHeader, projectID)
	}
	return req
}

// connectAs enrols a device into projectID and holds its socket open. It
// answers exec so a claimed task can finish.
func connectAs(t *testing.T, g *Gateway, ts *httptest.Server, projectID string) string {
	t.Helper()
	deviceID, token, err := g.Store().CreateDevice(context.Background(), projectID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/agent"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	hello, _ := protocol.NewMsg(protocol.TypeHello, 0, 0, protocol.Hello{Hostname: "box", OS: "linux"})
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatal(err)
	}
	var welcome protocol.Msg
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			var m protocol.Msg
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			if m.Type != protocol.TypeExec {
				continue
			}
			res, _ := protocol.NewMsg(protocol.TypeResult, m.Id, 0, protocol.ExecResult{ExitCode: 0, Stdout: "ok"})
			_ = conn.WriteJSON(res)
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if g.connForProject(projectID, deviceID) != nil {
			return deviceID
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("agent never attached")
	return ""
}

// A project the hub names is admitted on demand. That insert is the whole
// answer to "who provisions the second gateway": nobody does (18).
func TestNamedProjectIsAdmittedOnDemand(t *testing.T) {
	g := openTest(t, "")
	other := mustProject(t)

	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodPost, "/api/enroll-tokens", other, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var offer EnrollOffer
	if err := json.NewDecoder(rec.Body).Decode(&offer); err != nil {
		t.Fatal(err)
	}
	if offer.ProjectID != other {
		t.Fatalf("offer project = %q, want %q", offer.ProjectID, other)
	}
	stored, err := g.Store().Project(context.Background(), other)
	if err != nil || stored.ID != other {
		t.Fatalf("project row = %+v %v", stored, err)
	}
}

// EnsureProject reads before it writes, so serving a project again does not
// cost a write per request, and it keeps the recorded address.
func TestEnsureProjectIsIdempotent(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	prj := mustProject(t)

	first, err := g.Store().EnsureProject(ctx, prj, "127.0.0.1:4201")
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.Store().EnsureProject(ctx, prj, "127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Address != first.Address {
		t.Fatalf("second = %+v, first = %+v", second, first)
	}
	if _, err := g.Store().EnsureProject(ctx, "not-a-project", "x"); err == nil {
		t.Fatal("expected a bad prj- to be refused")
	}
}

func TestBadProjectHeaderIsRefused(t *testing.T) {
	g := openTest(t, "")
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodGet, "/api/devices", "tsk-wrongkind", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestNoProjectHeaderUsesTheBootstrapProject(t *testing.T) {
	g := openTest(t, "")
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/enroll-tokens", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var offer EnrollOffer
	if err := json.NewDecoder(rec.Body).Decode(&offer); err != nil {
		t.Fatal(err)
	}
	if offer.ProjectID != g.Project().ID {
		t.Fatalf("offer project = %q, want the bootstrap project", offer.ProjectID)
	}
}

// Devices are listed per project, so one project's fleet is not another's.
func TestDeviceListIsScopedToItsProject(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	other := mustProject(t)
	if _, err := g.Store().EnsureProject(ctx, other, "127.0.0.1:4201"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.Store().CreateDevice(ctx, g.Project().ID, "mine", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.Store().CreateDevice(ctx, other, "theirs", "", "", ""); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodGet, "/api/devices", other, nil))
	var views []DeviceView
	if err := json.NewDecoder(rec.Body).Decode(&views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Name != "theirs" {
		t.Fatalf("views = %+v, want only the other project's device", views)
	}
}

// The isolation guarantee in 01: a machine online for one project must not be
// picked for another project's task, even though both share the process.
func TestAnotherProjectsWorkerIsNotPicked(t *testing.T) {
	g := openTest(t, "")
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	connectAs(t, g, ts, g.Project().ID)

	empty := mustProject(t)
	if _, err := g.Store().EnsureProject(context.Background(), empty, "127.0.0.1:4201"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodPost, "/api/tasks", empty, []byte(`{"command":"true"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: a project with no worker must not borrow one", rec.Code)
	}
}

// Naming another project's dev- explicitly must not reach that machine.
func TestNamedForeignDeviceIsRefused(t *testing.T) {
	g := openTest(t, "")
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	mine := connectAs(t, g, ts, g.Project().ID)

	other := mustProject(t)
	if _, err := g.Store().EnsureProject(context.Background(), other, "127.0.0.1:4201"); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"command":"true","deviceId":"` + mine + `"}`)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodPost, "/api/tasks", other, body))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// Claim is scoped: another project's queue must not be drained onto this
// project's machine.
func TestClaimDoesNotCrossProjects(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	other := mustProject(t)
	if _, err := g.Store().EnsureProject(ctx, other, "127.0.0.1:4201"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: other, Command: "true"}); err != nil {
		t.Fatal(err)
	}

	claimed, _, err := g.Claim(ctx, g.Project().ID, mustDevice(t))
	if !errors.Is(err, scheduler.ErrNoFreeSlot) {
		t.Fatalf("Claim = %+v, %v; want nothing claimable from another project's queue", claimed, err)
	}

	// The same queue is claimable by its own project, which proves the test
	// above failed on scope rather than on an empty table.
	own, _, err := g.Claim(ctx, other, mustDevice(t))
	if err != nil || own == nil || own.ProjectID != other {
		t.Fatalf("own claim = %+v, %v", own, err)
	}
}

// A tsk- from another project answers 404 rather than the row, so task ids
// are not readable across the projects sharing this process.
func TestForeignTaskIsNotFound(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	other := mustProject(t)
	if _, err := g.Store().EnsureProject(ctx, other, "127.0.0.1:4201"); err != nil {
		t.Fatal(err)
	}
	task, err := g.Store().Enqueue(ctx, scheduler.Task{ProjectID: other, Command: "true"})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodGet, "/api/tasks/"+task.ID, g.Project().ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	own := httptest.NewRecorder()
	g.Handler().ServeHTTP(own, asProject(http.MethodGet, "/api/tasks/"+task.ID, other, nil))
	if own.Code != http.StatusOK {
		t.Fatalf("own project status = %d %s", own.Code, own.Body.String())
	}
}

// A task runs on the project that submitted it, and the row records that.
func TestTaskRunsOnItsOwnProject(t *testing.T) {
	g := openTest(t, "")
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	second := mustProject(t)
	if _, err := g.Store().EnsureProject(context.Background(), second, "127.0.0.1:4201"); err != nil {
		t.Fatal(err)
	}
	worker := connectAs(t, g, ts, second)

	body := []byte(`{"command":"true","deviceId":"` + worker + `"}`)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodPost, "/api/tasks", second, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskDone) {
		t.Fatalf("state = %q", view.State)
	}
	stored, err := g.Store().Task(context.Background(), view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ProjectID != second {
		t.Fatalf("task project = %q, want %q", stored.ProjectID, second)
	}
}

func TestProjectResolutionFailsAfterClose(t *testing.T) {
	g := openTest(t, "")
	other := mustProject(t)
	g.Close()
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, asProject(http.MethodGet, "/api/devices", other, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// --- the shared hub secret ---

func openSecured(t *testing.T, secret string) *Gateway {
	t.Helper()
	g, err := Open(Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201", HubSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestControlRoutesRequireTheSecret(t *testing.T) {
	g := openSecured(t, "shared-secret")
	controls := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/enroll-tokens"},
		{http.MethodGet, "/api/devices"},
		{http.MethodPost, "/api/tasks"},
		{http.MethodGet, "/api/tasks/tsk-1"},
	}
	for _, c := range controls {
		missing := httptest.NewRecorder()
		g.Handler().ServeHTTP(missing, httptest.NewRequest(c.method, c.path, nil))
		if missing.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a secret = %d, want 401", c.method, c.path, missing.Code)
		}

		wrong := httptest.NewRequest(c.method, c.path, nil)
		wrong.Header.Set("Authorization", "Bearer not-the-secret")
		wrongRec := httptest.NewRecorder()
		g.Handler().ServeHTTP(wrongRec, wrong)
		if wrongRec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a wrong secret = %d, want 401", c.method, c.path, wrongRec.Code)
		}
	}
}

// The secret is read from a Bearer header. A bare value is not the scheme
// the hub sends, so it must not be accepted as if it were.
func TestSecretWithoutBearerSchemeIsRefused(t *testing.T) {
	g := openSecured(t, "shared-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "shared-secret")
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCorrectSecretIsAdmitted(t *testing.T) {
	g := openSecured(t, "shared-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer shared-secret")
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

// The routes a worker or an installer uses are not behind the secret: they
// authenticate with an enroll token or a device credential, and the install
// script has to answer a machine that has no credential yet.
func TestWorkerRoutesStayOpenUnderTheSecret(t *testing.T) {
	g := openSecured(t, "shared-secret")

	health := httptest.NewRecorder()
	g.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("/health = %d", health.Code)
	}

	script := httptest.NewRequest(http.MethodGet, "/install/abc123.sh", nil)
	script.Host = "gw.example:4201"
	scriptRec := httptest.NewRecorder()
	g.Handler().ServeHTTP(scriptRec, script)
	if scriptRec.Code != http.StatusOK {
		t.Fatalf("/install = %d", scriptRec.Code)
	}

	token, err := g.Store().CreateEnrollToken(context.Background(), g.Project().ID, EnrollTTL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"token": token, "hostname": "box"})
	enroll := httptest.NewRecorder()
	g.Handler().ServeHTTP(enroll, httptest.NewRequest(http.MethodPost, "/api/enroll", bytes.NewReader(body)))
	if enroll.Code != http.StatusOK {
		t.Fatalf("/api/enroll = %d %s", enroll.Code, enroll.Body.String())
	}

	// The websocket rejects for the *device* credential, not the secret.
	ws := httptest.NewRecorder()
	g.Handler().ServeHTTP(ws, httptest.NewRequest(http.MethodGet, "/api/ws/agent", nil))
	if ws.Code != http.StatusUnauthorized || !strings.Contains(ws.Body.String(), "device token") {
		t.Fatalf("/api/ws/agent = %d %s", ws.Code, ws.Body.String())
	}
}

// An empty secret leaves the control routes open, which is the single-box
// self-host default this change must not disturb.
func TestEmptySecretLeavesControlRoutesOpen(t *testing.T) {
	g := openSecured(t, "")
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/devices", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
