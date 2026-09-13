// Package scheduler provides task placement, leasing, and completion tracking.
// See Draft 11 for design rationale.
package scheduler

import (
	"fmt"
	"strings"
	"time"
)

// TaskState represents the current state of a task in the state machine.
type TaskState string

const (
	TaskQueued            TaskState = "queued"             // waiting for a free slot
	TaskAssigned          TaskState = "assigned"           // slot claimed, run not started
	TaskRunning           TaskState = "running"            // run in progress
	TaskDone              TaskState = "done"               // completed successfully
	TaskFailed            TaskState = "failed"             // failed (nonzero exit or error)
	TaskAwaitingAttention TaskState = "awaiting_attention" // coder needs human decision
	TaskCancelled         TaskState = "cancelled"          // cancelled by a person
)

// Task represents a unit of work in the scheduling queue.
type Task struct {
	ID         string    // task ID (e.g., task-...)
	ProjectID  string    // project ID (e.g., project-...)
	OwnerID    string    // owner ID (e.g., account-...) or empty for shared pool
	ActorID    string    // persona ID (e.g., persona-...)
	State      TaskState // current state
	CoderKind  string    // coder kind (e.g., "aider", "openclaw")
	Command    string    // shell command for Milestone 0 exec dispatch
	LaunchMode string    // exec | process | send_keys; empty means exec

	// Placement
	AssignedWorkerID string    // connector ID (e.g., connector-...) when assigned
	LeaseExpiry      time.Time // when the lease expires (zero if not assigned)

	// Timing
	CreatedAt time.Time
	UpdatedAt time.Time

	// Completion
	ExitCode int    // exit code from coder (0 = success)
	Reason   string // completion reason or failure message
}

// IsActive reports whether the task holds a slot.
func (t *Task) IsActive() bool {
	return t.State == TaskAssigned || t.State == TaskRunning || t.State == TaskAwaitingAttention
}

// IsTerminal reports whether the task is in a final state.
func (t *Task) IsTerminal() bool {
	return t.State == TaskDone || t.State == TaskFailed || t.State == TaskCancelled
}

// CanTransition reports whether the task can transition to the given state.
// This implements the state machine rules from Draft 11.
func (t *Task) CanTransition(to TaskState) bool {
	switch t.State {
	case TaskQueued:
		return to == TaskAssigned || to == TaskCancelled

	case TaskAssigned:
		return to == TaskRunning || to == TaskQueued || to == TaskCancelled

	case TaskRunning:
		return to == TaskDone || to == TaskFailed || to == TaskAwaitingAttention ||
			to == TaskQueued || to == TaskCancelled

	case TaskAwaitingAttention:
		return to == TaskRunning || to == TaskCancelled || to == TaskFailed

	case TaskDone, TaskFailed, TaskCancelled:
		return false // terminal states

	default:
		return false
	}
}

const (
	LaunchExec     = "exec"
	LaunchProcess  = "process"
	LaunchSendKeys = "send_keys"
)

// NormalizeLaunch maps an empty value to exec and rejects unknown modes.
func NormalizeLaunch(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	switch mode {
	case "", LaunchExec:
		return LaunchExec, nil
	case LaunchProcess, LaunchSendKeys:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown launch mode %q", mode)
	}
}
