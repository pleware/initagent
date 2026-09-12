package id

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Four tests and their AST helpers were deleted when the registry became
// generated from `names/names.yaml`, because the invariant each one watched
// moved into the generator and is no longer reachable from here:
//
//   - TestEveryDeclaredKindIsRegistered and TestEveryRegisteredKindIsDeclared
//     walked this package's own constants and held them to the entities map.
//     Both now come from one YAML row in one emission, so a constant without a
//     registry entry cannot be written.
//   - TestKindIsTheLastSegment asserted a prefix spelled its entity's last
//     segment. The generator now *derives* the prefix from the name, so the two
//     are one fact rather than two agreeing ones; `internal/names`
//     TestValidateRefuses covers the source-side rules that remain (a prefix
//     that is upper case, carries the separator, or is claimed twice).
//   - TestDeclaredContextsAreTheFive walked the Context constants against
//     Contexts(). Both are emitted from the same list;
//     TestContextsAreTheFive below keeps the claim that there are five.
//
// The tests that survive are the ones that assert something about the
// *committed* data or about behaviour: a hand-edit of registry_gen.go is caught
// by the goldens test in `internal/names`, and these keep working if somebody
// tries one anyway.

// TestRegistryIsWellFormed is kept rather than replaced. Every rule in it is
// also a generator refusal, and it costs nothing to hold the artifact that
// actually ships to the same bar as the source it came from.
func TestRegistryIsWellFormed(t *testing.T) {
	seen := map[string]Kind{}
	for k, spec := range entities {
		prefix := string(k)
		if prefix == "" {
			t.Error("a registered entity has an empty prefix")
		}
		if strings.Contains(prefix, Separator) {
			t.Errorf("prefix %q contains the separator, which makes Parse ambiguous", prefix)
		}
		if prefix != strings.ToLower(prefix) {
			t.Errorf("prefix %q is not lowercase", prefix)
		}
		if strings.Count(spec.Name, ".") != 2 {
			t.Errorf("entity %q is not authority.context.entity", spec.Name)
		}
		if other, dup := seen[spec.Name]; dup {
			t.Errorf("entity %q is claimed by both %q and %q", spec.Name, other, k)
		}
		seen[spec.Name] = k
	}
}

