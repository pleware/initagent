// Package id mints and parses initagent identifiers.
//
// Every identifier is <prefix>-<uuid>, for example
//
//	dev-0198f3a1-7c4e-7b2a-9f31-2c6a8d4e5b70
//
// The UUID is version 7, so keys are time-ordered and append to an index
// instead of splitting pages at random. That costs nothing on SQLite and
// matters once the store is Postgres and the task queue is write-heavy.
//
// Identifiers are not secrets. A UUIDv7 carries a timestamp and roughly 74
// bits of randomness, which is enough to be unguessable in practice and not
// enough to authenticate with. Anything that authenticates stays a 32-byte
// CSPRNG value and only borrows the prefix so it is recognisable in a log.
//
// Upstream mints identifiers as randomToken()[:16]: a non-standard shape,
// drawn from the secret generator, and indistinguishable between entities in
// a log line. Replacing it is cheapest before there is data to migrate.
package id

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Separator divides the prefix from the UUID.
//
// A UUID contains hyphens of its own, so this separator is not unique within
// an identifier and splitting on all of them is wrong. Parse below cuts at
// the first one; no caller should be splitting by hand.
const Separator = "-"

// Kind is an entity prefix. The set below is the entity table in the naming
// ontology, and it is deliberately complete rather than limited to entities
// that have code today — the prefixes are the vocabulary, and a second list
// somewhere else is how "task" ends up meaning two different things.
type Kind string

const (
	Account Kind = "acc"
	Org     Kind = "org"
	Project Kind = "prj"
	Gateway Kind = "gwy"
	Token   Kind = "tok"
	Command Kind = "cmd"
	Event   Kind = "evt"

	Member    Kind = "mbr"
	Persona   Kind = "psn"
	MCPServer Kind = "mcs"
	Repo      Kind = "rep"
	Secret    Kind = "sec"
	Todo      Kind = "tdo"
	Task      Kind = "tsk"
	Run       Kind = "run"
	Work      Kind = "wrk"
	Draft     Kind = "drf"
	Evidence  Kind = "evd"
	Proof     Kind = "prf"
	Thread    Kind = "thr"
	Bridge    Kind = "brg"
	Attention Kind = "att"

	// ForeignProject is the surrogate for a project owned by a tool on a
	// worker — an OpenCode project, a Cursor workspace, an editor's own
	// project file. The correspondence to our project is many-to-many and
	// the foreign identifier can change under us, which is why it is a row
	// rather than a column. See drafts 07 and 44.
	ForeignProject Kind = "fpr"

	Host       Kind = "hst"
	Device     Kind = "dev"
	Enrollment Kind = "enr"
	Terminal   Kind = "trm"
	Attachment Kind = "atc"

	Workspace Kind = "wsp"
	Coder     Kind = "cdr"
)

// entities maps each prefix to its qualified entity name. Registration is
// what makes a prefix real: New and Parse both reject anything absent here,
// so a typo cannot quietly mint a new namespace.
var entities = map[Kind]string{
	Account: "initagent.hub.account",
	Org:     "initagent.hub.org",
	Project: "initagent.hub.project",
	Gateway: "initagent.hub.gateway",
	Token:   "initagent.hub.token",
	Command: "initagent.hub.command",
	Event:   "initagent.hub.event",

	Member:         "initagent.project.member",
	Persona:        "initagent.project.persona",
	MCPServer:      "initagent.project.mcp_server",
	Repo:           "initagent.project.repo",
	Secret:         "initagent.project.secret",
	Todo:           "initagent.project.todo",
	Task:           "initagent.project.task",
	Run:            "initagent.project.run",
	Work:           "initagent.project.work",
	Draft:          "initagent.project.draft",
	Evidence:       "initagent.project.evidence",
	Proof:          "initagent.project.proof",
	Thread:         "initagent.project.thread",
	Bridge:         "initagent.project.bridge",
	Attention:      "initagent.project.attention",
	ForeignProject: "initagent.project.foreign_project",

	Host:       "initagent.fleet.host",
	Device:     "initagent.fleet.device",
	Enrollment: "initagent.fleet.enrollment",
	Terminal:   "initagent.fleet.terminal",
	Attachment: "initagent.fleet.attachment",

	Workspace: "initagent.worker.workspace",
	Coder:     "initagent.worker.coder",
}

// Entity returns the qualified entity name for a prefix.
func Entity(k Kind) (string, bool) {
	name, ok := entities[k]
	return name, ok
}

// Kinds returns every registered prefix. Order is not stable.
func Kinds() []Kind {
	out := make([]Kind, 0, len(entities))
	for k := range entities {
		out = append(out, k)
	}
	return out
}

// New mints an identifier for kind k.
func New(k Kind) (string, error) {
	if _, ok := entities[k]; !ok {
		return "", fmt.Errorf("id: unregistered kind %q", string(k))
	}
	u, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("id: mint %s: %w", string(k), err)
	}
	return string(k) + Separator + u.String(), nil
}

// Parse splits an identifier into its kind and UUID, rejecting unregistered
// prefixes and malformed UUIDs.
func Parse(s string) (Kind, uuid.UUID, error) {
	prefix, rest, found := strings.Cut(s, Separator)
	if !found {
		return "", uuid.Nil, fmt.Errorf("id: %q has no %q separator", s, Separator)
	}
	k := Kind(prefix)
	if _, ok := entities[k]; !ok {
		return "", uuid.Nil, fmt.Errorf("id: unregistered kind %q in %q", prefix, s)
	}
	u, err := uuid.Parse(rest)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("id: %q: %w", s, err)
	}
	return k, u, nil
}

// Is reports whether s is a well-formed identifier of kind k. Use it at a
// boundary where passing the wrong entity's identifier is possible — a
// project id reaching a device lookup returns no rows and looks like missing
// data rather than a bug.
func Is(k Kind, s string) bool {
	got, _, err := Parse(s)
	return err == nil && got == k
}
