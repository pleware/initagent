package scheduler

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNoFreeSlot        = errors.New("no free slot available")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrTaskNotFound      = errors.New("task not found")
	ErrLeaseExpired      = errors.New("lease expired")
	ErrWrongWorker       = errors.New("task not assigned to this worker")
)

// Scheduler manages task placement, leasing, and completion.
type Scheduler interface {
	// Enqueue adds a task to the queue.
	Enqueue(ctx context.Context, task *Task) error

	// Claim attempts to claim a free slot for the next queued task.
	// Returns the task and a lease, or ErrNoFreeSlot if none available.
	// Placement follows Draft 11: hard constraints (project, owner, coder.kind)
	// then soft preferences (clone presence, load).
	Claim(ctx context.Context, workerID string) (*Task, *Lease, error)

	// UpdateState transitions a task to a new state.
	// Returns ErrInvalidTransition if the transition is not allowed.
	UpdateState(ctx context.Context, taskID string, to TaskState, reason string) error

	// Heartbeat refreshes the lease for an active task.
	// Returns ErrLeaseExpired if the lease is already expired.
	Heartbeat(ctx context.Context, taskID string, workerID string) error

	// Get retrieves a task by ID.
	Get(ctx context.Context, taskID string) (*Task, error)

	// ListQueued returns all queued tasks for a project.
	ListQueued(ctx context.Context, projectID string) ([]*Task, error)
}

// Lease represents a time-bound claim on a worker slot.
type Lease struct {
	TaskID            string
	WorkerID          string
	ExpiresAt         time.Time
	HeartbeatInterval time.Duration // recommended interval for heartbeats
}

// IsExpired reports whether the lease has expired.
func (l *Lease) IsExpired() bool {
	return time.Now().After(l.ExpiresAt)
}

// PlacementFilter carries the hard constraints for task placement.
type PlacementFilter struct {
	ProjectID string
	OwnerID   string // empty for shared pool
	CoderKind string
	Platform  string // "linux", "darwin", "windows", or empty for any
}

// PlacementPreference carries soft preferences for task placement.
type PlacementPreference struct {
	HasClone  bool // prefer workers with the repository already cloned
	LowLoad   bool // prefer workers with lowest recent load
	HasThread bool // prefer workers with an existing thread for this todo
}
