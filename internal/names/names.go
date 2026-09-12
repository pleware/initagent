// Package names reads the naming registry (`names/names.yaml`) and emits the
// artifacts every other plane derives from it: the Go registry in
// `internal/id`, and `names.json` for consumers in other repositories.
//
// The YAML parsing lives here and nowhere else. `internal/id` is a leaf — a
// UUID library and the standard library — because every plane that spells an
// entity name sits above it, and a vocabulary that had to parse a file to
// answer "is this prefix real" would invert that.
//
// Nothing here imports `internal/id`. The dependency would be a cycle in
// practice rather than in the compiler: a generated file this package rejected
// would stop the package that regenerates it from building.
//
// Invariants are enforced at emission, not in a test downstream. A generator
// that emits data a later test catches has already written the bad file to
// disk, and on a repository where the generated file is committed that is a
// diff somebody reviews. Load refuses instead.
package names

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// separator divides an identifier's prefix from its UUID. It is spelled here
// rather than imported from `internal/id` so this package can still build when
// the file it generates does not.
const separator = "-"

// segments is how many dot-separated parts a qualified name has:
// authority.context.entity.
const segments = 3

// ErrInvalid is the class of every refusal below, so a caller — the goldens
// test, `cmd/namesgen` — can tell bad data from a missing file.
var ErrInvalid = errors.New("names: invalid registry")

// Registry is the whole of `names/names.yaml`.
type Registry struct {
	// Schema is this file's own shape version. It moves when a consumer would
	// have to change to keep reading names.json, which is not the same event as
	// the seam envelope moving.
	Schema int `yaml:"schema"`

	// Authority prefixes every qualified name. One value, ours, checked rather
	// than assumed: a row that spelled a different authority would be claiming
	// somebody else's vocabulary.
	Authority string `yaml:"authority"`

	// Seam carries what the glass and this product negotiate on.
	Seam Seam `yaml:"seam"`

	// Contexts is the five planes, in the order the ontology gives them.
	Contexts []Context `yaml:"contexts"`

	// Entities is every entity the vocabulary owns, minted or not.
	Entities []Entity `yaml:"entities"`
}

// Seam is the envelope the gdesk seam negotiates.
type Seam struct {
	EnvelopeVersion int `yaml:"envelope_version"`
}

// Context is one plane and its Go constant.
type Context struct {
	Name string `yaml:"name"`

	// GoConst is the exported Go identifier for this plane. It is the one
	// language-specific binding in the source file: `gdesk` does not case-fold
	// to `GDesk` under any rule worth writing down, and these identifiers are
	// already public API.
	GoConst string `yaml:"go_const"`
}

// Entity is one row of the vocabulary.
//
// There is deliberately no prefix field. The prefix is the last segment of
// Name, spelled out in full, so storing it separately would reintroduce the
// disagreement this file exists to remove. Prefix derives it.
type Entity struct {
	Name        string `yaml:"name"`
	Context     string `yaml:"context"`
	GoConst     string `yaml:"go_const"`
	Mints       bool   `yaml:"mints"`
	Borrows     string `yaml:"borrows"`
	Lifetime    string `yaml:"lifetime"`
	Description string `yaml:"description"`

	// Capabilities are the bare verbs granted on this entity. The permission
	// plane's `verb:context.entity` strings derive from them, so a capability
	// cannot name an entity that is not here.
	Capabilities []string `yaml:"capabilities"`
}

// Prefix is the entity's identifier prefix: the last segment of its name.
func (e Entity) Prefix() string {
	i := strings.LastIndex(e.Name, ".")
	if i < 0 {
		return e.Name
	}
	return e.Name[i+1:]
}

// Tail is the `context.entity` half a capability and a happening both carry.
func (e Entity) Tail() string { return e.Context + "." + e.Prefix() }

// HappeningPrefix is what a producer puts in front of a verb to name a fact
// about this entity: `gdesk.surface` yields `gdesk.surface.opened`.
//
// It is the same string as Tail. They are separate methods because they are
// separate claims — the permission grammar and the happening grammar happen to
// share a left-hand side today, and a consumer reading one should not have to
// know that.
func (e Entity) HappeningPrefix() string { return e.Tail() }

