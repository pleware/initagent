package authz

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/id"
)

// The permission plane derives its right-hand side from the entity registry
// (05, "Four planes, one derivation"): a capability may only name an entity
// that exists. Nothing joined the two lists, so seven capabilities named four
// entities nobody had written down, and the drift was found by reading two
// files side by side rather than by anything failing.
//
// These tests live here rather than in internal/id because the dependency has
// to point this way. internal/id is a leaf — uuid and the standard library —
// and every plane that spells an entity name sits above it. Importing authz
// from the vocabulary would invert that, and the next plane to be joined (the
// seam's wire kinds, in a second repository) would invert it again.

// declaredCapabilities reads this package's own source for constants typed
// Capability, in the shape internal/registry/db/kinds uses.
//
// Reading the constants rather than Capabilities() is the point: a capability
// that is declared but absent from both maps below would be invisible to the
// derived list, and invisible is exactly how the drift happened.
func declaredCapabilities(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "authz.go", nil, 0)
	if err != nil {
		t.Fatalf("parse authz.go: %v", err)
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
			if !ok || ident.Name != "Capability" {
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
		t.Fatal("no Capability constants found; the AST walk is broken, not the grammar")
	}
	return found
}

func TestEveryCapabilityNamesARegisteredEntity(t *testing.T) {
	t.Parallel()
	for name, value := range declaredCapabilities(t) {
		verb, entity, found := strings.Cut(value, ":")
		if !found {
			t.Errorf("%s (%q) has no colon separating the verb from the entity", name, value)
			continue
		}
		if verb == "" || strings.ContainsAny(verb, ". ") {
			t.Errorf("%s (%q) has %q where a bare verb belongs", name, value, verb)
			continue
		}
		if strings.Count(entity, ".") != 1 {
			t.Errorf("%s (%q) names %q, which is not context.entity", name, value, entity)
			continue
		}
		// The authority is ours by construction: this package grants verbs on
		// our own surface only, so the capability carries the tail of a
		// qualified name and the registry holds the whole one.
		qualified := "initagent." + entity
		if _, ok := id.Lookup(qualified); !ok {
			t.Errorf("%s (%q) names %s, which no entity in internal/id registers", name, value, qualified)
		}
	}
}

// TestEveryCapabilityIsReachable is the other direction. Capabilities() is
// derived from the two maps rather than kept beside them, so a constant in
// neither is grantable by nobody and enforced by nothing — a permission that
// reads as real in a handler and refuses everyone.
func TestEveryCapabilityIsReachable(t *testing.T) {
	t.Parallel()
	reachable := map[string]bool{}
	for _, c := range Capabilities() {
		reachable[string(c)] = true
	}
	for name, value := range declaredCapabilities(t) {
		if !reachable[value] {
			t.Errorf("%s (%q) is declared but is neither an installation nor an org capability", name, value)
		}
	}
}
