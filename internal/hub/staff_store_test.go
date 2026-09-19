package hub

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/store"
)

// testStaff is a fully-populated staff member used where a test needs a set one.
func testStaff() Staff {
	return Staff{
		Slug:          "coder-zeta",
		Name:          "Zeta",
		Locale:        "pl",
		AvatarModel3D: "zeta.glb",
		Brief:         "a sharp coder",
		Age:           41,
		WordBudget:    2500,
		SoulCore:      "debug first, explain after",
		Voice:         "zeta-v1",
		BigFive: Character{
			Openness:          0.9,
			Conscientiousness: 0.8,
			Extraversion:      0.7,
			Agreeableness:     0.6,
			Neuroticism:       0.1,
		},
	}
}

// neutralBigFive is the placeholder profile EnsureSeedStaff writes.
func neutralBigFive() Character {
	return Character{
		Openness:          0.5,
		Conscientiousness: 0.5,
		Extraversion:      0.5,
		Agreeableness:     0.5,
		Neuroticism:       0.5,
	}
}

// strPtr returns a pointer to s for the optional override fields.
func strPtr(s string) *string { return &s }

// intPtr returns a pointer to n for the optional override fields.
func intPtr(n int) *int { return &n }

// findStaff returns the staff member with the given slug from a list.
func findStaff(t *testing.T, list []Staff, slug string) *Staff {
	t.Helper()
	for i := range list {
		if list[i].Slug == slug {
			return &list[i]
		}
	}
	t.Fatalf("list does not contain slug %q", slug)
	return nil
}

// newBaseStaff upserts the shared base row every walk-up subtest tunes.
func newBaseStaff(t *testing.T, s *Store) Staff {
	t.Helper()
	st := testStaff()
	created, err := s.UpsertStaff(st.Slug, st.Name, st.Locale, st.AvatarModel3D, st.Brief, st.SoulCore, st.Voice, "org", "", st.Age, st.WordBudget, st.BigFive)
	if err != nil {
		t.Fatal(err)
	}
	st.ID = created.ID
	return st
}

