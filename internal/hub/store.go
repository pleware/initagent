package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/auth"
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
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	device_id  TEXT NOT NULL,
	path       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
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
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	device_id  TEXT NOT NULL,
	path       TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
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

// CreateAdminAccount stores the hub's platform admin (`08`).
//
// "At most one" is enforced by the partial unique index on is_admin, not by
// counting rows here first. That matters: two concurrent claims that both
// read an empty table would both insert, and wrapping the read and the write
// in a transaction does not help either — under Postgres READ COMMITTED both
// snapshots still see no rows. A unique index is the only guard that holds at
// any isolation level, so the second writer gets a constraint violation and
// the caller reports the hub as already claimed.
//
// One consequence to keep in view: this makes a *second* platform admin
// impossible until someone deliberately drops the index. Relaxing that later
// is a migration; recovering from two accidental owners is not.
func (s *Store) CreateAdminAccount(email, passwordHash string) (*Account, error) {
	accountId, err := id.New(id.Account)
	if err != nil {
		return nil, err
	}
	a := &Account{
		Id: accountId, Email: email, IsAdmin: true,
		CreatedAt: time.Now().Unix(), passwordHash: passwordHash,
	}
	_, err = s.db.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, created_at)
		VALUES (?, ?, ?, 1, ?)`, a.Id, a.Email, a.passwordHash, a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// AccountByEmail looks up sign-in credentials. A missing account is
// (nil, nil): callers answer the same way for a wrong address and a wrong
// password, so the form cannot be used to enumerate who has an account.
func (s *Store) AccountByEmail(email string) (*Account, error) {
	var a Account
	var isAdmin int
	err := s.db.QueryRow(`SELECT id, email, password_hash, is_admin, created_at
		FROM accounts WHERE email = ?`, email).
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
type Project struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	DeviceId  string `json:"deviceId"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (s *Store) CreateProject(name, deviceId, path string) (*Project, error) {
	projectId, err := id.New(id.Project)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	p := &Project{
		Id: projectId, Name: name, DeviceId: deviceId, Path: path,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err = s.db.Exec(`INSERT INTO projects (id, name, device_id, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, p.Id, p.Name, p.DeviceId, p.Path, p.CreatedAt, p.UpdatedAt)
	return p, err
}

func (s *Store) ProjectById(id string) (*Project, error) {
	var p Project
	err := s.db.QueryRow(`SELECT id, name, device_id, path, created_at, updated_at FROM projects WHERE id = ?`, id).
		Scan(&p.Id, &p.Name, &p.DeviceId, &p.Path, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id, name, device_id, path, created_at, updated_at
		FROM projects ORDER BY updated_at DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Id, &p.Name, &p.DeviceId, &p.Path, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
