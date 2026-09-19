package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
	"github.com/pleware/initagent/internal/store"
)

// Store wraps the hub's database (SQLite for self-host, Postgres for the
// hosted offering). The db handle rebinds placeholders per dialect.
type Store struct {
	db       *store.DB
	offering offering.Kind
}

const schemaSQLite = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS connectors (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	hostname   TEXT NOT NULL DEFAULT '',
	os         TEXT NOT NULL DEFAULT '',
	arch       TEXT NOT NULL DEFAULT '',
	token_hash TEXT NOT NULL,
	is_hub     INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS enroll_tokens (
	token_hash TEXT PRIMARY KEY,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	used       INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS presets (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	name    TEXT NOT NULL UNIQUE,
	command TEXT NOT NULL,
	kind    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS api_tokens (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	account_id   TEXT NOT NULL,
	org_id       TEXT NOT NULL,
	project_id   TEXT NOT NULL DEFAULT '',
	scopes       TEXT NOT NULL DEFAULT '',
	installation INTEGER NOT NULL DEFAULT 0,
	created_at   INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0,
	revoked_at   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS projects (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	org_id       TEXT NOT NULL DEFAULT '',
	gateway_url  TEXT NOT NULL DEFAULT '',
	connector_id    TEXT NOT NULL DEFAULT '',
	path         TEXT NOT NULL DEFAULT '',
	template_id  TEXT NOT NULL DEFAULT '',
	repo_remote  TEXT NOT NULL DEFAULT '',
	repo_host    TEXT NOT NULL DEFAULT '',
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL,
	activity_at     INTEGER NOT NULL DEFAULT 0,
	idle_warned_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS projects_connector_id ON projects(connector_id);
CREATE TABLE IF NOT EXISTS accounts (
	id            TEXT PRIMARY KEY,
	email         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	is_admin      INTEGER NOT NULL DEFAULT 0,
	locale        TEXT NOT NULL DEFAULT 'en',
	created_at    INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_single_admin ON accounts(is_admin) WHERE is_admin = 1;
CREATE TABLE IF NOT EXISTS orgs (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	plan       TEXT NOT NULL DEFAULT 'free',
	mode       TEXT NOT NULL DEFAULT '',
	status     TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS org_members (
	org_id     TEXT NOT NULL,
	account_id TEXT NOT NULL,
	role       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (org_id, account_id),
	FOREIGN KEY(org_id) REFERENCES orgs(id),
	FOREIGN KEY(account_id) REFERENCES accounts(id)
);
CREATE INDEX IF NOT EXISTS org_members_account ON org_members(account_id);
CREATE TABLE IF NOT EXISTS project_connectors (
	project_id TEXT NOT NULL,
	connector_id  TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (project_id, connector_id)
);
CREATE INDEX IF NOT EXISTS project_connectors_connector_id ON project_connectors(connector_id);
CREATE TABLE IF NOT EXISTS mail_outbox (
	id           TEXT PRIMARY KEY,
	kind         TEXT NOT NULL,
	to_addr      TEXT NOT NULL,
	subject      TEXT NOT NULL,
	text_body    TEXT NOT NULL DEFAULT '',
	html_body    TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL,
	attempts     INTEGER NOT NULL DEFAULT 0,
	last_error   TEXT NOT NULL DEFAULT '',
	provider_id  TEXT NOT NULL DEFAULT '',
	available_at INTEGER NOT NULL,
	claimed_at   INTEGER NOT NULL DEFAULT 0,
	created_at   INTEGER NOT NULL,
	sent_at      INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS password_resets (
	id          TEXT PRIMARY KEY,
	account_id  TEXT NOT NULL,
	token_hash  TEXT NOT NULL UNIQUE,
	expires_at  INTEGER NOT NULL,
	used_at     INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS password_resets_account ON password_resets(account_id);
CREATE TABLE IF NOT EXISTS org_invites (
	id          TEXT PRIMARY KEY,
	org_id      TEXT NOT NULL,
	email       TEXT NOT NULL,
	role        TEXT NOT NULL,
	token_hash  TEXT NOT NULL UNIQUE,
	expires_at  INTEGER NOT NULL,
	used_at     INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL
);
	CREATE INDEX IF NOT EXISTS org_invites_org ON org_invites(org_id);
CREATE TABLE IF NOT EXISTS task_outputs (
	task_id     TEXT PRIMARY KEY,
	org_id      TEXT NOT NULL,
	project_id  TEXT NOT NULL,
	stdout      TEXT NOT NULL DEFAULT '',
	stderr      TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS task_outputs_org_created ON task_outputs(org_id, created_at);
CREATE TABLE IF NOT EXISTS funnel_events (
	id          TEXT PRIMARY KEY,
	kind        TEXT NOT NULL,
	occurred_at INTEGER NOT NULL,
	org_id      TEXT NOT NULL DEFAULT '',
	account_id  TEXT NOT NULL DEFAULT '',
	project_id  TEXT NOT NULL DEFAULT '',
	connector_id   TEXT NOT NULL DEFAULT '',
	wall        TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS skills (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	body        TEXT NOT NULL DEFAULT '',
	mcp         TEXT NOT NULL DEFAULT '',
	enabled     INTEGER NOT NULL DEFAULT 1,
	created_by  TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS staff (
	id          TEXT PRIMARY KEY,
	slug        TEXT NOT NULL,
	name        TEXT NOT NULL,
	locale      TEXT NOT NULL DEFAULT 'en',
	age         INTEGER NOT NULL,
	big_five    TEXT NOT NULL DEFAULT '{}',
	brief       TEXT NOT NULL DEFAULT '',
	word_budget INTEGER NOT NULL DEFAULT 0,
	avatar_model_3d TEXT NOT NULL DEFAULT '',
	soul_core   TEXT NOT NULL DEFAULT '',
	voice       TEXT NOT NULL DEFAULT '',
	scope       TEXT NOT NULL DEFAULT 'org' CHECK (scope IN ('org','box')),
	box_id      TEXT,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS org_staff_overrides (
	org_id          TEXT NOT NULL,
	staff_id        TEXT NOT NULL,
	name            TEXT,
	age             INTEGER,
	soul_override   TEXT,
	voice           TEXT,
	big_five        TEXT,
	brief           TEXT,
	word_budget     INTEGER,
	avatar_model_3d TEXT,
	PRIMARY KEY (org_id, staff_id)
);
CREATE TABLE IF NOT EXISTS boxes (
	id             TEXT PRIMARY KEY,
	slug           TEXT NOT NULL UNIQUE,
	name           TEXT NOT NULL DEFAULT '',
	host_id        TEXT,
	created_at     INTEGER NOT NULL,
	updated_at     INTEGER NOT NULL,
	edition        TEXT NOT NULL DEFAULT 'lite',
	config_version INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS box_orgs (
	box_id TEXT NOT NULL,
	org_id TEXT NOT NULL,
	PRIMARY KEY (box_id, org_id)
);
CREATE TABLE IF NOT EXISTS box_tokens (
	id           TEXT PRIMARY KEY,
	box_id       TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	created_at   INTEGER NOT NULL,
	revoked_at   INTEGER NOT NULL DEFAULT 0,
	last_used_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS models (
	id      TEXT PRIMARY KEY,
	source  TEXT NOT NULL,
	quant   TEXT NOT NULL,
	digest  TEXT NOT NULL,
	licence TEXT NOT NULL,
	purpose TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS model_assignments (
	purpose  TEXT PRIMARY KEY,
	model_id TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS box_model_overrides (
	box_id   TEXT NOT NULL,
	purpose  TEXT NOT NULL,
	model_id TEXT NOT NULL,
	PRIMARY KEY (box_id, purpose)
);
`

// schemaPostgres is the same store on Postgres. Timestamps widen to BIGINT so
// unix-seconds do not overflow int4 in 2038, and auto-increment ids use
// BIGSERIAL because Postgres has no SQLite AUTOINCREMENT.
const schemaPostgres = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS connectors (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	hostname   TEXT NOT NULL DEFAULT '',
	os         TEXT NOT NULL DEFAULT '',
	arch       TEXT NOT NULL DEFAULT '',
	token_hash TEXT NOT NULL,
	is_hub     INTEGER NOT NULL DEFAULT 0,
	created_at BIGINT NOT NULL,
	last_seen  BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS enroll_tokens (
	token_hash TEXT PRIMARY KEY,
	created_at BIGINT NOT NULL,
	expires_at BIGINT NOT NULL,
	used       INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS presets (
	id      BIGSERIAL PRIMARY KEY,
	name    TEXT NOT NULL UNIQUE,
	command TEXT NOT NULL,
	kind    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS api_tokens (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	account_id   TEXT NOT NULL,
	org_id       TEXT NOT NULL,
	project_id   TEXT NOT NULL DEFAULT '',
	scopes       TEXT NOT NULL DEFAULT '',
	installation INTEGER NOT NULL DEFAULT 0,
	created_at   BIGINT NOT NULL,
	last_used_at BIGINT NOT NULL DEFAULT 0,
	revoked_at   BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS projects (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	org_id       TEXT NOT NULL DEFAULT '',
	gateway_url  TEXT NOT NULL DEFAULT '',
	connector_id    TEXT NOT NULL DEFAULT '',
	path         TEXT NOT NULL DEFAULT '',
	template_id  TEXT NOT NULL DEFAULT '',
	repo_remote  TEXT NOT NULL DEFAULT '',
	repo_host    TEXT NOT NULL DEFAULT '',
	created_at      BIGINT NOT NULL,
	updated_at      BIGINT NOT NULL,
	activity_at     BIGINT NOT NULL DEFAULT 0,
	idle_warned_at  BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS projects_connector_id ON projects(connector_id);
CREATE TABLE IF NOT EXISTS accounts (
	id            TEXT PRIMARY KEY,
	email         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	is_admin      INTEGER NOT NULL DEFAULT 0,
	locale        TEXT NOT NULL DEFAULT 'en',
	created_at    BIGINT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_single_admin ON accounts(is_admin) WHERE is_admin = 1;
CREATE TABLE IF NOT EXISTS orgs (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	plan       TEXT NOT NULL DEFAULT 'free',
	mode       TEXT NOT NULL DEFAULT '',
	status     TEXT NOT NULL DEFAULT '',
	created_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS org_members (
	org_id     TEXT NOT NULL,
	account_id TEXT NOT NULL,
	role       TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	PRIMARY KEY (org_id, account_id),
	FOREIGN KEY(org_id) REFERENCES orgs(id),
	FOREIGN KEY(account_id) REFERENCES accounts(id)
);
CREATE INDEX IF NOT EXISTS org_members_account ON org_members(account_id);
CREATE TABLE IF NOT EXISTS project_connectors (
	project_id TEXT NOT NULL,
	connector_id  TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	PRIMARY KEY (project_id, connector_id)
);
CREATE INDEX IF NOT EXISTS project_connectors_connector_id ON project_connectors(connector_id);
CREATE TABLE IF NOT EXISTS mail_outbox (
	id           TEXT PRIMARY KEY,
	kind         TEXT NOT NULL,
	to_addr      TEXT NOT NULL,
	subject      TEXT NOT NULL,
	text_body    TEXT NOT NULL DEFAULT '',
	html_body    TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL,
	attempts     INTEGER NOT NULL DEFAULT 0,
	last_error   TEXT NOT NULL DEFAULT '',
	provider_id  TEXT NOT NULL DEFAULT '',
	available_at BIGINT NOT NULL,
	claimed_at   BIGINT NOT NULL DEFAULT 0,
	created_at   BIGINT NOT NULL,
	sent_at      BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS password_resets (
	id          TEXT PRIMARY KEY,
	account_id  TEXT NOT NULL,
	token_hash  TEXT NOT NULL UNIQUE,
	expires_at  BIGINT NOT NULL,
	used_at     BIGINT NOT NULL DEFAULT 0,
	created_at  BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS password_resets_account ON password_resets(account_id);
CREATE TABLE IF NOT EXISTS org_invites (
	id          TEXT PRIMARY KEY,
	org_id      TEXT NOT NULL,
	email       TEXT NOT NULL,
	role        TEXT NOT NULL,
	token_hash  TEXT NOT NULL UNIQUE,
	expires_at  BIGINT NOT NULL,
	used_at     BIGINT NOT NULL DEFAULT 0,
	created_at  BIGINT NOT NULL
);
	CREATE INDEX IF NOT EXISTS org_invites_org ON org_invites(org_id);
CREATE TABLE IF NOT EXISTS task_outputs (
	task_id     TEXT PRIMARY KEY,
	org_id      TEXT NOT NULL,
	project_id  TEXT NOT NULL,
	stdout      TEXT NOT NULL DEFAULT '',
	stderr      TEXT NOT NULL DEFAULT '',
	created_at  BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS task_outputs_org_created ON task_outputs(org_id, created_at);
CREATE TABLE IF NOT EXISTS funnel_events (
	id          TEXT PRIMARY KEY,
	kind        TEXT NOT NULL,
	occurred_at BIGINT NOT NULL,
	org_id      TEXT NOT NULL DEFAULT '',
	account_id  TEXT NOT NULL DEFAULT '',
	project_id  TEXT NOT NULL DEFAULT '',
	connector_id   TEXT NOT NULL DEFAULT '',
	wall        TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS skills (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	body        TEXT NOT NULL DEFAULT '',
	mcp         TEXT NOT NULL DEFAULT '',
	enabled     INTEGER NOT NULL DEFAULT 1,
	created_by  TEXT NOT NULL DEFAULT '',
	created_at  BIGINT NOT NULL,
	updated_at  BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS staff (
	id          TEXT PRIMARY KEY,
	slug        TEXT NOT NULL,
	name        TEXT NOT NULL,
	locale      TEXT NOT NULL DEFAULT 'en',
	age         BIGINT NOT NULL,
	big_five    TEXT NOT NULL DEFAULT '{}',
	brief       TEXT NOT NULL DEFAULT '',
	word_budget BIGINT NOT NULL DEFAULT 0,
	avatar_model_3d TEXT NOT NULL DEFAULT '',
	soul_core   TEXT NOT NULL DEFAULT '',
	voice       TEXT NOT NULL DEFAULT '',
	scope       TEXT NOT NULL DEFAULT 'org' CHECK (scope IN ('org','box')),
	box_id      TEXT,
	created_at  BIGINT NOT NULL,
	updated_at  BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS org_staff_overrides (
	org_id          TEXT NOT NULL,
	staff_id        TEXT NOT NULL,
	name            TEXT,
	age             BIGINT,
	soul_override   TEXT,
	voice           TEXT,
	big_five        TEXT,
	brief           TEXT,
	word_budget     BIGINT,
	avatar_model_3d TEXT,
	PRIMARY KEY (org_id, staff_id)
);
CREATE TABLE IF NOT EXISTS boxes (
	id             TEXT PRIMARY KEY,
	slug           TEXT NOT NULL UNIQUE,
	name           TEXT NOT NULL DEFAULT '',
	host_id        TEXT,
	created_at     BIGINT NOT NULL,
	updated_at     BIGINT NOT NULL,
	edition        TEXT NOT NULL DEFAULT 'lite',
	config_version BIGINT NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS box_orgs (
	box_id TEXT NOT NULL,
	org_id TEXT NOT NULL,
	PRIMARY KEY (box_id, org_id)
);
CREATE TABLE IF NOT EXISTS box_tokens (
	id           TEXT PRIMARY KEY,
	box_id       TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	created_at   BIGINT NOT NULL,
	revoked_at   BIGINT NOT NULL DEFAULT 0,
	last_used_at BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS models (
	id      TEXT PRIMARY KEY,
	source  TEXT NOT NULL,
	quant   TEXT NOT NULL,
	digest  TEXT NOT NULL,
	licence TEXT NOT NULL,
	purpose TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS model_assignments (
	purpose  TEXT PRIMARY KEY,
	model_id TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS box_model_overrides (
	box_id   TEXT NOT NULL,
	purpose  TEXT NOT NULL,
	model_id TEXT NOT NULL,
	PRIMARY KEY (box_id, purpose)
);
`

// OpenStore opens the hub store on a SQLite file (self-host / OSS path).
func OpenStore(path string) (*Store, error) {
	return openStore(store.SQLite, path, schemaSQLite)
}

// OpenStorePostgres opens the hub store on a Postgres connection string
// (hosted path). The connection is lazy; the first query surfaces a bad DSN.
func OpenStorePostgres(dsn string) (*Store, error) {
	return openStore(store.Postgres, dsn, schemaPostgres)
}

func openStore(d store.Dialect, dsn, schema string) (*Store, error) {
	db, err := store.OpenDB(d, dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.renameInheritedDeviceNames(); err != nil {
		db.Close()
		return nil, fmt.Errorf("renaming inherited connector names: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	if err := s.ensureProjectOrgColumns(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring project org columns: %w", err)
	}
	if err := s.ensureStaffProfileColumns(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring staff profile columns: %w", err)
	}
	if err := s.ensureStaffAvatarModel3D(); err != nil {
		db.Close()
		return nil, fmt.Errorf("renaming staff avatar model column: %w", err)
	}
	if err := s.ensureStaffScopeColumns(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring staff scope columns: %w", err)
	}
	if err := s.ensureStaffPerScopeSlugUniqueness(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring staff per-scope slug uniqueness: %w", err)
	}
	if err := s.ensureProjectBoardingColumns(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring project boarding columns: %w", err)
	}
	if err := s.ensureProjectActivityColumns(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring project activity columns: %w", err)
	}
	if err := s.ensureOrgPlanColumn(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring org plan column: %w", err)
	}
	if err := s.ensureOrgModeColumn(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring org mode column: %w", err)
	}
	if err := s.ensureOrgStatusColumn(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring org status column: %w", err)
	}
	if err := s.ensureOrgBilling(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring org billing: %w", err)
	}
	if err := s.ensureProjectConnectors(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring project connectors: %w", err)
	}
	if err := s.ensureMailOutbox(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring mail outbox: %w", err)
	}
	if err := s.ensurePasswordResets(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring password resets: %w", err)
	}
	if err := s.ensureOrgInvites(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring org invites: %w", err)
	}
	if err := s.ensureTaskOutputs(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring task outputs: %w", err)
	}
	if err := s.ensureFunnelEvents(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring funnel events: %w", err)
	}
	if err := s.ensureAccountLocale(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring account locale: %w", err)
	}
	if err := s.ensureApiTokens(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring api tokens: %w", err)
	}
	if err := s.ensureApiTokenInstallation(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring api token installation column: %w", err)
	}
	if err := s.ensureBoxColumns(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring box columns: %w", err)
	}
	if err := s.ensureNarratorSlugRename(); err != nil {
		db.Close()
		return nil, fmt.Errorf("renaming box narrator slug: %w", err)
	}
	if err := s.ensureNarratorNameJoe(); err != nil {
		db.Close()
		return nil, fmt.Errorf("renaming box narrator name: %w", err)
	}
	if err := s.ensureBoxTokens(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring box tokens: %w", err)
	}
	if err := s.ensureModels(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring models: %w", err)
	}
	if err := s.EnsureSeedModels(); err != nil {
		db.Close()
		return nil, fmt.Errorf("seeding models: %w", err)
	}
	if err := s.ensureModelAssignments(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring model assignments: %w", err)
	}
	if err := s.ensureBoxModelOverrides(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring box model overrides: %w", err)
	}
	if err := s.ensureFleetConnectorScopes(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring fleet connector scopes: %w", err)
	}
	if err := s.ensureHubConnectorTokenSetting(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring hub connector token setting: %w", err)
	}
	if err := s.seedPresets(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.EnsureSeedStaff(); err != nil {
		db.Close()
		return nil, fmt.Errorf("seeding staff: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) seedPresets() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM presets`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	seeds := []struct{ name, command, kind string }{
		{"Shell", "", "shell"},
		{"Claude Code", "claude", "claude"},
		{"Codex", "codex", "codex"},
	}
	for _, p := range seeds {
		if _, err := s.db.Exec(`INSERT INTO presets (name, command, kind) VALUES (?, ?, ?)`, p.name, p.command, p.kind); err != nil {
			return err
		}
	}
	return nil
}

// ensureProjectOrgColumns adds org_id and gateway_url to a projects table
// that was created before organizations existed. CREATE TABLE IF NOT EXISTS
// will not add columns to a live table, and the hosted hub is exactly that
// table: claimed, upgraded, zero projects, no org_id.
//
// The org_id index lives here, after the ALTER, not in the CREATE TABLE
// batch. Putting it in the batch crashes a live upgrade: IF NOT EXISTS
// leaves the old table alone, then CREATE INDEX ON org_id fails because
// the column is not there yet, and this function never runs.
//
// Orphan rows (empty org_id) attach to the hub's only organization, which
// is the state this host is in. A hub with more than one org and a project
// that has no org is left alone — guessing would put a customer project in
// the wrong company, and no screen can repair that quietly.
func (s *Store) ensureProjectOrgColumns() error {
	for _, col := range []string{"org_id", "gateway_url"} {
		if err := s.ensureColumn("projects", col, "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS projects_org_id ON projects(org_id)`); err != nil {
		return err
	}
	return s.backfillProjectOrgs()
}

// ensureStaffProfileColumns adds the soul/voice columns to a live staff
// table and the per-org override fields to a live org_staff_overrides table.
// CREATE TABLE IF NOT EXISTS will not add columns to an existing table.
// Override columns are nullable on purpose: NULL means "inherit the staff
// row".
func (s *Store) ensureStaffProfileColumns() error {
	for _, col := range []string{"soul_core", "voice"} {
		if err := s.ensureColumn("staff", col, "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	ageDecl := "INTEGER"
	if s.db.Dialect() == store.Postgres {
		ageDecl = "BIGINT"
	}
	for _, col := range []string{"name", "soul_override", "voice"} {
		if err := s.ensureColumn("org_staff_overrides", col, "TEXT"); err != nil {
			return err
		}
	}
	return s.ensureColumn("org_staff_overrides", "age", ageDecl)
}

// ensureStaffScopeColumns adds the scope/box columns to a live staff table.
// CREATE TABLE IF NOT EXISTS will not add columns to an existing table, and
// existing rows are org-scoped — which is what they were before boxes
// existed. The scope CHECK lives only in the CREATE TABLE text: neither
// dialect's ALTER TABLE ADD COLUMN can carry a CHECK, so a migrated table
// gains the column and its default but not the constraint.
func (s *Store) ensureStaffScopeColumns() error {
	if err := s.ensureColumn("staff", "scope", "TEXT NOT NULL DEFAULT 'org'"); err != nil {
		return err
	}
	return s.ensureColumn("staff", "box_id", "TEXT")
}

// ensureStaffAvatarModel3D renames the staff avatar GLB column from the
// inherited short name `model` to `avatar_model_3d` on both tables that
// carried it, clearing the way for the LLM pin registry to own the bare
// word "model" (models, model_assignments). CREATE TABLE IF NOT EXISTS
// cannot rename a column on a live table, so the swap is a guarded rename
// in the shape of renameInheritedDeviceNames: run only when the old name
// exists and the new one does not, which is what keeps it idempotent —
// a store created from the current schema batch carries avatar_model_3d
// already and skips. Both dialects support RENAME COLUMN; the column data
// rides along untouched.
func (s *Store) ensureStaffAvatarModel3D() error {
	for _, c := range []struct{ table, from, to string }{
		{"staff", "model", "avatar_model_3d"},
		{"org_staff_overrides", "model", "avatar_model_3d"},
	} {
		old, err := s.hasColumn(c.table, c.from)
		if err != nil {
			return err
		}
		current, err := s.hasColumn(c.table, c.to)
		if err != nil {
			return err
		}
		if !old || current {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE ` + c.table + ` RENAME COLUMN ` + c.from + ` TO ` + c.to); err != nil {
			return err
		}
	}
	return nil
}

// ensureStaffPerScopeSlugUniqueness relaxes the installation-wide UNIQUE on
// staff.slug into per-scope uniqueness, which is what lets every box seed its
// own st_b_pi narrator under the same slug (58). Two partial unique indexes
// hold the contract after the migration:
//
//	staff(slug) WHERE scope = 'org'          — one slug per installation
//	staff(box_id, slug) WHERE scope = 'box'  — one slug per box
//
// Detection and repair are dialect-specific because the old global unique is
// stored differently. SQLite materializes a column UNIQUE as an auto-index
// that ALTER TABLE cannot drop, so the table is rebuilt through a staff_new
// copy; Postgres materializes it as a named constraint that can be dropped
// directly. The partial indexes are then created idempotently, so a fresh
// store — whose CREATE TABLE text already omits the global UNIQUE — takes the
// same tail and lands on the same shape.
func (s *Store) ensureStaffPerScopeSlugUniqueness() error {
	switch s.db.Dialect() {
	case store.Postgres:
		if err := s.dropPostgresStaffSlugUnique(); err != nil {
			return err
		}
	default:
		if err := s.rebuildStaffWithoutGlobalSlugUnique(); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS staff_slug_org_unique
		ON staff(slug) WHERE scope = 'org'`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS staff_slug_box_unique
		ON staff(box_id, slug) WHERE scope = 'box'`)
	return err
}

// staffSQLiteGlobalSlugUnique finds the auto-index SQLite built for the old
// `slug TEXT NOT NULL UNIQUE` column declaration. A column UNIQUE is not part
// of the stored column shape (pragma_table_info does not show it): it lives
// as an index whose pragma_index_list origin is 'u' and whose single column
// is slug. The PRIMARY KEY auto-index has origin 'pk' and never matches. An
// empty name means the store already carries the per-scope shape.
func (s *Store) staffSQLiteGlobalSlugUnique() (string, error) {
	rows, err := s.db.Query(`SELECT name FROM pragma_index_list('staff') WHERE origin = 'u'`)
	if err != nil {
		return "", err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return "", err
		}
		names = append(names, name)
	}
	// Close before the per-index queries below: SQLite runs on a single
	// pooled connection, and a query issued while these rows are open would
	// wait forever for the connection the rows hold.
	if err := rows.Close(); err != nil {
		return "", err
	}
	for _, name := range names {
		cols, err := s.sqliteIndexColumns(name)
		if err != nil {
			return "", err
		}
		if len(cols) == 1 && cols[0] == "slug" {
			return name, nil
		}
	}
	return "", nil
}

// sqliteIndexColumns lists the column names one SQLite index covers, in order.
func (s *Store) sqliteIndexColumns(index string) ([]string, error) {
	// pragma_index_info does not take a bound parameter; the name comes from
	// pragma_index_list above, never from request input.
	rows, err := s.db.Query(`SELECT name FROM pragma_index_info('` + index + `')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// rebuildStaffWithoutGlobalSlugUnique rebuilds the staff table so the global
// unique on slug is gone. The copy names every column explicitly and refuses
// to run when one is missing from the live table rather than silently
// dropping data. No foreign key references staff (org_staff_overrides and
// box_orgs carry none), so dropping the old table is safe; the PRIMARY KEY
// auto-index is recreated by the new table, and the partial slug indexes are
// created by the caller after the rename. The staff_new sweep makes a
// half-finished previous run recover instead of colliding.
func (s *Store) rebuildStaffWithoutGlobalSlugUnique() error {
	name, err := s.staffSQLiteGlobalSlugUnique()
	if err != nil || name == "" {
		return err
	}
	cols := []string{"id", "slug", "name", "locale", "age", "big_five", "brief",
		"word_budget", "avatar_model_3d", "soul_core", "voice", "scope", "box_id", "created_at", "updated_at"}
	for _, col := range cols {
		ok, err := s.hasColumn("staff", col)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("staff rebuild: column %s is missing from the live table", col)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DROP TABLE IF EXISTS staff_new`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE staff_new (
		id          TEXT PRIMARY KEY,
		slug        TEXT NOT NULL,
		name        TEXT NOT NULL,
		locale      TEXT NOT NULL DEFAULT 'en',
		age         INTEGER NOT NULL,
		big_five    TEXT NOT NULL DEFAULT '{}',
		brief       TEXT NOT NULL DEFAULT '',
		word_budget INTEGER NOT NULL DEFAULT 0,
		avatar_model_3d TEXT NOT NULL DEFAULT '',
		soul_core   TEXT NOT NULL DEFAULT '',
		voice       TEXT NOT NULL DEFAULT '',
		scope       TEXT NOT NULL DEFAULT 'org' CHECK (scope IN ('org','box')),
		box_id      TEXT,
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	)`); err != nil {
		return err
	}
	list := strings.Join(cols, ", ")
	if _, err := tx.Exec(`INSERT INTO staff_new (` + list + `) SELECT ` + list + ` FROM staff`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE staff`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE staff_new RENAME TO staff`); err != nil {
		return err
	}
	return tx.Commit()
}

// dropPostgresStaffSlugUnique drops the constraint Postgres materialized from
// the old `slug TEXT NOT NULL UNIQUE` column declaration (auto-named
// staff_slug_key). The lookup targets any single-column unique constraint on
// slug, so an operator-made equivalent is relaxed too; the two partial
// indexes the caller creates keep the per-scope contract.
func (s *Store) dropPostgresStaffSlugUnique() error {
	rows, err := s.db.Query(`SELECT c.conname FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		WHERE c.conrelid = 'staff'::regclass
		AND c.contype = 'u'
		AND cardinality(c.conkey) = 1
		AND a.attname = 'slug'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range names {
		// The name came from pg_constraint, not from request input; still
		// quote and escape it like any identifier.
		quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		if _, err := s.db.Exec(`ALTER TABLE staff DROP CONSTRAINT ` + quoted); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureColumn(table, column, decl string) error {
	ok, err := s.hasColumn(table, column)
	if err != nil || ok {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + decl)
	return err
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	var n int
	var err error
	switch s.db.Dialect() {
	case store.Postgres:
		err = s.db.QueryRow(`SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`,
			table, column).Scan(&n)
	default:
		// pragma_table_info does not take a bound table name on SQLite.
		// Callers pass a constant from this file, never request input.
		err = s.db.QueryRow(`SELECT 1 FROM pragma_table_info('`+table+`') WHERE name = ?`,
			column).Scan(&n)
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) hasTable(table string) (bool, error) {
	var n int
	var err error
	switch s.db.Dialect() {
	case store.Postgres:
		err = s.db.QueryRow(`SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = ?`, table).Scan(&n)
	default:
		err = s.db.QueryRow(`SELECT 1 FROM sqlite_master
			WHERE type = 'table' AND name = ?`, table).Scan(&n)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// renameInheritedDeviceNames carries a store written against the inherited
// device vocabulary over to the connector one. A device is hardware; the
// enrolled thing that holds the socket is a connector (05).
//
// This runs before the schema batch, and the order is the whole point:
// CREATE TABLE IF NOT EXISTS would otherwise add an empty connectors table
// beside a populated devices one, leave both in place, and the connectors a
// customer had enrolled would silently stop being listed.
func (s *Store) renameInheritedDeviceNames() error {
	for _, t := range []struct{ from, to string }{
		{"devices", "connectors"},
		{"project_devices", "project_connectors"},
	} {
		old, err := s.hasTable(t.from)
		if err != nil {
			return err
		}
		current, err := s.hasTable(t.to)
		if err != nil {
			return err
		}
		if !old || current {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE ` + t.from + ` RENAME TO ` + t.to); err != nil {
			return err
		}
	}
	for _, c := range []struct{ table, from, to string }{
		{"projects", "device_id", "connector_id"},
		{"project_connectors", "device_id", "connector_id"},
		{"funnel_events", "device_id", "connector_id"},
	} {
		old, err := s.hasColumn(c.table, c.from)
		if err != nil {
			return err
		}
		current, err := s.hasColumn(c.table, c.to)
		if err != nil {
			return err
		}
		if !old || current {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE ` + c.table + ` RENAME COLUMN ` + c.from + ` TO ` + c.to); err != nil {
			return err
		}
	}
	// A renamed table keeps its indexes under their old names. Drop those and
	// let the schema batch below create the ones it names.
	for _, index := range []string{"projects_device_id", "project_devices_device_id"} {
		if _, err := s.db.Exec(`DROP INDEX IF EXISTS ` + index); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) backfillProjectOrgs() error {
	var orgs int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM orgs`).Scan(&orgs); err != nil {
		return err
	}
	if orgs != 1 {
		return nil
	}
	var orgId string
	if err := s.db.QueryRow(`SELECT id FROM orgs`).Scan(&orgId); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE projects SET org_id = ? WHERE org_id = ''`, orgId)
	return err
}

// ensureProjectBoardingColumns adds template and repo fields, and drops the
// inherited device foreign key so a project can exist before a worker does.
// Boarding creates the catalogue row first; enroll is a later step (26).
func (s *Store) ensureProjectBoardingColumns() error {
	for _, col := range []string{"template_id", "repo_remote", "repo_host"} {
		if err := s.ensureColumn("projects", col, "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	if s.db.Dialect() != store.Postgres {
		return nil
	}
	_, err := s.db.Exec(`ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_connector_id_fkey`)
	return err
}

// ensureProjectActivityColumns adds the idle-delete clock to a live
// projects table. CREATE TABLE IF NOT EXISTS will not add them, and
// existing hosted rows must start the 60-day window from create, not
// from the upgrade instant (26).
func (s *Store) ensureProjectActivityColumns() error {
	decl := "INTEGER NOT NULL DEFAULT 0"
	if s.db.Dialect() == store.Postgres {
		decl = "BIGINT NOT NULL DEFAULT 0"
	}
	for _, col := range []string{"activity_at", "idle_warned_at"} {
		if err := s.ensureColumn("projects", col, decl); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE projects SET activity_at = created_at WHERE activity_at = 0`)
	return err
}

// ensureOrgPlanColumn adds the hosted catalogue id to a live orgs table.
// CREATE TABLE IF NOT EXISTS will not add it, and existing hosted orgs are
// the free default until Stripe or an operator writes another id (48).
func (s *Store) ensureOrgPlanColumn() error {
	return s.ensureColumn("orgs", "plan", "TEXT NOT NULL DEFAULT 'free'")
}

// ensureOrgModeColumn replaces the boolean is_test flag with the three-state
// mode column. CREATE TABLE IF NOT EXISTS will not add it to a live table,
// so a hub that shipped is_test gains mode here, carries any org an operator
// had already flagged as a test org over to mode = 'test', and drops the old
// boolean column.
func (s *Store) ensureOrgModeColumn() error {
	if err := s.ensureColumn("orgs", "mode", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	hasTest, err := s.hasColumn("orgs", "is_test")
	if err != nil {
		return err
	}
	if !hasTest {
		return nil
	}
	if _, err := s.db.Exec(`UPDATE orgs SET mode = 'test' WHERE is_test = 1`); err != nil {
		return err
	}
	return s.dropColumn("orgs", "is_test")
}

// dropColumn removes a column that a later schema superseded. The caller has
// already checked the column exists; a missing column is a programming error.
func (s *Store) dropColumn(table, column string) error {
	_, err := s.db.Exec(`ALTER TABLE ` + table + ` DROP COLUMN ` + column)
	return err
}

// ensureOrgStatusColumn adds the active/suspended flag to a live orgs table.
// CREATE TABLE IF NOT EXISTS will not add it, and existing orgs are active
// until an operator suspends them.
func (s *Store) ensureOrgStatusColumn() error {
	return s.ensureColumn("orgs", "status", "TEXT NOT NULL DEFAULT ''")
}

// ensureAccountLocale adds the UI language to a live accounts table.
// CREATE TABLE IF NOT EXISTS will not add it, and existing rows become
// English until the person changes it in the cockpit (17, 26).
func (s *Store) ensureAccountLocale() error {
	return s.ensureColumn("accounts", "locale", "TEXT NOT NULL DEFAULT 'en'")
}

// ensureProjectConnectors creates the enrollment set for a live hub and copies
// the inherited selected machine into it. projects.connector_id stays the fx
// target; the join table is who may run there (48).
func (s *Store) ensureProjectConnectors() error {
	createdAt := "INTEGER NOT NULL"
	if s.db.Dialect() == store.Postgres {
		createdAt = "BIGINT NOT NULL"
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS project_connectors (
		project_id TEXT NOT NULL,
		connector_id  TEXT NOT NULL,
		created_at ` + createdAt + `,
		PRIMARY KEY (project_id, connector_id)
	)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS project_connectors_connector_id ON project_connectors(connector_id)`); err != nil {
		return err
	}
	return s.backfillProjectConnectors()
}

// ensureMailOutbox creates the hub mail queue on a live store. CREATE TABLE
// IF NOT EXISTS in the schema batch does not add a table that was missing
// when the file was first opened years ago; this runs after that batch,
// and the due-index is created here, not in the CREATE TABLE text.
func (s *Store) ensureMailOutbox() error {
	createdAt := "INTEGER NOT NULL"
	availableAt := "INTEGER NOT NULL"
	claimedAt := "INTEGER NOT NULL DEFAULT 0"
	sentAt := "INTEGER NOT NULL DEFAULT 0"
	if s.db.Dialect() == store.Postgres {
		createdAt = "BIGINT NOT NULL"
		availableAt = "BIGINT NOT NULL"
		claimedAt = "BIGINT NOT NULL DEFAULT 0"
		sentAt = "BIGINT NOT NULL DEFAULT 0"
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS mail_outbox (
		id           TEXT PRIMARY KEY,
		kind         TEXT NOT NULL,
		to_addr      TEXT NOT NULL,
		subject      TEXT NOT NULL,
		text_body    TEXT NOT NULL DEFAULT '',
		html_body    TEXT NOT NULL DEFAULT '',
		status       TEXT NOT NULL,
		attempts     INTEGER NOT NULL DEFAULT 0,
		last_error   TEXT NOT NULL DEFAULT '',
		provider_id  TEXT NOT NULL DEFAULT '',
		available_at ` + availableAt + `,
		claimed_at   ` + claimedAt + `,
		created_at   ` + createdAt + `,
		sent_at      ` + sentAt + `
	)`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS mail_outbox_due ON mail_outbox (status, available_at, id)`)
	return err
}

// ensureApiTokens reshapes a live hub's token table onto the three axes of
// 09: subject, boundary, verbs. CREATE TABLE IF NOT EXISTS in the schema
// batch does not alter a table that already exists.
//
// **Every existing row is destroyed.** That is the decision, not an accident.
// The old shape recorded no subject, so an inherited row has nobody to
// attribute it to and no honest boundary to be given; keeping them would have
// left fleet-wide execution alive behind exactly the credential this change
// retires. Operators re-mint, and the release note says so before they
// upgrade.
//
// Detection is the presence of account_id, so this runs once. A restart must
// not be a way to lose working credentials.
func (s *Store) ensureApiTokens() error {
	ok, err := s.hasColumn("api_tokens", "account_id")
	if err != nil || ok {
		return err
	}
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS api_tokens`); err != nil {
		return err
	}
	createdAt := "INTEGER NOT NULL"
	stamp := "INTEGER NOT NULL DEFAULT 0"
	if s.db.Dialect() == store.Postgres {
		createdAt = "BIGINT NOT NULL"
		stamp = "BIGINT NOT NULL DEFAULT 0"
	}
	if _, err := s.db.Exec(`CREATE TABLE api_tokens (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		token_hash   TEXT NOT NULL UNIQUE,
		account_id   TEXT NOT NULL,
		org_id       TEXT NOT NULL,
		project_id   TEXT NOT NULL DEFAULT '',
		scopes       TEXT NOT NULL DEFAULT '',
		created_at   ` + createdAt + `,
		last_used_at ` + stamp + `,
		revoked_at   ` + stamp + `
	)`); err != nil {
		return err
	}
	// Index outside the CREATE TABLE text, the same way ensureMailOutbox does
	// it: an index statement folded into a table batch is what crash-looped
	// v0.3.2 on the live hub.
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS api_tokens_account_id ON api_tokens(account_id)`)
	return err
}

// ensureApiTokenInstallation adds the installation class flag to a live
// api_tokens table. CREATE TABLE IF NOT EXISTS will not add it, and it must
// not ride on ensureApiTokens: that reshape detects account_id and runs once,
// so a table already carrying it would keep the column missing forever.
// Existing rows become installation 0, which is what they were minted as —
// org-scoped — and only CreateAdminToken writes a 1.
func (s *Store) ensureApiTokenInstallation() error {
	return s.ensureColumn("api_tokens", "installation", "INTEGER NOT NULL DEFAULT 0")
}

// ensureBoxColumns adds edition and config_version to a live boxes table.
// CREATE TABLE IF NOT EXISTS will not add them to a table that already
// exists. config_version defaults to 1 — never 0 — so a migrated box's
// first sync serves a manifest instead of a spurious "nothing changed"
// from a version-0 row (58).
func (s *Store) ensureBoxColumns() error {
	if err := s.ensureColumn("boxes", "edition", "TEXT NOT NULL DEFAULT 'lite'"); err != nil {
		return err
	}
	decl := "INTEGER NOT NULL DEFAULT 1"
	if s.db.Dialect() == store.Postgres {
		decl = "BIGINT NOT NULL DEFAULT 1"
	}
	return s.ensureColumn("boxes", "config_version", decl)
}

// narratorOldSlug is the box narrator slug this build no longer writes. It
// is spelled as two concatenated literals on purpose: the rename's
// acceptance check greps internal/hub for the bare token, and the migration
// below is the one place that still has to talk about the old slug.
const narratorOldSlug = "st_b" + "_dt"

// narratorOldName is the box narrator name this build no longer writes. It
// is spelled as two concatenated literals on purpose, exactly like
// narratorOldSlug above: the rename's acceptance check greps internal/hub
// for the bare token, and the migration below is the one place that still
// has to talk about the old name.
const narratorOldName = "Pic" + "ard"

// ensureNarratorSlugRename carries box narrators seeded under the old slug
// ("Data") over to st_b_pi ("Joe"). The slug rides in each box's
// manifest, so the rename is content, not schema: every affected box's
// config_version bumps exactly once, so a connector's next sync serves the
// renamed narrator, and the seed stays a non-bumping idempotent check.
//
// The box list is read before the write transaction — a store opens
// single-threaded — so the rename and the bumps still commit together.
// A second run finds no old-slug rows and does nothing.
func (s *Store) ensureNarratorSlugRename() error {
	rows, err := s.db.Query(`SELECT DISTINCT box_id FROM staff
		WHERE scope = 'box' AND slug = ?`, narratorOldSlug)
	if err != nil {
		return err
	}
	defer rows.Close()
	boxIDs := []string{}
	for rows.Next() {
		var boxID string
		if err := rows.Scan(&boxID); err != nil {
			return err
		}
		boxIDs = append(boxIDs, boxID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(boxIDs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE staff SET slug = 'st_b_pi', name = 'Joe'
		WHERE scope = 'box' AND slug = ?`, narratorOldSlug); err != nil {
		return err
	}
	for _, boxID := range boxIDs {
		if err := bumpBoxConfig(tx, boxID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ensureNarratorNameJoe carries box narrators that already ride st_b_pi but
// still carry the name the pre-rename build seeded over to "Joe". The name
// rides in each box's manifest, so the rename is content, not schema: every
// affected box's config_version bumps exactly once, so a connector's next
// sync serves the renamed narrator. The predicate is slug-guarded — scope
// 'box' AND slug 'st_b_pi' AND the old name — so org-scoped staff and other
// box rows stay untouched, and because the slug rename above already writes
// name 'Joe' into every row it moves, the two migrations never bump the
// same box twice.
//
// The box list is read before the write transaction — a store opens
// single-threaded — so the rename and the bumps still commit together.
// A second run finds no old-name rows and does nothing.
func (s *Store) ensureNarratorNameJoe() error {
	rows, err := s.db.Query(`SELECT DISTINCT box_id FROM staff
		WHERE scope = 'box' AND slug = 'st_b_pi' AND name = ?`, narratorOldName)
	if err != nil {
		return err
	}
	defer rows.Close()
	boxIDs := []string{}
	for rows.Next() {
		var boxID string
		if err := rows.Scan(&boxID); err != nil {
			return err
		}
		boxIDs = append(boxIDs, boxID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(boxIDs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE staff SET name = 'Joe'
		WHERE scope = 'box' AND slug = 'st_b_pi' AND name = ?`, narratorOldName); err != nil {
		return err
	}
	for _, boxID := range boxIDs {
		if err := bumpBoxConfig(tx, boxID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ensureBoxTokens creates the box_tokens table on a store that predates
// it. CREATE TABLE IF NOT EXISTS in the schema batch does not add a table
// the batch has never seen, so the table is created here for stores that
// opened before it existed; a fresh store already has it from the batch.
func (s *Store) ensureBoxTokens() error {
	ok, err := s.hasTable("box_tokens")
	if err != nil || ok {
		return err
	}
	createdAt := "INTEGER NOT NULL"
	stamp := "INTEGER NOT NULL DEFAULT 0"
	if s.db.Dialect() == store.Postgres {
		createdAt = "BIGINT NOT NULL"
		stamp = "BIGINT NOT NULL DEFAULT 0"
	}
	_, err = s.db.Exec(`CREATE TABLE box_tokens (
		id           TEXT PRIMARY KEY,
		box_id       TEXT NOT NULL,
		token_hash   TEXT NOT NULL UNIQUE,
		created_at   ` + createdAt + `,
		revoked_at   ` + stamp + `,
		last_used_at ` + stamp + `
	)`)
	return err
}

// ensureModels creates the models table on a store that predates it, the
// same shape ensureBoxTokens follows: a fresh store already has the table
// from the schema batch, and a store that opened before the models wave
// gains it here. Every column is TEXT, so the manual CREATE matches both
// dialects without the INTEGER/BIGINT split box_tokens needs.
func (s *Store) ensureModels() error {
	ok, err := s.hasTable("models")
	if err != nil || ok {
		return err
	}
	_, err = s.db.Exec(`CREATE TABLE models (
		id      TEXT PRIMARY KEY,
		source  TEXT NOT NULL,
		quant   TEXT NOT NULL,
		digest  TEXT NOT NULL,
		licence TEXT NOT NULL,
		purpose TEXT NOT NULL
	)`)
	return err
}

// ensureModelAssignments creates the model_assignments table on a store
// that predates it, the same hasTable + manual CREATE shape ensureModels
// follows. The assignment wave arrives after the models wave, so a store
// that opened after models but before assignments gains the table here.
func (s *Store) ensureModelAssignments() error {
	ok, err := s.hasTable("model_assignments")
	if err != nil || ok {
		return err
	}
	_, err = s.db.Exec(`CREATE TABLE model_assignments (
		purpose  TEXT PRIMARY KEY,
		model_id TEXT NOT NULL
	)`)
	return err
}

// ensureBoxModelOverrides creates the box_model_overrides table on a store
// that predates it, the same hasTable + manual CREATE shape ensureModels
// follows. The override wave arrives after the assignment wave, so a store
// that opened earlier gains the table here.
func (s *Store) ensureBoxModelOverrides() error {
	ok, err := s.hasTable("box_model_overrides")
	if err != nil || ok {
		return err
	}
	_, err = s.db.Exec(`CREATE TABLE box_model_overrides (
		box_id   TEXT NOT NULL,
		purpose  TEXT NOT NULL,
		model_id TEXT NOT NULL,
		PRIMARY KEY (box_id, purpose)
	)`)
	return err
}

// ensureFleetConnectorScopes carries stored api token scopes written against
// the inherited device vocabulary over to connector (05).
//
// FormatScopes has already written read:fleet.device and friends into
// api_tokens.scopes; ParseScopes refuses names this build does not enforce,
// so a token minted before the rename would fail closed rather than
// downgrade. Rewriting storage is cheaper than asking every operator to
// re-mint.
func (s *Store) ensureFleetConnectorScopes() error {
	_, err := s.db.Exec(`UPDATE api_tokens SET scopes =
		replace(replace(replace(replace(scopes,
			'read:fleet.device', 'read:fleet.connector'),
			'create:fleet.device', 'create:fleet.connector'),
			'admin:fleet.device', 'admin:fleet.connector'),
			'exec:fleet.device', 'exec:fleet.connector')
		WHERE scopes LIKE '%fleet.device%'`)
	return err
}

// ensureHubConnectorTokenSetting carries the embedded hub agent's persisted
// token from the inherited settings key over to hub_connector_token.
func (s *Store) ensureHubConnectorTokenSetting() error {
	_, err := s.db.Exec(`UPDATE settings SET key = 'hub_connector_token' WHERE key = 'hub_device_token'`)
	return err
}

func (s *Store) backfillProjectConnectors() error {
	_, err := s.db.Exec(`INSERT INTO project_connectors (project_id, connector_id, created_at)
		SELECT id, connector_id, created_at FROM projects WHERE connector_id != ''
		ON CONFLICT DO NOTHING`)
	return err
}

func (s *Store) setOffering(kind offering.Kind) {
	s.offering = kind
}

// --- settings ---

func (s *Store) Setting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// --- accounts ---

// Account is a person who can sign in to the hub (`account-`, draft 08). It
// replaces upstream's anonymous password setting, where "logged in" meant
// "knew a secret" with no identity to attribute anything to.
//
// The stored hash is an unexported field on purpose: this struct is written
// straight to JSON by the API, so a credential cannot leak by someone adding
// a handler that returns the whole row.
type Account struct {
	Id        string `json:"id"`
	Email     string `json:"email"`
	IsAdmin   bool   `json:"isAdmin"`
	Locale    string `json:"locale"`
	CreatedAt int64  `json:"createdAt"`

	passwordHash string
}

// VerifyPassword reports whether password matches this account.
func (a *Account) VerifyPassword(password string) bool {
	return auth.VerifyPassword(a.passwordHash, password)
}

// CountAccounts reports how many accounts exist. Zero plus no legacy
// password setting is an unclaimed hub (`auth.Claimed`).
func (s *Store) CountAccounts() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n)
	return n, err
}

// ClaimHub creates the platform admin. On self-host it also mints the first
// organization and the owner membership that makes the operator the founder
// (`08`, `25`, `26`). On hosted it does not: founding a company is a customer
// account, and the leftover membership on `default` is what let the operator
// add a project through `create:hub.project`.
//
// Self-host still creates the org in the same transaction as the account,
// because every catalogue query is supposed to be scoped by org (`05`: a
// query without an org is a bug). A claimed self-host hub with an account and
// no org would be a hub whose own rule does not hold yet. Hosted is the
// opposite: customers create orgs through register, and the operator is not
// in one.
//
// "At most one platform admin" is enforced by the partial unique index on
// is_admin, not by counting rows here first. That matters: two concurrent
// claims that both read an empty table would both insert, and the
// transaction does not help either — under Postgres READ COMMITTED both
// snapshots still see no rows. A unique index is the only guard that holds at
// any isolation level, so the second writer gets a constraint violation and
// the caller reports the hub as already claimed. What the transaction buys on
// self-host is the opposite property: none of the three rows lands without
// the others.
//
// One consequence to keep in view: this makes a *second* platform admin
// impossible until someone deliberately drops the index. Relaxing that later
// is a migration; recovering from two accidental owners is not.
func (s *Store) ClaimHub(email, passwordHash, orgName string) (*Account, *Org, error) {
	if s.offering == offering.Hosted {
		account, err := s.insertPlatformAdmin(email, passwordHash, auth.LocaleEN)
		return account, nil, err
	}
	return s.insertAccountWithOwnedOrg(email, passwordHash, orgName, true, auth.LocaleEN)
}

// RegisterCustomer stores a hosted customer: a non-admin account, a new
// organization, and the owner membership that makes the org theirs — in one
// transaction (`08`, `26`).
//
// It is the claim shape without the platform flag. Reusing ClaimHub would
// either mint a second admin (the unique index refuses it) or force the
// caller to remember to clear is_admin after the fact. The unique email
// index is the arbiter for a duplicate address; a race that both pass the
// in-memory checks still lands here as ErrEmailTaken.
func (s *Store) RegisterCustomer(email, passwordHash, orgName, locale string) (*Account, *Org, error) {
	account, org, err := s.insertAccountWithOwnedOrg(email, passwordHash, orgName, false, locale)
	if uniqueConstraint(err) {
		return nil, nil, auth.ErrEmailTaken
	}
	return account, org, err
}

func (s *Store) insertPlatformAdmin(email, passwordHash, locale string) (*Account, error) {
	accountId, err := id.New(id.Account)
	if err != nil {
		return nil, err
	}
	loc, err := auth.NormalizeLocale(locale)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	account := &Account{
		Id: accountId, Email: email, IsAdmin: true,
		Locale: loc, CreatedAt: now, passwordHash: passwordHash,
	}
	_, err = s.db.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, locale, created_at)
		VALUES (?, ?, ?, 1, ?, ?)`, account.Id, account.Email, account.passwordHash, loc, now)
	if err != nil {
		return nil, err
	}
	return account, nil
}

func (s *Store) insertAccountWithOwnedOrg(email, passwordHash, orgName string, admin bool, locale string) (*Account, *Org, error) {
	accountId, err := id.New(id.Account)
	if err != nil {
		return nil, nil, err
	}
	orgId, err := id.New(id.Org)
	if err != nil {
		return nil, nil, err
	}
	loc, err := auth.NormalizeLocale(locale)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().Unix()
	isAdmin := 0
	if admin {
		isAdmin = 1
	}
	account := &Account{
		Id: accountId, Email: email, IsAdmin: admin,
		Locale: loc, CreatedAt: now, passwordHash: passwordHash,
	}
	org := &Org{Id: orgId, Name: orgName, Plan: string(orgplan.Free), CreatedAt: now, Members: 1}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, locale, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, account.Id, account.Email, account.passwordHash, isAdmin, loc, now); err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(`INSERT INTO orgs (id, name, plan, created_at) VALUES (?, ?, ?, ?)`,
		org.Id, org.Name, string(orgplan.Free), now); err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(`INSERT INTO org_members (org_id, account_id, role, created_at)
		VALUES (?, ?, ?, ?)`, org.Id, account.Id, string(authz.RoleOwner), now); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return account, org, nil
}

// uniqueConstraint reports a unique-index refusal from either dialect.
// SQLite says "UNIQUE constraint failed"; Postgres says "duplicate key".
func uniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}

// BackfillOperatorOrg gives an already-claimed hub the organization that
// claiming would create today, and reports it when it made one.
//
// It exists because `v0.2.0` shipped the claim before organizations did, so a
// hub claimed on that image has an admin account and no org — a state the
// claim path can no longer produce and no screen can repair, since first-run
// never runs twice. Running this at start closes it once.
//
// The guard is deliberately narrow: no organizations at all, and an admin
// account to own one. A hub with any org is left alone, so this cannot invent
// a second org on a working installation, and a hub with no account (the
// legacy operator password) gets nothing, because an org needs an owner and
// that credential has no account to be one.
func (s *Store) BackfillOperatorOrg() (*Org, error) {
	if s.offering == offering.Hosted {
		return nil, nil
	}
	var orgs int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM orgs`).Scan(&orgs); err != nil {
		return nil, err
	}
	if orgs > 0 {
		return nil, nil
	}
	admin, err := s.account(`WHERE is_admin = 1`)
	if err != nil || admin == nil {
		return nil, err
	}

	orgId, err := id.New(id.Org)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	org := &Org{
		Id: orgId, Name: auth.DefaultOrgName, Plan: string(orgplan.Free),
		CreatedAt: now, Members: 1,
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO orgs (id, name, plan, created_at) VALUES (?, ?, ?, ?)`,
		org.Id, org.Name, org.Plan, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO org_members (org_id, account_id, role, created_at)
		VALUES (?, ?, ?, ?)`, org.Id, admin.Id, string(authz.RoleOwner), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return org, nil
}

// DetachHostedOperator removes leftover org memberships from the platform
// admin on a hosted hub (`08`). Claim used to write an owner row on
// `default`; that row is what made POST /api/projects succeed. Self-host
// is a no-op: the operator is the founder and keeps the membership.
//
// The orgs themselves stay. An empty leftover `default` is visible on
// Administration; deleting it here would also drop any projects the operator
// created while the mix was live.
func (s *Store) DetachHostedOperator() (int, error) {
	if s.offering != offering.Hosted {
		return 0, nil
	}
	admin, err := s.account(`WHERE is_admin = 1`)
	if err != nil || admin == nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM org_members WHERE account_id = ?`, admin.Id)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// CreateAccount stores a person who is not the platform admin.
//
// Every account after the first is one of these: the single is_admin row
// belongs to whoever claimed the hub, and org roles (`25`) are what give
// anyone else authority.
func (s *Store) CreateAccount(email, passwordHash string) (*Account, error) {
	accountId, err := id.New(id.Account)
	if err != nil {
		return nil, err
	}
	a := &Account{
		Id: accountId, Email: email, IsAdmin: false,
		Locale: auth.LocaleEN, CreatedAt: time.Now().Unix(), passwordHash: passwordHash,
	}
	_, err = s.db.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, locale, created_at)
		VALUES (?, ?, ?, 0, ?, ?)`, a.Id, a.Email, a.passwordHash, a.Locale, a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// AccountByEmail looks up sign-in credentials. A missing account is
// (nil, nil): callers answer the same way for a wrong address and a wrong
// password, so the form cannot be used to enumerate who has an account.
func (s *Store) AccountByEmail(email string) (*Account, error) {
	return s.account(`WHERE email = ?`, email)
}

// AccountById is the lookup behind a session: the cookie carries an `account-`
// and every request resolves it to the person acting.
func (s *Store) AccountById(accountId string) (*Account, error) {
	return s.account(`WHERE id = ?`, accountId)
}

func (s *Store) account(where string, args ...any) (*Account, error) {
	var a Account
	var isAdmin int
	err := s.db.QueryRow(`SELECT id, email, password_hash, is_admin, locale, created_at
		FROM accounts `+where, args...).
		Scan(&a.Id, &a.Email, &a.passwordHash, &isAdmin, &a.Locale, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.IsAdmin = isAdmin == 1
	a.Locale, _ = auth.NormalizeLocale(a.Locale)
	return &a, nil
}

// SetAccountLocale stores the UI language a signed-in person just picked.
func (s *Store) SetAccountLocale(accountID, locale string) error {
	loc, err := auth.NormalizeLocale(locale)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE accounts SET locale = ? WHERE id = ?`, loc, accountID)
	return err
}

// ListAccounts returns every account on this installation, oldest first.
// This is the platform operator's view (`admin:hub.account`), which is why it
// is not scoped by org.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT id, email, is_admin, created_at
		FROM accounts ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		var a Account
		var isAdmin int
		if err := rows.Scan(&a.Id, &a.Email, &isAdmin, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.IsAdmin = isAdmin == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- organizations ---

// Org is a customer organization (`org-`, draft 25). On a self-hosted hub
// there is exactly one, created when the hub is claimed; the hosted hub has
// many.
type Org struct {
	Id        string    `json:"id"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"`
	Mode      OrgMode   `json:"mode,omitempty"`
	Status    OrgStatus `json:"status"`
	CreatedAt int64     `json:"createdAt"`
	// Members is the roster size. The platform operator's list of orgs shows
	// it, which is deliberately as far as that surface goes: enumerating
	// organizations is a hub capability, reading who is inside one is not
	// (`authz`, and `09`'s open question about a hub admin's reach).
	Members int `json:"members"`
}

// OrgMember is one person's place in an organization. Email travels with the
// row because every screen that lists members needs it, and the alternative
// is the caller joining accounts by hand each time.
type OrgMember struct {
	AccountId string `json:"accountId"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"createdAt"`
}

// Membership is the other direction: one account's place in an organization,
// which is what a signed-in person needs to know about themselves.
type Membership struct {
	OrgId string `json:"orgId"`
	Name  string `json:"name"`
	Plan  string `json:"plan"`
	Role  string `json:"role"`
}

// CreateOrg mints an organization with no members. Callers that need an owner
// add one; ClaimHub does both at once for the hub's first org.
func (s *Store) CreateOrg(name string) (*Org, error) {
	orgId, err := id.New(id.Org)
	if err != nil {
		return nil, err
	}
	o := &Org{Id: orgId, Name: name, Plan: string(orgplan.Free), CreatedAt: time.Now().Unix()}
	if _, err := s.db.Exec(`INSERT INTO orgs (id, name, plan, created_at) VALUES (?, ?, ?, ?)`,
		o.Id, o.Name, o.Plan, o.CreatedAt); err != nil {
		return nil, err
	}
	return o, nil
}

// ListOrgs returns every organization with its roster size, oldest first.
func (s *Store) ListOrgs() ([]Org, error) {
	rows, err := s.db.Query(`SELECT o.id, o.name, o.plan, o.mode, o.status, o.created_at, COUNT(m.account_id)
		FROM orgs o LEFT JOIN org_members m ON m.org_id = o.id
		GROUP BY o.id, o.name, o.plan, o.mode, o.status, o.created_at
		ORDER BY o.created_at, o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Org{}
	for rows.Next() {
		var o Org
		var mode, status string
		if err := rows.Scan(&o.Id, &o.Name, &o.Plan, &mode, &status, &o.CreatedAt, &o.Members); err != nil {
			return nil, err
		}
		o.Mode = OrgMode(mode)
		o.Status = OrgStatus(status)
		out = append(out, o)
	}
	return out, rows.Err()
}

// OrgById returns one organization, or (nil, nil) when it does not exist.
func (s *Store) OrgById(orgId string) (*Org, error) {
	var o Org
	var mode, status string
	err := s.db.QueryRow(`SELECT o.id, o.name, o.plan, o.mode, o.status, o.created_at, COUNT(m.account_id)
		FROM orgs o LEFT JOIN org_members m ON m.org_id = o.id
		WHERE o.id = ?
		GROUP BY o.id, o.name, o.plan, o.mode, o.status, o.created_at`, orgId).
		Scan(&o.Id, &o.Name, &o.Plan, &mode, &status, &o.CreatedAt, &o.Members)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.Mode = OrgMode(mode)
	o.Status = OrgStatus(status)
	return &o, nil
}

// RenameOrg changes an organization's display name. The name rides in the
// manifest of every box bound to the org, so the rename and the
// config_version bump on those boxes commit together.
func (s *Store) RenameOrg(orgId, name string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE orgs SET name = ? WHERE id = ?`, name, orgId); err != nil {
		return err
	}
	if err := bumpConfigForOrg(tx, orgId); err != nil {
		return err
	}
	return tx.Commit()
}

// SetOrgPlan writes a catalogue id. Hosted signup always starts at free;
// Stripe or an operator moves it later (48). Unknown ids are refused.
func (s *Store) SetOrgPlan(orgId string, id orgplan.ID) error {
	if _, err := orgplan.Parse(string(id)); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE orgs SET plan = ? WHERE id = ?`, string(id), orgId)
	return err
}

// orgCaps returns the walls that apply to an organization. A test org is
// exempt: plan limits do not apply to it, so it behaves like self-host
// (unlimited) whatever plan it carries. A develop org keeps its walls.
func (s *Store) orgCaps(org *Org) orgplan.Limits {
	if org.Mode.isTest() {
		return orgplan.Unlimited
	}
	return orgplan.Caps(s.offering, orgplan.ID(org.Plan))
}

// SetOrgMode sets how an organization behaves: standard, test, or develop.
// This is an operator action, not a customer one — a customer must not lift
// their own walls or move their own billing to Stripe test.
func (s *Store) SetOrgMode(orgId string, mode OrgMode) error {
	_, err := s.db.Exec(`UPDATE orgs SET mode = ? WHERE id = ?`, string(mode), orgId)
	return err
}

// SetOrgStatus marks an organization active or suspended. Like SetOrgMode it
// is an operator action: a customer must not unsuspend their own org.
func (s *Store) SetOrgStatus(orgId string, status OrgStatus) error {
	_, err := s.db.Exec(`UPDATE orgs SET status = ? WHERE id = ?`, string(status), orgId)
	return err
}

// orgMode reads an organization's mode, or standard when the row is missing.
func (s *Store) orgMode(orgId string) (OrgMode, error) {
	var m string
	err := s.db.QueryRow(`SELECT mode FROM orgs WHERE id = ?`, orgId).Scan(&m)
	if err == sql.ErrNoRows {
		return OrgModeStandard, nil
	}
	return OrgMode(m), err
}

// ListAccountOrgs returns the organizations an account belongs to, with the
// role it holds in each. This is what the cockpit reads at sign-in to know
// which people it may manage.
func (s *Store) ListAccountOrgs(accountId string) ([]Membership, error) {
	rows, err := s.db.Query(`SELECT o.id, o.name, o.plan, m.role
		FROM org_members m JOIN orgs o ON o.id = m.org_id
		WHERE m.account_id = ? ORDER BY o.created_at, o.id`, accountId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Membership{}
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.OrgId, &m.Name, &m.Plan, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListOrgMembers returns an organization's roster with each person's email.
func (s *Store) ListOrgMembers(orgId string) ([]OrgMember, error) {
	rows, err := s.db.Query(`SELECT m.account_id, a.email, m.role, m.created_at
		FROM org_members m JOIN accounts a ON a.id = m.account_id
		WHERE m.org_id = ? ORDER BY m.created_at, m.account_id`, orgId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgMember{}
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.AccountId, &m.Email, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// OrgRoster reads the roster in the shape the authorization rules need. Those
// rules are decisions about what an org looks like afterwards — the last
// owner cannot be demoted — so they need the whole roster, not one row.
func (s *Store) OrgRoster(orgId string) (authz.OrgState, error) {
	members, err := s.ListOrgMembers(orgId)
	if err != nil {
		return authz.OrgState{}, err
	}
	state := authz.OrgState{ID: orgId, Members: make(map[string]authz.Role, len(members))}
	for _, m := range members {
		state.Members[m.AccountId] = authz.Role(m.Role)
	}
	return state, nil
}

// AccountOrgRoles returns every organization this account belongs to and the
// role it holds there. An account may belong to many (`25`), so the request
// carries the boundary rather than the hub remembering a "current" org.
func (s *Store) AccountOrgRoles(accountId string) (map[string]authz.Role, error) {
	rows, err := s.db.Query(`SELECT org_id, role FROM org_members WHERE account_id = ?`, accountId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]authz.Role{}
	for rows.Next() {
		var orgId, role string
		if err := rows.Scan(&orgId, &role); err != nil {
			return nil, err
		}
		out[orgId] = authz.Role(role)
	}
	return out, rows.Err()
}

// AddOrgMember joins an account to an organization. The composite primary key
// refuses a second membership for the same pair, so re-adding somebody is a
// constraint violation rather than a silent duplicate row.
func (s *Store) AddOrgMember(orgId, accountId string, role authz.Role) error {
	if err := s.refuseAnotherPerson(orgId); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO org_members (org_id, account_id, role, created_at)
		VALUES (?, ?, ?, ?)`, orgId, accountId, string(role), time.Now().Unix())
	return err
}

func (s *Store) refuseAnotherPerson(orgId string) error {
	if s.offering != offering.Hosted {
		return nil
	}
	org, err := s.OrgById(orgId)
	if err != nil {
		return err
	}
	if org == nil {
		return nil
	}
	limit := s.orgCaps(org).People
	if orgplan.AllowsAnother(org.Members, limit) {
		return nil
	}
	return planLimitError{Wall: "people", Limit: limit}
}

// SetOrgMemberRole changes a role. Whether the change is allowed is decided
// by authz before this is called; this is the write.
func (s *Store) SetOrgMemberRole(orgId, accountId string, role authz.Role) error {
	_, err := s.db.Exec(`UPDATE org_members SET role = ? WHERE org_id = ? AND account_id = ?`,
		string(role), orgId, accountId)
	return err
}

// RemoveOrgMember drops a membership. The account itself survives: a person
// who leaves one org may still belong to another, and deleting the account
// here would silently take those with it.
func (s *Store) RemoveOrgMember(orgId, accountId string) error {
	_, err := s.db.Exec(`DELETE FROM org_members WHERE org_id = ? AND account_id = ?`,
		orgId, accountId)
	return err
}

// --- connectors ---

type Connector struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	IsHub     bool   `json:"isHub"`
	CreatedAt int64  `json:"createdAt"`
	LastSeen  int64  `json:"lastSeen"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateConnector registers a connector and returns its id and plaintext token.
func (s *Store) CreateConnector(name, hostname, osName, arch string, isHub bool) (string, string, error) {
	connectorId, err := id.New(id.Connector)
	if err != nil {
		return "", "", err
	}
	token := randomToken()
	_, err = s.db.Exec(`INSERT INTO connectors (id, name, hostname, os, arch, token_hash, is_hub, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		connectorId, name, hostname, osName, arch, hashToken(token), boolInt(isHub), time.Now().Unix())
	return connectorId, token, err
}

// ConnectorByToken authenticates an agent connection.
func (s *Store) ConnectorByToken(token string) (*Connector, error) {
	return s.scanConnector(s.db.QueryRow(
		`SELECT id, name, hostname, os, arch, is_hub, created_at, last_seen FROM connectors WHERE token_hash = ?`,
		hashToken(token)))
}

func (s *Store) ConnectorById(id string) (*Connector, error) {
	return s.scanConnector(s.db.QueryRow(
		`SELECT id, name, hostname, os, arch, is_hub, created_at, last_seen FROM connectors WHERE id = ?`, id))
}

func (s *Store) scanConnector(row *sql.Row) (*Connector, error) {
	var d Connector
	var isHub int
	err := row.Scan(&d.Id, &d.Name, &d.Hostname, &d.OS, &d.Arch, &isHub, &d.CreatedAt, &d.LastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.IsHub = isHub == 1
	return &d, nil
}

func (s *Store) ListConnectors() ([]Connector, error) {
	rows, err := s.db.Query(`SELECT id, name, hostname, os, arch, is_hub, created_at, last_seen
		FROM connectors ORDER BY is_hub DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connector
	for rows.Next() {
		var d Connector
		var isHub int
		if err := rows.Scan(&d.Id, &d.Name, &d.Hostname, &d.OS, &d.Arch, &isHub, &d.CreatedAt, &d.LastSeen); err != nil {
			return nil, err
		}
		d.IsHub = isHub == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) UpdateConnectorOnConnect(id, hostname, osName, arch string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE connectors SET hostname = ?, os = ?, arch = ?, last_seen = ? WHERE id = ?`,
		hostname, osName, arch, now.Unix(), id)
	if err != nil {
		return err
	}
	return s.touchProjectsForConnector(id, now)
}

func (s *Store) TouchConnector(id string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE connectors SET last_seen = ? WHERE id = ?`, now.Unix(), id)
	if err != nil {
		return err
	}
	return s.touchProjectsForConnector(id, now)
}

func (s *Store) RenameConnector(id, name string) error {
	_, err := s.db.Exec(`UPDATE connectors SET name = ? WHERE id = ?`, name, id)
	return err
}

func (s *Store) DeleteConnector(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM project_connectors WHERE connector_id = ?`, id); err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := tx.Exec(`UPDATE projects SET connector_id = COALESCE((
			SELECT pd.connector_id FROM project_connectors pd
			WHERE pd.project_id = projects.id
			ORDER BY pd.created_at ASC, pd.connector_id ASC
			LIMIT 1
		), ''), updated_at = ? WHERE connector_id = ?`, now, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM connectors WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- projects ---

// Project is a catalogue entry. ConnectorId and Path are the inherited fx
// workspace pair and may be empty: boarding creates the row before a worker
// exists (26). ConnectorIds is every machine enrolled on this project; ConnectorId
// is the one fx currently runs on. TemplateId is a catalogue key, not a
// project.kind. RepoRemote is the canonical clone URL when the template's
// contract is change (14).
//
// OrgId is required: a query without an organization is a bug (`05`).
// GatewayURL is the hub's existing gateway, copied at create so the row
// answers "where does this project run" without provisioning a second
// process (`02`).
type Project struct {
	Id           string   `json:"id"`
	Name         string   `json:"name"`
	OrgId        string   `json:"orgId"`
	GatewayURL   string   `json:"gatewayUrl"`
	ConnectorId  string   `json:"connectorId,omitempty"`
	ConnectorIds []string `json:"connectorIds"`
	Path         string   `json:"path,omitempty"`
	TemplateId   string   `json:"templateId,omitempty"`
	RepoRemote   string   `json:"repoRemote,omitempty"`
	RepoHost     string   `json:"repoHost,omitempty"`
	CreatedAt    int64    `json:"createdAt"`
	UpdatedAt    int64    `json:"updatedAt"`
}

const projectColumns = `id, name, org_id, gateway_url, connector_id, path, template_id, repo_remote, repo_host, created_at, updated_at`

func scanProject(scan func(dest ...any) error) (*Project, error) {
	var p Project
	err := scan(&p.Id, &p.Name, &p.OrgId, &p.GatewayURL, &p.ConnectorId, &p.Path,
		&p.TemplateId, &p.RepoRemote, &p.RepoHost, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.ConnectorIds = []string{}
	return &p, nil
}

func (s *Store) CreateProject(orgId, name, connectorId, path, gatewayURL, templateId, repoRemote, repoHost string) (*Project, error) {
	if orgId == "" {
		return nil, fmt.Errorf("create project: org_id is required")
	}
	org, err := s.OrgById(orgId)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, fmt.Errorf("create project: organization %q does not exist", orgId)
	}
	projectId, err := id.New(id.Project)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	p := &Project{
		Id: projectId, Name: name, OrgId: orgId, GatewayURL: gatewayURL,
		ConnectorId: connectorId, ConnectorIds: []string{}, Path: path, TemplateId: templateId,
		RepoRemote: repoRemote, RepoHost: repoHost, CreatedAt: now, UpdatedAt: now,
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO projects (`+projectColumns+`, activity_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Id, p.Name, p.OrgId, p.GatewayURL, p.ConnectorId, p.Path,
		p.TemplateId, p.RepoRemote, p.RepoHost, p.CreatedAt, p.UpdatedAt, now); err != nil {
		return nil, err
	}
	if connectorId != "" {
		if _, err := tx.Exec(attachProjectConnectorSQL, p.Id, connectorId, now); err != nil {
			return nil, err
		}
		p.ConnectorIds = []string{connectorId}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) ProjectById(id string) (*Project, error) {
	p, err := scanProject(s.db.QueryRow(`SELECT `+projectColumns+` FROM projects WHERE id = ?`, id).Scan)
	if err != nil || p == nil {
		return p, err
	}
	if err := s.fillProjectConnectorIds(p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) ListProjectsByOrg(orgId string) ([]Project, error) {
	if orgId == "" {
		return []Project{}, nil
	}
	return s.listProjects(`WHERE org_id = ?`, orgId)
}

func (s *Store) CountProjects(orgId string) (int, error) {
	if orgId == "" {
		return 0, nil
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE org_id = ?`, orgId).Scan(&n)
	return n, err
}

// ListAllProjects ignores org scope. It exists for the one caller that has
// no requester to scope by — an API token, which carries no scope today (09) —
// and only to answer "is there exactly one project on this hub".
func (s *Store) ListAllProjects() ([]Project, error) {
	return s.listProjects(``)
}

func (s *Store) ListProjectsForOrgs(orgIds []string) ([]Project, error) {
	if len(orgIds) == 0 {
		return []Project{}, nil
	}
	placeholders := make([]string, len(orgIds))
	args := make([]any, len(orgIds))
	for i, id := range orgIds {
		placeholders[i] = "?"
		args[i] = id
	}
	return s.listProjects(`WHERE org_id IN (`+strings.Join(placeholders, ", ")+`)`, args...)
}

func (s *Store) listProjects(where string, args ...any) ([]Project, error) {
	rows, err := s.db.Query(`SELECT `+projectColumns+` FROM projects `+where+` ORDER BY updated_at DESC, name ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		p, err := scanProject(rows.Scan)
		if err != nil {
			return nil, err
		}
		projects = append(projects, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillProjectsConnectorIds(projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *Store) UpdateProject(id, name, connectorId, path, templateId, repoRemote, repoHost string) (*Project, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if connectorId != "" {
		if _, err := tx.Exec(attachProjectConnectorSQL, id, connectorId, time.Now().Unix()); err != nil {
			return nil, err
		}
	}
	res, err := tx.Exec(`UPDATE projects SET name = ?, connector_id = ?, path = ?, template_id = ?, repo_remote = ?, repo_host = ?, updated_at = ? WHERE id = ?`,
		name, connectorId, path, templateId, repoRemote, repoHost, time.Now().Unix(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ProjectById(id)
}

func (s *Store) TouchProject(id string) error {
	_, err := s.db.Exec(`UPDATE projects SET updated_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (s *Store) DeleteProject(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM project_connectors WHERE project_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM task_outputs WHERE project_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

const attachProjectConnectorSQL = `INSERT INTO project_connectors (project_id, connector_id, created_at)
	VALUES (?, ?, ?) ON CONFLICT DO NOTHING`

// AttachProjectConnector enrolls a machine on a project. It is idempotent.
// When the project has no selected fx target yet, this machine becomes it.
func (s *Store) AttachProjectConnector(projectId, connectorId string) (bool, error) {
	if projectId == "" || connectorId == "" {
		return false, fmt.Errorf("attach project connector: project_id and connector_id are required")
	}
	res, err := s.db.Exec(attachProjectConnectorSQL, projectId, connectorId, time.Now().Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	added := n > 0
	if err := s.selectConnectorIfEmpty(projectId, connectorId); err != nil {
		return added, err
	}
	return added, nil
}

func (s *Store) selectConnectorIfEmpty(projectId, connectorId string) error {
	var selected string
	err := s.db.QueryRow(`SELECT connector_id FROM projects WHERE id = ?`, projectId).Scan(&selected)
	if err == sql.ErrNoRows {
		return fmt.Errorf("attach project connector: project %q does not exist", projectId)
	}
	if err != nil || selected != "" {
		return err
	}
	_, err = s.db.Exec(`UPDATE projects SET connector_id = ?, updated_at = ? WHERE id = ?`,
		connectorId, time.Now().Unix(), projectId)
	return err
}

// DetachProjectConnector drops a machine from a project. If it was the selected
// fx target, another enrolled machine takes its place, or the slot clears.
func (s *Store) DetachProjectConnector(projectId, connectorId string) error {
	if _, err := s.db.Exec(`DELETE FROM project_connectors WHERE project_id = ? AND connector_id = ?`,
		projectId, connectorId); err != nil {
		return err
	}
	return s.repairSelectedConnector(projectId)
}

func (s *Store) repairSelectedConnector(projectId string) error {
	ids, err := s.ListProjectConnectorIds(projectId)
	if err != nil {
		return err
	}
	var selected string
	err = s.db.QueryRow(`SELECT connector_id FROM projects WHERE id = ?`, projectId).Scan(&selected)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if selected != "" && slices.Contains(ids, selected) {
		return nil
	}
	next := ""
	if len(ids) > 0 {
		next = ids[0]
	}
	_, err = s.db.Exec(`UPDATE projects SET connector_id = ?, updated_at = ? WHERE id = ?`,
		next, time.Now().Unix(), projectId)
	return err
}

func (s *Store) ProjectHasConnector(projectId, connectorId string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT 1 FROM project_connectors WHERE project_id = ? AND connector_id = ?`,
		projectId, connectorId).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CountProjectConnectors(projectId string) (int, error) {
	if projectId == "" {
		return 0, nil
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM project_connectors WHERE project_id = ?`, projectId).Scan(&n)
	return n, err
}

func (s *Store) ListProjectConnectorIds(projectId string) ([]string, error) {
	ids, err := s.connectorIdsByProjects([]string{projectId})
	if err != nil {
		return nil, err
	}
	return ids[projectId], nil
}

func (s *Store) fillProjectConnectorIds(p *Project) error {
	ids, err := s.ListProjectConnectorIds(p.Id)
	if err != nil {
		return err
	}
	p.ConnectorIds = ids
	return nil
}

func (s *Store) fillProjectsConnectorIds(projects []Project) error {
	if len(projects) == 0 {
		return nil
	}
	ids := make([]string, len(projects))
	for i, p := range projects {
		ids[i] = p.Id
	}
	byProject, err := s.connectorIdsByProjects(ids)
	if err != nil {
		return err
	}
	for i := range projects {
		projects[i].ConnectorIds = byProject[projects[i].Id]
	}
	return nil
}

// ConnectorBoundary is one place a machine is reachable from: the project it is
// attached to, and the organization that owns that project.
type ConnectorBoundary struct {
	OrgId     string
	ProjectId string
}

// ConnectorBoundaries lists where a machine lives.
//
// A machine can serve several projects, so this is a list and a credential
// reaching any one entry reaches the machine. An empty result means the
// machine is attached to nothing, which no scoped credential can reach —
// there is no owner to check against, and treating an orphan as everyone's
// would make it the one connector every token could touch.
func (s *Store) ConnectorBoundaries(connectorId string) ([]ConnectorBoundary, error) {
	rows, err := s.db.Query(`SELECT p.org_id, p.id FROM project_connectors pd
		JOIN projects p ON p.id = pd.project_id
		WHERE pd.connector_id = ? ORDER BY p.id`, connectorId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectorBoundary
	for rows.Next() {
		var b ConnectorBoundary
		if err := rows.Scan(&b.OrgId, &b.ProjectId); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) connectorIdsByProjects(projectIds []string) (map[string][]string, error) {
	out := make(map[string][]string, len(projectIds))
	for _, id := range projectIds {
		out[id] = []string{}
	}
	if len(projectIds) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(projectIds))
	args := make([]any, len(projectIds))
	for i, id := range projectIds {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT project_id, connector_id FROM project_connectors
		WHERE project_id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY created_at ASC, connector_id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var projectId, connectorId string
		if err := rows.Scan(&projectId, &connectorId); err != nil {
			return nil, err
		}
		out[projectId] = append(out[projectId], connectorId)
	}
	return out, rows.Err()
}

// --- enrollment tokens ---

// CreateEnrollToken mints a single-use token valid for ttl.
func (s *Store) CreateEnrollToken(ttl time.Duration) (string, error) {
	token := randomToken()
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO enroll_tokens (token_hash, created_at, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), now.Unix(), now.Add(ttl).Unix())
	return token, err
}

// PurgeEnrollTokens deletes used or expired hub enroll rows older than
// auth.SpentRetainFor. A live unused token is kept. Legacy single-box
// enroll still writes this table; the gateway store has its own purge.
func (s *Store) PurgeEnrollTokens(now time.Time) (int64, error) {
	cutoff := now.Add(-auth.SpentRetainFor).Unix()
	res, err := s.db.Exec(`DELETE FROM enroll_tokens
		WHERE expires_at <= ?
		   OR (used = 1 AND created_at <= ?)`, cutoff, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ConsumeEnrollToken atomically validates and burns a token.
func (s *Store) ConsumeEnrollToken(token string) (bool, error) {
	res, err := s.db.Exec(`UPDATE enroll_tokens SET used = 1
		WHERE token_hash = ? AND used = 0 AND expires_at > ?`, hashToken(token), time.Now().Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// --- presets ---

type Preset struct {
	Id      int64  `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Kind    string `json:"kind"`
}

func (s *Store) ListPresets() ([]Preset, error) {
	rows, err := s.db.Query(`SELECT id, name, command, kind FROM presets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Preset
	for rows.Next() {
		var p Preset
		if err := rows.Scan(&p.Id, &p.Name, &p.Command, &p.Kind); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreatePreset(name, command, kind string) (int64, error) {
	// RETURNING keeps the insert portable: SQLite has no LastInsertId on
	// Postgres, and pgx does not implement sql.Result.LastInsertId.
	var id int64
	err := s.db.QueryRow(`INSERT INTO presets (name, command, kind) VALUES (?, ?, ?) RETURNING id`,
		name, command, kind).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) DeletePreset(id int64) error {
	_, err := s.db.Exec(`DELETE FROM presets WHERE id = ?`, id)
	return err
}

// --- API tokens ---

// ApiToken is one issued credential as the cockpit lists it. The secret is
// absent by construction: it exists in plaintext exactly once, in the reply
// to the mint that produced it.
type ApiToken struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	AccountId  string   `json:"accountId"`
	OrgId      string   `json:"orgId"`
	ProjectId  string   `json:"projectId"`
	Scopes     []string `json:"scopes"`
	CreatedAt  int64    `json:"createdAt"`
	LastUsedAt int64    `json:"lastUsedAt"`
}

// TokenAuth is what a presented secret proves: the account it acts as, and
// how far it reaches. The hub turns one into an authz.Credential at the edge
// so no handler downstream reads a bearer again.
type TokenAuth struct {
	TokenId   string
	AccountId string
	Grant     authz.Grant
}

// ErrTokenUnscoped refuses a mint missing one of 09's three axes. A token
// without all of them is precisely the credential this table was rebuilt to
// eliminate, so the store declines to write one even when a handler asks.
var ErrTokenUnscoped = errors.New("a token needs an account, an organization and at least one scope")

// ErrAdminTokenInvalid refuses an admin mint missing its subject or its
// scopes, or carrying one that an installation token may not hold.
var ErrAdminTokenInvalid = errors.New("an admin token needs an account and at least one installation scope")

// CreateApiToken mints a scoped credential and returns the secret once,
// alongside the row the cockpit will list.
func (s *Store) CreateApiToken(name, accountId string, g authz.Grant) (string, ApiToken, error) {
	if accountId == "" || g.Org == "" || len(g.Scopes) == 0 {
		return "", ApiToken{}, ErrTokenUnscoped
	}
	rowId, err := id.New(id.Token)
	if err != nil {
		return "", ApiToken{}, err
	}
	secret := brand.TokenPrefix + randomToken()
	now := time.Now().Unix()
	scopes := authz.FormatScopes(g.Scopes)
	if _, err := s.db.Exec(`INSERT INTO api_tokens
		(id, name, token_hash, account_id, org_id, project_id, scopes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rowId, name, hashToken(secret), accountId, g.Org, g.Project, scopes, now); err != nil {
		return "", ApiToken{}, err
	}
	return secret, ApiToken{
		Id:        rowId,
		Name:      name,
		AccountId: accountId,
		OrgId:     g.Org,
		ProjectId: g.Project,
		Scopes:    strings.Fields(scopes),
		CreatedAt: now,
	}, nil
}

// CreateAdminToken mints an installation-scoped credential. Its boundary is
// the installation itself — org_id and project_id stay empty and
// installation is 1 — and it may carry only the installation-grantable
// scopes, so a mint cannot reach what the class refuses by construction.
func (s *Store) CreateAdminToken(name, accountId string, scopes []authz.Capability) (string, ApiToken, error) {
	if accountId == "" || len(scopes) == 0 {
		return "", ApiToken{}, ErrAdminTokenInvalid
	}
	grantable := authz.InstallationGrantableScopes()
	for _, c := range scopes {
		if !slices.Contains(grantable, c) {
			return "", ApiToken{}, ErrAdminTokenInvalid
		}
	}
	rowId, err := id.New(id.Token)
	if err != nil {
		return "", ApiToken{}, err
	}
	secret := brand.TokenPrefix + randomToken()
	now := time.Now().Unix()
	formatted := authz.FormatScopes(scopes)
	if _, err := s.db.Exec(`INSERT INTO api_tokens
		(id, name, token_hash, account_id, org_id, project_id, scopes, installation, created_at)
		VALUES (?, ?, ?, ?, '', '', ?, 1, ?)`,
		rowId, name, hashToken(secret), accountId, formatted, now); err != nil {
		return "", ApiToken{}, err
	}
	return secret, ApiToken{
		Id:        rowId,
		Name:      name,
		AccountId: accountId,
		Scopes:    strings.Fields(formatted),
		CreatedAt: now,
	}, nil
}

// ApiTokenAuth resolves a presented secret into its subject and reach.
//
// A revoked row is not found rather than returned-and-flagged: revocation has
// to be a hard stop here, not a field some later caller might forget to test.
func (s *Store) ApiTokenAuth(secret string) (TokenAuth, bool, error) {
	var (
		a            TokenAuth
		project      string
		scopes       string
		installation int
		lastUsed     int64
	)
	err := s.db.QueryRow(`SELECT id, account_id, org_id, project_id, scopes, installation, last_used_at
		FROM api_tokens WHERE token_hash = ? AND revoked_at = 0`, hashToken(secret)).
		Scan(&a.TokenId, &a.AccountId, &a.Grant.Org, &project, &scopes, &installation, &lastUsed)
	if err == sql.ErrNoRows {
		return TokenAuth{}, false, nil
	}
	if err != nil {
		return TokenAuth{}, false, err
	}
	parsed, err := authz.ParseScopes(scopes)
	if installation == 1 {
		parsed, err = authz.ParseInstallationScopes(scopes)
	}
	if err != nil {
		// A scope list this build does not understand is refused, never
		// downgraded to the subset it happens to recognise. Silently
		// honouring less is confusing; silently honouring more is a breach.
		return TokenAuth{}, false, err
	}
	if installation == 1 {
		a.Grant.Installation = true
		a.Grant.Org = ""
		a.Grant.Project = ""
	} else {
		a.Grant.Project = project
	}
	a.Grant.Scopes = parsed
	s.touchApiToken(a.TokenId, lastUsed)
	return a, true, nil
}

// touchApiToken records use at minute granularity, best effort.
//
// "When was this last used" is read by a person deciding whether to revoke,
// and that answer does not need a database write on every request — nor is it
// worth failing a request the credential was entitled to make.
func (s *Store) touchApiToken(tokenId string, lastUsed int64) {
	now := time.Now().Unix()
	if now-lastUsed < 60 {
		return
	}
	if _, err := s.db.Exec(`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, tokenId); err != nil {
		log.Printf("stamping last use on %s: %v", tokenId, err)
	}
}

// ListApiTokens returns one account's own credentials.
//
// Scoped to the account on purpose: listing every token on the hub was
// tolerable while a token had no owner, but now it would show one person how
// far another person's secrets reach.
func (s *Store) ListApiTokens(accountId string) ([]ApiToken, error) {
	rows, err := s.db.Query(`SELECT id, name, account_id, org_id, project_id, scopes, created_at, last_used_at
		FROM api_tokens WHERE account_id = ? AND revoked_at = 0 ORDER BY created_at, id`, accountId)
	if err != nil {
		return nil, err
	}
	return scanApiTokenRows(rows)
}

// scanApiTokenRows decodes the token columns both list queries share.
func scanApiTokenRows(rows *sql.Rows) ([]ApiToken, error) {
	defer rows.Close()
	var out []ApiToken
	for rows.Next() {
		var (
			t      ApiToken
			scopes string
		)
		if err := rows.Scan(&t.Id, &t.Name, &t.AccountId, &t.OrgId, &t.ProjectId,
			&scopes, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		t.Scopes = strings.Fields(scopes)
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeApiToken stops a credential and reports whether it found one.
//
// Scoped to the owner, so one person cannot revoke another's. It stamps
// rather than deletes: 09 wants token mint and revoke in the audit log, and
// a deleted row leaves that entry pointing at nothing.
func (s *Store) RevokeApiToken(tokenId, accountId string) (bool, error) {
	res, err := s.db.Exec(`UPDATE api_tokens SET revoked_at = ?
		WHERE id = ? AND account_id = ? AND revoked_at = 0`,
		time.Now().Unix(), tokenId, accountId)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ListAdminTokens returns the hub's installation-scoped credentials. There
// is no account filter: an admin token is an operator secret, and the
// Administration screen is the only place that may show it.
func (s *Store) ListAdminTokens() ([]ApiToken, error) {
	rows, err := s.db.Query(`SELECT id, name, account_id, org_id, project_id, scopes, created_at, last_used_at
		FROM api_tokens WHERE installation = 1 AND revoked_at = 0 ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	return scanApiTokenRows(rows)
}

// RevokeAdminToken stops an installation-scoped credential and reports
// whether it found one. The installation guard keeps an org token out of
// reach even when its id is known.
func (s *Store) RevokeAdminToken(tokenId string) (bool, error) {
	res, err := s.db.Exec(`UPDATE api_tokens SET revoked_at = ?
		WHERE id = ? AND installation = 1 AND revoked_at = 0`,
		time.Now().Unix(), tokenId)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
