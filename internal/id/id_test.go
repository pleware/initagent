package id

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// declaredKinds reads this package's own source for constants typed Kind.
//
// The alternative is a hand-maintained list in the test, which drifts exactly
// when it matters: someone adds a prefix, forgets the registry, and the test
// that was supposed to catch it was never told about the new constant.
func declaredKinds(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "id.go", nil, 0)
	if err != nil {
		t.Fatalf("parse id.go: %v", err)
	}
	found := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Kind" {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				found[name.Name] = strings.Trim(lit.Value, `"`)
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no Kind constants found; the AST walk is broken, not the registry")
	}
	return found
}

func TestEveryDeclaredKindIsRegistered(t *testing.T) {
	for name, prefix := range declaredKinds(t) {
		if _, ok := entities[Kind(prefix)]; !ok {
			t.Errorf("Kind %s (%q) is declared but missing from entities", name, prefix)
		}
	}
}

func TestRegistryIsWellFormed(t *testing.T) {
	seen := map[string]Kind{}
	for k, entity := range entities {
		prefix := string(k)
		if len(prefix) != 3 {
			t.Errorf("prefix %q is %d characters; the table uses three", prefix, len(prefix))
		}
		if strings.Contains(prefix, Separator) {
			t.Errorf("prefix %q contains the separator, which makes Parse ambiguous", prefix)
		}
		if prefix != strings.ToLower(prefix) {
			t.Errorf("prefix %q is not lowercase", prefix)
		}
		if strings.Count(entity, ".") != 2 {
			t.Errorf("entity %q is not authority.context.entity", entity)
		}
		if other, dup := seen[entity]; dup {
			t.Errorf("entity %q is claimed by both %q and %q", entity, other, k)
		}
		seen[entity] = k
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
		{"missing uuid", "dev-"},
		{"malformed uuid", "dev-not-a-uuid"},
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
	v4 := "dev-" + uuid.NewString()
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
