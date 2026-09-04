package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/store"
)

// Store wraps the hub's database (SQLite for self-host, Postgres for the
// hosted offering). The db handle rebinds placeholders per dialect.
type Store struct {
	db *store.DB
}

const schemaSQLite = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS devices (
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
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	org_id      TEXT NOT NULL DEFAULT '',
	gateway_url TEXT NOT NULL DEFAULT '',
	device_id   TEXT NOT NULL,
	path        TEXT NOT NULL,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL,
	FOREIGN KEY(device_id) REFERENCES devices(id)
);
CREATE INDEX IF NOT EXISTS projects_device_id ON projects(device_id);
CREATE TABLE IF NOT EXISTS accounts (
	id            TEXT PRIMARY KEY,
	email         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	is_admin      INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_single_admin ON accounts(is_admin) WHERE is_admin = 1;
CREATE TABLE IF NOT EXISTS orgs (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
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
`

// schemaPostgres is the same store on Postgres. Timestamps widen to BIGINT so
// unix-seconds do not overflow int4 in 2038, and auto-increment ids use
// BIGSERIAL because Postgres has no SQLite AUTOINCREMENT.
const schemaPostgres = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS devices (
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
	id         BIGSERIAL PRIMARY KEY,
	name       TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	created_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	org_id      TEXT NOT NULL DEFAULT '',
	gateway_url TEXT NOT NULL DEFAULT '',
	device_id   TEXT NOT NULL,
	path        TEXT NOT NULL,
	created_at  BIGINT NOT NULL,
	updated_at  BIGINT NOT NULL,
	FOREIGN KEY(device_id) REFERENCES devices(id)
);
CREATE INDEX IF NOT EXISTS projects_device_id ON projects(device_id);
CREATE TABLE IF NOT EXISTS accounts (
	id            TEXT PRIMARY KEY,
	email         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	is_admin      INTEGER NOT NULL DEFAULT 0,
	created_at    BIGINT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_single_admin ON accounts(is_admin) WHERE is_admin = 1;
CREATE TABLE IF NOT EXISTS orgs (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
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
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	s := &Store{db: db}
	if err := s.ensureProjectOrgColumns(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensuring project org columns: %w", err)
	}
	if err := s.seedPresets(); err != nil {
		db.Close()
		return nil, err
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

// Account is a person who can sign in to the hub (`acc-`, draft 08). It
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

// ClaimHub creates the platform admin, the hub's first organization, and the
// membership that makes the operator its owner — in one transaction (`08`,
// `25`, `26`).
//
// The org exists from the first moment rather than being created lazily
// because every catalogue query is supposed to be scoped by org (`05`: a
// query without an org is a bug). A claimed hub with an account and no org
// would be a hub whose own rule does not hold yet.
//
// "At most one platform admin" is enforced by the partial unique index on
// is_admin, not by counting rows here first. That matters: two concurrent
// claims that both read an empty table would both insert, and the
// transaction does not help either — under Postgres READ COMMITTED both
// snapshots still see no rows. A unique index is the only guard that holds at
// any isolation level, so the second writer gets a constraint violation and
// the caller reports the hub as already claimed. What the transaction buys is
// the opposite property: none of the three rows lands without the others.
//
// One consequence to keep in view: this makes a *second* platform admin
// impossible until someone deliberately drops the index. Relaxing that later
// is a migration; recovering from two accidental owners is not.
func (s *Store) ClaimHub(email, passwordHash, orgName string) (*Account, *Org, error) {
	return s.insertAccountWithOwnedOrg(email, passwordHash, orgName, true)
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
func (s *Store) RegisterCustomer(email, passwordHash, orgName string) (*Account, *Org, error) {
	account, org, err := s.insertAccountWithOwnedOrg(email, passwordHash, orgName, false)
	if uniqueConstraint(err) {
		return nil, nil, auth.ErrEmailTaken
	}
	return account, org, err
}

func (s *Store) insertAccountWithOwnedOrg(email, passwordHash, orgName string, admin bool) (*Account, *Org, error) {
	accountId, err := id.New(id.Account)
	if err != nil {
		return nil, nil, err
	}
	orgId, err := id.New(id.Org)
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
		CreatedAt: now, passwordHash: passwordHash,
	}
	org := &Org{Id: orgId, Name: orgName, CreatedAt: now, Members: 1}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, created_at)
		VALUES (?, ?, ?, ?, ?)`, account.Id, account.Email, account.passwordHash, isAdmin, now); err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(`INSERT INTO orgs (id, name, created_at) VALUES (?, ?, ?)`,
		org.Id, org.Name, now); err != nil {
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
		Id: orgId, Name: auth.DefaultOrgName,
		CreatedAt: now, Members: 1,
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO orgs (id, name, created_at) VALUES (?, ?, ?)`,
		org.Id, org.Name, now); err != nil {
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
		CreatedAt: time.Now().Unix(), passwordHash: passwordHash,
	}
	_, err = s.db.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, created_at)
		VALUES (?, ?, ?, 0, ?)`, a.Id, a.Email, a.passwordHash, a.CreatedAt)
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

// AccountById is the lookup behind a session: the cookie carries an `acc-`
// and every request resolves it to the person acting.
func (s *Store) AccountById(accountId string) (*Account, error) {
	return s.account(`WHERE id = ?`, accountId)
}

func (s *Store) account(where string, args ...any) (*Account, error) {
	var a Account
	var isAdmin int
	err := s.db.QueryRow(`SELECT id, email, password_hash, is_admin, created_at
		FROM accounts `+where, args...).
		Scan(&a.Id, &a.Email, &a.passwordHash, &isAdmin, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.IsAdmin = isAdmin == 1
	return &a, nil
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
	Id        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
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
	Role  string `json:"role"`
}

// CreateOrg mints an organization with no members. Callers that need an owner
// add one; ClaimHub does both at once for the hub's first org.
func (s *Store) CreateOrg(name string) (*Org, error) {
	orgId, err := id.New(id.Org)
	if err != nil {
		return nil, err
	}
	o := &Org{Id: orgId, Name: name, CreatedAt: time.Now().Unix()}
	if _, err := s.db.Exec(`INSERT INTO orgs (id, name, created_at) VALUES (?, ?, ?)`,
		o.Id, o.Name, o.CreatedAt); err != nil {
		return nil, err
	}
	return o, nil
}

// ListOrgs returns every organization with its roster size, oldest first.
func (s *Store) ListOrgs() ([]Org, error) {
	rows, err := s.db.Query(`SELECT o.id, o.name, o.created_at, COUNT(m.account_id)
		FROM orgs o LEFT JOIN org_members m ON m.org_id = o.id
		GROUP BY o.id, o.name, o.created_at
		ORDER BY o.created_at, o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Org{}
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.Id, &o.Name, &o.CreatedAt, &o.Members); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// OrgById returns one organization, or (nil, nil) when it does not exist.
func (s *Store) OrgById(orgId string) (*Org, error) {
	var o Org
	err := s.db.QueryRow(`SELECT o.id, o.name, o.created_at, COUNT(m.account_id)
		FROM orgs o LEFT JOIN org_members m ON m.org_id = o.id
		WHERE o.id = ?
		GROUP BY o.id, o.name, o.created_at`, orgId).
		Scan(&o.Id, &o.Name, &o.CreatedAt, &o.Members)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// RenameOrg changes an organization's display name.
func (s *Store) RenameOrg(orgId, name string) error {
	_, err := s.db.Exec(`UPDATE orgs SET name = ? WHERE id = ?`, name, orgId)
	return err
}

// ListAccountOrgs returns the organizations an account belongs to, with the
// role it holds in each. This is what the cockpit reads at sign-in to know
// which people it may manage.
func (s *Store) ListAccountOrgs(accountId string) ([]Membership, error) {
	rows, err := s.db.Query(`SELECT o.id, o.name, m.role
		FROM org_members m JOIN orgs o ON o.id = m.org_id
		WHERE m.account_id = ? ORDER BY o.created_at, o.id`, accountId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Membership{}
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.OrgId, &m.Name, &m.Role); err != nil {
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
	_, err := s.db.Exec(`INSERT INTO org_members (org_id, account_id, role, created_at)
		VALUES (?, ?, ?, ?)`, orgId, accountId, string(role), time.Now().Unix())
	return err
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

// --- devices ---

type Device struct {
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

// CreateDevice registers a device and returns its id and plaintext token.
func (s *Store) CreateDevice(name, hostname, osName, arch string, isHub bool) (string, string, error) {
	deviceId, err := id.New(id.Device)
	if err != nil {
		return "", "", err
	}
	token := randomToken()
	_, err = s.db.Exec(`INSERT INTO devices (id, name, hostname, os, arch, token_hash, is_hub, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		deviceId, name, hostname, osName, arch, hashToken(token), boolInt(isHub), time.Now().Unix())
	return deviceId, token, err
}

// DeviceByToken authenticates an agent connection.
func (s *Store) DeviceByToken(token string) (*Device, error) {
	return s.scanDevice(s.db.QueryRow(
		`SELECT id, name, hostname, os, arch, is_hub, created_at, last_seen FROM devices WHERE token_hash = ?`,
		hashToken(token)))
}

func (s *Store) DeviceById(id string) (*Device, error) {
	return s.scanDevice(s.db.QueryRow(
		`SELECT id, name, hostname, os, arch, is_hub, created_at, last_seen FROM devices WHERE id = ?`, id))
}

func (s *Store) scanDevice(row *sql.Row) (*Device, error) {
	var d Device
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

func (s *Store) ListDevices() ([]Device, error) {
	rows, err := s.db.Query(`SELECT id, name, hostname, os, arch, is_hub, created_at, last_seen
		FROM devices ORDER BY is_hub DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		var isHub int
		if err := rows.Scan(&d.Id, &d.Name, &d.Hostname, &d.OS, &d.Arch, &isHub, &d.CreatedAt, &d.LastSeen); err != nil {
			return nil, err
		}
		d.IsHub = isHub == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) UpdateDeviceOnConnect(id, hostname, osName, arch string) error {
	_, err := s.db.Exec(`UPDATE devices SET hostname = ?, os = ?, arch = ?, last_seen = ? WHERE id = ?`,
		hostname, osName, arch, time.Now().Unix(), id)
	return err
}

func (s *Store) TouchDevice(id string) error {
	_, err := s.db.Exec(`UPDATE devices SET last_seen = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (s *Store) RenameDevice(id, name string) error {
	_, err := s.db.Exec(`UPDATE devices SET name = ? WHERE id = ?`, name, id)
	return err
}

func (s *Store) DeleteDevice(id string) error {
	if _, err := s.db.Exec(`DELETE FROM projects WHERE device_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM devices WHERE id = ?`, id)
	return err
}

// --- projects ---

// Project binds a named coding workspace to one directory on one device.
// The browser-hosted fx runtime uses this pair as its remote workspace.
//
// OrgId is required: a query without an organization is a bug (`05`).
// GatewayURL is the hub's existing gateway, copied at create so the row
// answers "where does this project run" without provisioning a second
// process (`02`).
type Project struct {
	Id         string `json:"id"`
	Name       string `json:"name"`
	OrgId      string `json:"orgId"`
	GatewayURL string `json:"gatewayUrl"`
	DeviceId   string `json:"deviceId"`
	Path       string `json:"path"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

func (s *Store) CreateProject(orgId, name, deviceId, path, gatewayURL string) (*Project, error) {
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
		DeviceId: deviceId, Path: path, CreatedAt: now, UpdatedAt: now,
	}
	_, err = s.db.Exec(`INSERT INTO projects (id, name, org_id, gateway_url, device_id, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Id, p.Name, p.OrgId, p.GatewayURL, p.DeviceId, p.Path, p.CreatedAt, p.UpdatedAt)
	return p, err
}

func (s *Store) ProjectById(id string) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`SELECT id, name, org_id, gateway_url, device_id, path, created_at, updated_at
		FROM projects WHERE id = ?`, id).
		Scan(&p.Id, &p.Name, &p.OrgId, &p.GatewayURL, &p.DeviceId, &p.Path, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListProjectsByOrg(orgId string) ([]Project, error) {
	if orgId == "" {
		return []Project{}, nil
	}
	return s.listProjects(`WHERE org_id = ?`, orgId)
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
	rows, err := s.db.Query(`SELECT id, name, org_id, gateway_url, device_id, path, created_at, updated_at
		FROM projects `+where+` ORDER BY updated_at DESC, name ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Id, &p.Name, &p.OrgId, &p.GatewayURL, &p.DeviceId, &p.Path, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateProject(id, name, deviceId, path string) (*Project, error) {
	res, err := s.db.Exec(`UPDATE projects SET name = ?, device_id = ?, path = ?, updated_at = ? WHERE id = ?`,
		name, deviceId, path, time.Now().Unix(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	return s.ProjectById(id)
}

func (s *Store) TouchProject(id string) error {
	_, err := s.db.Exec(`UPDATE projects SET updated_at = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

func (s *Store) DeleteProject(id string) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
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

type ApiToken struct {
	Id        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

func (s *Store) CreateApiToken(name string) (string, error) {
	token := brand.TokenPrefix + randomToken()
	_, err := s.db.Exec(`INSERT INTO api_tokens (name, token_hash, created_at) VALUES (?, ?, ?)`,
		name, hashToken(token), time.Now().Unix())
	return token, err
}

func (s *Store) ValidApiToken(token string) (bool, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM api_tokens WHERE token_hash = ?`, hashToken(token)).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ListApiTokens() ([]ApiToken, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM api_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ApiToken
	for rows.Next() {
		var t ApiToken
		if err := rows.Scan(&t.Id, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteApiToken(id int64) error {
	_, err := s.db.Exec(`DELETE FROM api_tokens WHERE id = ?`, id)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
