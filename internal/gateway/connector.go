package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/protocol"
)

// Connector is a worker enrolled into one project. ProjectID travels with the
// row so a credential answers which project it belongs to — a gateway
// process serves many projects, so the socket cannot inherit one (18).
type Connector struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	IsHub     bool   `json:"isHub"`
	CreatedAt int64  `json:"createdAt"`
	LastSeen  int64  `json:"lastSeen"`
}

// ConnectorView is what the hub proxies to the cockpit.
type ConnectorView struct {
	Connector
	Online          bool            `json:"online"`
	Stats           *protocol.Stats `json:"stats,omitempty"`
	Tmux            bool            `json:"tmux"`
	AgentVersion    string          `json:"agentVersion,omitempty"`
	Platform        string          `json:"platform,omitempty"`
	PlatformVersion string          `json:"platformVersion,omitempty"`
	KernelVersion   string          `json:"kernelVersion,omitempty"`
}

// CreateConnector registers a worker and returns its id and plaintext credential.
func (s *Store) CreateConnector(ctx context.Context, projectID, name, hostname, osName, arch string) (connectorID, token string, err error) {
	if !id.Is(id.Project, projectID) {
		return "", "", fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	connectorID, err = id.New(id.Connector)
	if err != nil {
		return "", "", err
	}
	token, err = randomToken()
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO connectors (id, project_id, name, hostname, os, arch, token_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, connectorID, projectID, name, hostname, osName, arch, hashToken(token), unixTime(now))
	if err != nil {
		return "", "", fmt.Errorf("create connector: %w", err)
	}
	return connectorID, token, nil
}

// ConnectorByToken authenticates a connector.
func (s *Store) ConnectorByToken(ctx context.Context, token string) (*Connector, error) {
	return s.scanConnector(s.db.QueryRowContext(ctx, connectorSelect+` WHERE token_hash = ?`, hashToken(token)))
}

// ConnectorByID loads one worker.
func (s *Store) ConnectorByID(ctx context.Context, connectorID string) (*Connector, error) {
	if !id.Is(id.Connector, connectorID) {
		return nil, fmt.Errorf("%w: %s", ErrBadConnectorID, connectorID)
	}
	return s.scanConnector(s.db.QueryRowContext(ctx, connectorSelect+` WHERE id = ?`, connectorID))
}

// ListConnectors returns every worker for a project, oldest first.
func (s *Store) ListConnectors(ctx context.Context, projectID string) ([]Connector, error) {
	if !id.Is(id.Project, projectID) {
		return nil, fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	rows, err := s.db.QueryContext(ctx, connectorSelect+` WHERE project_id = ? ORDER BY created_at ASC, id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connector
	for rows.Next() {
		d, err := scanConnectorRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateConnectorOnConnect records hello fields and last_seen.
func (s *Store) UpdateConnectorOnConnect(ctx context.Context, connectorID, hostname, osName, arch string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE connectors SET hostname = ?, os = ?, arch = ?, last_seen = ? WHERE id = ?
	`, hostname, osName, arch, unixTime(time.Now().UTC()), connectorID)
	return err
}

const connectorSelect = `SELECT id, project_id, name, hostname, os, arch, created_at, last_seen FROM connectors`

func (s *Store) scanConnector(row *sql.Row) (*Connector, error) {
	d, err := scanConnectorRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

type connectorRow interface {
	Scan(dest ...any) error
}

func scanConnectorRow(row connectorRow) (Connector, error) {
	var d Connector
	var created, lastSeen int64
	err := row.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Hostname, &d.OS, &d.Arch, &created, &lastSeen)
	if err != nil {
		return Connector{}, err
	}
	d.CreatedAt = created
	d.LastSeen = lastSeen
	return d, nil
}
