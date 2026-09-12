package names

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestEmitGoIsCompilableAndImportsNothing(t *testing.T) {
	t.Parallel()
	got, err := EmitGo(valid())
	if err != nil {
		t.Fatalf("EmitGo: %v", err)
	}
	src := string(got)

	if !strings.HasPrefix(src, generatedBy+"\n") {
		t.Errorf("first line is not the generated marker:\n%s", firstLine(src))
	}

	file, err := parser.ParseFile(token.NewFileSet(), "registry_gen.go", got, parser.ParseComments)
	if err != nil {
		t.Fatalf("emitted Go does not parse: %v", err)
	}
	if file.Name.Name != GoPackage {
		t.Errorf("package %s, want %s", file.Name.Name, GoPackage)
	}
	// The whole reason the YAML is parsed in this package: internal/id stays a
	// leaf, so the file it receives may not add an import.
	if len(file.Imports) != 0 {
		t.Errorf("generated file imports %d packages; internal/id must stay a leaf", len(file.Imports))
	}
}

func TestEmitGoCarriesTheDataAndDerivesThePrefix(t *testing.T) {
	t.Parallel()
	got, err := EmitGo(valid())
	if err != nil {
		t.Fatalf("EmitGo: %v", err)
	}
	// gofmt aligns a const block, so these are matched with the whitespace
	// collapsed rather than by column.
	src := strings.Join(strings.Fields(string(got)), " ")
	for _, want := range []string{
		`ContextHub Context = "hub"`,
		`contexts = []Context{ContextHub, ContextFleet}`,
		`Project Kind = "project"`,
		`Name: "initagent.hub.project"`,
		`Lifetime: "project lifetime"`,
		`Borrows: Project`,
		"var unminted = []Spec{",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated Go does not contain %q", want)
		}
	}
	// An unminted entity may not gain a prefix constant: New would start
	// handing out identifiers for a thing that has none.
	if strings.Contains(src, `Kind = "file"`) {
		t.Error("an unminted entity was emitted as a Kind constant")
	}
}

// TestEmitGoRefusesWhatValidateWouldHaveRejected covers the one path that only
// exists because EmitGo does not re-validate: a caller that skipped Load hands
// it a row whose Go identifier is not one, and the emitted text does not parse.
func TestEmitGoRefusesWhatValidateWouldHaveRejected(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Registry){
		"a Go constant that is not an identifier": func(r *Registry) { r.Entities[0].GoConst = "1project" },
		"a plane with no Go constant":             func(r *Registry) { r.Contexts[0].GoConst = "" },
		"a borrow of a prefix nothing mints":      func(r *Registry) { r.Entities[2].Borrows = "invented" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := valid()
			mutate(r)
			if _, err := EmitGo(r); err == nil {
				t.Fatalf("EmitGo accepted %s", name)
			}
		})
	}
}

