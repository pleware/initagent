package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ErzenXz/overseer/internal/id"
)

// BindProject records the shared prj- on this gateway. Calling it again
// with the same id updates the advertised address — the first project is
// started by hand, and the listen address can change between restarts.
func (s *Store) BindProject(ctx context.Context, projectID, address string) (Project, error) {
	if !id.Is(id.Project, projectID) {
		return Project{}, fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	if address == "" {
		return Project{}, fmt.Errorf("project address is required")
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, address, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET address = excluded.address
	`, projectID, address, unixTime(now))
	if err != nil {
		return Project{}, fmt.Errorf("bind project: %w", err)
	}
	return s.Project(ctx, projectID)
}

// Project loads a bound project by the shared prj-.
func (s *Store) Project(ctx context.Context, projectID string) (Project, error) {
	if !id.Is(id.Project, projectID) {
		return Project{}, fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	var p Project
	var created int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, address, created_at FROM projects WHERE id = ?
	`, projectID).Scan(&p.ID, &p.Address, &created)
	if err != nil {
		if err == sql.ErrNoRows {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, err
	}
	p.CreatedAt = fromUnix(created)
	return p, nil
}
