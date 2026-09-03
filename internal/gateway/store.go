// Package gateway is the project plane: one process, one SQLite file, the
// shared prj- and the tasks table the scheduler will persist into.
//
// Milestone 0 keeps this additive beside upstream. Enroll and completion
// wiring come in later slices. See drafts 02, 07, 11, and 44.
package gateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ErzenXz/overseer/internal/brand"
	"github.com/ErzenXz/overseer/internal/id"
	_ "modernc.org/sqlite"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrBadProjectID    = errors.New("project id must be a prj- identifier")
	ErrBadTaskID       = errors.New("task id must be a tsk- identifier")
)

// Project is our project on this gateway. The id is the same prj- the hub
// minted — not an alias and not a foreign tool project (fpr-).
type Project struct {
	ID        string
	Address   string
	CreatedAt time.Time
}

// Options open a gateway store and bind one project to this process.
type Options struct {
	// DataDir holds gateway.db. Empty means $HOME/.initagent.
	DataDir string
	// Addr is the listen address recorded on the project row.
	Addr string
	// ProjectID is the shared prj-. Empty mints a new one (first project,
	// started by hand).
	ProjectID string
}

// Gateway is one process: a store, a bound project, and a health handler.
type Gateway struct {
	store   *Store
	project Project
	addr    string
}

// Store is the SQLite file behind the gateway.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id         TEXT PRIMARY KEY,
	address    TEXT NOT NULL,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks (
	id                 TEXT PRIMARY KEY,
	project_id         TEXT NOT NULL,
	owner_id           TEXT NOT NULL DEFAULT '',
	actor_id           TEXT NOT NULL DEFAULT '',
	state              TEXT NOT NULL,
	coder_kind         TEXT NOT NULL DEFAULT '',
	assigned_worker_id TEXT NOT NULL DEFAULT '',
	lease_expiry       INTEGER NOT NULL DEFAULT 0,
	created_at         INTEGER NOT NULL,
	updated_at         INTEGER NOT NULL,
	exit_code          INTEGER NOT NULL DEFAULT 0,
	reason             TEXT NOT NULL DEFAULT '',
	FOREIGN KEY(project_id) REFERENCES projects(id)
);
CREATE INDEX IF NOT EXISTS tasks_project_id ON tasks(project_id);
CREATE INDEX IF NOT EXISTS tasks_project_state ON tasks(project_id, state);
`

// Open creates the data dir, opens SQLite, and binds one project.
func Open(opts Options) (*Gateway, error) {
	dir := opts.DataDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("data dir: %w", err)
		}
		dir = filepath.Join(home, brand.ConfigDir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("data dir: %w", err)
	}

	store, err := openStore(filepath.Join(dir, brand.GatewayDBFile))
	if err != nil {
		return nil, err
	}

	addr := opts.Addr
	if addr == "" {
		addr = ":4201"
	}

	projectID := opts.ProjectID
	if projectID == "" {
		minted, err := id.New(id.Project)
		if err != nil {
			store.Close()
			return nil, err
		}
		projectID = minted
	}

	project, err := store.BindProject(context.Background(), projectID, addr)
	if err != nil {
		store.Close()
		return nil, err
	}

	return &Gateway{store: store, project: project, addr: addr}, nil
}

func openStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database.
func (g *Gateway) Close() error {
	if g == nil || g.store == nil {
		return nil
	}
	return g.store.Close()
}

// Close releases the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Project returns the project bound to this process.
func (g *Gateway) Project() Project {
	return g.project
}

// Addr returns the listen address recorded on the project.
func (g *Gateway) Addr() string {
	return g.addr
}

// Store returns the persistence handle.
func (g *Gateway) Store() *Store {
	return g.store
}

func unixTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromUnix(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}
