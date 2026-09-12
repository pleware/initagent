package names

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// valid is the smallest registry that passes every rule: two planes, one minted
// entity carrying a verb, one entity that mints nothing, and one that borrows.
//
// Every refusal test below starts from this and breaks exactly one thing, so a
// failure names the rule rather than the fixture.
func valid() *Registry {
	return &Registry{
		Schema:    1,
		Authority: "initagent",
		Seam:      Seam{EnvelopeVersion: 2},
		Contexts: []Context{
			{Name: "hub", GoConst: "ContextHub"},
			{Name: "fleet", GoConst: "ContextFleet"},
		},
		Entities: []Entity{
			{
				Name:         "initagent.hub.project",
				Context:      "hub",
				GoConst:      "Project",
				Mints:        true,
				Lifetime:     "project lifetime",
				Description:  "The hub's catalogue entry for one project.",
				Capabilities: []string{"read", "create"},
			},
			{
				Name:        "initagent.fleet.file",
				Context:     "fleet",
				Mints:       false,
				Lifetime:    "the file's own",
				Description: "A file on an enrolled machine's disk.",
			},
			{
				Name:        "initagent.hub.binding",
				Context:     "hub",
				Mints:       false,
				Borrows:     "project",
				Lifetime:    "project lifetime",
				Description: "Carries the project prefix rather than one of its own.",
			},
		},
	}
}

func TestValidFixturePasses(t *testing.T) {
	t.Parallel()
	if err := valid().Validate(); err != nil {
		t.Fatalf("the fixture every refusal test mutates is itself invalid: %v", err)
	}
}