func TestEmitJSONCarriesEveryPlaneAndNoProse(t *testing.T) {
	t.Parallel()
	raw, err := EmitJSON(valid())
	if err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		t.Error("names.json has no trailing newline")
	}
	// The descriptions are the most valuable thing in the source and the least
	// useful thing in a generated artifact in another language: a six-line
	// paragraph makes every diff unreadable and is read by nobody.
	if bytes.Contains(raw, []byte("catalogue entry")) {
		t.Error("a description leaked into names.json")
	}

	var doc struct {
		Schema              int    `json:"schema"`
		Authority           string `json:"authority"`
		SeamEnvelopeVersion int    `json:"seamEnvelopeVersion"`
		Grammar             struct{ Happening, Permission, Separator string }
		Contexts            []string `json:"contexts"`
		Capabilities        []string `json:"capabilities"`
		HappeningPrefixes   []string `json:"happeningPrefixes"`
		Entities            []struct {
			Name             string   `json:"name"`
			Context          string   `json:"context"`
			Entity           string   `json:"entity"`
			IdentifierPrefix string   `json:"identifierPrefix"`
			Mints            bool     `json:"mints"`
			Borrows          string   `json:"borrows"`
			Capabilities     []string `json:"capabilities"`
			HappeningPrefix  string   `json:"happeningPrefix"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("names.json does not decode: %v", err)
	}
	if doc.Schema != 1 || doc.Authority != "initagent" || doc.SeamEnvelopeVersion != 2 {
		t.Errorf("envelope = %+v", doc)
	}
	if doc.Grammar.Happening != "<context>.<entity>.<verb>" {
		t.Errorf("happening grammar = %q", doc.Grammar.Happening)
	}
	if doc.Grammar.Separator != "-" {
		t.Errorf("separator = %q", doc.Grammar.Separator)
	}
	if len(doc.Entities) != 3 {
		t.Fatalf("%d entities, want 3", len(doc.Entities))
	}
	byName := map[string]int{}
	for i, e := range doc.Entities {
		byName[e.Name] = i
	}

	project := doc.Entities[byName["initagent.hub.project"]]
	if project.IdentifierPrefix != "project-" {
		t.Errorf("identifierPrefix = %q, want the separator included", project.IdentifierPrefix)
	}
	if project.HappeningPrefix != "hub.project" {
		t.Errorf("happeningPrefix = %q", project.HappeningPrefix)
	}
	if len(project.Capabilities) != 2 || project.Capabilities[0] != "create:hub.project" {
		t.Errorf("capabilities = %v, want sorted verb:context.entity", project.Capabilities)
	}

	file := doc.Entities[byName["initagent.fleet.file"]]
	if file.Mints || file.IdentifierPrefix != "" {
		t.Errorf("an unminted entity reports a prefix: %+v", file)
	}
	// A consumer holding `project-…` on a gateway row has to resolve it to the
	// binding, and prose in a Go comment does not answer that.
	binding := doc.Entities[byName["initagent.hub.binding"]]
	if binding.Borrows != "project" || binding.IdentifierPrefix != "project-" {
		t.Errorf("the borrowed prefix is not resolvable: %+v", binding)
	}

	if len(doc.Capabilities) != 2 {
		t.Errorf("capabilities = %v", doc.Capabilities)
	}
	if len(doc.HappeningPrefixes) != 3 {
		t.Errorf("happeningPrefixes = %v", doc.HappeningPrefixes)
	}
	// File order, not sorted: the ontology's order is a fact about the planes.
	if len(doc.Contexts) != 2 || doc.Contexts[0] != "hub" {
		t.Errorf("contexts = %v", doc.Contexts)
	}
}

func TestEmitJSONDoesNotEscapeTheGrammar(t *testing.T) {
	t.Parallel()
	raw, err := EmitJSON(valid())
	if err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	if bytes.Contains(raw, []byte(`\u003c`)) {
		t.Error("the grammar placeholders arrived HTML-escaped, which nobody can read")
	}
}

func TestEmitJSONNeverWritesNull(t *testing.T) {
	t.Parallel()
	// An entity with no verbs must carry an empty list, not null: a consumer
	// iterating the field should not have to special-case a missing one.
	r := valid()
	raw, err := EmitJSON(r)
	if err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	if bytes.Contains(raw, []byte("null")) {
		t.Errorf("names.json contains null:\n%s", raw)
	}

	// The same for a registry that grants nothing at all.
	for i := range r.Entities {
		r.Entities[i].Capabilities = nil
	}
	raw, err = EmitJSON(r)
	if err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	if bytes.Contains(raw, []byte("null")) {
		t.Errorf("names.json contains null with no capabilities anywhere:\n%s", raw)
	}
}

// TestEmissionIsDeterministic is what lets the generated files be committed and
// the goldens test below mean something: a regeneration with no source change
// has to produce the same bytes, or every unrelated commit carries a diff.
func TestEmissionIsDeterministic(t *testing.T) {
	t.Parallel()
	for i := range 3 {
		first := valid()
		second := valid()
		// Row order in the file is deliberately not the emitted order.
		second.Entities[0], second.Entities[2] = second.Entities[2], second.Entities[0]

		goA, err := EmitGo(first)
		if err != nil {
			t.Fatalf("EmitGo: %v", err)
		}
		goB, err := EmitGo(second)
		if err != nil {
			t.Fatalf("EmitGo: %v", err)
		}
		if !bytes.Equal(goA, goB) {
			t.Fatalf("run %d: the Go output depends on row order", i)
		}

		jsonA, err := EmitJSON(first)
		if err != nil {
			t.Fatalf("EmitJSON: %v", err)
		}
		jsonB, err := EmitJSON(second)
		if err != nil {
			t.Fatalf("EmitJSON: %v", err)
		}
		if !bytes.Equal(jsonA, jsonB) {
			t.Fatalf("run %d: the JSON output depends on row order", i)
		}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
