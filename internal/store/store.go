// Package store is the thin database seam the hub and gateway open through.
//
// It owns two things and nothing more: the driver choice (modernc sqlite for
// OSS/self-host, pgx for the hosted Postgres offering) and the placeholder
// dialect ("?" for SQLite, "$n" for Postgres). Per-table schema stays with the
// caller (hub, gateway) because that schema is product state, not a generic
// concern. See drafts 18, 23, and 32.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Dialect selects the SQL dialect a store speaks.
type Dialect string

const (
	// SQLite is the zero-config self-host / OSS backend.
	SQLite Dialect = "sqlite"
	// Postgres is the hosted (SaaS) backend, reached by a pgx connection string.
	Postgres Dialect = "postgres"
)

// Open returns a *sql.DB for the dialect. dsn is a file path for SQLite and a
// pgx connection string for Postgres. The connection is lazy: a bad Postgres
// DSN does not error here, it errors on the first query.
func Open(d Dialect, dsn string) (*sql.DB, error) {
	switch d {
	case SQLite:
		db, err := sql.Open("sqlite", dsn+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1) // modernc sqlite prefers a single writer
		return db, nil
	case Postgres:
		return sql.Open("pgx", dsn)
	default:
		return nil, fmt.Errorf("unknown store dialect %q", d)
	}
}

// Rebind rewrites SQLite-style "?" placeholders to Postgres "$1..$n".
// SQLite queries are returned unchanged. A "?" inside a single-quoted string
// literal is left alone.
func (d Dialect) Rebind(query string) string {
	if d != Postgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	inLiteral := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch c {
		case '\'':
			inLiteral = !inLiteral
			b.WriteByte(c)
		case '?':
			if inLiteral {
				b.WriteByte(c)
			} else {
				n++
				b.WriteByte('$')
				b.WriteString(strconv.Itoa(n))
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// DB wraps *sql.DB with a dialect so a call site writes one query using "?"
// placeholders and the wrapper rebinds it for Postgres. The method set mirrors
// the *sql.DB surface the stores use.
type DB struct {
	sql *sql.DB
	d   Dialect
}

// OpenDB opens and wraps a database in one step.
func OpenDB(d Dialect, dsn string) (*DB, error) {
	db, err := Open(d, dsn)
	if err != nil {
		return nil, err
	}
	return &DB{sql: db, d: d}, nil
}

// Dialect reports which backend this handle speaks.
func (db *DB) Dialect() Dialect { return db.d }

// Raw returns the underlying *sql.DB for operations the wrapper does not
// rebind (for example applying a schema that has no placeholders).
func (db *DB) Raw() *sql.DB { return db.sql }

// Close releases the underlying database.
func (db *DB) Close() error { return db.sql.Close() }

// Exec runs a query and rebinds its placeholders for the backend.
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.sql.Exec(db.d.Rebind(query), args...)
}

// ExecContext is Exec with a context.
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.sql.ExecContext(ctx, db.d.Rebind(query), args...)
}

// Query runs a query and rebinds its placeholders for the backend.
func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.sql.Query(db.d.Rebind(query), args...)
}

// QueryContext is Query with a context.
func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.sql.QueryContext(ctx, db.d.Rebind(query), args...)
}

// QueryRow runs a single-row query and rebinds its placeholders.
func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.sql.QueryRow(db.d.Rebind(query), args...)
}

// QueryRowContext is QueryRow with a context.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.sql.QueryRowContext(ctx, db.d.Rebind(query), args...)
}
