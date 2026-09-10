package hub

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/pleware/initagent/internal/orgplan"
	"github.com/pleware/initagent/internal/store"
)

// TaskOutput is stdout/stderr from one finished task, kept on the hub
// because the org plan (and therefore the retention window) lives here.
type TaskOutput struct {
	TaskID    string `json:"taskId"`
	OrgID     string `json:"orgId"`
	ProjectID string `json:"projectId"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	CreatedAt int64  `json:"createdAt"`
}

func (s *Store) ensureTaskOutputs() error {
	createdAt := "INTEGER NOT NULL"
	if s.db.Dialect() == store.Postgres {
		createdAt = "BIGINT NOT NULL"
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS task_outputs (
		task_id     TEXT PRIMARY KEY,
		org_id      TEXT NOT NULL,
		project_id  TEXT NOT NULL,
		stdout      TEXT NOT NULL DEFAULT '',
		stderr      TEXT NOT NULL DEFAULT '',
		created_at  ` + createdAt + `
	)`); err != nil {
		return fmt.Errorf("task_outputs: %w", err)
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS task_outputs_org_created ON task_outputs(org_id, created_at)`)
	return err
}

// SaveTaskOutput stores one task's streams. The same task id is replaced,
// so a retried persist of one result is not a second row.
func (s *Store) SaveTaskOutput(out TaskOutput) error {
	if out.TaskID == "" || out.OrgID == "" || out.ProjectID == "" {
		return fmt.Errorf("task output: task, org and project are required")
	}
	if out.CreatedAt == 0 {
		out.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.Exec(`INSERT INTO task_outputs (task_id, org_id, project_id, stdout, stderr, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			org_id = excluded.org_id,
			project_id = excluded.project_id,
			stdout = excluded.stdout,
			stderr = excluded.stderr,
			created_at = excluded.created_at`,
		out.TaskID, out.OrgID, out.ProjectID, out.Stdout, out.Stderr, out.CreatedAt)
	return err
}

// TaskOutputByID returns a stored stream, or (nil, nil) when missing.
func (s *Store) TaskOutputByID(taskID string) (*TaskOutput, error) {
	var out TaskOutput
	err := s.db.QueryRow(`SELECT task_id, org_id, project_id, stdout, stderr, created_at
		FROM task_outputs WHERE task_id = ?`, taskID).
		Scan(&out.TaskID, &out.OrgID, &out.ProjectID, &out.Stdout, &out.Stderr, &out.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &out, err
}

// PurgeTaskOutputs deletes streams older than each org's logDays.
// Self-host and a catalogue 0 keep every row.
func (s *Server) runTaskOutputPurge(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	purgeOnce := func() {
		n, err := s.store.PurgeTaskOutputs(time.Now())
		if err != nil {
			log.Printf("task output purge: %v", err)
			return
		}
		if n > 0 {
			log.Printf("task output: purged %d rows past the org log window", n)
		}
	}
	purgeOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			purgeOnce()
		}
	}
}

func (s *Store) PurgeTaskOutputs(now time.Time) (int64, error) {
	orgs, err := s.ListOrgs()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, org := range orgs {
		cutoff, ok := orgplan.LogCutoff(now, orgplan.Caps(s.offering, orgplan.ID(org.Plan)).LogDays)
		if !ok {
			continue
		}
		res, err := s.db.Exec(`DELETE FROM task_outputs WHERE org_id = ? AND created_at <= ?`,
			org.Id, cutoff.Unix())
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
