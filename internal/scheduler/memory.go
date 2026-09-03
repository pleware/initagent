package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryScheduler is an in-memory implementation of Scheduler for testing.
// Not suitable for production (no persistence, no atomic claim across processes).
type MemoryScheduler struct {
	mu sync.RWMutex

	tasks         map[string]*Task
	queue         []string       // task IDs in queue order
	workerSlots   map[string]int // workerID -> total slots
	workerActive  map[string]int // workerID -> active tasks count
	leaseDuration time.Duration
}

// NewMemoryScheduler creates an in-memory scheduler.
func NewMemoryScheduler(leaseDuration time.Duration) *MemoryScheduler {
	return &MemoryScheduler{
		tasks:         make(map[string]*Task),
		queue:         make([]string, 0),
		workerSlots:   make(map[string]int),
		workerActive:  make(map[string]int),
		leaseDuration: leaseDuration,
	}
}

// RegisterWorker declares capacity for a worker.
func (m *MemoryScheduler) RegisterWorker(workerID string, slots int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workerSlots[workerID] = slots
	m.workerActive[workerID] = 0
}

func (m *MemoryScheduler) Enqueue(ctx context.Context, task *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if task.State != TaskQueued {
		task.State = TaskQueued
	}
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()

	m.tasks[task.ID] = task
	m.queue = append(m.queue, task.ID)
	return nil
}

func (m *MemoryScheduler) Claim(ctx context.Context, workerID string) (*Task, *Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worker has free slots
	totalSlots := m.workerSlots[workerID]
	activeSlots := m.workerActive[workerID]
	if activeSlots >= totalSlots {
		return nil, nil, ErrNoFreeSlot
	}

	// Find first eligible task in queue
	for i, taskID := range m.queue {
		task := m.tasks[taskID]
		if task == nil || task.State != TaskQueued {
			continue
		}

		// Hard constraint: owner affinity
		if task.OwnerID != "" && task.OwnerID != workerID {
			// Owner set but this worker doesn't match - skip
			// (Real impl would check worker ownership properly)
			continue
		}

		// Claim the task
		task.State = TaskAssigned
		task.AssignedWorkerID = workerID
		task.LeaseExpiry = time.Now().Add(m.leaseDuration)
		task.UpdatedAt = time.Now()

		// Remove from queue
		m.queue = append(m.queue[:i], m.queue[i+1:]...)
		m.workerActive[workerID]++

		lease := &Lease{
			TaskID:            task.ID,
			WorkerID:          workerID,
			ExpiresAt:         task.LeaseExpiry,
			HeartbeatInterval: m.leaseDuration / 3,
		}

		return task, lease, nil
	}

	return nil, nil, ErrNoFreeSlot
}

func (m *MemoryScheduler) UpdateState(ctx context.Context, taskID string, to TaskState, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := m.tasks[taskID]
	if task == nil {
		return ErrTaskNotFound
	}

	if !task.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, task.State, to)
	}

	// Release slot if transitioning away from active state
	wasActive := task.IsActive()
	task.State = to
	task.Reason = reason
	task.UpdatedAt = time.Now()

	if wasActive && !task.IsActive() {
		m.workerActive[task.AssignedWorkerID]--
	}

	return nil
}

func (m *MemoryScheduler) Heartbeat(ctx context.Context, taskID string, workerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := m.tasks[taskID]
	if task == nil {
		return ErrTaskNotFound
	}

	if task.AssignedWorkerID != workerID {
		return ErrWrongWorker
	}

	if task.LeaseExpiry.Before(time.Now()) {
		return ErrLeaseExpired
	}

	// Extend lease
	task.LeaseExpiry = time.Now().Add(m.leaseDuration)
	task.UpdatedAt = time.Now()
	return nil
}

func (m *MemoryScheduler) Get(ctx context.Context, taskID string) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task := m.tasks[taskID]
	if task == nil {
		return nil, ErrTaskNotFound
	}

	// Return a copy to avoid race conditions
	taskCopy := *task
	return &taskCopy, nil
}

func (m *MemoryScheduler) ListQueued(ctx context.Context, projectID string) ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Task
	for _, taskID := range m.queue {
		task := m.tasks[taskID]
		if task != nil && task.ProjectID == projectID && task.State == TaskQueued {
			taskCopy := *task
			result = append(result, &taskCopy)
		}
	}
	return result, nil
}
