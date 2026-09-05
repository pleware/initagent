package orgplan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

func TestCatalogueLocksFreeCaps(t *testing.T) {
	t.Parallel()
	got := Catalogue()
	if len(got) == 0 {
		t.Fatal("empty catalogue")
	}
	var selfServe []ID
	for _, p := range got {
		if p.ID == "" || p.Label == "" || p.ThemeFamily == "" {
			t.Fatalf("plan missing id, label, or theme: %+v", p)
		}
		if p.SelfServe {
			selfServe = append(selfServe, p.ID)
		}
	}
	if !slices.Equal(selfServe, []ID{Free, Starter, Team}) {
		t.Fatalf("self-serve plans = %v, want free, starter, team", selfServe)
	}
	var contact []ID
	for _, p := range got {
		if ContactSales(string(p.ID)) {
			contact = append(contact, p.ID)
		}
	}
	if !slices.Equal(contact, []ID{Enterprise}) {
		t.Fatalf("contact-sales plans = %v, want only enterprise", contact)
	}
	free, ok := Lookup(string(Free))
	if !ok {
		t.Fatal("free missing")
	}
	if free.Limits.Projects != 1 || free.Limits.WorkersPerProject != 2 || free.Limits.People != 1 {
		t.Fatalf("free caps = %+v, want 1 project / 2 machines per project / 1 person", free.Limits)
	}
	if free.Limits.IdleDays != 60 || free.Limits.LogDays != 7 {
		t.Fatalf("free hygiene = %+v, want 60 idle / 7 log days", free.Limits)
	}
	if free.Charge.Kind != ChargeFree || free.Charge.USD != 0 {
		t.Fatalf("free charge = %+v", free.Charge)
	}
	starter, ok := Lookup(string(Starter))
	if !ok {
		t.Fatal("starter missing")
	}
	if starter.Limits.Projects != 2 || starter.Limits.WorkersPerProject != 3 {
		t.Fatalf("starter caps = %+v, want 2 projects / 3 workers per project", starter.Limits)
	}
	if starter.Charge != (Charge{Kind: ChargeUSD, USD: PersonUSD, PerPerson: true}) || PersonUSD != 5 {
		t.Fatalf("starter charge = %+v, want $%d per person", starter.Charge, PersonUSD)
	}
	team, ok := Lookup(string(Team))
	if !ok {
		t.Fatal("team missing")
	}
	if team.Limits.Projects != 5 || team.Limits.WorkersPerProject != 5 {
		t.Fatalf("team caps = %+v, want 5 projects / 5 workers per project", team.Limits)
	}
	if team.Charge != (Charge{Kind: ChargeUSD, USD: PersonUSD, PerPerson: true}) {
		t.Fatalf("team charge = %+v, want $%d per person", team.Charge, PersonUSD)
	}
	ent, ok := Lookup(string(Enterprise))
	if !ok || ent.ThemeFamily != ThemeEnterprise || ent.Charge.Kind != ChargeContact || ent.Limits != Unlimited {
		t.Fatalf("enterprise = %+v", ent)
	}
}

func TestLookupAndParse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id        string
		wantOK    bool
		selfServe bool
	}{
		{"free", true, true},
		{"starter", true, true},
		{"team", true, true},
		{"enterprise", true, false},
		{"", false, false},
		{"pro", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			got, ok := Lookup(tc.id)
			if ok != tc.wantOK {
				t.Fatalf("Lookup(%q) ok=%v, want %v", tc.id, ok, tc.wantOK)
			}
			if !ok {
				if SelfServe(tc.id) || ContactSales(tc.id) {
					t.Fatalf("unknown %q must not be self-serve or contact-sales", tc.id)
				}
				return
			}
			if got.SelfServe != tc.selfServe || SelfServe(tc.id) != tc.selfServe {
				t.Fatalf("Lookup(%q) selfServe=%v", tc.id, got.SelfServe)
			}
			wantContact := tc.id == "enterprise"
			if ContactSales(tc.id) != wantContact {
				t.Fatalf("ContactSales(%q) = %v, want %v", tc.id, ContactSales(tc.id), wantContact)
			}
		})
	}

	id, err := Parse(" STARTER\n")
	if err != nil || id != Starter {
		t.Fatalf("Parse starter: %q %v", id, err)
	}
	if _, err := Parse(""); err == nil {
		t.Fatal("Parse empty: want error")
	}
	if _, err := Parse("hobby"); err == nil {
		t.Fatal("Parse hobby: want error")
	}
}

func TestDefault(t *testing.T) {
	t.Parallel()
	if Default().ID != Free {
		t.Fatalf("Default = %q", Default().ID)
	}
}

func TestCaps(t *testing.T) {
	t.Parallel()
	free := Caps(offering.Hosted, Free)
	if free.Projects != 1 || free.WorkersPerProject != 2 || free.People != 1 {
		t.Fatalf("hosted free = %+v", free)
	}
	starter := Caps(offering.Hosted, Starter)
	if starter.Projects != 2 || starter.WorkersPerProject != 3 {
		t.Fatalf("hosted starter = %+v", starter)
	}
	team := Caps(offering.Hosted, Team)
	if team.Projects != 5 || team.WorkersPerProject != 5 {
		t.Fatalf("hosted team = %+v", team)
	}
	if Caps(offering.Hosted, Enterprise) != Unlimited {
		t.Fatal("hosted enterprise must be unlimited")
	}
	if Caps(offering.Selfhost, Free) != Unlimited {
		t.Fatal("selfhost must ignore plan walls")
	}
	unknown := Caps(offering.Hosted, ID("hobby"))
	if unknown != Caps(offering.Hosted, Free) {
		t.Fatalf("unknown hosted id = %+v, want free (fail closed)", unknown)
	}
}

func TestAllows(t *testing.T) {
	t.Parallel()
	if !Allows(2, 2) || Allows(3, 2) {
		t.Fatal("Allows at the wall")
	}
	if !Allows(99, 0) {
		t.Fatal("Allows unlimited")
	}
	if AllowsAnother(2, 2) || !AllowsAnother(1, 2) {
		t.Fatal("AllowsAnother at the wall")
	}
	if !AllowsAnother(99, 0) {
		t.Fatal("AllowsAnother unlimited")
	}
}

func TestCatalogueIsACopy(t *testing.T) {
	t.Parallel()
	got := Catalogue()
	got[0].SelfServe = false
	again, ok := Lookup(string(Free))
	if !ok || !again.SelfServe {
		t.Fatal("Catalogue exposed the backing slice")
	}
}

func TestDeclaredIDsAreInCatalogue(t *testing.T) {
	t.Parallel()
	declared := declaredIDs(t)
	got := Catalogue()
	if len(declared) != len(got) {
		t.Fatalf("ID constants = %d, catalogue = %d", len(declared), len(got))
	}
	for name, val := range declared {
		p, ok := Lookup(val)
		if !ok {
			t.Fatalf("constant %s = %q missing from catalogue", name, val)
		}
		if string(p.ID) != val {
			t.Fatalf("constant %s = %q, catalogue id %q", name, val, p.ID)
		}
	}
}

func declaredIDs(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "plan.go", nil, 0)
	if err != nil {
		t.Fatalf("parse plan.go: %v", err)
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
			if !ok || ident.Name != "ID" {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", name.Name, err)
				}
				found[name.Name] = val
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no ID constants in plan.go")
	}
	return found
}
