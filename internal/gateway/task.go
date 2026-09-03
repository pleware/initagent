package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/scheduler"
)

// Enqueue inserts a queued task for a bound project. An empty ID is minted
// as tsk-. The state is forced to queued — callers do not pick a starting
// state.
func (s *Store) Enqueue(ctx context.Context, task scheduler.Task) (scheduler.Task, error) {
	if !id.Is(id.Project, task.ProjectID) {
		return scheduler.Task{}, fmt.Errorf("%w: %s", ErrBadProjectID, task.ProjectID)
	}
	if _, err := s.Project(ctx, task.ProjectID); err != nil {
		return scheduler.Task{}, err
	}
	if task.ID == "" {
		minted, err := id.New(id.Task)
		if err != nil {
			return scheduler.Task{}, err
		}
		task.ID = minted
	}
	if !id.Is(id.Task, task.ID) {
		return scheduler.Task{}, fmt.Errorf("%w: %s", ErrBadTaskID, task.ID)
	}

	now := time.Now().UTC()
	task.State = scheduler.TaskQueued
	task.CreatedAt = now
	task.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (
			id, project_id, owner_id, actor_id, state, coder_kind,
			assigned_worker_id, lease_expiry, created_at, updated_at,
			exit_code, reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.ProjectID, task.OwnerID, task.ActorID, string(task.State),
		task.CoderKind, task.AssignedWorkerID, unixTime(task.LeaseExpiry),
		unixTime(task.CreatedAt), unixTime(task.UpdatedAt), task.ExitCode, task.Reason)
	if err != nil {
		return scheduler.Task{}, fmt.Errorf("enqueue task: %w", err)
	}
	return task, nil
}

// Task loads one task by tsk-.
func (s *Store) Task(ctx context.Context, taskID string) (scheduler.Task, error) {
	if !id.Is(id.Task, taskID) {
		return scheduler.Task{}, fmt.Errorf("%w: %s", ErrBadTaskID, taskID)
	}
	t, err := scanTask(s.db.QueryRowContext(ctx, taskSelect+` WHERE id = ?`, taskID))
	if err != nil {
		if err == sql.ErrNoRows {
			return scheduler.Task{}, scheduler.ErrTaskNotFound
		}
		return scheduler.Task{}, err
	}
	return t, nil
}

// ListTasks returns every task for a project, newest first.
func (s *Store) ListTasks(ctx context.Context, projectID string) ([]scheduler.Task, error) {
	if !id.Is(id.Project, projectID) {
		return nil, fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	rows, err := s.db.QueryContext(ctx, taskSelect+` WHERE project_id = ? ORDER BY created_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []scheduler.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListQueued returns queued tasks for a project, oldest first — the order
// a claim would walk. Satisfies the read side of scheduler.Scheduler.
func (s *Store) ListQueued(ctx context.Context, projectID string) ([]scheduler.Task, error) {
	if !id.Is(id.Project, projectID) {
		return nil, fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	rows, err := s.db.QueryContext(ctx, taskSelect+`
		WHERE project_id = ? AND state = ?
		ORDER BY created_at ASC, id ASC
	`, projectID, string(scheduler.TaskQueued))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []scheduler.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetState applies a legal transition from scheduler.CanTransition.
func (s *Store) SetState(ctx context.Context, taskID string, to scheduler.TaskState, reason string) error {
	task, err := s.Task(ctx, taskID)
	if err != nil {
		return err
	}
	if !task.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", scheduler.ErrInvalidTransition, task.State, to)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		UPDATE tasks SET state = ?, reason = ?, updated_at = ? WHERE id = ?
	`, string(to), reason, unixTime(now), taskID)
	return err
}

const taskSelect = `
SELECT id, project_id, owner_id, actor_id, state, coder_kind,
	assigned_worker_id, lease_expiry, created_at, updated_at, exit_code, reason
FROM tasks`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (scheduler.Task, error) {
	var t scheduler.Task
	var state string
	var lease, created, updated int64
	err := row.Scan(
		&t.ID, &t.ProjectID, &t.OwnerID, &t.ActorID, &state, &t.CoderKind,
		&t.AssignedWorkerID, &lease, &created, &updated, &t.ExitCode, &t.Reason,
	)
	if err != nil {
		return scheduler.Task{}, err
	}
	t.State = scheduler.TaskState(state)
	t.LeaseExpiry = fromUnix(lease)
	t.CreatedAt = fromUnix(created)
	t.UpdatedAt = fromUnix(updated)
	return t, nil
}
