package hub

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/funnel"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/store"
)

func (s *Store) ensureFunnelEvents() error {
	occurred := "INTEGER NOT NULL"
	if s.db.Dialect() == store.Postgres {
		occurred = "BIGINT NOT NULL"
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS funnel_events (
		id          TEXT PRIMARY KEY,
		kind        TEXT NOT NULL,
		occurred_at ` + occurred + `,
		org_id      TEXT NOT NULL DEFAULT '',
		account_id  TEXT NOT NULL DEFAULT '',
		project_id  TEXT NOT NULL DEFAULT '',
		connector_id   TEXT NOT NULL DEFAULT '',
		wall        TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("funnel_events: %w", err)
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS funnel_events_kind_occurred ON funnel_events(kind, occurred_at)`)
	return err
}

// RecordFunnelEvent inserts one counting row. It is insert-only.
func (s *Store) RecordFunnelEvent(e funnel.Event) error {
	if !funnel.Known(e.Kind) {
		return fmt.Errorf("funnel event: unknown kind %q", e.Kind)
	}
	if e.ID == "" {
		rowID, err := id.New(id.Event)
		if err != nil {
			return err
		}
		e.ID = rowID
	}
	occurred := e.OccurredAt.Unix()
	if e.OccurredAt.IsZero() {
		occurred = time.Now().Unix()
	}
	_, err := s.db.Exec(`INSERT INTO funnel_events
		(id, kind, occurred_at, org_id, account_id, project_id, connector_id, wall)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Kind, occurred, e.OrgID, e.AccountID, e.ProjectID, e.ConnectorID, e.Wall)
	return err
}

// ListFunnelEvents returns every stored counting row, oldest first.
func (s *Store) ListFunnelEvents() ([]funnel.Event, error) {
	rows, err := s.db.Query(`SELECT id, kind, occurred_at, org_id, account_id, project_id, connector_id, wall
		FROM funnel_events ORDER BY occurred_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []funnel.Event{}
	for rows.Next() {
		var e funnel.Event
		var occurred int64
		if err := rows.Scan(&e.ID, &e.Kind, &occurred, &e.OrgID, &e.AccountID, &e.ProjectID, &e.ConnectorID, &e.Wall); err != nil {
			return nil, err
		}
		e.OccurredAt = time.Unix(occurred, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// FunnelFacts is the live activation side of a KPI snapshot.
func (s *Store) FunnelFacts() (funnel.Facts, error) {
	var facts funnel.Facts
	accRows, err := s.db.Query(`SELECT id, created_at, is_admin FROM accounts ORDER BY created_at, id`)
	if err != nil {
		return facts, err
	}
	defer accRows.Close()
	for accRows.Next() {
		var a funnel.AccountFact
		var created int64
		var admin int
		if err := accRows.Scan(&a.ID, &created, &admin); err != nil {
			return facts, err
		}
		a.CreatedAt = time.Unix(created, 0).UTC()
		a.IsAdmin = admin != 0
		facts.Accounts = append(facts.Accounts, a)
	}
	if err := accRows.Err(); err != nil {
		return facts, err
	}

	orgRows, err := s.db.Query(`SELECT o.id, o.created_at, o.plan,
		(SELECT COUNT(*) FROM projects p WHERE p.org_id = o.id),
		(SELECT COUNT(*) FROM project_connectors pd
			INNER JOIN projects p ON p.id = pd.project_id WHERE p.org_id = o.id),
		(SELECT MIN(created_at) FROM task_outputs t WHERE t.org_id = o.id)
		FROM orgs o ORDER BY o.created_at, o.id`)
	if err != nil {
		return facts, err
	}
	defer orgRows.Close()
	for orgRows.Next() {
		var o funnel.OrgFact
		var created int64
		var projects, connectors int
		var first sql.NullInt64
		if err := orgRows.Scan(&o.ID, &created, &o.Plan, &projects, &connectors, &first); err != nil {
			return facts, err
		}
		o.CreatedAt = time.Unix(created, 0).UTC()
		o.HasProject = projects > 0
		o.HasConnector = connectors > 0
		if first.Valid {
			o.FirstTaskAt = time.Unix(first.Int64, 0).UTC()
		}
		facts.Orgs = append(facts.Orgs, o)
	}
	return facts, orgRows.Err()
}
