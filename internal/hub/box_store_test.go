package hub

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pleware/initagent/internal/id"
)

func TestCreateBoxRoundTrip(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateBox("box-alpha", "Alpha box", "host-00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if !id.Is(id.Box, created.ID) {
		t.Errorf("minted id %q is not a box identifier", created.ID)
	}
	if created.Slug != "box-alpha" || created.Name != "Alpha box" {
		t.Errorf("created = %+v, want the submitted slug and name", created)
	}
	if created.CreatedAt == 0 || created.UpdatedAt == 0 {
		t.Errorf("timestamps not set: %+v", created)
	}

	got, err := s.GetBox(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetBox returned nil for a created box")
	}
	if !reflect.DeepEqual(*got, *created) {
		t.Errorf("read-back = %+v, want %+v", *got, *created)
	}
}

func TestCreateBoxSlugUnique(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateBox("box-dup", "One", ""); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateBox("box-dup", "Two", "")
	if !errors.Is(err, ErrBoxSlugTaken) {
		t.Fatalf("second CreateBox = %v, want ErrBoxSlugTaken", err)
	}
}

func TestGetBoxMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.GetBox("box-00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("GetBox = %+v, want nil for a missing box", got)
	}
}

func TestCreateBoxSeedsNarrator(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-narrated", "Narrated", "")
	if err != nil {
		t.Fatal(err)
	}
	roster, err := s.StaffForBox(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 {
		t.Fatalf("StaffForBox = %d rows, want exactly the narrator", len(roster))
	}
	got := roster[0]
	if got.Slug != "st_b_dt" || got.Name != "Data" || got.Locale != "pl" ||
		got.Voice != "pl_PL-mc_speech-medium" || got.Scope != "box" || got.BoxID != box.ID {
		t.Errorf("narrator = %+v, want the box-scoped st_b_dt seed for %s", got, box.ID)
	}
	if got.SoulCore != "" {
		t.Errorf("narrator soul_core = %q, want empty (content later)", got.SoulCore)
	}
	if !reflect.DeepEqual(got.BigFive, neutralBigFive()) {
		t.Errorf("narrator BigFive = %+v, want %+v", got.BigFive, neutralBigFive())
	}
}

func TestEnsureSeedBoxNarratorIdempotent(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-idem", "Idem", "")
	if err != nil {
		t.Fatal(err)
	}
	roster, err := s.StaffForBox(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 {
		t.Fatalf("StaffForBox after CreateBox = %d rows, want 1", len(roster))
	}
	seeded := roster[0]

	// Content written over the seed survives a second run: the seed must
	// not clobber a tuned narrator nor mint a second row.
	if _, err := s.UpsertStaff("st_b_dt", "Lore", "en", "", "", "", "custom-voice", "box", box.ID, 0, 0, neutralBigFive()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSeedBoxNarrator(box.ID); err != nil {
		t.Fatal(err)
	}
	roster, err = s.StaffForBox(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 {
		t.Fatalf("StaffForBox after the second seed = %d rows, want 1", len(roster))
	}
	got := roster[0]
	if got.ID != seeded.ID {
		t.Errorf("second seed minted a new narrator row: %s -> %s", seeded.ID, got.ID)
	}
	if got.Name != "Lore" || got.Voice != "custom-voice" || got.Locale != "en" {
		t.Errorf("second seed overwrote the tuned narrator: %+v", got)
	}
}

func TestEnsureSeedBoxNarratorLeavesForeignRowAlone(t *testing.T) {
	s := testStore(t)
	boxA, err := s.CreateBox("box-a", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	// st_b_dt already narrates box A, so box B's seed must not re-home the
	// row through UpsertStaff's slug-keyed update.
	boxB, err := s.CreateBox("box-b", "B", "")
	if err != nil {
		t.Fatal(err)
	}

	rosterA, err := s.StaffForBox(boxA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterA) != 1 || rosterA[0].Slug != "st_b_dt" || rosterA[0].BoxID != boxA.ID {
		t.Fatalf("box A narrator = %+v, want the st_b_dt row still bound to box A", rosterA)
	}
	rosterB, err := s.StaffForBox(boxB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterB) != 0 {
		t.Errorf("box B roster = %+v, want empty: the narrator slug is installation-unique", rosterB)
	}
}

func TestListBoxesOrderedBySlug(t *testing.T) {
	s := testStore(t)
	for _, slug := range []string{"box-zeta", "box-alpha", "box-mike"} {
		if _, err := s.CreateBox(slug, slug, ""); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListBoxes()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"box-alpha", "box-mike", "box-zeta"}
	if len(list) != len(want) {
		t.Fatalf("ListBoxes = %d rows, want %d", len(list), len(want))
	}
	for i, b := range list {
		if b.Slug != want[i] {
			t.Errorf("row %d slug = %q, want %q", i, b.Slug, want[i])
		}
	}
}

func TestUpdateBox(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateBox("box-update", "Before", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateBox(created.ID, "After", "host-00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("UpdateBox returned nil for an existing box")
	}
	if got.ID != created.ID || got.Name != "After" || got.HostID != "host-00000000-0000-0000-0000-000000000001" {
		t.Errorf("updated = %+v, want the same row with the submitted fields", got)
	}
	if got.UpdatedAt < created.UpdatedAt {
		t.Errorf("UpdatedAt = %d, want not before the created %d", got.UpdatedAt, created.UpdatedAt)
	}
}

func TestUpdateBoxMissingAndClearHost(t *testing.T) {
	s := testStore(t)
	if got, err := s.UpdateBox("box-00000000-0000-0000-0000-000000000000", "Nope", ""); err != nil || got != nil {
		t.Fatalf("UpdateBox on a missing box = (%v, %v), want (nil, nil)", got, err)
	}

	created, err := s.CreateBox("box-unbind", "Bound", "host-00000000-0000-0000-0000-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateBox(created.ID, "Unbound", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.HostID != "" {
		t.Errorf("HostID after clearing = %q, want empty", got.HostID)
	}
}

func TestSetBoxOrgsReplacesSet(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-orgs", "Orgs", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(box.ID, []string{"org-a", "org-b"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListBoxOrgs(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"org-a", "org-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBoxOrgs = %v, want %v", got, want)
	}

	// The second call replaces the set, not appends to it.
	if err := s.SetBoxOrgs(box.ID, []string{"org-b", "org-c"}); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListBoxOrgs(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"org-b", "org-c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBoxOrgs after replace = %v, want %v", got, want)
	}

	// Duplicate ids in the input collapse; an empty set clears the box.
	if err := s.SetBoxOrgs(box.ID, []string{"org-d", "org-d", "org-e"}); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListBoxOrgs(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"org-d", "org-e"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBoxOrgs after dedupe = %v, want %v", got, want)
	}
	if err := s.SetBoxOrgs(box.ID, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListBoxOrgs(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("ListBoxOrgs after clearing = %v, want empty", got)
	}
}

func TestListBoxOrgsEmptyBox(t *testing.T) {
	s := testStore(t)
	got, err := s.ListBoxOrgs("box-00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("ListBoxOrgs for a box with no orgs = %v, want a non-nil empty slice", got)
	}
}
