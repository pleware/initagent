// Package id mints and parses initagent identifiers.
//
// Every identifier is <prefix>-<uuid>, for example
//
//	connector-0198f3a1-7c4e-7b2a-9f31-2c6a8d4e5b70
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
//
// # The registry is generated
//
// The vocabulary itself — the prefixes, the five contexts, and every described
// entity — lives in `names/names.yaml` at the repository root and arrives here
// as registry_gen.go. This file holds the types, the rules and the functions;
// it holds no data.
//
// The source is a language-neutral file, and top-level rather than under
// `internal/`, because three planes derive from the same rows and only one of
// them is Go: `internal/authz` spells `verb:context.entity`, the gdesk seam
// spells `context.entity.verb`, and a sensing process in another repository and
// another language has to spell the second one too. When this package was the
// source of truth, every one of those was a hand-kept mirror — which is how a
// producer came to emit `desk.people.changed` while the product said `gdesk.`.
//
// This package stays a leaf: a UUID library and the standard library. The YAML
// is parsed by `internal/names`, at generation time, never here.
package id

//go:generate go run github.com/pleware/initagent/cmd/namesgen -source ../../names/names.yaml -go registry_gen.go -json ../../names/names.json

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Separator divides the prefix from the UUID.
//
// A UUID contains hyphens of its own, so this separator is not unique within
// an identifier and splitting on all of them is wrong. Parse below cuts at
// the first one; no caller should be splitting by hand.
const Separator = "-"

// Kind is an entity prefix. The registered set is the entity table in the
// naming ontology, and it is deliberately complete rather than limited to
// entities that have code today — the prefixes are the vocabulary, and a second
// list somewhere else is how "task" ends up meaning two different things.
//
// A prefix is the last segment of the entity's qualified name, spelled out
// in full: initagent.fleet.connector mints connector-, initagent.hub.password_reset
// mints password_reset-. There is no length limit and no abbreviation, so
// nothing has to be looked up to read a log line, and there is no judgement
// call left for the rule to drift through. An underscore inside a name is
// kept; the hyphen stays reserved for the separator, which is what lets
// Parse cut at the first one.
//
// Two consequences worth knowing. A leaf noun can no longer repeat across
// contexts — two entities ending in the same word would want the same prefix,
// and the generator refuses that before it can reach the compiler. And a Kind
// is no longer written down twice: the generator derives it from the entity's
// name, so the drift an earlier AST test guarded against is now structurally
// impossible rather than merely caught.
//
// Not every entity has one. The ones that mint nothing are registered in
// unminted, so the vocabulary is whole even where the identifier plane has
// nothing to say.
type Kind string

// Context is the plane an entity belongs to: the middle segment of every
// qualified name.
//
// There are exactly five (05). A sixth is an extension of the ontology made
// on purpose, not something a new entity may invent in passing — half of
// every name, event and scope is spelled from this list, so a context
// invented here forks the vocabulary in three places at once.
type Context string

// Contexts lists the five planes in the order 05 gives them.
func Contexts() []Context { return slices.Clone(contexts) }

// Spec is everything this package knows about one entity.
//
// The qualified name alone was not enough: a reader arriving at `bridge-` or
// `attention-` still has to find the draft to learn what the row is for — the
// prefix rule buys the noun, not the reason for it — and seven
// permissions naming entities nobody had registered were caught only because
// somebody read two files side by side. One described record per entity is
// what makes the registry answerable instead of merely complete.
type Spec struct {
	// Name is the qualified entity name — authority.context.entity, always
	// `initagent.` here, because this package registers only our own words.
	Name string

	// Context is the plane, and it must be the middle segment of Name. Kept
	// as a field rather than parsed out of the string so a caller routing by
	// context is not writing a parser; the generator refuses a row where the
	// two disagree.
	Context Context

	// Description says what the entity is and why it exists. It is mandatory
	// — the generator refuses an empty one — because the cheap failure mode
	// here is a row that restates its own name and teaches nobody anything.
	Description string

	// Lifetime is when the thing appears and when it stops mattering. It
	// carries the Lifetime column of 05's entity table, so retention and
	// cleanup questions have one answer rather than one per package.
	Lifetime string

	// Borrows is the kind whose prefix an unminted entity carries instead of
	// one of its own. It is empty for everything that mints, and for everything
	// that nothing identifies.
	//
	// Only `initagent.project.binding` uses it today: the gateway's own row for
	// a project carries the `project-` the hub minted, which is the one place
	// 07's golden rule is deliberately narrowed. It is a field rather than a
	// sentence because a caller holding `project-…` on a gateway has to resolve
	// it, and prose does not answer that — the generator also refuses a borrow
	// of a prefix nothing mints.
	Borrows Kind
}

// Entity returns the qualified entity name for a prefix.
func Entity(k Kind) (string, bool) {
	spec, ok := entities[k]
	return spec.Name, ok
}

// Describe returns everything the registry knows about a prefix. Use it where
// a caller needs more than the name — an operator screen, a generated
// reference — so the explanation travels with the vocabulary instead of being
// rewritten beside it.
func Describe(k Kind) (Spec, bool) {
	spec, ok := entities[k]
	return spec, ok
}

// Kinds returns every registered prefix. Order is not stable.
func Kinds() []Kind {
	out := make([]Kind, 0, len(entities))
	for k := range entities {
		out = append(out, k)
	}
	return out
}

// Entities returns every entity in the vocabulary — the ones with a prefix
// and the ones without — sorted by qualified name.
//
// This is the list a permission, an event kind or a document checks itself
// against. Before there was one, nothing enumerated the words a permission
// could name, which is how seven capabilities came to name four entities that
// did not exist.
//
// Three reasons an entity lands in the unminted half. Nothing identifies it on
// our side (a file is named by a device and a path; an identity by a provider
// and a subject). Its identifier belongs to something else (a preset carries
// the store's integer key inherited from upstream, a template a catalogue slug
// that ships in the binary). Or it shares an identifier minted for another
// entity — Spec.Borrows, which today is only the project binding. They are kept
// out of the prefix map because its keys are exactly what New may mint:
// inventing a prefix so a row fits would make New able to mint an identifier
// for a thing that has none, or a second one for a thing that already carries
// somebody else's, which is a worse lie than the gap it closes.
func Entities() []Spec {
	out := make([]Spec, 0, len(entities)+len(unminted))
	for _, spec := range entities {
		out = append(out, spec)
	}
	out = append(out, unminted...)
	slices.SortFunc(out, func(a, b Spec) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Lookup finds an entity by its qualified name, with or without a prefix.
//
// The scan is linear over a few dozen rows, which is cheaper than a second
// index that has to be kept agreeing with the first.
func Lookup(name string) (Spec, bool) {
	for _, spec := range entities {
		if spec.Name == name {
			return spec, true
		}
	}
	for _, spec := range unminted {
		if spec.Name == name {
			return spec, true
		}
	}
	return Spec{}, false
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