// TestEverySpecIsDescribed is kept for the same reason: the description and the
// lifetime are the most valuable content in the registry, and a generated file
// is exactly where a truncated one would go unnoticed.
func TestEverySpecIsDescribed(t *testing.T) {
	allowed := map[Context]bool{}
	for _, c := range Contexts() {
		allowed[c] = true
	}
	seen := map[string]bool{}
	for _, spec := range Entities() {
		if spec.Name == "" {
			t.Error("a registered entity has no name")
			continue
		}
		if seen[spec.Name] {
			t.Errorf("entity %q is registered twice", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Description == "" {
			t.Errorf("entity %q has no description", spec.Name)
		}
		if spec.Lifetime == "" {
			t.Errorf("entity %q has no lifetime", spec.Name)
		}
		if !allowed[spec.Context] {
			t.Errorf("entity %q is in context %q, which is not one of the five", spec.Name, spec.Context)
		}
		rest, ok := strings.CutPrefix(spec.Name, "initagent.")
		if !ok {
			t.Errorf("entity %q does not start with the authority \"initagent.\"", spec.Name)
			continue
		}
		// The context is a field *and* the middle segment of the name. The
		// generator refuses a row where the two disagree; this holds the
		// emitted result to the same thing.
		context, _, ok := strings.Cut(rest, ".")
		if !ok {
			t.Errorf("entity %q has no entity segment after its context", spec.Name)
			continue
		}
		if context != string(spec.Context) {
			t.Errorf("entity %q spells context %q but its Context field is %q", spec.Name, context, spec.Context)
		}
	}
}

// TestContextsAreTheFive replaces TestDeclaredContextsAreTheFive, which walked
// the AST. The constants and Contexts() are one emission now, so comparing them
// to each other proves nothing; comparing them to the ontology still does. A
// sixth plane arriving in names.yaml fails here, which is the point — a context
// spells half of every name, event and scope.
func TestContextsAreTheFive(t *testing.T) {
	want := []Context{ContextHub, ContextProject, ContextGDesk, ContextFleet, ContextWorker}
	got := Contexts()
	if !slices.Equal(got, want) {
		t.Fatalf("Contexts() = %v, want the five planes %v", got, want)
	}
	if got := Contexts(); len(got) > 0 {
		// Contexts() hands out a copy: a caller sorting the result must not be
		// reordering the ontology for everybody else.
		got[0] = Context("mutated")
		if Contexts()[0] != ContextHub {
			t.Error("Contexts() exposes the package's own slice")
		}
	}
}

func TestDescribe(t *testing.T) {
	spec, ok := Describe(Device)
	if !ok {
		t.Fatal("Describe(Device) reported unregistered")
	}
	if spec.Name != "initagent.fleet.device" || spec.Context != ContextFleet {
		t.Errorf("Describe(Device) = %+v", spec)
	}
	if _, ok := Describe(Kind("zzz")); ok {
		t.Error("Describe accepted an unregistered kind")
	}
}

func TestEntitiesCoversBothHalvesAndSorts(t *testing.T) {
	all := Entities()
	if want := len(entities) + len(unminted); len(all) != want {
		t.Errorf("Entities() returned %d, want %d", len(all), want)
	}
	if !slices.IsSortedFunc(all, func(a, b Spec) int { return strings.Compare(a.Name, b.Name) }) {
		t.Error("Entities() is not sorted by name")
	}
}

func TestLookup(t *testing.T) {
	// One with a prefix and one without: an entity that mints no identifier
	// still has to resolve, because that is the whole reason Lookup exists.
	for _, name := range []string{"initagent.hub.org", "initagent.fleet.file"} {
		spec, ok := Lookup(name)
		if !ok {
			t.Errorf("Lookup(%q) found nothing", name)
			continue
		}
		if spec.Name != name {
			t.Errorf("Lookup(%q) returned %q", name, spec.Name)
		}
	}
	if _, ok := Lookup("initagent.hub.invented"); ok {
		t.Error("Lookup resolved an entity that is not registered")
	}
}

func TestUnmintedEntitiesMintNothing(t *testing.T) {
	// The point of the second list is that these words resolve while minting
	// nothing. If one ever gained a prefix, New would start handing out
	// identifiers for a thing that has none of its own — and for the project
	// binding it would hand out a second one for a row that already carries
	// the hub's.
	for _, spec := range unminted {
		if _, ok := Lookup(spec.Name); !ok {
			t.Errorf("unminted entity %q does not resolve", spec.Name)
		}
		for k, registered := range entities {
			if registered.Name == spec.Name {
				t.Errorf("entity %q is both unminted and registered under %q", spec.Name, k)
			}
		}
	}
}

// TestBorrowedPrefixesResolve is new with Spec.Borrows. The gateway's row for a
// project carries the `project-` the hub minted, and a caller holding that
// identifier has to be able to get from the borrowing entity to the minting one
// without reading a comment. A borrow of a prefix nothing mints is refused by
// the generator; this is the other half — that the value is a live Kind here.
func TestBorrowedPrefixesResolve(t *testing.T) {
	borrowed := 0
	for _, spec := range Entities() {
		if spec.Borrows == "" {
			continue
		}
		borrowed++
		if _, ok := Describe(spec.Borrows); !ok {
			t.Errorf("entity %q borrows %q, which is not a registered kind", spec.Name, spec.Borrows)
		}
		if _, ok := Describe(Kind(strings.TrimPrefix(spec.Name, "initagent."))); ok {
			t.Errorf("entity %q borrows a prefix and also mints one", spec.Name)
		}
	}
	if borrowed == 0 {
		t.Error("no entity borrows a prefix; the project binding should")
	}
	binding, ok := Lookup("initagent.project.binding")
	if !ok {
		t.Fatal("the project binding is not registered")
	}
	if binding.Borrows != Project {
		t.Errorf("the project binding borrows %q, want %q", binding.Borrows, Project)
	}
}

func TestNewRoundTrips(t *testing.T) {
	for _, k := range Kinds() {
		got, err := New(k)
		if err != nil {
			t.Fatalf("New(%q): %v", k, err)
		}
		if !strings.HasPrefix(got, string(k)+Separator) {
			t.Errorf("New(%q) = %q, missing prefix", k, got)
		}
		kind, u, err := Parse(got)
		if err != nil {
			t.Fatalf("Parse(%q): %v", got, err)
		}
		if kind != k {
			t.Errorf("Parse(%q) kind = %q, want %q", got, kind, k)
		}
		if u.Version() != 7 {
			t.Errorf("Parse(%q) uuid version = %d, want 7", got, u.Version())
		}
		if !Is(k, got) {
			t.Errorf("Is(%q, %q) = false", k, got)
		}
	}
}

func TestNewRejectsUnregisteredKind(t *testing.T) {
	if _, err := New(Kind("zzz")); err == nil {
		t.Fatal("New accepted an unregistered kind")
	}
}

func TestNewIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		got, err := New(Task)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if seen[got] {
			t.Fatalf("New returned %q twice", got)
		}
		seen[got] = true
	}
}

func TestParseRejects(t *testing.T) {
	// The UUID's own hyphens are why Parse cuts at the first separator; the
	// bare-uuid case below is what a naive split would mangle into a prefix.
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"no separator", "dev0198f3a17c4e7b2a9f312c6a8d4e5b70"},
		{"unregistered prefix", "zzz-0198f3a1-7c4e-7b2a-9f31-2c6a8d4e5b70"},
		{"missing uuid", "device-"},
		{"malformed uuid", "device-not-a-uuid"},
		{"bare uuid", "0198f3a1-7c4e-7b2a-9f31-2c6a8d4e5b70"},
		{"upstream shape", "a3f9c2d1e8b7f4a0"},
		{"prefix only", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Parse(tc.in); err == nil {
				t.Errorf("Parse(%q) succeeded", tc.in)
			}
		})
	}
}

func TestIsRejectsWrongKind(t *testing.T) {
	project, err := New(Project)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if Is(Device, project) {
		t.Errorf("Is(Device, %q) = true; a project id must not pass as a device id", project)
	}
}

func TestParseAcceptsAnyUUIDVersion(t *testing.T) {
	// Parse validates shape, not provenance: identifiers minted before we
	// moved to v7 must still resolve.
	v4 := "device-" + uuid.NewString()
	if _, _, err := Parse(v4); err != nil {
		t.Errorf("Parse rejected a v4-backed identifier: %v", err)
	}
}

func TestEntity(t *testing.T) {
	if name, ok := Entity(Device); !ok || name != "initagent.fleet.device" {
		t.Errorf("Entity(Device) = %q, %v", name, ok)
	}
	if _, ok := Entity(Kind("zzz")); ok {
		t.Error("Entity accepted an unregistered kind")
	}
}
