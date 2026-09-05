package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/scheduler"
)

func mustDevice(t *testing.T) string {
	t.Helper()
	dev, err := id.New(id.Device)
	if err != nil {
		t.Fatal(err)
	}
	return dev
}

func enqueueQueued(t *testing.T, g *Gateway, command string) scheduler.Task {
	t.Helper()
	task, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID: g.Project().ID,
		Command:   command,
	})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestClaimAssignsOldestAndLease(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	worker := mustDevice(t)
	first := enqueueQueued(t, g, "true")
	second := enqueueQueued(t, g, "false")

	claimed, lease, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed.ID != first.ID {
		t.Fatalf("claimed %q, want oldest %q (not %q)", claimed.ID, first.ID, second.ID)
	}
	if claimed.State != scheduler.TaskAssigned {
		t.Fatalf("state = %q", claimed.State)
	}
	if claimed.AssignedWorkerID != worker {
		t.Fatalf("worker = %q", claimed.AssignedWorkerID)
	}
	if claimed.Command != "true" {
		t.Fatalf("command = %q", claimed.Command)
	}
	if lease == nil || lease.TaskID != first.ID || lease.WorkerID != worker {
		t.Fatalf("lease = %+v", lease)
	}
	if lease.IsExpired() {
		t.Fatal("fresh lease expired")
	}

	got, err := g.Store().Get(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != scheduler.TaskAssigned || got.Command != "true" {
		t.Fatalf("Get = %+v", got)
	}
}

func TestClaimNoQueued(t *testing.T) {
	g := openTest(t, "")
	_, _, err := g.Claim(context.Background(), g.Project().ID, mustDevice(t))
	if err != scheduler.ErrNoFreeSlot {
		t.Fatalf("err = %v, want ErrNoFreeSlot", err)
	}
}

func TestClaimOneSlotPerWorker(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	worker := mustDevice(t)
	enqueueQueued(t, g, "")
	enqueueQueued(t, g, "")

	if _, _, err := g.Claim(ctx, g.Project().ID, worker); err != nil {
		t.Fatal(err)
	}
	_, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != scheduler.ErrNoFreeSlot {
		t.Fatalf("err = %v, want ErrNoFreeSlot", err)
	}
}

func TestClaimTwoWorkers(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	a := enqueueQueued(t, g, "")
	b := enqueueQueued(t, g, "")
	w1, w2 := mustDevice(t), mustDevice(t)

	c1, _, err := g.Claim(ctx, g.Project().ID, w1)
	if err != nil {
		t.Fatal(err)
	}
	c2, _, err := g.Claim(ctx, g.Project().ID, w2)
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID != a.ID || c2.ID != b.ID {
		t.Fatalf("got %q then %q, want %q then %q", c1.ID, c2.ID, a.ID, b.ID)
	}
}

func TestClaimRejectsBadIDs(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	_, _, err := g.Store().Claim(ctx, "not-a-project", mustDevice(t), time.Minute)
	if !errors.Is(err, ErrBadProjectID) {
		t.Fatalf("project: %v", err)
	}
	_, _, err = g.Store().Claim(ctx, g.Project().ID, "not-a-device", time.Minute)
	if !errors.Is(err, ErrBadDeviceID) {
		t.Fatalf("device: %v", err)
	}
}

func TestWalkingSkeletonReachDone(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	worker := mustDevice(t)
	first := enqueueQueued(t, g, "echo")
	second := enqueueQueued(t, g, "")

	claimed, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Store().UpdateState(ctx, claimed.ID, scheduler.TaskRunning, "started"); err != nil {
		t.Fatal(err)
	}
	if err := g.Store().Finish(ctx, claimed.ID, scheduler.TaskDone, 0, "process"); err != nil {
		t.Fatal(err)
	}
	got, err := g.Store().Get(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != scheduler.TaskDone || got.ExitCode != 0 || got.Reason != "process" {
		t.Fatalf("done = %+v", got)
	}

	next, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatalf("slot should be free after done: %v", err)
	}
	if next.ID != second.ID {
		t.Fatalf("next = %q, want %q", next.ID, second.ID)
	}
}

func TestFinishRejectsQueued(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	task := enqueueQueued(t, g, "")
	err := g.Store().Finish(ctx, task.ID, scheduler.TaskDone, 0, "no")
	if err == nil || !errors.Is(err, scheduler.ErrInvalidTransition) {
		t.Fatalf("err = %v", err)
	}
}

func TestHeartbeatExtendsLease(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	worker := mustDevice(t)
	enqueueQueued(t, g, "")
	claimed, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	short := time.Now().UTC().Add(10 * time.Second)
	if _, err := g.Store().db.Exec(`UPDATE tasks SET lease_expiry = ? WHERE id = ?`, unixTime(short), claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := g.Heartbeat(ctx, claimed.ID, worker); err != nil {
		t.Fatal(err)
	}
	got, err := g.Store().Task(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unixTime(got.LeaseExpiry) < unixTime(time.Now().UTC())+int64(DefaultLease.Seconds())-5 {
		t.Fatalf("lease not extended: %v", got.LeaseExpiry)
	}
}

func TestHeartbeatWrongWorkerAndExpired(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	worker := mustDevice(t)
	other := mustDevice(t)
	enqueueQueued(t, g, "")
	claimed, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Heartbeat(ctx, claimed.ID, other); err != scheduler.ErrWrongWorker {
		t.Fatalf("wrong worker: %v", err)
	}
	if _, err := g.Store().db.Exec(`UPDATE tasks SET lease_expiry = 1 WHERE id = ?`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := g.Store().Heartbeat(ctx, claimed.ID, worker, time.Minute); err != scheduler.ErrLeaseExpired {
		t.Fatalf("expired: %v", err)
	}
}

func TestHeartbeatRejectsBadIDs(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	if err := g.Store().Heartbeat(ctx, "not-a-task", mustDevice(t), time.Minute); !errors.Is(err, ErrBadTaskID) {
		t.Fatalf("task: %v", err)
	}
	missing, err := id.New(id.Task)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Store().Heartbeat(ctx, missing, "not-a-device", time.Minute); !errors.Is(err, ErrBadDeviceID) {
		t.Fatalf("device: %v", err)
	}
	if err := g.Store().Heartbeat(ctx, missing, mustDevice(t), time.Minute); err != scheduler.ErrTaskNotFound {
		t.Fatalf("missing: %v", err)
	}
}

func TestHeartbeatInactiveAfterDone(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	worker := mustDevice(t)
	enqueueQueued(t, g, "")
	claimed, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Store().UpdateState(ctx, claimed.ID, scheduler.TaskRunning, "started"); err != nil {
		t.Fatal(err)
	}
	if err := g.Store().Finish(ctx, claimed.ID, scheduler.TaskDone, 0, "process"); err != nil {
		t.Fatal(err)
	}
	if err := g.Heartbeat(ctx, claimed.ID, worker); err != scheduler.ErrLeaseExpired {
		t.Fatalf("err = %v, want ErrLeaseExpired", err)
	}
}

func TestClaimReapsExpiredLease(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	worker := mustDevice(t)
	task := enqueueQueued(t, g, "")
	claimed, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != task.ID {
		t.Fatal(claimed.ID)
	}
	if _, err := g.Store().db.Exec(`UPDATE tasks SET lease_expiry = 1 WHERE id = ?`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	again, _, err := g.Claim(ctx, g.Project().ID, worker)
	if err != nil {
		t.Fatalf("reap should free the slot: %v", err)
	}
	if again.ID != task.ID {
		t.Fatalf("requeued task should be claimed again, got %q", again.ID)
	}
	got, err := g.Store().Task(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != scheduler.TaskAssigned {
		t.Fatalf("state = %q", got.State)
	}
}

func TestClaimZeroLeaseUsesDefault(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	enqueueQueued(t, g, "")
	claimed, lease, err := g.Store().Claim(ctx, g.Project().ID, mustDevice(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Now().UTC().Add(DefaultLease)
	delta := claimed.LeaseExpiry.Sub(want)
	if delta < -2*time.Second || delta > 2*time.Second {
		t.Fatalf("expiry %v, want ~%v", claimed.LeaseExpiry, want)
	}
	if lease.HeartbeatInterval != DefaultLease/3 {
		t.Fatalf("heartbeat interval = %v", lease.HeartbeatInterval)
	}
}

func TestGetAndFinishMissing(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	missing, err := id.New(id.Task)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Store().Get(ctx, missing); err != scheduler.ErrTaskNotFound {
		t.Fatalf("Get: %v", err)
	}
	if err := g.Store().Finish(ctx, missing, scheduler.TaskDone, 0, ""); err != scheduler.ErrTaskNotFound {
		t.Fatalf("Finish: %v", err)
	}
}
