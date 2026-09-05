package projecttemplate

import (
	"slices"
	"testing"

	"github.com/pleware/initagent/internal/registry/ai/capability"
)

func TestCatalogueShipsSoftwareOnly(t *testing.T) {
	t.Parallel()
	got := Catalogue()
	if len(got) == 0 {
		t.Fatal("empty catalogue")
	}
	var live []ID
	for _, tmpl := range got {
		if tmpl.ID == "" || tmpl.Label == "" {
			t.Fatalf("template missing id or label: %+v", tmpl)
		}
		if tmpl.Live {
			live = append(live, tmpl.ID)
		}
		if tmpl.Live && tmpl.Contract == "" {
			t.Fatalf("live template %q has no contract", tmpl.ID)
		}
		for _, task := range tmpl.RequiredTasks {
			if !capability.IsValid(task) {
				t.Fatalf("template %q requires unregistered task %q", tmpl.ID, task)
			}
		}
	}
	if !slices.Equal(live, []ID{Software}) {
		t.Fatalf("live templates = %v, want only software", live)
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id       string
		wantOK   bool
		wantLive bool
		repo     bool
	}{
		{"software", true, true, true},
		{"website", true, false, true},
		{"poem", true, false, false},
		{"", false, false, false},
		{"project.kind", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			got, ok := Lookup(tc.id)
			if ok != tc.wantOK {
				t.Fatalf("Lookup(%q) ok=%v, want %v", tc.id, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.Live != tc.wantLive || got.NeedsRepo != tc.repo {
				t.Fatalf("Lookup(%q) live=%v needsRepo=%v", tc.id, got.Live, got.NeedsRepo)
			}
			if Live(tc.id) != tc.wantLive {
				t.Fatalf("Live(%q) = %v, want %v", tc.id, Live(tc.id), tc.wantLive)
			}
		})
	}
}

func TestCatalogueIsACopy(t *testing.T) {
	t.Parallel()
	got := Catalogue()
	got[0].Live = false
	again, ok := Lookup(string(Software))
	if !ok || !again.Live {
		t.Fatal("Catalogue exposed the backing slice")
	}
}