// mustStaffForOrg calls StaffForOrg and fails the test on error.
func mustStaffForOrg(t *testing.T, s *Store, orgID string) []Staff {
	t.Helper()
	list, err := s.StaffForOrg(orgID)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// countSlug counts rows with the given slug in a staff list.
func countSlug(list []Staff, slug string) int {
	n := 0
	for _, st := range list {
		if st.Slug == slug {
			n++
		}
	}
	return n
}

func TestListStaffOrderedBySlug(t *testing.T) {
	s := testStore(t)
	for _, slug := range []string{"zeta", "alpha", "mike"} {
		if _, err := s.UpsertStaff(slug, slug, "en", "", "", "", "", "org", "", 30, 0, Character{}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	// The seed members (staff-female-00, staff-male-00) are present in every
	// store, so the ordering check includes them.
	want := []string{"alpha", "mike", "staff-female-00", "staff-male-00", "zeta"}
	if len(list) != len(want) {
		t.Fatalf("ListStaff has %d rows, want %d", len(list), len(want))
	}
	for i, st := range list {
		if st.Slug != want[i] {
			t.Errorf("row %d slug = %q, want %q", i, st.Slug, want[i])
		}
	}
}

func TestStaffByIdMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.StaffById("staff-00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("StaffById = %+v, want nil", got)
	}
}

// The canonical list excludes box-scoped rows: a box's narrator stays
// visible through StaffForBox but never reaches ListStaff.
func TestListStaffExcludesBoxScoped(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-scope", "Scoped", "", "")
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range list {
		if st.Scope == "box" {
			t.Errorf("ListStaff carries the box-scoped %q", st.Slug)
		}
	}
	roster, err := s.StaffForBox(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) != 1 || roster[0].Slug != "st_b_pi" {
		t.Errorf("StaffForBox = %+v, want the box's one narrator", roster)
	}
}

// UpdateBoxNarrator round-trips the nine editable fields, bumps only the
// edited box by exactly one, and creates the row when the seed has not
// run; the seed itself still does not bump (a fresh box stays at 1).
func TestUpdateBoxNarrator(t *testing.T) {
	s := testStore(t)
	boxA, err := s.CreateBox("box-narr-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-narr-b", "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// The seed does not bump: both fresh boxes sit at version 1.
	if got, _ := s.GetBox(boxA.ID); got.ConfigVersion != 1 {
		t.Fatalf("fresh box A config_version = %d, want 1", got.ConfigVersion)
	}
	if got, _ := s.GetBox(boxB.ID); got.ConfigVersion != 1 {
		t.Fatalf("fresh box B config_version = %d, want 1", got.ConfigVersion)
	}

	want := Character{
		Openness: 0.9, Conscientiousness: 0.8, Extraversion: 0.7,
		Agreeableness: 0.6, Neuroticism: 0.2,
	}
	st, err := s.UpdateBoxNarrator(boxA.ID, "Lore", "en", "lore.glb", "warm and precise", "explain first", "lore-v2", 42, 1200, want)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID == "" || st.Slug != "st_b_pi" || st.Scope != "box" || st.BoxID != boxA.ID {
		t.Errorf("narrator identity = %+v, want the box-scoped st_b_pi row", st)
	}
	if st.Name != "Lore" || st.Locale != "en" || st.Age != 42 || st.WordBudget != 1200 ||
		st.AvatarModel3D != "lore.glb" || st.Voice != "lore-v2" || st.Brief != "warm and precise" ||
		st.SoulCore != "explain first" || st.BigFive != want {
		t.Errorf("narrator after the edit = %+v, want the submitted nine fields", st)
	}
	if got, _ := s.GetBox(boxA.ID); got.ConfigVersion != 2 {
		t.Errorf("box A after the edit = %d, want 2", got.ConfigVersion)
	}
	if got, _ := s.GetBox(boxB.ID); got.ConfigVersion != 1 {
		t.Errorf("box B after editing box A = %d, want 1 (untouched)", got.ConfigVersion)
	}

	// The upsert core creates the row when the seed has not run, and the
	// create path bumps too.
	if _, err := s.db.Exec(`DELETE FROM staff WHERE scope = 'box' AND box_id = ?`, boxB.ID); err != nil {
		t.Fatal(err)
	}
	created, err := s.UpdateBoxNarrator(boxB.ID, "Data", "pl", "", "", "", "pl-v1", 0, 0, Character{})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Slug != "st_b_pi" || created.BoxID != boxB.ID {
		t.Errorf("created narrator = %+v, want a minted st_b_pi row on box B", created)
	}
	if got, _ := s.GetBox(boxB.ID); got.ConfigVersion != 2 {
		t.Errorf("box B after the create-if-missing edit = %d, want 2", got.ConfigVersion)
	}
}

func TestUpsertStaffCreate(t *testing.T) {
	tests := []struct {
		name string
		in   Staff // slug, name, locale, avatar model, brief, age, wordBudget, bigFive
	}{
		{
			name: "minimal profile",
			in:   Staff{Slug: "staff-min", Name: "Min", Locale: "en"},
		},
		{
			name: "full profile",
			in:   testStaff(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			created, err := s.UpsertStaff(tt.in.Slug, tt.in.Name, tt.in.Locale, tt.in.AvatarModel3D, tt.in.Brief, tt.in.SoulCore, tt.in.Voice, "org", "", tt.in.Age, tt.in.WordBudget, tt.in.BigFive)
			if err != nil {
				t.Fatal(err)
			}
			if !id.Is(id.Staff, created.ID) {
				t.Errorf("minted id %q is not a staff identifier", created.ID)
			}

			got, err := s.StaffById(created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("StaffById returned nil for a created staff member")
			}
			if got.Slug != tt.in.Slug || got.Name != tt.in.Name || got.Locale != tt.in.Locale ||
				got.AvatarModel3D != tt.in.AvatarModel3D || got.Brief != tt.in.Brief || got.Age != tt.in.Age ||
				got.WordBudget != tt.in.WordBudget || got.SoulCore != tt.in.SoulCore || got.Voice != tt.in.Voice {
				t.Errorf("round trip = %+v, want %+v", got, tt.in)
			}
			if !reflect.DeepEqual(got.BigFive, tt.in.BigFive) {
				t.Errorf("BigFive = %+v, want %+v", got.BigFive, tt.in.BigFive)
			}
			if got.CreatedAt == 0 || got.UpdatedAt == 0 {
				t.Errorf("timestamps not set: %+v", got)
			}
			if got.CreatedAt != created.CreatedAt || got.UpdatedAt != created.UpdatedAt {
				t.Errorf("read-back timestamps %d/%d differ from created %d/%d",
					got.CreatedAt, got.UpdatedAt, created.CreatedAt, created.UpdatedAt)
			}
		})
	}
}

func TestUpsertStaffUpdate(t *testing.T) {
	s := testStore(t)
	base := testStaff()
	created, err := s.UpsertStaff(base.Slug, base.Name, base.Locale, base.AvatarModel3D, base.Brief, base.SoulCore, base.Voice, "org", "", base.Age, base.WordBudget, base.BigFive)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate updated_at so the bump below is observable even within the
	// same wall-clock second.
	if _, err := s.db.Exec(`UPDATE staff SET updated_at = 1 WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.UpsertStaff("coder-zeta", "Zeta Two", "en", "zeta2.glb", "sharper now", "explain first", "zeta-v2", "org", "", 42, 3000,
		Character{Openness: 1, Conscientiousness: 1, Extraversion: 1, Agreeableness: 1, Neuroticism: 0})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("UpsertStaff returned nil for an existing slug")
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q (update must not mint a new id)", got.ID, created.ID)
	}
	if got.Name != "Zeta Two" || got.Locale != "en" || got.AvatarModel3D != "zeta2.glb" ||
		got.Brief != "sharper now" || got.Age != 42 || got.WordBudget != 3000 ||
		got.SoulCore != "explain first" || got.Voice != "zeta-v2" {
		t.Errorf("updated fields = %+v", got)
	}
	wantBigFive := Character{Openness: 1, Conscientiousness: 1, Extraversion: 1, Agreeableness: 1, Neuroticism: 0}
	if !reflect.DeepEqual(got.BigFive, wantBigFive) {
		t.Errorf("BigFive = %+v, want %+v", got.BigFive, wantBigFive)
	}
	if got.UpdatedAt <= 1 {
		t.Errorf("UpdatedAt = %d, want bumped past the backdated 1", got.UpdatedAt)
	}
	if got.CreatedAt != created.CreatedAt {
		t.Errorf("CreatedAt = %d, want %d", got.CreatedAt, created.CreatedAt)
	}
	// An update of an existing slug must not add a row.
	list, err := s.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlug(list, "coder-zeta"); n != 1 {
		t.Errorf("rows with slug coder-zeta = %d, want 1", n)
	}
}

func TestUpsertStaffSlugUnique(t *testing.T) {
	s := testStore(t)
	if _, err := s.UpsertStaff("staff-taken", "One", "en", "", "", "", "", "org", "", 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}
	// A direct second insert on the same slug is refused by the unique index.
	_, err := s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d, created_at, updated_at)
		VALUES ('staff-00000000-0000-0000-0000-000000000000', 'staff-taken', 'Two', 'en', 31, '{}', '', 0, '', 1, 1)`)
	if err == nil {
		t.Fatal("second insert with the same slug succeeded, want a unique constraint refusal")
	}
}

// Slug uniqueness is per scope: an org-scoped slug stays unique across the
// installation, while a box-scoped slug keys on the box — two boxes may
// carry the same st_b_* slug, one box may not carry it twice, and tuning one
// box's row must not touch another box's.
func TestUpsertStaffSlugUniquePerScope(t *testing.T) {
	s := testStore(t)
	boxA, err := s.CreateBox("box-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-b", "B", "", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpsertStaff("staff-org-only", "One", "en", "", "", "", "", "org", "", 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d, created_at, updated_at)
		VALUES ('staff-00000000-0000-0000-0000-000000000000', 'staff-org-only', 'Two', 'en', 31, '{}', '', 0, '', 1, 1)`)
	if err == nil {
		t.Fatal("second org-scoped insert with the same slug succeeded, want a unique constraint refusal")
	}

	// CreateBox already seeded st_b_pi for both boxes; the box-keyed update
	// tunes only box B's row.
	if _, err := s.UpsertStaff("st_b_pi", "Data B", "en", "", "", "", "", "box", boxB.ID, 0, 0, neutralBigFive()); err != nil {
		t.Fatal(err)
	}
	rosterA, err := s.StaffForBox(boxA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterA) != 1 || rosterA[0].Name != "Joe" {
		t.Errorf("box A narrator = %+v, want the untouched seed", rosterA)
	}
	rosterB, err := s.StaffForBox(boxB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterB) != 1 || rosterB[0].Name != "Data B" {
		t.Errorf("box B narrator = %+v, want the tuned row", rosterB)
	}

	// The same slug twice in one box is refused by the per-box partial index.
	_, err = s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d, scope, box_id, created_at, updated_at)
		VALUES ('staff-00000000-0000-0000-0000-000000000001', 'st_b_pi', 'Twin', 'en', 0, '{}', '', 0, '', 'box', ?, 1, 1)`, boxA.ID)
	if err == nil {
		t.Fatal("second insert of the same slug in the same box succeeded, want a unique constraint refusal")
	}
}

func TestUpsertStaffScopeValidation(t *testing.T) {
	s := testStore(t)
	const boxID = "box-00000000-0000-0000-0000-000000000000"
	tests := []struct {
		name      string
		slug      string
		scope     string
		boxID     string
		wantError error
	}{
		{name: "box slug with box scope", slug: "st_b_pi", scope: "box", boxID: boxID},
		{name: "box slug with org scope", slug: "st_b_pi", scope: "org", wantError: ErrStaffScopeMismatch},
		{name: "box slug without a box", slug: "st_b_pi", scope: "box", wantError: ErrStaffScopeMismatch},
		{name: "org slug with org scope", slug: "staff-mike-00", scope: "org"},
		{name: "org slug with box scope", slug: "staff-mike-00", scope: "box", boxID: boxID, wantError: ErrStaffScopeMismatch},
		{name: "org slug carrying a box", slug: "sto_da", scope: "org", boxID: boxID, wantError: ErrStaffScopeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.UpsertStaff(tt.slug, tt.slug, "en", "", "", "", "", tt.scope, tt.boxID, 30, 0, Character{})
			if tt.wantError != nil {
				if !errors.Is(err, tt.wantError) {
					t.Fatalf("UpsertStaff error = %v, want %v", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpsertStaff = %v, want success", err)
			}
		})
	}
}

func TestUpsertStaffScopeRoundTrip(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-one", "One", "", "")
	if err != nil {
		t.Fatal(err)
	}
	narrator, err := s.UpsertStaff("st_b_pi", "Data", "en", "", "", "", "", "box", box.ID, 30, 0, Character{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.StaffById(narrator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != "box" || got.BoxID != box.ID {
		t.Errorf("box-scoped row = scope %q box_id %q, want box/%s", got.Scope, got.BoxID, box.ID)
	}

	orgStaff, err := s.UpsertStaff("staff-nova-00", "Nova", "en", "", "", "", "", "org", "", 30, 0, Character{})
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.StaffById(orgStaff.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != "org" || got.BoxID != "" {
		t.Errorf("org-scoped row = scope %q box_id %q, want org/empty", got.Scope, got.BoxID)
	}
}

func TestStaffForBoxReturnsBoxScopedOnly(t *testing.T) {
	s := testStore(t)
	boxA, err := s.CreateBox("box-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-b", "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertStaff("staff-org-00", "Org Narr", "en", "", "", "", "", "org", "", 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertStaff("st_b_pi", "Data", "en", "", "", "", "", "box", boxA.ID, 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertStaff("st_b_jl", "Jean-Luc", "en", "", "", "", "", "box", boxB.ID, 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}

	rosterA, err := s.StaffForBox(boxA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterA) != 1 {
		t.Fatalf("StaffForBox(%s) = %d rows, want only the box narrator", boxA.ID, len(rosterA))
	}
	if got := rosterA[0]; got.Slug != "st_b_pi" || got.Scope != "box" || got.BoxID != boxA.ID {
		t.Errorf("box A narrator = %+v, want the st_b_pi box-scoped row", got)
	}

	rosterB, err := s.StaffForBox(boxB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterB) != 2 {
		t.Fatalf("StaffForBox(%s) = %d rows, want the seeded st_b_pi and the added st_b_jl", boxB.ID, len(rosterB))
	}
	for _, st := range rosterB {
		if st.BoxID != boxB.ID {
			t.Errorf("box B row %s bound to %q, want box B", st.Slug, st.BoxID)
		}
	}
	if countSlug(rosterB, "st_b_jl") != 1 {
		t.Errorf("StaffForBox(%s) = %+v, want the st_b_jl row", boxB.ID, rosterB)
	}

	empty, err := s.StaffForBox("box-00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("StaffForBox for an unknown box = %d rows, want 0", len(empty))
	}
}

func TestStaffForOrgExcludesBoxScoped(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("box-x", "X", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertStaff("st_b_pi", "Data", "en", "", "", "", "", "box", box.ID, 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}

	roster, err := s.StaffForOrg("org-anyone")
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range roster {
		if st.Scope != "org" {
			t.Errorf("org roster row %s has scope %q, want org", st.Slug, st.Scope)
		}
		if st.BoxID != "" {
			t.Errorf("org roster row %s carries box_id %q, want empty", st.Slug, st.BoxID)
		}
	}
	if n := countSlug(roster, "st_b_pi"); n != 0 {
		t.Errorf("org roster contains the box narrator %d times, want 0", n)
	}
	if n := countSlug(roster, "staff-male-00"); n != 1 {
		t.Errorf("org roster contains the seed staff-male-00 %d times, want 1", n)
	}
}

func TestStaffForOrgWalkUp(t *testing.T) {
	const (
		orgA = "org-a"
		orgB = "org-b"
	)
	base := testStaff()
	overrideBF := Character{Openness: 0.1, Conscientiousness: 0.2, Extraversion: 0.3, Agreeableness: 0.4, Neuroticism: 0.9}
	zeroBF := Character{}
	overrideName := "Zed"
	overrideAge := 7
	overrideSoul := "walk-up soul"
	overrideVoice := "walk-up-v1"
	overrideBrief := "override brief"
	overrideAvatarModel3D := "override.glb"
	overrideBudget := 99
	full := OrgStaffOverride{
		OrgID:         orgA,
		Name:          &overrideName,
		Age:           &overrideAge,
		SoulOverride:  &overrideSoul,
		Voice:         &overrideVoice,
		BigFive:       &overrideBF,
		Brief:         &overrideBrief,
		AvatarModel3D: &overrideAvatarModel3D,
		WordBudget:    &overrideBudget,
	}

	tests := []struct {
		name              string
		override          *OrgStaffOverride // applied to org A; nil means none
		clearAfter        bool
		org               string
		wantName          string
		wantAge           int
		wantSoulCore      string
		wantSoulOverride  string
		wantVoice         string
		wantBigFive       Character
		wantBrief         string
		wantAvatarModel3D string
		wantWordBudget    int
	}{
		{
			name:              "missing override returns base",
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "full override replaces every overridable field",
			override:          &full,
			org:               orgA,
			wantName:          overrideName,
			wantAge:           overrideAge,
			wantSoulCore:      base.SoulCore,
			wantSoulOverride:  overrideSoul,
			wantVoice:         overrideVoice,
			wantBigFive:       overrideBF,
			wantBrief:         overrideBrief,
			wantAvatarModel3D: overrideAvatarModel3D,
			wantWordBudget:    overrideBudget,
		},
		{
			name:              "name override only",
			override:          &OrgStaffOverride{OrgID: orgA, Name: &overrideName},
			org:               orgA,
			wantName:          overrideName,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "age override only",
			override:          &OrgStaffOverride{OrgID: orgA, Age: &overrideAge},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           overrideAge,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "soul override only",
			override:          &OrgStaffOverride{OrgID: orgA, SoulOverride: &overrideSoul},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantSoulOverride:  overrideSoul,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "voice override only",
			override:          &OrgStaffOverride{OrgID: orgA, Voice: &overrideVoice},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         overrideVoice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "big five override only",
			override:          &OrgStaffOverride{OrgID: orgA, BigFive: &overrideBF},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       overrideBF,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "brief override only",
			override:          &OrgStaffOverride{OrgID: orgA, Brief: &overrideBrief},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         overrideBrief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "avatar model override only",
			override:          &OrgStaffOverride{OrgID: orgA, AvatarModel3D: &overrideAvatarModel3D},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: overrideAvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "word budget override only",
			override:          &OrgStaffOverride{OrgID: orgA, WordBudget: &overrideBudget},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    overrideBudget,
		},
		{
			name:              "empty name override still wins over base",
			override:          &OrgStaffOverride{OrgID: orgA, Name: strPtr("")},
			org:               orgA,
			wantName:          "",
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "zero age override still wins over base",
			override:          &OrgStaffOverride{OrgID: orgA, Age: intPtr(0)},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           0,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "empty soul override still records the override",
			override:          &OrgStaffOverride{OrgID: orgA, SoulOverride: strPtr("")},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "empty voice override still wins over base",
			override:          &OrgStaffOverride{OrgID: orgA, Voice: strPtr("")},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         "",
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "empty brief override still wins over base",
			override:          &OrgStaffOverride{OrgID: orgA, Brief: strPtr("")},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         "",
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "empty avatar model override still wins over base",
			override:          &OrgStaffOverride{OrgID: orgA, AvatarModel3D: strPtr("")},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: "",
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "zero word budget override still wins over base",
			override:          &OrgStaffOverride{OrgID: orgA, WordBudget: intPtr(0)},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    0,
		},
		{
			name:              "zero big five override still wins over base",
			override:          &OrgStaffOverride{OrgID: orgA, BigFive: &zeroBF},
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       zeroBF,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "org B does not inherit org A override",
			override:          &full,
			org:               orgB,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
		{
			name:              "clear override returns to base",
			override:          &full,
			clearAfter:        true,
			org:               orgA,
			wantName:          base.Name,
			wantAge:           base.Age,
			wantSoulCore:      base.SoulCore,
			wantVoice:         base.Voice,
			wantBigFive:       base.BigFive,
			wantBrief:         base.Brief,
			wantAvatarModel3D: base.AvatarModel3D,
			wantWordBudget:    base.WordBudget,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			st := newBaseStaff(t, s)
			if tt.override != nil {
				if err := s.SetOrgStaffOverride(orgA, st.ID, tt.override.Name, tt.override.Age, tt.override.SoulOverride, tt.override.Voice, tt.override.BigFive, tt.override.Brief, tt.override.AvatarModel3D, tt.override.WordBudget); err != nil {
					t.Fatal(err)
				}
				if tt.clearAfter {
					if err := s.ClearOrgStaffOverride(orgA, st.ID); err != nil {
						t.Fatal(err)
					}
				}
			}

			got := findStaff(t, mustStaffForOrg(t, s, tt.org), base.Slug)
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Age != tt.wantAge {
				t.Errorf("Age = %d, want %d", got.Age, tt.wantAge)
			}
			if got.SoulCore != tt.wantSoulCore {
				t.Errorf("SoulCore = %q, want %q", got.SoulCore, tt.wantSoulCore)
			}
			if got.SoulOverride != tt.wantSoulOverride {
				t.Errorf("SoulOverride = %q, want %q", got.SoulOverride, tt.wantSoulOverride)
			}
			if got.Voice != tt.wantVoice {
				t.Errorf("Voice = %q, want %q", got.Voice, tt.wantVoice)
			}
			if !reflect.DeepEqual(got.BigFive, tt.wantBigFive) {
				t.Errorf("BigFive = %+v, want %+v", got.BigFive, tt.wantBigFive)
			}
			if got.Brief != tt.wantBrief {
				t.Errorf("Brief = %q, want %q", got.Brief, tt.wantBrief)
			}
			if got.AvatarModel3D != tt.wantAvatarModel3D {
				t.Errorf("AvatarModel3D = %q, want %q", got.AvatarModel3D, tt.wantAvatarModel3D)
			}
			if got.WordBudget != tt.wantWordBudget {
				t.Errorf("WordBudget = %d, want %d", got.WordBudget, tt.wantWordBudget)
			}
			// The locale is the one field an org cannot override; it always
			// comes from the base row.
			if got.Locale != base.Locale {
				t.Errorf("locale = %q, want %q", got.Locale, base.Locale)
			}
		})
	}
}

func TestSetOrgStaffOverrideUpsert(t *testing.T) {
	s := testStore(t)
	st := newBaseStaff(t, s)
	org := "org-a"

	if err := s.SetOrgStaffOverride(org, st.ID, nil, nil, nil, nil, nil, strPtr("first brief"), strPtr("first.glb"), nil); err != nil {
		t.Fatal(err)
	}
	// The second write replaces the row in place: a new brief, the avatar
	// model cleared back to NULL so the base row wins again.
	if err := s.SetOrgStaffOverride(org, st.ID, nil, nil, nil, nil, nil, strPtr("second brief"), nil, nil); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM org_staff_overrides WHERE org_id = ? AND staff_id = ?`, org, st.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("override rows = %d, want 1", n)
	}
	got := findStaff(t, mustStaffForOrg(t, s, org), st.Slug)
	if got.Brief != "second brief" {
		t.Errorf("Brief = %q, want second brief", got.Brief)
	}
	if got.AvatarModel3D != st.AvatarModel3D {
		t.Errorf("AvatarModel3D = %q, want base %q after the second write cleared it", got.AvatarModel3D, st.AvatarModel3D)
	}
}

func TestClearOrgStaffOverride(t *testing.T) {
	s := testStore(t)
	st := newBaseStaff(t, s)
	org := "org-a"
	bf := Character{Openness: 0.2, Conscientiousness: 0.2, Extraversion: 0.2, Agreeableness: 0.2, Neuroticism: 0.2}
	if err := s.SetOrgStaffOverride(org, st.ID, nil, nil, nil, nil, &bf, strPtr("override"), strPtr("override.glb"), intPtr(7)); err != nil {
		t.Fatal(err)
	}

	if err := s.ClearOrgStaffOverride(org, st.ID); err != nil {
		t.Fatal(err)
	}
	got := findStaff(t, mustStaffForOrg(t, s, org), st.Slug)
	if !reflect.DeepEqual(got.BigFive, st.BigFive) || got.Brief != st.Brief || got.AvatarModel3D != st.AvatarModel3D || got.WordBudget != st.WordBudget {
		t.Errorf("after clear = %+v, want base %+v", got, st)
	}
	// Clearing a missing override is not an error, for either org.
	if err := s.ClearOrgStaffOverride(org, st.ID); err != nil {
		t.Fatalf("second ClearOrgStaffOverride = %v, want nil", err)
	}
	if err := s.ClearOrgStaffOverride("org-b", st.ID); err != nil {
		t.Fatalf("ClearOrgStaffOverride on an untouched org = %v, want nil", err)
	}
}

func TestEnsureSeedStaffIdempotent(t *testing.T) {
	s := testStore(t)

	tests := []struct {
		name              string
		slug              string
		wantName          string
		wantAge           int
		wantAvatarModel3D string
		wantVoice         string
	}{
		{name: "male seed", slug: "staff-male-00", wantName: "Adam", wantAge: 35, wantVoice: "pl_PL-mc_speech-medium"},
		{name: "female seed", slug: "staff-female-00", wantName: "Ewa", wantAge: 32, wantAvatarModel3D: "arianna.glb", wantVoice: "pl_PL-gosia-medium"},
	}
	assertSeeds := func(t *testing.T) {
		t.Helper()
		list, err := s.ListStaff()
		if err != nil {
			t.Fatal(err)
		}
		for _, tt := range tests {
			got := findStaff(t, list, tt.slug)
			if got.Name != tt.wantName {
				t.Errorf("%s name = %q, want %q", tt.slug, got.Name, tt.wantName)
			}
			if got.Locale != "pl" {
				t.Errorf("%s locale = %q, want pl", tt.slug, got.Locale)
			}
			if got.Age != tt.wantAge {
				t.Errorf("%s age = %d, want %d", tt.slug, got.Age, tt.wantAge)
			}
			if got.AvatarModel3D != tt.wantAvatarModel3D {
				t.Errorf("%s avatar_model_3d = %q, want %q", tt.slug, got.AvatarModel3D, tt.wantAvatarModel3D)
			}
			if got.Voice != tt.wantVoice {
				t.Errorf("%s voice = %q, want %q", tt.slug, got.Voice, tt.wantVoice)
			}
			if got.SoulCore != "" {
				t.Errorf("%s soul_core = %q, want empty", tt.slug, got.SoulCore)
			}
			if got.Brief != "" || got.WordBudget != 0 {
				t.Errorf("%s brief/word_budget = %q/%d, want empty/0", tt.slug, got.Brief, got.WordBudget)
			}
			if !reflect.DeepEqual(got.BigFive, neutralBigFive()) {
				t.Errorf("%s BigFive = %+v, want %+v", tt.slug, got.BigFive, neutralBigFive())
			}
		}
	}

	// openStore already ran the seed once; the rows carry the placeholder
	// values.
	assertSeeds(t)

	// A second run changes nothing and mints no new rows.
	if err := s.EnsureSeedStaff(); err != nil {
		t.Fatal(err)
	}
	assertSeeds(t)

	// A store with no staff rows gets exactly the two seeds back, even when
	// the seed runs twice in a row.
	if _, err := s.db.Exec(`DELETE FROM staff`); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSeedStaff(); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSeedStaff(); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ListStaff after two seed runs = %d rows, want 2", len(list))
	}
}

// A store whose staff and org_staff_overrides tables predate the profile
// columns gains them on reopen. CREATE TABLE IF NOT EXISTS will not add
// columns to a live table, so ensureStaffProfileColumns is the only path a
// claimed, upgraded hub takes: old staff rows read the empty defaults the
// column declaration writes, and old override rows read NULL on the re-added
// columns, which means "inherit the base row" until the org tunes them again.
func TestOpenStoreMigratesLegacyStaffColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "staff-migration.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	// One org override row makes the override table live too; brief is the
	// survivor that must keep applying across the migration.
	list, err := s.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("store opened with no staff rows to migrate")
	}
	base := list[0]
	org := "org-migration"
	overrideName := "Pre"
	overrideAge := 60
	if err := s.SetOrgStaffOverride(org, base.ID, &overrideName, &overrideAge, nil, nil, nil, strPtr("kept brief"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate the pre-migration shape: none of the profile columns exist.
	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range []string{
		// The per-scope slug indexes index box_id/scope, and SQLite refuses
		// to drop an indexed column: the truly pre-scope shape has neither.
		`DROP INDEX IF EXISTS staff_slug_org_unique`,
		`DROP INDEX IF EXISTS staff_slug_box_unique`,
		`ALTER TABLE staff DROP COLUMN soul_core`,
		`ALTER TABLE staff DROP COLUMN voice`,
		`ALTER TABLE org_staff_overrides DROP COLUMN name`,
		`ALTER TABLE org_staff_overrides DROP COLUMN age`,
		`ALTER TABLE org_staff_overrides DROP COLUMN soul_override`,
		`ALTER TABLE org_staff_overrides DROP COLUMN voice`,
		`ALTER TABLE staff DROP COLUMN scope`,
		`ALTER TABLE staff DROP COLUMN box_id`,
		`DROP TABLE boxes`,
		`DROP TABLE box_orgs`,
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
		t.Fatalf("reopen on a pre-profile staff schema: %v", err)
	}
	t.Cleanup(func() { again.Close() })

	// The migration path must have re-added every column. A regression that
	// drops one from ensureStaffProfileColumns fails here, not in a later,
	// unrelated query.
	for _, col := range []string{"soul_core", "voice", "scope", "box_id"} {
		ok, err := again.hasColumn("staff", col)
		if err != nil || !ok {
			t.Fatalf("staff.%s after reopen: ok=%v err=%v", col, ok, err)
		}
	}
	// The Faza B tables are created by the schema batch on every open, so a
	// live store that predates boxes gains them without a dedicated
	// migration.
	for _, table := range []string{"boxes", "box_orgs"} {
		ok, err := again.hasTable(table)
		if err != nil || !ok {
			t.Fatalf("table %s after reopen: ok=%v err=%v", table, ok, err)
		}
	}
	// Old staff rows read the org default, not a NULL.
	var scope string
	if err := again.db.QueryRow(`SELECT scope FROM staff LIMIT 1`).Scan(&scope); err != nil {
		t.Fatal(err)
	}
	if scope != "org" {
		t.Errorf("migrated staff scope = %q, want the org default", scope)
	}
	for _, col := range []string{"name", "age", "soul_override", "voice"} {
		ok, err := again.hasColumn("org_staff_overrides", col)
		if err != nil || !ok {
			t.Fatalf("org_staff_overrides.%s after reopen: ok=%v err=%v", col, ok, err)
		}
	}

	// Old staff rows read the empty defaults, not their pre-migration values.
	after, err := again.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 {
		t.Fatalf("ListStaff after migration = %d rows, want the 2 carried rows", len(after))
	}
	for _, got := range after {
		if got.Voice != "" || got.SoulCore != "" {
			t.Errorf("%s after migration: voice=%q soul_core=%q, want the empty defaults", got.Slug, got.Voice, got.SoulCore)
		}
	}

	// The pre-migration override row reads NULL on the re-added columns, so
	// the base row wins there while the surviving brief still applies.
	roster, err := again.StaffForOrg(org)
	if err != nil {
		t.Fatal(err)
	}
	got := findStaff(t, roster, base.Slug)
	if got.Name != base.Name || got.Age != base.Age {
		t.Errorf("migrated override row name/age = %q/%d, want the base %q/%d", got.Name, got.Age, base.Name, base.Age)
	}
	if got.Voice != "" || got.SoulCore != "" {
		t.Errorf("migrated override row voice/soul_core = %q/%q, want the empty base defaults", got.Voice, got.SoulCore)
	}
	if got.Brief != "kept brief" {
		t.Errorf("migrated override row brief = %q, want the surviving override value", got.Brief)
	}
}

// A store whose staff and org_staff_overrides tables still carry the avatar
// GLB under the inherited short column name `model` gains avatar_model_3d on
// reopen. The rename is a guarded column swap, not a data migration: values
// ride along untouched, the old name is gone, and a second reopen on the
// already-renamed store skips the swap — idempotent, so an interrupted or
// repeated open never loses the column or its data.
func TestOpenStoreMigratesAvatarModelColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "staff-avatar-model-migration.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	// Make the override table live with a row whose avatar model must
	// survive the rename; the female seed carries the staff-side value.
	list, err := s.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	base := findStaff(t, list, "staff-female-00")
	org := "org-avatar-model-migration"
	avatarModel3D := "custom.glb"
	if err := s.SetOrgStaffOverride(org, base.ID, nil, nil, nil, nil, nil, nil, &avatarModel3D, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Rewind both tables to the pre-rename shape: the short `model` column.
	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, rewind := range []string{
		`ALTER TABLE staff RENAME COLUMN avatar_model_3d TO model`,
		`ALTER TABLE org_staff_overrides RENAME COLUMN avatar_model_3d TO model`,
	} {
		if _, err := db.Exec(rewind); err != nil {
			_ = db.Close()
			t.Fatalf("%s: %v", rewind, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen on a pre-rename avatar model column: %v", err)
	}
	t.Cleanup(func() { again.Close() })

	for _, table := range []string{"staff", "org_staff_overrides"} {
		ok, err := again.hasColumn(table, "avatar_model_3d")
		if err != nil || !ok {
			t.Fatalf("%s.avatar_model_3d after reopen: ok=%v err=%v", table, ok, err)
		}
		ok, err = again.hasColumn(table, "model")
		if err != nil || ok {
			t.Fatalf("%s.model after reopen: ok=%v err=%v, want the column renamed away", table, ok, err)
		}
	}

	// The staff-side value survived the rename: the female seed still reads
	// arianna.glb, and the org override still applies its own value.
	after, err := again.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	if got := findStaff(t, after, "staff-female-00"); got.AvatarModel3D != "arianna.glb" {
		t.Errorf("seed avatar_model_3d after migration = %q, want arianna.glb", got.AvatarModel3D)
	}
	roster, err := again.StaffForOrg(org)
	if err != nil {
		t.Fatal(err)
	}
	if got := findStaff(t, roster, "staff-female-00"); got.AvatarModel3D != "custom.glb" {
		t.Errorf("override avatar_model_3d after migration = %q, want custom.glb", got.AvatarModel3D)
	}

	// Idempotent: a second reopen finds the new name already in place and
	// changes nothing.
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := OpenStore(path)
	if err != nil {
		t.Fatalf("second reopen: %v", err)
	}
	t.Cleanup(func() { third.Close() })
	thirdRoster, err := third.StaffForOrg(org)
	if err != nil {
		t.Fatal(err)
	}
	if got := findStaff(t, thirdRoster, "staff-female-00"); got.AvatarModel3D != "custom.glb" {
		t.Errorf("override avatar_model_3d after the second reopen = %q, want custom.glb", got.AvatarModel3D)
	}
}

// A store whose staff table predates per-scope slugs still carries the old
// installation-wide UNIQUE on slug as an auto-index. The reopen must rebuild
// the table without it, keep every row, and replace it with the two partial
// indexes, so two boxes can each seed their own st_b_pi while an org-scoped
// slug stays unique across the installation.
func TestOpenStoreRelaxesGlobalStaffSlugUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "staff-slug-migration.db")
	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	// The pre-migration shape: slug UNIQUE, no profile/scope columns yet.
	for _, ddl := range []string{
		`CREATE TABLE staff (
			id          TEXT PRIMARY KEY,
			slug        TEXT NOT NULL UNIQUE,
			name        TEXT NOT NULL,
			locale      TEXT NOT NULL DEFAULT 'en',
			age         INTEGER NOT NULL,
			big_five    TEXT NOT NULL DEFAULT '{}',
			brief       TEXT NOT NULL DEFAULT '',
			word_budget INTEGER NOT NULL DEFAULT 0,
			model       TEXT NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL
		)`,
		`INSERT INTO staff (id, slug, name, locale, age, big_five, created_at, updated_at)
			VALUES ('staff-carried', 'staff-carried-00', 'Carried', 'en', 40, '{}', 1, 1)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			_ = db.Close()
			t.Fatalf("building the pre-migration schema: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen on a globally-unique staff slug schema: %v", err)
	}

	// The carried row survives the rebuild with its values.
	carried, err := s.StaffById("staff-carried")
	if err != nil || carried == nil {
		t.Fatalf("StaffById on the migrated row: %v %v", carried, err)
	}
	if carried.Slug != "staff-carried-00" || carried.Name != "Carried" || carried.Age != 40 {
		t.Errorf("migrated row = %+v, want the carried values", carried)
	}

	// The global unique is gone: two boxes seed their own st_b_pi.
	boxA, err := s.CreateBox("box-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-b", "B", "", "")
	if err != nil {
		t.Fatalf("second box under the relaxed slug contract: %v", err)
	}
	rosterA, err := s.StaffForBox(boxA.ID)
	if err != nil {
		t.Fatal(err)
	}
	rosterB, err := s.StaffForBox(boxB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rosterA) != 1 || len(rosterB) != 1 || rosterA[0].ID == rosterB[0].ID {
		t.Errorf("narrators after migration = %+v / %+v, want one distinct st_b_pi per box", rosterA, rosterB)
	}

	// Both partial indexes exist…
	for _, index := range []string{"staff_slug_org_unique", "staff_slug_box_unique"} {
		ok, err := s.sqliteIndexExists("staff", index)
		if err != nil || !ok {
			t.Fatalf("index %s after reopen: ok=%v err=%v", index, ok, err)
		}
	}

	// …and the org contract still holds: the partial org index refuses a
	// duplicate org-scoped slug.
	if _, err := s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, created_at, updated_at)
		VALUES ('staff-00000000-0000-0000-0000-000000000000', 'staff-carried-00', 'Twin', 'en', 1, '{}', 1, 1)`); err == nil {
		t.Error("duplicate org-scoped slug succeeded after migration, want a unique constraint refusal")
	}

	// Idempotent: a second open changes nothing and loses no rows.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("second reopen: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	list, err := again.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	// The canonical list carries the org-scoped rows only: the carried row
	// and the two seeds. The two narrators are box-scoped — visible through
	// StaffForBox, never through ListStaff.
	if len(list) != 3 {
		t.Errorf("ListStaff after the second open = %d rows, want the carried row and the two seeds", len(list))
	}
	for _, st := range list {
		if st.Scope != "org" {
			t.Errorf("ListStaff carries %q with scope %q, want org-scoped rows only", st.Slug, st.Scope)
		}
	}
}

// sqliteIndexExists reports whether one named SQLite index exists on a table.
func (s *Store) sqliteIndexExists(table, index string) (bool, error) {
	var n int
	// pragma_index_list does not take a bound table name; the callers pass a
	// constant from this file.
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_index_list('`+table+`') WHERE name = ?`, index).Scan(&n)
	return n > 0, err
}