// CapabilityNames is the `verb:context.entity` strings this entity grants,
// sorted.
func (e Entity) CapabilityNames() []string {
	out := make([]string, 0, len(e.Capabilities))
	for _, verb := range e.Capabilities {
		out = append(out, verb+":"+e.Tail())
	}
	slices.Sort(out)
	return out
}

// SortedEntities is every entity ordered by qualified name.
//
// Emission reads this and never the file order, so moving a row in the YAML
// cannot change a generated byte.
func (r *Registry) SortedEntities() []Entity {
	out := slices.Clone(r.Entities)
	slices.SortFunc(out, func(a, b Entity) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Minted is every entity this registry hands out an identifier for, sorted.
func (r *Registry) Minted() []Entity {
	var out []Entity
	for _, e := range r.SortedEntities() {
		if e.Mints {
			out = append(out, e)
		}
	}
	return out
}

// Unminted is every entity something else names, sorted.
func (r *Registry) Unminted() []Entity {
	var out []Entity
	for _, e := range r.SortedEntities() {
		if !e.Mints {
			out = append(out, e)
		}
	}
	return out
}

// Capabilities is every `verb:context.entity` string the vocabulary grants,
// sorted. This is the list `internal/authz` restates under Go names.
func (r *Registry) Capabilities() []string {
	var out []string
	for _, e := range r.SortedEntities() {
		out = append(out, e.CapabilityNames()...)
	}
	slices.Sort(out)
	return out
}

// ContextNames is the five planes in file order — the order matters, so this
// is not sorted.
func (r *Registry) ContextNames() []string {
	out := make([]string, 0, len(r.Contexts))
	for _, c := range r.Contexts {
		out = append(out, c.Name)
	}
	return out
}

// contextConst maps a plane to its Go identifier.
func (r *Registry) contextConst(name string) string {
	for _, c := range r.Contexts {
		if c.Name == name {
			return c.GoConst
		}
	}
	return ""
}

// Load reads and validates the registry at path.
func Load(path string) (*Registry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("names: read %s: %w", path, err)
	}
	r, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("names: %s: %w", path, err)
	}
	return r, nil
}

// Parse decodes and validates the registry.
//
// Decoding is strict: an unknown key is refused rather than ignored, because a
// misspelled `capabilties:` that parsed silently would drop a permission and
// leave nothing behind to notice.
func Parse(raw []byte) (*Registry, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	var r Registry
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// Validate refuses a registry that would emit a lie.
//
// Every problem is reported, not just the first: a hand-edited file usually
// has more than one, and fixing them one build at a time is how somebody stops
// reading the message.
func (r *Registry) Validate() error {
	var problems []error
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	if r.Schema < 1 {
		report("schema is %d, which is not a released shape", r.Schema)
	}
	if r.Authority == "" {
		report("authority is empty, so no name can be checked against it")
	}
	if r.Seam.EnvelopeVersion < 1 {
		report("seam.envelope_version is %d; a consumer has nothing to negotiate on", r.Seam.EnvelopeVersion)
	}

	planes := r.validateContexts(report)
	r.validateEntities(report, planes)

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrInvalid, errors.Join(problems...))
}

// validateContexts checks the plane list and returns the set of legal planes.
func (r *Registry) validateContexts(report func(string, ...any)) map[string]bool {
	planes := map[string]bool{}
	consts := map[string]bool{}
	for _, c := range r.Contexts {
		switch {
		case c.Name == "":
			report("a context has no name")
			continue
		case c.Name != strings.ToLower(c.Name):
			report("context %q is not lower case", c.Name)
		case strings.ContainsAny(c.Name, ". "):
			report("context %q is not a bare word", c.Name)
		}
		if planes[c.Name] {
			report("context %q is declared twice", c.Name)
		}
		planes[c.Name] = true
		if !exportedGoIdent(c.GoConst) {
			report("context %q has go_const %q, which is not an exported Go identifier", c.Name, c.GoConst)
			continue
		}
		if consts[c.GoConst] {
			report("go_const %q is claimed by two contexts", c.GoConst)
		}
		consts[c.GoConst] = true
	}
	if len(planes) == 0 {
		report("no contexts are declared, so every entity is in an unknown plane")
	}
	return planes
}

