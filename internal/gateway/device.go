package gateway

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/protocol"
)

// Device is a worker enrolled into one project. ProjectID travels with the
// row so a credential answers which project it belongs to — a gateway
// process serves many projects, so the socket cannot inherit one (18).
type Device struct {
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

// DeviceView is what the hub proxies to the cockpit.
type DeviceView struct {
	Device
	Online          bool            `json:"online"`
	Stats           *protocol.Stats `json:"stats,omitempty"`
	Tmux            bool            `json:"tmux"`
	AgentVersion    string          `json:"agentVersion,omitempty"`
	Platform        string          `json:"platform,omitempty"`
	PlatformVersion string          `json:"platformVersion,omitempty"`
	KernelVersion   string          `json:"kernelVersion,omitempty"`
}

// CreateDevice registers a worker and returns its id and plaintext credential.
func (s *Store) CreateDevice(ctx context.Context, projectID, name, hostname, osName, arch string) (deviceID, token string, err error) {
	if !id.Is(id.Project, projectID) {
		return "", "", fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	deviceID, err = id.New(id.Device)
	if err != nil {
		return "", "", err
	}
	token, err = randomToken()
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO devices (id, project_id, name, hostname, os, arch, token_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, deviceID, projectID, name, hostname, osName, arch, hashToken(token), unixTime(now))
	if err != nil {
		return "", "", fmt.Errorf("create device: %w", err)
	}
	return deviceID, token, nil
}

// DeviceByToken authenticates a connector.
func (s *Store) DeviceByToken(ctx context.Context, token string) (*Device, error) {
	return s.scanDevice(s.db.QueryRowContext(ctx, deviceSelect+` WHERE token_hash = ?`, hashToken(token)))
}

// DeviceByID loads one worker.
func (s *Store) DeviceByID(ctx context.Context, deviceID string) (*Device, error) {
	if !id.Is(id.Device, deviceID) {
		return nil, fmt.Errorf("%w: %s", ErrBadDeviceID, deviceID)
	}
	return s.scanDevice(s.db.QueryRowContext(ctx, deviceSelect+` WHERE id = ?`, deviceID))
}

// ListDevices returns every worker for a project, oldest first.
func (s *Store) ListDevices(ctx context.Context, projectID string) ([]Device, error) {
	if !id.Is(id.Project, projectID) {
		return nil, fmt.Errorf("%w: %s", ErrBadProjectID, projectID)
	}
	rows, err := s.db.QueryContext(ctx, deviceSelect+` WHERE project_id = ? ORDER BY created_at ASC, id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		d, err := scanDeviceRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDeviceOnConnect records hello fields and last_seen.
func (s *Store) UpdateDeviceOnConnect(ctx context.Context, deviceID, hostname, osName, arch string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE devices SET hostname = ?, os = ?, arch = ?, last_seen = ? WHERE id = ?
	`, hostname, osName, arch, unixTime(time.Now().UTC()), deviceID)
	return err
}

const deviceSelect = `SELECT id, project_id, name, hostname, os, arch, created_at, last_seen FROM devices`

func (s *Store) scanDevice(row *sql.Row) (*Device, error) {
	d, err := scanDeviceRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

type deviceRow interface {
	Scan(dest ...any) error
}

func scanDeviceRow(row deviceRow) (Device, error) {
	var d Device
	var created, lastSeen int64
	err := row.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Hostname, &d.OS, &d.Arch, &created, &lastSeen)
	if err != nil {
		return Device{}, err
	}
	d.CreatedAt = created
	d.LastSeen = lastSeen
	return d, nil
}
