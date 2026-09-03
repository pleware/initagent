// Package kinds catalogues the database (DSN) connection kinds a project
// secret can point at. The worker never opens a customer database itself
// (Draft 28); it injects a connection string into the harness MCP child.
// This package is the closed vocabulary for that injection (Draft 24).
package kinds

import (
	"maps"
	"slices"
)

// Kind is a database connection kind identifier. It is the value stored as
// the secret kind on a sec- row, drawn from this closed set rather than an
// ad-hoc slug.
type Kind string

const (
	KindPostgres   Kind = "postgres"
	KindMySQL      Kind = "mysql"
	KindMSSQL      Kind = "mssql"
	KindSQLiteFile Kind = "sqlite_file"
	KindRedis      Kind = "redis"
)

// Spec carries the static metadata a caller needs to route a DSN without
// opening it: which URL scheme it uses, and whether the secret names a file
// on the worker disk rather than a reachable host.
type Spec struct {
	// Scheme is the conventional URL scheme the DSN starts with
	// ("postgres", "file", …). It is a routing hint, not a wire contract.
	Scheme string
	// Local reports that the secret names a path on the worker's disk
	// (sqlite_file) rather than a network host. The walk-up (Draft 24) and
	// the capability report (Draft 39) treat these two cases differently.
	Local bool
}

// Registry is the single source of truth for DSN kind metadata. An AST test
// asserts every Kind constant appears here before CI goes green.
var Registry = map[Kind]Spec{
	KindPostgres:   {Scheme: "postgres", Local: false},
	KindMySQL:      {Scheme: "mysql", Local: false},
	KindMSSQL:      {Scheme: "sqlserver", Local: false},
	KindSQLiteFile: {Scheme: "file", Local: true},
	KindRedis:      {Scheme: "redis", Local: false},
}

// Kinds returns all registered kind identifiers in sorted order.
func Kinds() []Kind {
	return slices.Sorted(maps.Keys(Registry))
}

// IsValid reports whether a kind is registered.
func IsValid(k Kind) bool {
	_, ok := Registry[k]
	return ok
}