// validateEntities is where the four rules in names.yaml's header are enforced.
func (r *Registry) validateEntities(report func(string, ...any), planes map[string]bool) {
	var (
		names    = map[string]bool{}
		prefixes = map[string]string{}
		consts   = map[string]string{}
		minted   = map[string]bool{}
	)
	for _, e := range r.Entities {
		if e.Name == "" {
			report("an entity has no name")
			continue
		}
		if names[e.Name] {
			report("entity %q is declared twice", e.Name)
		}
		names[e.Name] = true

		parts := strings.Split(e.Name, ".")
		switch {
		case len(parts) != segments:
			report("entity %q has %d segments, not authority.context.entity", e.Name, len(parts))
			continue
		case parts[0] != r.Authority:
			report("entity %q does not start with the authority %q", e.Name, r.Authority+".")
		}
		if parts[1] != e.Context {
			report("entity %q spells context %q but declares %q", e.Name, parts[1], e.Context)
		}
		if !planes[e.Context] {
			report("entity %q is in context %q, which is not a declared plane", e.Name, e.Context)
		}

		prefix := parts[2]
		switch {
		case prefix == "":
			report("entity %q has an empty prefix", e.Name)
		case prefix != strings.ToLower(prefix):
			report("prefix %q of %q is not lower case", prefix, e.Name)
		case strings.Contains(prefix, separator):
			report("prefix %q of %q contains %q, which makes an identifier ambiguous to split", prefix, e.Name, separator)
		}
		if other, dup := prefixes[prefix]; dup {
			report("prefix %q is claimed by both %q and %q", prefix, other, e.Name)
		}
		prefixes[prefix] = e.Name

		if e.Lifetime == "" {
			report("entity %q has no lifetime", e.Name)
		}
		if strings.TrimSpace(e.Description) == "" {
			report("entity %q has no description", e.Name)
		}

		if e.Mints {
			minted[prefix] = true
			if e.Borrows != "" {
				report("entity %q mints %q and also borrows %q; it cannot be both", e.Name, prefix, e.Borrows)
			}
			if !exportedGoIdent(e.GoConst) {
				report("entity %q has go_const %q, which is not an exported Go identifier", e.Name, e.GoConst)
			} else if other, dup := consts[e.GoConst]; dup {
				report("go_const %q is claimed by both %q and %q", e.GoConst, other, e.Name)
			} else {
				consts[e.GoConst] = e.Name
			}
		} else if e.GoConst != "" {
			report("entity %q mints nothing but declares go_const %q, which would let New hand out an identifier it has no right to", e.Name, e.GoConst)
		}

		r.validateVerbs(report, e)
	}
	r.validateBorrows(report, minted)
}

// validateVerbs checks the bare verbs a row grants.
func (r *Registry) validateVerbs(report func(string, ...any), e Entity) {
	seen := map[string]bool{}
	for _, verb := range e.Capabilities {
		switch {
		case verb == "":
			report("entity %q grants an empty verb", e.Name)
			continue
		case verb != strings.ToLower(verb):
			report("verb %q on %q is not lower case", verb, e.Name)
		case strings.ContainsAny(verb, ".: "):
			report("verb %q on %q is not a bare word; the grammar adds the colon", verb, e.Name)
		}
		if seen[verb] {
			report("verb %q is granted twice on %q", verb, e.Name)
		}
		seen[verb] = true
	}
}

// validateBorrows runs after every prefix is known: a row may only borrow a
// prefix somebody actually mints.
func (r *Registry) validateBorrows(report func(string, ...any), minted map[string]bool) {
	for _, e := range r.Entities {
		if e.Borrows == "" || e.Mints {
			continue
		}
		if !minted[e.Borrows] {
			report("entity %q borrows prefix %q, which nothing mints", e.Name, e.Borrows)
		}
	}
}

// exportedGoIdent reports whether s is a Go identifier this generator may emit
// as an exported name. Digits and underscores are allowed after the first
// rune; the first must be an upper-case letter.
func exportedGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsUpper(r) {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}
