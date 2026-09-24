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
		BiologicalGender: "male",
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
	created, err := s.UpsertStaff(st.Slug, st.Name, st.Locale, st.AvatarModel3D, st.Brief, st.SoulCore, st.Voice, st.BiologicalGender, st.Age, st.WordBudget, st.BigFive)
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
		if _, err := s.UpsertStaff(slug, slug, "en", "", "", "", "", "female", 30, 0, Character{}); err != nil {
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

// A box's narrator is a box property, not a staff row: ListStaff never
// carries it, and GetBoxNarrator serves it keyed by the box.
func TestListStaffExcludesNarrator(t *testing.T) {
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
		if st.Slug == "st_b_pi" {
			t.Errorf("ListStaff carries the narrator %q", st.Slug)
		}
	}
	narrator, err := s.GetBoxNarrator(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if narrator == nil || narrator.Slug != "st_b_pi" {
		t.Errorf("GetBoxNarrator = %+v, want the box's st_b_pi narrator", narrator)
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
	st, err := s.UpdateBoxNarrator(boxA.ID, "Lore", "en", "lore.glb", "warm and precise", "explain first", "lore-v2", "female", 42, 1200, want)
	if err != nil {
		t.Fatal(err)
	}
	if st.Slug != "st_b_pi" {
		t.Errorf("narrator identity = %+v, want the box's st_b_pi narrator", st)
	}
	if st.Name != "Lore" || st.Locale != "en" || st.Age != 42 || st.WordBudget != 1200 ||
		st.AvatarModel3D != "lore.glb" || st.Voice != "lore-v2" || st.BiologicalGender != "female" ||
		st.Brief != "warm and precise" || st.SoulCore != "explain first" || st.BigFive != want {
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
	if _, err := s.db.Exec(`DELETE FROM box_narrator WHERE box_id = ?`, boxB.ID); err != nil {
		t.Fatal(err)
	}
	created, err := s.UpdateBoxNarrator(boxB.ID, "Data", "pl", "", "", "", "pl-v1", "male", 0, 0, Character{})
	if err != nil {
		t.Fatal(err)
	}
	if created.Slug != "st_b_pi" {
		t.Errorf("created narrator = %+v, want a st_b_pi narrator on box B", created)
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
			in:   Staff{Slug: "staff-min", Name: "Min", Locale: "en", BiologicalGender: "female"},
		},
		{
			name: "full profile",
			in:   testStaff(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			created, err := s.UpsertStaff(tt.in.Slug, tt.in.Name, tt.in.Locale, tt.in.AvatarModel3D, tt.in.Brief, tt.in.SoulCore, tt.in.Voice, tt.in.BiologicalGender, tt.in.Age, tt.in.WordBudget, tt.in.BigFive)
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
	created, err := s.UpsertStaff(base.Slug, base.Name, base.Locale, base.AvatarModel3D, base.Brief, base.SoulCore, base.Voice, base.BiologicalGender, base.Age, base.WordBudget, base.BigFive)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate updated_at so the bump below is observable even within the
	// same wall-clock second.
	if _, err := s.db.Exec(`UPDATE staff SET updated_at = 1 WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.UpsertStaff("coder-zeta", "Zeta Two", "en", "zeta2.glb", "sharper now", "explain first", "zeta-v2", "male", 42, 3000,
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
	if _, err := s.UpsertStaff("staff-taken", "One", "en", "", "", "", "", "female", 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}
	// A direct second insert on the same slug is refused by the unique index.
	_, err := s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d, created_at, updated_at)
		VALUES ('staff-00000000-0000-0000-0000-000000000000', 'staff-taken', 'Two', 'en', 31, '{}', '', 0, '', 1, 1)`)
	if err == nil {
		t.Fatal("second insert with the same slug succeeded, want a unique constraint refusal")
	}
}

// Staff slugs are unique across the installation, and a st_b_* slug — the
// narrator namespace — is refused: staff are org-scoped only, and a box's
// narrator is the box's own being, not a staff row (58).
func TestUpsertStaffRejectsBoxScopedSlug(t *testing.T) {
	s := testStore(t)
	if _, err := s.UpsertStaff("st_b_pi", "Data", "en", "", "", "", "", "male", 30, 0, Character{}); err == nil {
		t.Fatal("UpsertStaff accepted a st_b_* slug, want ErrBoxScopedSlugRejected")
	} else if !errors.Is(err, ErrBoxScopedSlugRejected) {
		t.Fatalf("UpsertStaff error = %v, want ErrBoxScopedSlugRejected", err)
	}
	if _, err := s.UpsertStaff("st_o_da", "Data", "en", "", "", "", "", "male", 30, 0, Character{}); err != nil {
		t.Fatalf("UpsertStaff on an org-scoped st_o_* slug = %v, want success", err)
	}
}

// A narrator is the box's own 1:1 being: CreateBox seeds one per box, each
// carries the fixed slug st_b_pi, and none of them reach ListStaff or an org
// roster. The box's id is the narrator's identity — there is no minted staff
// id, no scope and no boxId on it.
func TestGetBoxNarrator(t *testing.T) {
	s := testStore(t)
	boxA, err := s.CreateBox("box-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-b", "B", "", "")
	if err != nil {
		t.Fatal(err)
	}

	narrA, err := s.GetBoxNarrator(boxA.ID)
	if err != nil {
		t.Fatal(err)
	}
	narrB, err := s.GetBoxNarrator(boxB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if narrA == nil || narrB == nil {
		t.Fatalf("GetBoxNarrator = (%+v, %+v), want one narrator per box", narrA, narrB)
	}
	if narrA.Slug != "st_b_pi" || narrB.Slug != "st_b_pi" {
		t.Errorf("narrator slugs = %q / %q, want st_b_pi for both", narrA.Slug, narrB.Slug)
	}

	// Editing one box's narrator never touches the other.
	tuned, err := s.UpdateBoxNarrator(boxB.ID, "Lore", "en", "", "", "", "lore-v2", "female", 0, 0, Character{})
	if err != nil {
		t.Fatal(err)
	}
	if tuned.Slug != "st_b_pi" || tuned.Name != "Lore" {
		t.Errorf("tuned narrator = %+v, want the st_b_pi Lore row", tuned)
	}
	if again, _ := s.GetBoxNarrator(boxA.ID); again.Name != "Ania" {
		t.Errorf("box A narrator after editing B = %q, want the untouched Ania", again.Name)
	}

	// A box with no narrator row answers nil, never an empty object.
	if missing, err := s.GetBoxNarrator("box-00000000-0000-0000-0000-000000000000"); err != nil || missing != nil {
		t.Errorf("GetBoxNarrator for an unknown box = (%v, %v), want (nil, nil)", missing, err)
	}

	// The narrator never reaches the installation roster.
	list, err := s.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlug(list, "st_b_pi"); n != 0 {
		t.Errorf("ListStaff carries the narrator %d times, want 0", n)
	}
}

// The org roster never carries a narrator: staff are org-scoped only, and a
// box's narrator is a box property (58).
func TestStaffForOrgExcludesNarrator(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateBox("box-x", "X", "", ""); err != nil {
		t.Fatal(err)
	}
	roster, err := s.StaffForOrg("org-anyone")
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlug(roster, "st_b_pi"); n != 0 {
		t.Errorf("org roster contains the narrator %d times, want 0", n)
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
		`ALTER TABLE staff DROP COLUMN soul_core`,
		`ALTER TABLE staff DROP COLUMN voice`,
		`ALTER TABLE org_staff_overrides DROP COLUMN name`,
		`ALTER TABLE org_staff_overrides DROP COLUMN age`,
		`ALTER TABLE org_staff_overrides DROP COLUMN soul_override`,
		`ALTER TABLE org_staff_overrides DROP COLUMN voice`,
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

	// The migration path must have re-added every profile column. A
	// regression that drops one from ensureStaffProfileColumns fails here,
	// not in a later, unrelated query.
	for _, col := range []string{"soul_core", "voice"} {
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

// A store whose staff table still carries the scope/box_id columns and the
// two partial slug indexes the box-narrator model introduced gains the
// re-tightened shape on reopen: the columns and indexes are dropped and slug
// uniqueness returns to installation-wide, because staff is org-scoped only
// again. The org-scoped rows survive the rebuild, and the drop is idempotent.
func TestOpenStoreDropsStaffScopeColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "staff-scope-drop-migration.db")
	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	// The pre-cleanup shape: scope/box_id columns and per-scope slug
	// uniqueness (no installation-wide UNIQUE on slug).
	for _, ddl := range []string{
		`CREATE TABLE staff (
			id          TEXT PRIMARY KEY,
			slug        TEXT NOT NULL,
			name        TEXT NOT NULL,
			locale      TEXT NOT NULL DEFAULT 'en',
			age         INTEGER NOT NULL,
			big_five    TEXT NOT NULL DEFAULT '{}',
			brief       TEXT NOT NULL DEFAULT '',
			word_budget INTEGER NOT NULL DEFAULT 0,
			avatar_model_3d TEXT NOT NULL DEFAULT '',
			soul_core   TEXT NOT NULL DEFAULT '',
			voice       TEXT NOT NULL DEFAULT '',
			biological_gender TEXT NOT NULL DEFAULT '',
			scope       TEXT NOT NULL DEFAULT 'org' CHECK (scope IN ('org','box')),
			box_id      TEXT,
			created_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX staff_slug_org_unique ON staff(slug) WHERE scope = 'org'`,
		`CREATE UNIQUE INDEX staff_slug_box_unique ON staff(box_id, slug) WHERE scope = 'box'`,
		`INSERT INTO staff (id, slug, name, locale, age, big_five, created_at, updated_at)
			VALUES ('staff-carried', 'staff-carried-00', 'Carried', 'en', 40, '{}', 1, 1)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			_ = db.Close()
			t.Fatalf("building the pre-cleanup schema: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen on a per-scope staff schema: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// The carried row survives the rebuild with its values.
	carried, err := s.StaffById("staff-carried")
	if err != nil || carried == nil {
		t.Fatalf("StaffById on the migrated row: %v %v", carried, err)
	}
	if carried.Slug != "staff-carried-00" || carried.Name != "Carried" || carried.Age != 40 {
		t.Errorf("migrated row = %+v, want the carried values", carried)
	}

	// scope/box_id and the partial indexes are gone.
	for _, col := range []string{"scope", "box_id"} {
		ok, err := s.hasColumn("staff", col)
		if err != nil || ok {
			t.Fatalf("staff.%s after reopen: ok=%v err=%v, want the column dropped", col, ok, err)
		}
	}
	for _, index := range []string{"staff_slug_org_unique", "staff_slug_box_unique"} {
		ok, err := s.sqliteIndexExists("staff", index)
		if err != nil || ok {
			t.Fatalf("index %s after reopen: ok=%v err=%v, want the index dropped", index, ok, err)
		}
	}

	// slug uniqueness is installation-wide again: a duplicate org-scoped
	// slug is refused.
	if _, err := s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, created_at, updated_at)
		VALUES ('staff-00000000-0000-0000-0000-000000000000', 'staff-carried-00', 'Twin', 'en', 1, '{}', 1, 1)`); err == nil {
		t.Error("duplicate slug succeeded after cleanup, want a unique constraint refusal")
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
	if list, err := again.ListStaff(); err != nil {
		t.Fatal(err)
	} else if len(list) == 0 {
		t.Errorf("ListStaff after the second open = 0 rows, want the carried row and the seeds")
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
