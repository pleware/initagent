// Package gateway is the project plane: one process, one SQLite file, the
// shared project-, the tasks table, and enroll so workers dial this process
// rather than the hub. See drafts 02, 07, 10, 11, and 44.
package gateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/join"
	"github.com/pleware/initagent/internal/protocol"
	_ "modernc.org/sqlite"
)

var (
	ErrProjectNotFound   = errors.New("project not found")
	ErrBadProjectID      = errors.New("project id must be a project- identifier")
	ErrBadTaskID         = errors.New("task id must be a task- identifier")
	ErrBadConnectorID    = errors.New("connector id must be a connector- identifier")
	ErrConnectorOffline  = errors.New("connector is not connected")
	ErrConnectorDraining = errors.New("connector is draining")
	ErrEmptyCommand      = errors.New("task command is empty")
	ErrUnknownLaunch     = errors.New("unknown launch mode")
)

// EnrollTTL is how long a minted enroll token can be exchanged.
const EnrollTTL = 15 * time.Minute

// EnrollRetainFor is how long a used or expired enroll token stays on
// disk. Same window as auth.SpentRetainFor / mailer.RetainFor. Live
// unused tokens are not deleted.
const EnrollRetainFor = 30 * 24 * time.Hour

// Project is our project on this gateway. The id is the same project- the hub
// minted — not an alias and not a foreign tool project (foreign_project-).
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
	// ProjectID is the shared project-. Empty mints a new one (first project,
	// started by hand).
	ProjectID string
	// PublicURL is the URL baked into install commands when set. Empty
	// means derive http(s):// from the request Host.
	PublicURL string
	// Version is advertised on agent welcome so a connector can self-update.
	Version string
	// GithubRepo is "owner/name" used to fetch connector binaries for other
	// platforms. Empty uses brand.ReleaseSource.
	GithubRepo string
	// Lease is how long a claimed slot lasts without a heartbeat.
	// Zero means DefaultLease.
	Lease time.Duration
	// HubSecret is required on the control routes the hub calls. Empty
	// leaves them open, which is today's single-box self-host default.
	// It carries no project scope; scoped tokens are 09.
	HubSecret string
}

// DefaultLease is the Milestone 0 claim duration (capacity of one slot).
const DefaultLease = 5 * time.Minute

// workerSlots is the Milestone 0 capacity stub: one active task per device.
const workerSlots = 1

// presence is a live connector socket. projectID is copied from the connector row
// at connect so worker selection can be scoped without a query per pick — a
// task for one project must never land on another project's machine.
type presence struct {
	projectID string
	hello     protocol.Hello
	stats     *protocol.Stats
	conn      *agentConn
	// draining is set at Hello when the advertised fleet version is newer
	// than this connector. Claim skips it until the next Hello matches (`10`).
	draining bool
}

// Gateway is one process serving many projects: a store, the project this
// process was started with, enroll, and health. The project a request acts on
// arrives per request (brand.ProjectHeader); the bootstrap project below is
// what a self-host gateway started by hand answers with when no header is
// set, which is what keeps OSS a single flag (18).
type Gateway struct {
	store   *Store
	project Project
	addr    string
	// hubSecret guards the control routes. Empty means open (self-host).
	hubSecret string
	// joiner serves /install/<token> and the agent binary. It is the shared
	// implementation, not a gateway-local copy — the hub serves the same one.
	joiner join.Installer
	lease  time.Duration

	mu     sync.Mutex
	online map[string]presence
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
	command            TEXT NOT NULL DEFAULT '',
	launch_mode        TEXT NOT NULL DEFAULT 'exec',
	FOREIGN KEY(project_id) REFERENCES projects(id)
);
CREATE INDEX IF NOT EXISTS tasks_project_id ON tasks(project_id);
CREATE INDEX IF NOT EXISTS tasks_project_state ON tasks(project_id, state);
CREATE TABLE IF NOT EXISTS connectors (
	id         TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	name       TEXT NOT NULL,
	hostname   TEXT NOT NULL DEFAULT '',
	os         TEXT NOT NULL DEFAULT '',
	arch       TEXT NOT NULL DEFAULT '',
	token_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY(project_id) REFERENCES projects(id)
);
CREATE INDEX IF NOT EXISTS connectors_project_id ON connectors(project_id);
CREATE TABLE IF NOT EXISTS enroll_tokens (
	token_hash TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	used       INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY(project_id) REFERENCES projects(id)
);
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

	repo := opts.GithubRepo
	if repo == "" {
		repo = brand.ReleaseSource
	}
	lease := opts.Lease
	if lease <= 0 {
		lease = DefaultLease
	}
	return &Gateway{
		store:     store,
		project:   project,
		addr:      addr,
		hubSecret: opts.HubSecret,
		joiner: join.Installer{
			DataDir:    dir,
			GithubRepo: repo,
			Version:    opts.Version,
			PublicURL:  opts.PublicURL,
		},
		lease:  lease,
		online: map[string]presence{},
	}, nil
}

func openStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := renameInheritedDeviceTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("renaming the inherited device table: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	// Existing files created before command existed; ignore duplicate-column.
	_, _ = db.Exec(`ALTER TABLE tasks ADD COLUMN command TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE tasks ADD COLUMN launch_mode TEXT NOT NULL DEFAULT 'exec'`)
	return &Store{db: db}, nil
}

// renameInheritedDeviceTable carries a gateway file written against the
// inherited device vocabulary over to the connector one (05). It runs before
// the schema batch, and the order is the whole point: CREATE TABLE IF NOT
// EXISTS would otherwise add an empty connectors table beside the populated
// devices one, leave both in place, and every connector enrolled into this
// project would silently stop being listed.
func renameInheritedDeviceTable(db *sql.DB) error {
	hasTable := func(name string) (bool, error) {
		var n int
		err := db.QueryRow(`SELECT 1 FROM sqlite_master
			WHERE type = 'table' AND name = ?`, name).Scan(&n)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil, err
	}
	old, err := hasTable("devices")
	if err != nil {
		return err
	}
	current, err := hasTable("connectors")
	if err != nil {
		return err
	}
	if !old || current {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE devices RENAME TO connectors`); err != nil {
		return err
	}
	// The renamed table keeps its index under the old name; the schema batch
	// creates connectors_project_id.
	_, err = db.Exec(`DROP INDEX IF EXISTS devices_project_id`)
	return err
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

// Project returns the project this process was started with. It is the
// fallback for a request that names none, not the only project served.
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
