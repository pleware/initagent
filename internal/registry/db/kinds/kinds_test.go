package kinds

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// declaredKinds reads this package's own source for constants typed Kind.
//
// The alternative is a hand-maintained list in the test, which drifts exactly
// when it matters: someone adds a Kind constant, forgets the Registry entry,
// and the test that was supposed to catch it was never told about the new
// constant.
func declaredKinds(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "kinds.go", nil, 0)
	if err != nil {
		t.Fatalf("parse kinds.go: %v", err)
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
	t.Parallel()
	for name, kind := range declaredKinds(t) {
		if _, ok := Registry[Kind(kind)]; !ok {
			t.Errorf("Kind %s (%q) is declared but missing from Registry", name, kind)
		}
	}
}

func TestEveryRegisteredKindIsDeclared(t *testing.T) {
	t.Parallel()
	declared := declaredKinds(t)
	declaredSet := map[string]bool{}
	for _, kind := range declared {
		declaredSet[kind] = true
	}
	for kind := range Registry {
		if !declaredSet[string(kind)] {
			t.Errorf("Kind %q is in Registry but has no constant declared", kind)
		}
	}
}

func TestRegistrySpecsAreValid(t *testing.T) {
	t.Parallel()
	for kind, spec := range Registry {
		if spec.Scheme == "" {
			t.Errorf("Kind %q has empty Scheme", kind)
		}
	}
}

func TestIsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind Kind
		want bool
	}{
		{KindPostgres, true},
		{KindSQLiteFile, true},
		{Kind("nonexistent"), false},
		{Kind(""), false},
	}
	for _, tc := range cases {
		if got := IsValid(tc.kind); got != tc.want {
			t.Errorf("IsValid(%q) = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestKindsReturnsAll(t *testing.T) {
	t.Parallel()
	got := Kinds()
	if len(got) != len(Registry) {
		t.Errorf("Kinds() returned %d kinds, Registry has %d", len(got), len(Registry))
	}
	seen := map[Kind]bool{}
	for _, k := range got {
		if seen[k] {
			t.Errorf("Kinds() returned duplicate: %q", k)
		}
		seen[k] = true
	}
}

func TestLocalFlagLocked(t *testing.T) {
	t.Parallel()
	// Only file-backed kinds are local; every network kind must report false
	// so the walk-up and capability report treat them as reachable hosts.
	for kind, spec := range Registry {
		if kind == KindSQLiteFile {
			if !spec.Local {
				t.Errorf("Kind %q must be Local", kind)
			}
		} else if spec.Local {
			t.Errorf("Kind %q is a network DSN but reports Local=true", kind)
		}
	}
}

func TestSchemeLocked(t *testing.T) {
	t.Parallel()
	// Pin the conventional URL schemes so a DSN kind does not drift silently.
	want := map[Kind]string{
		KindPostgres:   "postgres",
		KindMySQL:      "mysql",
		KindMSSQL:      "sqlserver",
		KindSQLiteFile: "file",
		KindRedis:      "redis",
	}
	for kind, scheme := range want {
		if got := Registry[kind].Scheme; got != scheme {
			t.Errorf("Kind %q Scheme = %q, want %q", kind, got, scheme)
		}
	}
}
