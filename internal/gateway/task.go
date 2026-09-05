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
			exit_code, reason, command
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.ProjectID, task.OwnerID, task.ActorID, string(task.State),
		task.CoderKind, task.AssignedWorkerID, unixTime(task.LeaseExpiry),
		unixTime(task.CreatedAt), unixTime(task.UpdatedAt), task.ExitCode, task.Reason,
		task.Command)
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
	assigned_worker_id, lease_expiry, created_at, updated_at, exit_code, reason, command
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
		&t.Command,
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

// Get implements scheduler.Scheduler.
func (s *Store) Get(ctx context.Context, taskID string) (*scheduler.Task, error) {
	t, err := s.Task(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateState implements scheduler.Scheduler.
func (s *Store) UpdateState(ctx context.Context, taskID string, to scheduler.TaskState, reason string) error {
	return s.SetState(ctx, taskID, to, reason)
}

// Finish records a terminal outcome, including the exit code.
func (s *Store) Finish(ctx context.Context, taskID string, to scheduler.TaskState, exitCode int, reason string) error {
	task, err := s.Task(ctx, taskID)
	if err != nil {
		return err
	}
	if !task.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", scheduler.ErrInvalidTransition, task.State, to)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		UPDATE tasks SET state = ?, reason = ?, exit_code = ?, updated_at = ? WHERE id = ?
	`, string(to), reason, exitCode, unixTime(now), taskID)
	return err
}

// Claim atomically takes the oldest queued task for projectID onto workerID
// when the worker has a free slot (Milestone 0: one slot).
func (s *Store) Claim(ctx context.Context, projectID, workerID string, lease time.Duration) (*scheduler.Task, *scheduler.Lease, error) {
	if !id.Is(id.Project, projectID) {
		return nil, nil, fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	if !id.Is(id.Device, workerID) {
		return nil, nil, fmt.Errorf("%w: %s", ErrBadDeviceID, workerID)
	}
	if lease <= 0 {
		lease = DefaultLease
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if err := reapExpired(ctx, tx, now); err != nil {
		return nil, nil, err
	}

	var active int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM tasks
		WHERE assigned_worker_id = ? AND state IN (?, ?, ?)
	`, workerID, string(scheduler.TaskAssigned), string(scheduler.TaskRunning),
		string(scheduler.TaskAwaitingAttention)).Scan(&active)
	if err != nil {
		return nil, nil, err
	}
	if active >= workerSlots {
		return nil, nil, scheduler.ErrNoFreeSlot
	}

	// Owner affinity needs a device→account map (11). Milestone 0 claims
	// the oldest queued row for this project; owner_id is stored, not used
	// as a placement key.
	row := tx.QueryRowContext(ctx, taskSelect+`
		WHERE project_id = ? AND state = ?
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`, projectID, string(scheduler.TaskQueued))
	task, err := scanTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, scheduler.ErrNoFreeSlot
		}
		return nil, nil, err
	}

	expiry := now.Add(lease)
	res, err := tx.ExecContext(ctx, `
		UPDATE tasks SET state = ?, assigned_worker_id = ?, lease_expiry = ?, updated_at = ?
		WHERE id = ? AND state = ?
	`, string(scheduler.TaskAssigned), workerID, unixTime(expiry), unixTime(now),
		task.ID, string(scheduler.TaskQueued))
	if err != nil {
		return nil, nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, nil, err
	}
	if n != 1 {
		return nil, nil, scheduler.ErrNoFreeSlot
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	task.State = scheduler.TaskAssigned
	task.AssignedWorkerID = workerID
	task.LeaseExpiry = expiry
	task.UpdatedAt = now
	return &task, &scheduler.Lease{
		TaskID:            task.ID,
		WorkerID:          workerID,
		ExpiresAt:         expiry,
		HeartbeatInterval: lease / 3,
	}, nil
}

// Heartbeat extends the lease when the assigned worker still holds it.
func (s *Store) Heartbeat(ctx context.Context, taskID, workerID string, lease time.Duration) error {
	if !id.Is(id.Task, taskID) {
		return fmt.Errorf("%w: %s", ErrBadTaskID, taskID)
	}
	if !id.Is(id.Device, workerID) {
		return fmt.Errorf("%w: %s", ErrBadDeviceID, workerID)
	}
	if lease <= 0 {
		lease = DefaultLease
	}

	task, err := s.Task(ctx, taskID)
	if err != nil {
		return err
	}
	if task.AssignedWorkerID != workerID {
		return scheduler.ErrWrongWorker
	}
	if !task.IsActive() {
		return scheduler.ErrLeaseExpired
	}
	now := time.Now().UTC()
	if !task.LeaseExpiry.IsZero() && unixTime(task.LeaseExpiry) < unixTime(now) {
		return scheduler.ErrLeaseExpired
	}
	expiry := now.Add(lease)
	_, err = s.db.ExecContext(ctx, `
		UPDATE tasks SET lease_expiry = ?, updated_at = ? WHERE id = ?
	`, unixTime(expiry), unixTime(now), taskID)
	return err
}

func reapExpired(ctx context.Context, tx *sql.Tx, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET state = ?, assigned_worker_id = '', lease_expiry = 0, updated_at = ?, reason = ?
		WHERE state IN (?, ?)
		  AND lease_expiry > 0
		  AND lease_expiry < ?
	`, string(scheduler.TaskQueued), unixTime(now), "lease expired",
		string(scheduler.TaskAssigned), string(scheduler.TaskRunning), unixTime(now))
	return err
}

// Claim takes the oldest queued task for projectID onto workerID.
func (g *Gateway) Claim(ctx context.Context, projectID, workerID string) (*scheduler.Task, *scheduler.Lease, error) {
	return g.store.Claim(ctx, projectID, workerID, g.lease)
}

// Heartbeat refreshes the lease using this gateway's lease duration.
func (g *Gateway) Heartbeat(ctx context.Context, taskID, workerID string) error {
	return g.store.Heartbeat(ctx, taskID, workerID, g.lease)
}
