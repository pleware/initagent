package orgplan

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/registry/config"
)

func TestLoadEmbedded(t *testing.T) {
	t.Parallel()
	got, usd, err := Load(config.YAML)
	if err != nil {
		t.Fatal(err)
	}
	want := Catalogue()
	if len(got) != len(want) || usd != PersonUSD() || usd != 5 {
		t.Fatalf("load = %d plans usd %d, catalogue %d usd %d", len(got), usd, len(want), PersonUSD())
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("plan %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "empty catalogue"},
		{"blank", []byte("  \n"), "empty catalogue"},
		{
			"unknown field",
			bytes.Replace(config.YAML, []byte("    free:\n"), []byte("    free:\n      label: Free\n"), 1),
			"label",
		},
		{
			"negative idle",
			bytes.Replace(config.YAML, []byte("idleDays: 60"), []byte("idleDays: -1"), 1),
			"negative",
		},
		{
			"unknown kind",
			bytes.Replace(config.YAML, []byte("kind: contact"), []byte("kind: coins"), 1),
			"unknown charge kind",
		},
		{
			"usd mismatch",
			bytes.Replace(config.YAML, []byte("        usd: 5\n        perPerson: true\n      themeFamily: default\n      limits:\n        projects: 5"), []byte("        usd: 9\n        perPerson: true\n      themeFamily: default\n      limits:\n        projects: 5"), 1),
			"usd 9, want 5",
		},
		{
			"hobby slug",
			[]byte(`config:
  plansOrder: [hobby, starter, team, enterprise]
  plans:
    hobby:
      selfServe: true
      charge: {kind: free}
      themeFamily: default
      limits: {projects: 1, workersPerProject: 2, people: 1, idleDays: 60, logDays: 7}
    starter:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 2, workersPerProject: 3, people: 0, idleDays: 0, logDays: 0}
    team:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 5, workersPerProject: 5, people: 0, idleDays: 0, logDays: 0}
    enterprise:
      selfServe: false
      charge: {kind: contact}
      themeFamily: enterprise
      limits: {projects: 0, workersPerProject: 0, people: 0, idleDays: 0, logDays: 0}
`),
			"unknown slug",
		},
		{
			"short order",
			[]byte(`config:
  plansOrder: [free]
  plans:
    free:
      selfServe: true
      charge: {kind: free}
      themeFamily: default
      limits: {projects: 1, workersPerProject: 2, people: 1, idleDays: 60, logDays: 7}
`),
			"want 4 typed ids",
		},
		{
			"order missing map key",
			[]byte(`config:
  plansOrder: [free, starter, team, enterprise]
  plans:
    free:
      selfServe: true
      charge: {kind: free}
      themeFamily: default
      limits: {projects: 1, workersPerProject: 2, people: 1, idleDays: 60, logDays: 7}
    starter:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 2, workersPerProject: 3, people: 0, idleDays: 0, logDays: 0}
    team:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 5, workersPerProject: 5, people: 0, idleDays: 0, logDays: 0}
`),
			"missing from config.plans",
		},
		{
			"map key missing from order",
			[]byte(`config:
  plansOrder: [free, starter, team, enterprise]
  plans:
    free:
      selfServe: true
      charge: {kind: free}
      themeFamily: default
      limits: {projects: 1, workersPerProject: 2, people: 1, idleDays: 60, logDays: 7}
    starter:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 2, workersPerProject: 3, people: 0, idleDays: 0, logDays: 0}
    team:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 5, workersPerProject: 5, people: 0, idleDays: 0, logDays: 0}
    enterprise:
      selfServe: false
      charge: {kind: contact}
      themeFamily: enterprise
      limits: {projects: 0, workersPerProject: 0, people: 0, idleDays: 0, logDays: 0}
    hobby:
      selfServe: true
      charge: {kind: free}
      themeFamily: default
      limits: {projects: 1, workersPerProject: 1, people: 1, idleDays: 0, logDays: 0}
`),
			"missing from plansOrder",
		},
		{
			"duplicate order",
			[]byte(`config:
  plansOrder: [free, free, team, enterprise]
  plans:
    free:
      selfServe: true
      charge: {kind: free}
      themeFamily: default
      limits: {projects: 1, workersPerProject: 2, people: 1, idleDays: 60, logDays: 7}
    starter:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 2, workersPerProject: 3, people: 0, idleDays: 0, logDays: 0}
    team:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 5, workersPerProject: 5, people: 0, idleDays: 0, logDays: 0}
    enterprise:
      selfServe: false
      charge: {kind: contact}
      themeFamily: enterprise
      limits: {projects: 0, workersPerProject: 0, people: 0, idleDays: 0, logDays: 0}
`),
			"duplicate slug",
		},
		{
			"free with usd",
			bytes.Replace(config.YAML, []byte("        kind: free\n"), []byte("        kind: free\n        usd: 5\n"), 1),
			"free charge must be usd 0",
		},
		{
			"usd not per person",
			bytes.Replace(config.YAML, []byte("        usd: 5\n        perPerson: true\n      themeFamily: default\n      limits:\n        projects: 2"), []byte("        usd: 5\n        perPerson: false\n      themeFamily: default\n      limits:\n        projects: 2"), 1),
			"usd > 0 and perPerson",
		},
		{
			"contact self-serve",
			bytes.Replace(config.YAML, []byte("    enterprise:\n      selfServe: false\n"), []byte("    enterprise:\n      selfServe: true\n"), 1),
			"contact-sales must not be self-serve",
		},
		{
			"unknown theme",
			bytes.Replace(config.YAML, []byte("themeFamily: enterprise"), []byte("themeFamily: neon"), 1),
			"unknown themeFamily",
		},
		{
			"not yaml",
			[]byte("config: ["),
			"parse catalogue",
		},
		{
			"empty slug",
			[]byte(`config:
  plansOrder: ["", starter, team, enterprise]
  plans:
    "":
      selfServe: true
      charge: {kind: free}
      themeFamily: default
      limits: {projects: 1, workersPerProject: 2, people: 1, idleDays: 60, logDays: 7}
    starter:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 2, workersPerProject: 3, people: 0, idleDays: 0, logDays: 0}
    team:
      selfServe: true
      charge: {kind: usd, usd: 5, perPerson: true}
      themeFamily: default
      limits: {projects: 5, workersPerProject: 5, people: 0, idleDays: 0, logDays: 0}
    enterprise:
      selfServe: false
      charge: {kind: contact}
      themeFamily: enterprise
      limits: {projects: 0, workersPerProject: 0, people: 0, idleDays: 0, logDays: 0}
`),
			"empty slug",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Load(tc.data)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestTypeScriptListsEverySlug(t *testing.T) {
	t.Parallel()
	got := TypeScript()
	if !strings.Contains(got, `export const PLAN_ORDER = ["free", "starter", "team", "enterprise"] as const;`) {
		t.Fatalf("PLAN_ORDER = %s", got)
	}
	if !strings.Contains(got, "export const PERSON_USD = 5;") {
		t.Fatal("missing PERSON_USD")
	}
	for _, p := range Catalogue() {
		needle := `  "` + string(p.ID) + `": {`
		if !strings.Contains(got, needle) {
			t.Fatalf("TypeScript missing %s", needle)
		}
	}
}