// TestValidateRefuses is the gate that used to be several tests downstream.
//
// It replaces internal/id's TestEveryDeclaredKindIsRegistered,
// TestEveryRegisteredKindIsDeclared and TestKindIsTheLastSegment, and the
// structural half of TestRegistryIsWellFormed / TestEverySpecIsDescribed. Those
// tests existed because a human could declare a constant and forget the
// registry row, or spell a prefix that was not the name's last segment. Both
// halves now come from one row in names.yaml, so the drift they watched for is
// no longer reachable — what is reachable is bad data in the source file, and
// that is what these cases cover. A generator that emits and lets a test catch
// it has already written the file somebody reviews.
func TestValidateRefuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*Registry)
		want   string
	}{
		{
			"a schema version below one",
			func(r *Registry) { r.Schema = 0 },
			"not a released shape",
		},
		{
			"no authority to check names against",
			func(r *Registry) { r.Authority = "" },
			"authority is empty",
		},
		{
			"an unnegotiable envelope version",
			func(r *Registry) { r.Seam.EnvelopeVersion = 0 },
			"nothing to negotiate on",
		},
		{
			"no planes at all",
			func(r *Registry) { r.Contexts = nil },
			"no contexts are declared",
		},
		{
			"a nameless plane",
			func(r *Registry) { r.Contexts[0].Name = "" },
			"a context has no name",
		},
		{
			"an upper-case plane",
			func(r *Registry) { r.Contexts[0].Name = "Hub"; r.Entities[0].Context = "Hub" },
			"is not lower case",
		},
		{
			"a plane that is not a bare word",
			func(r *Registry) { r.Contexts[0].Name = "hub.inner" },
			"is not a bare word",
		},
		{
			"one plane declared twice",
			func(r *Registry) { r.Contexts[1].Name = "hub" },
			"is declared twice",
		},
		{
			"a plane with no Go constant",
			func(r *Registry) { r.Contexts[0].GoConst = "" },
			"which is not an exported Go identifier",
		},
		{
			"a plane with an unexported Go constant",
			func(r *Registry) { r.Contexts[0].GoConst = "contextHub" },
			"which is not an exported Go identifier",
		},
		{
			"two planes sharing a Go constant",
			func(r *Registry) { r.Contexts[1].GoConst = "ContextHub" },
			"claimed by two contexts",
		},
		{
			"a nameless entity",
			func(r *Registry) { r.Entities[0].Name = "" },
			"an entity has no name",
		},
		{
			"one entity declared twice",
			func(r *Registry) { r.Entities[1].Name = "initagent.hub.project" },
			"is declared twice",
		},
		{
			"two segments instead of three",
			func(r *Registry) { r.Entities[0].Name = "initagent.project" },
			"segments, not authority.context.entity",
		},
		{
			"four segments instead of three",
			func(r *Registry) { r.Entities[0].Name = "initagent.hub.project.row" },
			"segments, not authority.context.entity",
		},
		{
			"somebody else's authority",
			func(r *Registry) { r.Entities[0].Name = "overseer.hub.project" },
			"does not start with the authority",
		},
		{
			"a context field disagreeing with the name",
			func(r *Registry) { r.Entities[0].Context = "fleet" },
			"but declares",
		},
		{
			"a plane that is not declared",
			func(r *Registry) {
				r.Entities[0].Name = "initagent.desk.project"
				r.Entities[0].Context = "desk"
			},
			"which is not a declared plane",
		},
		{
			"an empty prefix",
			func(r *Registry) { r.Entities[0].Name = "initagent.hub." },
			"has an empty prefix",
		},
		{
			"an upper-case prefix",
			func(r *Registry) { r.Entities[0].Name = "initagent.hub.Project" },
			"is not lower case",
		},
		{
			"a prefix carrying the separator",
			func(r *Registry) { r.Entities[0].Name = "initagent.hub.password-reset" },
			"which makes an identifier ambiguous to split",
		},
		{
			"two entities wanting one prefix",
			func(r *Registry) { r.Entities[1].Name = "initagent.fleet.project" },
			"is claimed by both",
		},
		{
			"no lifetime",
			func(r *Registry) { r.Entities[0].Lifetime = "" },
			"has no lifetime",
		},
		{
			"no description",
			func(r *Registry) { r.Entities[0].Description = "" },
			"has no description",
		},
		{
			"a description of whitespace",
			func(r *Registry) { r.Entities[0].Description = "   \n" },
			"has no description",
		},
		{
			"a minted entity with no Go constant",
			func(r *Registry) { r.Entities[0].GoConst = "" },
			"which is not an exported Go identifier",
		},
		{
			"a minted entity with an unexported Go constant",
			func(r *Registry) { r.Entities[0].GoConst = "project" },
			"which is not an exported Go identifier",
		},
		{
			"a Go constant that is not an identifier at all",
			func(r *Registry) { r.Entities[0].GoConst = "Pro ject" },
			"which is not an exported Go identifier",
		},
		{
			"two entities sharing a Go constant",
			func(r *Registry) {
				r.Entities[1].Mints = true
				r.Entities[1].GoConst = "Project"
			},
			"claimed by both",
		},
		{
			"an unminted entity claiming a Go constant",
			func(r *Registry) { r.Entities[1].GoConst = "File" },
			"would let New hand out an identifier it has no right to",
		},
		{
			"minting and borrowing at once",
			func(r *Registry) {
				r.Entities[2].Mints = true
				r.Entities[2].GoConst = "Binding"
			},
			"it cannot be both",
		},
		{
			"borrowing a prefix nothing mints",
			func(r *Registry) { r.Entities[2].Borrows = "invented" },
			"which nothing mints",
		},
		{
			"an empty verb",
			func(r *Registry) { r.Entities[0].Capabilities = []string{""} },
			"grants an empty verb",
		},
		{
			"an upper-case verb",
			func(r *Registry) { r.Entities[0].Capabilities = []string{"Read"} },
			"is not lower case",
		},
		{
			"a verb carrying the grammar's own punctuation",
			func(r *Registry) { r.Entities[0].Capabilities = []string{"read:hub.project"} },
			"the grammar adds the colon",
		},
		{
			"one verb granted twice",
			func(r *Registry) { r.Entities[0].Capabilities = []string{"read", "read"} },
			"is granted twice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := valid()
			tc.mutate(r)
			err := r.Validate()
			if err == nil {
				t.Fatalf("Validate accepted %s", tc.name)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error is not ErrInvalid: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not say %q:\n%v", tc.want, err)
			}
		})
	}
}

// TestValidateReportsEveryProblem is why Validate joins instead of returning
// the first failure: a hand-edited file usually has several, and one build per
// fix is how somebody stops reading the message.
func TestValidateReportsEveryProblem(t *testing.T) {
	t.Parallel()
	r := valid()
	r.Entities[0].Lifetime = ""
	r.Entities[0].Description = ""
	err := r.Validate()
	if err == nil {
		t.Fatal("Validate accepted two empty fields")
	}
	for _, want := range []string{"has no lifetime", "has no description"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not say %q:\n%v", want, err)
		}
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	const src = `schema: 1
authority: initagent
seam:
  envelope_version: 2
contexts:
  - name: hub
    go_const: ContextHub
entities:
  - name: initagent.hub.org
    context: hub
    go_const: Org
    mints: true
    lifetime: until deleted
    capabilities: [read]
    description: |-
      A customer's organization.
`
	r, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Authority != "initagent" || r.Seam.EnvelopeVersion != 2 {
		t.Errorf("Parse = %+v", r)
	}
	if got := r.Entities[0].Description; got != "A customer's organization." {
		t.Errorf("block scalar arrived as %q", got)
	}
}

func TestParseRefuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"not YAML at all", "\tschema: [1", "invalid registry"},
		{
			// Strict decoding is the point: a misspelled key that parsed
			// silently would drop a permission and leave nothing to notice.
			"a misspelled key",
			"schema: 1\nauthority: initagent\ncapabilties: [read]\n",
			"field capabilties not found",
		},
		{"valid YAML, invalid registry", "schema: 1\n", "authority is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tc.src))
			if err == nil {
				t.Fatalf("Parse accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not say %q: %v", tc.want, err)
			}
		})
	}
}

func TestLoadReportsThePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	missing := filepath.Join(dir, "absent.yaml")
	if _, err := Load(missing); err == nil || !strings.Contains(err.Error(), "absent.yaml") {
		t.Errorf("Load of a missing file = %v", err)
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("schema: 1\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("Load of an invalid file = %v", err)
	}
}

func TestDerivedFields(t *testing.T) {
	t.Parallel()
	e := Entity{Name: "initagent.fleet.device", Context: "fleet", Capabilities: []string{"read", "admin"}}
	if got := e.Prefix(); got != "device" {
		t.Errorf("Prefix() = %q", got)
	}
	if got := e.Tail(); got != "fleet.device" {
		t.Errorf("Tail() = %q", got)
	}
	if got := e.HappeningPrefix(); got != "fleet.device" {
		t.Errorf("HappeningPrefix() = %q", got)
	}
	got := e.CapabilityNames()
	want := []string{"admin:fleet.device", "read:fleet.device"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("CapabilityNames() = %v, want %v", got, want)
	}
	// A name with no dot cannot reach Prefix through a validated registry, but
	// EmitGo's error path is reached with one, so the fallback is not dead.
	if got := (Entity{Name: "device"}).Prefix(); got != "device" {
		t.Errorf("Prefix() of a bare word = %q", got)
	}
}

func TestOrderIsIndependentOfFileOrder(t *testing.T) {
	t.Parallel()
	r := valid()
	forward := r.SortedEntities()
	r.Entities[0], r.Entities[2] = r.Entities[2], r.Entities[0]
	reversed := r.SortedEntities()
	if len(forward) != len(reversed) {
		t.Fatalf("%d entities became %d", len(forward), len(reversed))
	}
	for i := range forward {
		if forward[i].Name != reversed[i].Name {
			t.Fatalf("row %d is %q one way and %q the other", i, forward[i].Name, reversed[i].Name)
		}
	}
}

func TestHalvesAndDerivedLists(t *testing.T) {
	t.Parallel()
	r := valid()
	if got := r.Minted(); len(got) != 1 || got[0].Name != "initagent.hub.project" {
		t.Errorf("Minted() = %v", got)
	}
	if got := r.Unminted(); len(got) != 2 {
		t.Errorf("Unminted() = %v", got)
	}
	if got := r.Capabilities(); len(got) != 2 || got[0] != "create:hub.project" || got[1] != "read:hub.project" {
		t.Errorf("Capabilities() = %v", got)
	}
	if got := r.ContextNames(); len(got) != 2 || got[0] != "hub" || got[1] != "fleet" {
		t.Errorf("ContextNames() = %v, want file order", got)
	}
	if got := r.contextConst("worker"); got != "" {
		t.Errorf("contextConst of an undeclared plane = %q", got)
	}
	if got := r.mintingConst("invented"); got != "" {
		t.Errorf("mintingConst of an unminted prefix = %q", got)
	}
}

func TestExportedGoIdent(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"":            false,
		"Project":     true,
		"MCPServer":   true,
		"Reset2":      true,
		"Password_Do": true,
		"project":     false,
		"_Project":    false,
		"1Project":    false,
		"Pro-ject":    false,
	}
	for in, want := range cases {
		if got := exportedGoIdent(in); got != want {
			t.Errorf("exportedGoIdent(%q) = %v, want %v", in, got, want)
		}
	}
}
