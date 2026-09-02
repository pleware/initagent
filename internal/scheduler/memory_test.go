package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestMemoryScheduler_EnqueueAndClaim(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	s.RegisterWorker("worker-1", 2)

	task := &Task{
		ID:        "task-1",
		ProjectID: "project-1",
		State:     TaskQueued,
	}

	ctx := context.Background()

	if err := s.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	claimed, lease, err := s.Claim(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}

	if claimed.ID != "task-1" {
		t.Fatalf("claimed task ID = %s, want task-1", claimed.ID)
	}
	if claimed.State != TaskAssigned {
		t.Fatalf("claimed task state = %s, want assigned", claimed.State)
	}
	if claimed.AssignedWorkerID != "worker-1" {
		t.Fatalf("assigned worker = %s, want worker-1", claimed.AssignedWorkerID)
	}

	if lease.TaskID != "task-1" {
		t.Fatalf("lease TaskID = %s, want task-1", lease.TaskID)
	}
	if lease.WorkerID != "worker-1" {
		t.Fatalf("lease WorkerID = %s, want worker-1", lease.WorkerID)
	}
	if lease.IsExpired() {
		t.Fatal("lease is already expired")
	}
}

func TestMemoryScheduler_NoFreeSlot(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	s.RegisterWorker("worker-1", 1) // only 1 slot

	ctx := context.Background()

	// Enqueue and claim first task
	task1 := &Task{ID: "task-1", ProjectID: "project-1", State: TaskQueued}
	s.Enqueue(ctx, task1)
	s.Claim(ctx, "worker-1")

	// Try to claim second task (should fail - no free slot)
	task2 := &Task{ID: "task-2", ProjectID: "project-1", State: TaskQueued}
	s.Enqueue(ctx, task2)

	_, _, err := s.Claim(ctx, "worker-1")
	if err != ErrNoFreeSlot {
		t.Fatalf("expected ErrNoFreeSlot, got: %v", err)
	}
}

func TestMemoryScheduler_UpdateState(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	s.RegisterWorker("worker-1", 2)

	task := &Task{ID: "task-1", ProjectID: "project-1", State: TaskQueued}
	ctx := context.Background()

	s.Enqueue(ctx, task)
	s.Claim(ctx, "worker-1")

	// Transition assigned -> running
	if err := s.UpdateState(ctx, "task-1", TaskRunning, "started"); err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	got, _ := s.Get(ctx, "task-1")
	if got.State != TaskRunning {
		t.Fatalf("task state = %s, want running", got.State)
	}

	// Transition running -> done
	if err := s.UpdateState(ctx, "task-1", TaskDone, "completed"); err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	got, _ = s.Get(ctx, "task-1")
	if got.State != TaskDone {
		t.Fatalf("task state = %s, want done", got.State)
	}
}

func TestMemoryScheduler_InvalidTransition(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)

	task := &Task{ID: "task-1", ProjectID: "project-1", State: TaskQueued}
	ctx := context.Background()

	s.Enqueue(ctx, task)

	// Try invalid transition: queued -> done (must go through assigned/running)
	err := s.UpdateState(ctx, "task-1", TaskDone, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}
	// Error is wrapped, so check message contains "invalid state transition"
	errMsg := err.Error()
	if !contains(errMsg, "invalid state transition") {
		t.Fatalf("expected invalid state transition error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMemoryScheduler_GetNonExistent(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent")
	if err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound, got: %v", err)
	}
}

func TestMemoryScheduler_HeartbeatNonExistent(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	ctx := context.Background()

	err := s.Heartbeat(ctx, "nonexistent", "worker-1")
	if err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound, got: %v", err)
	}
}

func TestMemoryScheduler_HeartbeatWrongWorker(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	s.RegisterWorker("worker-1", 2)

	task := &Task{ID: "task-1", ProjectID: "project-1", State: TaskQueued}
	ctx := context.Background()

	s.Enqueue(ctx, task)
	s.Claim(ctx, "worker-1")

	// Try to heartbeat from a different worker
	err := s.Heartbeat(ctx, "task-1", "worker-2")
	if err == nil {
		t.Fatal("expected error for heartbeat from wrong worker")
	}
}

