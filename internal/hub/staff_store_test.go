package hub

import (
	"reflect"
	"testing"

	"github.com/pleware/initagent/internal/id"
)

// testStaff is a fully-populated staff member used where a test needs a set one.
func testStaff() Staff {
	return Staff{
		Slug:       "coder-zeta",
		Name:       "Zeta",
		Locale:     "pl",
		Model:      "zeta.glb",
		Brief:      "a sharp coder",
		Age:        41,
		WordBudget: 2500,
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
	created, err := s.UpsertStaff(st.Slug, st.Name, st.Locale, st.Model, st.Brief, st.Age, st.WordBudget, st.BigFive)
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
		if _, err := s.UpsertStaff(slug, slug, "en", "", "", 30, 0, Character{}); err != nil {
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

func TestUpsertStaffCreate(t *testing.T) {
	tests := []struct {
		name string
		in   Staff // slug, name, locale, model, brief, age, wordBudget, bigFive
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
			created, err := s.UpsertStaff(tt.in.Slug, tt.in.Name, tt.in.Locale, tt.in.Model, tt.in.Brief, tt.in.Age, tt.in.WordBudget, tt.in.BigFive)
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
				got.Model != tt.in.Model || got.Brief != tt.in.Brief || got.Age != tt.in.Age ||
				got.WordBudget != tt.in.WordBudget {
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
	created, err := s.UpsertStaff(base.Slug, base.Name, base.Locale, base.Model, base.Brief, base.Age, base.WordBudget, base.BigFive)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate updated_at so the bump below is observable even within the
	// same wall-clock second.
	if _, err := s.db.Exec(`UPDATE staff SET updated_at = 1 WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.UpsertStaff("coder-zeta", "Zeta Two", "en", "zeta2.glb", "sharper now", 42, 3000,
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
	if got.Name != "Zeta Two" || got.Locale != "en" || got.Model != "zeta2.glb" ||
		got.Brief != "sharper now" || got.Age != 42 || got.WordBudget != 3000 {
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
	if _, err := s.UpsertStaff("staff-taken", "One", "en", "", "", 30, 0, Character{}); err != nil {
		t.Fatal(err)
	}
	// A direct second insert on the same slug is refused by the unique index.
	_, err := s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, model, created_at, updated_at)
		VALUES ('staff-00000000-0000-0000-0000-000000000000', 'staff-taken', 'Two', 'en', 31, '{}', '', 0, '', 1, 1)`)
	if err == nil {
		t.Fatal("second insert with the same slug succeeded, want a unique constraint refusal")
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
	overrideBrief := "override brief"
	overrideModel := "override.glb"
	overrideBudget := 99
	full := OrgStaffOverride{
		OrgID:      orgA,
		BigFive:    &overrideBF,
		Brief:      &overrideBrief,
		Model:      &overrideModel,
		WordBudget: &overrideBudget,
	}

	tests := []struct {
		name           string
		override       *OrgStaffOverride // applied to org A; nil means none
		clearAfter     bool
		org            string
		wantBigFive    Character
		wantBrief      string
		wantModel      string
		wantWordBudget int
	}{
		{
			name:           "missing override returns base",
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      base.Brief,
			wantModel:      base.Model,
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "full override replaces every overridable field",
			override:       &full,
			org:            orgA,
			wantBigFive:    overrideBF,
			wantBrief:      overrideBrief,
			wantModel:      overrideModel,
			wantWordBudget: overrideBudget,
		},
		{
			name:           "big five override only",
			override:       &OrgStaffOverride{OrgID: orgA, BigFive: &overrideBF},
			org:            orgA,
			wantBigFive:    overrideBF,
			wantBrief:      base.Brief,
			wantModel:      base.Model,
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "brief override only",
			override:       &OrgStaffOverride{OrgID: orgA, Brief: &overrideBrief},
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      overrideBrief,
			wantModel:      base.Model,
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "model override only",
			override:       &OrgStaffOverride{OrgID: orgA, Model: &overrideModel},
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      base.Brief,
			wantModel:      overrideModel,
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "word budget override only",
			override:       &OrgStaffOverride{OrgID: orgA, WordBudget: &overrideBudget},
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      base.Brief,
			wantModel:      base.Model,
			wantWordBudget: overrideBudget,
		},
		{
			name:           "empty brief override still wins over base",
			override:       &OrgStaffOverride{OrgID: orgA, Brief: strPtr("")},
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      "",
			wantModel:      base.Model,
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "empty model override still wins over base",
			override:       &OrgStaffOverride{OrgID: orgA, Model: strPtr("")},
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      base.Brief,
			wantModel:      "",
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "zero word budget override still wins over base",
			override:       &OrgStaffOverride{OrgID: orgA, WordBudget: intPtr(0)},
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      base.Brief,
			wantModel:      base.Model,
			wantWordBudget: 0,
		},
		{
			name:           "zero big five override still wins over base",
			override:       &OrgStaffOverride{OrgID: orgA, BigFive: &zeroBF},
			org:            orgA,
			wantBigFive:    zeroBF,
			wantBrief:      base.Brief,
			wantModel:      base.Model,
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "org B does not inherit org A override",
			override:       &full,
			org:            orgB,
			wantBigFive:    base.BigFive,
			wantBrief:      base.Brief,
			wantModel:      base.Model,
			wantWordBudget: base.WordBudget,
		},
		{
			name:           "clear override returns to base",
			override:       &full,
			clearAfter:     true,
			org:            orgA,
			wantBigFive:    base.BigFive,
			wantBrief:      base.Brief,
			wantModel:      base.Model,
			wantWordBudget: base.WordBudget,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			st := newBaseStaff(t, s)
			if tt.override != nil {
				if err := s.SetOrgStaffOverride(orgA, st.ID, tt.override.BigFive, tt.override.Brief, tt.override.Model, tt.override.WordBudget); err != nil {
					t.Fatal(err)
				}
				if tt.clearAfter {
					if err := s.ClearOrgStaffOverride(orgA, st.ID); err != nil {
						t.Fatal(err)
					}
				}
			}

			got := findStaff(t, mustStaffForOrg(t, s, tt.org), base.Slug)
			if !reflect.DeepEqual(got.BigFive, tt.wantBigFive) {
				t.Errorf("BigFive = %+v, want %+v", got.BigFive, tt.wantBigFive)
			}
			if got.Brief != tt.wantBrief {
				t.Errorf("Brief = %q, want %q", got.Brief, tt.wantBrief)
			}
			if got.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", got.Model, tt.wantModel)
			}
			if got.WordBudget != tt.wantWordBudget {
				t.Errorf("WordBudget = %d, want %d", got.WordBudget, tt.wantWordBudget)
			}
			// Non-overridable fields always come from the base row.
			if got.Name != base.Name || got.Locale != base.Locale || got.Age != base.Age {
				t.Errorf("non-overridable fields = %q/%q/%d, want %q/%q/%d",
					got.Name, got.Locale, got.Age, base.Name, base.Locale, base.Age)
			}
		})
	}
}

func TestSetOrgStaffOverrideUpsert(t *testing.T) {
	s := testStore(t)
	st := newBaseStaff(t, s)
	org := "org-a"

	if err := s.SetOrgStaffOverride(org, st.ID, nil, strPtr("first brief"), strPtr("first.glb"), nil); err != nil {
		t.Fatal(err)
	}
	// The second write replaces the row in place: a new brief, the model
	// cleared back to NULL so the base row wins again.
	if err := s.SetOrgStaffOverride(org, st.ID, nil, strPtr("second brief"), nil, nil); err != nil {
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
	if got.Model != st.Model {
		t.Errorf("Model = %q, want base %q after the second write cleared it", got.Model, st.Model)
	}
}

func TestClearOrgStaffOverride(t *testing.T) {
	s := testStore(t)
	st := newBaseStaff(t, s)
	org := "org-a"
	bf := Character{Openness: 0.2, Conscientiousness: 0.2, Extraversion: 0.2, Agreeableness: 0.2, Neuroticism: 0.2}
	if err := s.SetOrgStaffOverride(org, st.ID, &bf, strPtr("override"), strPtr("override.glb"), intPtr(7)); err != nil {
		t.Fatal(err)
	}

	if err := s.ClearOrgStaffOverride(org, st.ID); err != nil {
		t.Fatal(err)
	}
	got := findStaff(t, mustStaffForOrg(t, s, org), st.Slug)
	if !reflect.DeepEqual(got.BigFive, st.BigFive) || got.Brief != st.Brief || got.Model != st.Model || got.WordBudget != st.WordBudget {
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
		name      string
		slug      string
		wantName  string
		wantAge   int
		wantModel string
	}{
		{name: "male seed", slug: "staff-male-00", wantName: "Adam", wantAge: 35},
		{name: "female seed", slug: "staff-female-00", wantName: "Ewa", wantAge: 32, wantModel: "arianna.glb"},
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
			if got.Model != tt.wantModel {
				t.Errorf("%s model = %q, want %q", tt.slug, got.Model, tt.wantModel)
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
