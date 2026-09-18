package hub

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/store"
)

func TestCreateBoxRoundTrip(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateBox("box-alpha", "Alpha box", "host-00000000-0000-0000-0000-000000000000", "")
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
	if _, err := s.CreateBox("box-dup", "One", "", ""); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateBox("box-dup", "Two", "", "")
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
	box, err := s.CreateBox("box-narrated", "Narrated", "", "")
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
	box, err := s.CreateBox("box-idem", "Idem", "", "")
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

// Each box seeds its own st_b_dt: the slug is unique per box, not per
// installation, so box B's seed mints a second st_b_dt row rather than
// re-homing box A's through the slug-keyed update path.
func TestEnsureSeedBoxNarratorPerBox(t *testing.T) {
	s := testStore(t)
	boxA, err := s.CreateBox("box-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-b", "B", "", "")
	if err != nil {
		t.Fatal(err)
	}

	rosterA, err := s.StaffForBox(boxA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterA) != 1 || rosterA[0].Slug != "st_b_dt" || rosterA[0].BoxID != boxA.ID {
		t.Fatalf("box A narrator = %+v, want its own st_b_dt row", rosterA)
	}
	rosterB, err := s.StaffForBox(boxB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterB) != 1 || rosterB[0].Slug != "st_b_dt" || rosterB[0].BoxID != boxB.ID {
		t.Fatalf("box B narrator = %+v, want its own st_b_dt row", rosterB)
	}
	if rosterA[0].ID == rosterB[0].ID {
		t.Error("both boxes share one narrator row; each box must own its own")
	}

	// A re-seed still keys on the box: box A's tuned narrator is left alone,
	// and box B keeps the row it owns.
	if _, err := s.UpsertStaff("st_b_dt", "Lore", "en", "", "", "", "custom-voice", "box", boxA.ID, 0, 0, neutralBigFive()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSeedBoxNarrator(boxB.ID); err != nil {
		t.Fatal(err)
	}
	rosterA, err = s.StaffForBox(boxA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rosterA[0].Name != "Lore" || rosterA[0].Voice != "custom-voice" {
		t.Errorf("box A narrator after box B's re-seed = %+v, want the tuned row untouched", rosterA[0])
	}
	rosterB, err = s.StaffForBox(boxB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterB) != 1 {
		t.Fatalf("box B roster after re-seed = %+v, want its one narrator", rosterB)
	}
}

func TestListBoxesOrderedBySlug(t *testing.T) {
	s := testStore(t)
	for _, slug := range []string{"box-zeta", "box-alpha", "box-mike"} {
		if _, err := s.CreateBox(slug, slug, "", ""); err != nil {
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
	created, err := s.CreateBox("box-update", "Before", "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateBox(created.ID, "After", "host-00000000-0000-0000-0000-000000000001", "")
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
	if got, err := s.UpdateBox("box-00000000-0000-0000-0000-000000000000", "Nope", "", ""); err != nil || got != nil {
		t.Fatalf("UpdateBox on a missing box = (%v, %v), want (nil, nil)", got, err)
	}

	created, err := s.CreateBox("box-unbind", "Bound", "host-00000000-0000-0000-0000-000000000002", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateBox(created.ID, "Unbound", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.HostID != "" {
		t.Errorf("HostID after clearing = %q, want empty", got.HostID)
	}
}

func TestSetBoxOrgsReplacesSet(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-orgs", "Orgs", "", "")
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

func TestCreateBoxEditionRoundTrip(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateBox("box-edition", "Edition", "", "care")
	if err != nil {
		t.Fatal(err)
	}
	if created.Edition != "care" {
		t.Errorf("created edition = %q, want care", created.Edition)
	}
	if created.ConfigVersion != 1 {
		t.Errorf("created config_version = %d, want 1", created.ConfigVersion)
	}

	got, err := s.GetBox(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Edition != "care" || got.ConfigVersion != 1 {
		t.Errorf("read-back = %+v, want the submitted edition and version 1", got)
	}
}

func TestCreateBoxEditionDefaultsLite(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateBox("box-lite", "Lite", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Edition != "lite" {
		t.Errorf("empty edition stored as %q, want lite", created.Edition)
	}
	if _, err := s.CreateBox("box-bad", "Bad", "", "gaming"); err == nil {
		t.Fatal("CreateBox with an unknown edition returned nil error, want a refusal")
	}
}

func TestUpdateBoxBumpsConfigVersion(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateBox("box-bump", "One", "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateBox(created.ID, "Two", "", "company")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version after the first update = %d, want 2", got.ConfigVersion)
	}
	if got.Edition != "company" {
		t.Errorf("edition after update = %q, want company", got.Edition)
	}
	got, err = s.UpdateBox(created.ID, "Three", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigVersion != 3 {
		t.Errorf("config_version after the second update = %d, want 3", got.ConfigVersion)
	}
}

func TestSetBoxOrgsBumpsConfigVersion(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-org-bump", "Orgs", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(box.ID, []string{"org-a"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBox(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version after SetBoxOrgs = %d, want 2", got.ConfigVersion)
	}
}

func TestDeleteBoxCascades(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-doomed", "Doomed", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(box.ID, []string{"org-a", "org-b"}); err != nil {
		t.Fatal(err)
	}
	// CreateBox seeds the box-scoped narrator, so the staff table has a row
	// for this box before the delete.
	roster, err := s.StaffForBox(box.ID)
	if err != nil || len(roster) != 1 {
		t.Fatalf("StaffForBox before delete = (%v, %d), want one narrator", err, len(roster))
	}
	// A sync token, revoked so even a non-live row has to die with the box.
	_, tokenRow, err := s.CreateBoxToken(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revoked, err := s.RevokeBoxToken(tokenRow.Id, box.ID); err != nil || !revoked {
		t.Fatalf("RevokeBoxToken = (%v, %v), want (true, nil)", revoked, err)
	}

	deleted, err := s.DeleteBox(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteBox on an existing box reported false")
	}
	got, err := s.GetBox(box.ID)
	if err != nil || got != nil {
		t.Fatalf("GetBox after delete = (%v, %v), want (nil, nil)", got, err)
	}
	orgs, err := s.ListBoxOrgs(box.ID)
	if err != nil || len(orgs) != 0 {
		t.Fatalf("ListBoxOrgs after delete = (%v, %v), want empty", orgs, err)
	}
	roster, err = s.StaffForBox(box.ID)
	if err != nil || len(roster) != 0 {
		t.Fatalf("StaffForBox after delete = (%v, %v), want empty", roster, err)
	}
	var tokens int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM box_tokens WHERE box_id = ?`, box.ID).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if tokens != 0 {
		t.Errorf("box_tokens rows after delete = %d, want 0", tokens)
	}

	// A second delete finds no row and leaves the store untouched.
	deleted, err = s.DeleteBox(box.ID)
	if err != nil || deleted {
		t.Fatalf("second DeleteBox = (%v, %v), want (false, nil)", deleted, err)
	}
}

// A store whose boxes table predates the edition columns gains them on
// reopen with the schema defaults: edition lite and config_version 1, never
// 0, so a migrated box's first sync serves a manifest. The migration is
// idempotent — a third open finds both columns and skips.
func TestOpenStoreMigratesBoxColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "box-migration.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("box-migrated", "Migrated", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate the pre-migration shape: the two columns do not exist.
	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range []string{
		`ALTER TABLE boxes DROP COLUMN edition`,
		`ALTER TABLE boxes DROP COLUMN config_version`,
	} {
		if _, err := db.Exec(drop); err != nil {
			_ = db.Close()
			t.Fatalf("%s: %v", drop, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen on a pre-edition boxes table: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	for _, col := range []string{"edition", "config_version"} {
		ok, err := again.hasColumn("boxes", col)
		if err != nil || !ok {
			t.Fatalf("boxes.%s after reopen: ok=%v err=%v", col, ok, err)
		}
	}
	got, err := again.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox after migration = (%v, %v), want the carried row", got, err)
	}
	if got.Edition != "lite" || got.ConfigVersion != 1 {
		t.Errorf("migrated row = %+v, want edition lite and config_version 1", got)
	}

	// The migration is a no-op on a store that already carries the columns.
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := OpenStore(path)
	if err != nil {
		t.Fatalf("third open on a migrated store: %v", err)
	}
	t.Cleanup(func() { third.Close() })
}