func TestLease_IsExpired(t *testing.T) {
	// Future expiry
	future := &Lease{ExpiresAt: time.Now().Add(1 * time.Hour)}
	if future.IsExpired() {
		t.Fatal("future lease reported as expired")
	}

	// Past expiry
	past := &Lease{ExpiresAt: time.Now().Add(-1 * time.Hour)}
	if !past.IsExpired() {
		t.Fatal("past lease reported as not expired")
	}
}

func TestMemoryScheduler_Heartbeat(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	s.RegisterWorker("worker-1", 2)

	task := &Task{ID: "task-1", ProjectID: "project-1", State: TaskQueued}
	ctx := context.Background()

	s.Enqueue(ctx, task)
	claimed, _, _ := s.Claim(ctx, "worker-1")

	oldExpiry := claimed.LeaseExpiry

	// Wait a bit then heartbeat
	time.Sleep(100 * time.Millisecond)

	if err := s.Heartbeat(ctx, "task-1", "worker-1"); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	got, _ := s.Get(ctx, "task-1")
	if !got.LeaseExpiry.After(oldExpiry) {
		t.Fatal("lease expiry not extended after heartbeat")
	}
}

func TestMemoryScheduler_HeartbeatExpiredLease(t *testing.T) {
	s := NewMemoryScheduler(100 * time.Millisecond) // very short lease
	s.RegisterWorker("worker-1", 2)

	task := &Task{ID: "task-1", ProjectID: "project-1", State: TaskQueued}
	ctx := context.Background()

	s.Enqueue(ctx, task)
	s.Claim(ctx, "worker-1")

	// Wait for lease to expire
	time.Sleep(150 * time.Millisecond)

	err := s.Heartbeat(ctx, "task-1", "worker-1")
	if err != ErrLeaseExpired {
		t.Fatalf("expected ErrLeaseExpired, got: %v", err)
	}
}

func TestMemoryScheduler_ListQueued(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)

	ctx := context.Background()

	s.Enqueue(ctx, &Task{ID: "task-1", ProjectID: "project-A", State: TaskQueued})
	s.Enqueue(ctx, &Task{ID: "task-2", ProjectID: "project-A", State: TaskQueued})
	s.Enqueue(ctx, &Task{ID: "task-3", ProjectID: "project-B", State: TaskQueued})

	queued, err := s.ListQueued(ctx, "project-A")
	if err != nil {
		t.Fatalf("ListQueued failed: %v", err)
	}

	if len(queued) != 2 {
		t.Fatalf("len(queued) = %d, want 2", len(queued))
	}

	ids := map[string]bool{}
	for _, task := range queued {
		ids[task.ID] = true
	}
	if !ids["task-1"] || !ids["task-2"] {
		t.Fatalf("unexpected tasks: %v", ids)
	}
}

func TestMemoryScheduler_SlotReleaseOnCompletion(t *testing.T) {
	s := NewMemoryScheduler(5 * time.Minute)
	s.RegisterWorker("worker-1", 1)

	ctx := context.Background()

	// Claim one task
	task1 := &Task{ID: "task-1", ProjectID: "project-1", State: TaskQueued}
	s.Enqueue(ctx, task1)
	s.Claim(ctx, "worker-1")

	// Try to claim another (should fail - no slot)
	task2 := &Task{ID: "task-2", ProjectID: "project-1", State: TaskQueued}
	s.Enqueue(ctx, task2)

	_, _, err := s.Claim(ctx, "worker-1")
	if err != ErrNoFreeSlot {
		t.Fatal("expected no free slot before completion")
	}

	// Complete first task
	s.UpdateState(ctx, "task-1", TaskRunning, "started")
	s.UpdateState(ctx, "task-1", TaskDone, "completed")

	// Now second task should be claimable
	claimed, _, err := s.Claim(ctx, "worker-1")
	if err != nil {
		t.Fatalf("expected slot to be free after completion, got: %v", err)
	}
	if claimed.ID != "task-2" {
		t.Fatalf("claimed task = %s, want task-2", claimed.ID)
	}
}
