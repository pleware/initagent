package scheduler

import "testing"

func TestTask_IsActive(t *testing.T) {
	tests := []struct {
		state  TaskState
		active bool
	}{
		{TaskQueued, false},
		{TaskAssigned, true},
		{TaskRunning, true},
		{TaskAwaitingAttention, true},
		{TaskDone, false},
		{TaskFailed, false},
		{TaskCancelled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			task := &Task{State: tt.state}
			if got := task.IsActive(); got != tt.active {
				t.Fatalf("IsActive() = %v, want %v for state %s", got, tt.active, tt.state)
			}
		})
	}
}

func TestTask_IsTerminal(t *testing.T) {
	tests := []struct {
		state    TaskState
		terminal bool
	}{
		{TaskQueued, false},
		{TaskAssigned, false},
		{TaskRunning, false},
		{TaskAwaitingAttention, false},
		{TaskDone, true},
		{TaskFailed, true},
		{TaskCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			task := &Task{State: tt.state}
			if got := task.IsTerminal(); got != tt.terminal {
				t.Fatalf("IsTerminal() = %v, want %v for state %s", got, tt.terminal, tt.state)
			}
		})
	}
}

func TestTask_CanTransition(t *testing.T) {
	tests := []struct {
		from TaskState
		to   TaskState
		can  bool
	}{
		// Queued
		{TaskQueued, TaskAssigned, true},
		{TaskQueued, TaskCancelled, true},
		{TaskQueued, TaskRunning, false},

		// Assigned
		{TaskAssigned, TaskRunning, true},
		{TaskAssigned, TaskQueued, true},
		{TaskAssigned, TaskCancelled, true},
		{TaskAssigned, TaskDone, false},

		// Running
		{TaskRunning, TaskDone, true},
		{TaskRunning, TaskFailed, true},
		{TaskRunning, TaskAwaitingAttention, true},
		{TaskRunning, TaskQueued, true},
		{TaskRunning, TaskCancelled, true},
		{TaskRunning, TaskAssigned, false},

		// Awaiting attention
		{TaskAwaitingAttention, TaskRunning, true},
		{TaskAwaitingAttention, TaskCancelled, true},
		{TaskAwaitingAttention, TaskFailed, true},
		{TaskAwaitingAttention, TaskDone, false},

		// Terminal states
		{TaskDone, TaskQueued, false},
		{TaskFailed, TaskRunning, false},
		{TaskCancelled, TaskAssigned, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			task := &Task{State: tt.from}
			if got := task.CanTransition(tt.to); got != tt.can {
				t.Fatalf("CanTransition(%s -> %s) = %v, want %v", tt.from, tt.to, got, tt.can)
			}
		})
	}
}
