package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRebindSQLiteIdentity(t *testing.T) {
	t.Parallel()
	q := `SELECT id FROM tasks WHERE project_id = ? AND state = ?`
	if got := SQLite.Rebind(q); got != q {
		t.Errorf("SQLite.Rebind changed the query: %q", got)
	}
}

func TestRebindPostgres(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "ordered placeholders",
			in:   `SELECT id FROM tasks WHERE a = ? AND b = ? AND c = ?`,
			want: `SELECT id FROM tasks WHERE a = $1 AND b = $2 AND c = $3`,
		},
		{
			name: "no placeholders unchanged",
			in:   `SELECT 1`,
			want: `SELECT 1`,
		},
		{
			name: "literal question mark preserved",
			in:   `INSERT INTO t (v) VALUES (?) AND note = 'what?'`,
			want: `INSERT INTO t (v) VALUES ($1) AND note = 'what?'`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Postgres.Rebind(tc.in); got != tc.want {
				t.Errorf("Rebind(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestOpenUnknownDialect(t *testing.T) {
	t.Parallel()
	if _, err := Open(Dialect("bogus"), "x"); err == nil {
		t.Fatal("Open with an unknown dialect should error")
	}
}

func TestOpenPostgresRegistersDriver(t *testing.T) {
	t.Parallel()
	// sql.Open only fails when the driver is unregistered; this asserts the
	// pgx stdlib blank import ran. No server is contacted (lazy connect).
	db, err := Open(Postgres, "postgres://user:pass@localhost:5432/db")
	if err != nil {
		t.Fatalf("Open(Postgres, …): %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOpenDBAndQueryThroughWrapper(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := OpenDB(SQLite, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if db.Dialect() != SQLite {
		t.Fatalf("Dialect() = %q, want sqlite", db.Dialect())
	}

	if _, err := db.Exec(`CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Two placeholders exercise the SQLite identity path end to end.
	if _, err := db.Exec(`INSERT INTO kv (k, v) VALUES (?, ?)`, "a", "1"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var v string
	if err := db.QueryRow(`SELECT v FROM kv WHERE k = ?`, "a").Scan(&v); err != nil {
		t.Fatalf("select: %v", err)
	}
	if v != "1" {
		t.Fatalf("got %q, want %q", v, "1")
	}
}

func TestDBContextVariantsAndRaw(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db, err := OpenDB(SQLite, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		t.Fatalf("ExecContext create: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO kv (k, v) VALUES (?, ?)`, "a", "1"); err != nil {
		t.Fatalf("ExecContext insert: %v", err)
	}
	var v string
	if err := db.QueryRowContext(ctx, `SELECT v FROM kv WHERE k = ?`, "a").Scan(&v); err != nil {
		t.Fatalf("QueryRowContext: %v", err)
	}
	if v != "1" {
		t.Fatalf("got %q, want %q", v, "1")
	}
	rows, err := db.QueryContext(ctx, `SELECT k, v FROM kv`)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	_ = rows.Close()

	// Raw exposes the underlying *sql.DB; Ping proves it is live.
	if err := db.Raw().Ping(); err != nil {
		t.Fatalf("Raw().Ping: %v", err)
	}
}
