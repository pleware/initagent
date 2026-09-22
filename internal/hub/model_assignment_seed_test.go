package hub

import "testing"

// testStoreNoAssignments opens a fresh store and clears its factory defaults.
// It is the starting point for the tests that examine the assignment layer's
// own behaviour — set, clear, validate, resolve, serialize: the seeded persona
// would otherwise be a second row in every assertion about that layer, and
// rewriting those assertions around it would hide what they are testing. The
// marker stays, which is exactly the state of an installation whose admin has
// cleared everything: the seed must not fill those purposes again.
func testStoreNoAssignments(t *testing.T) *Store {
	t.Helper()
	s := testStore(t)
	if _, err := s.db.Exec(`DELETE FROM model_assignments`); err != nil {
		t.Fatal(err)
	}
	return s
}

// The factory default is part of a fresh installation: a store opened for the
// first time answers persona with Qwen3.6 while nobody has touched the admin.
func TestOpenStoreSeedsFactoryPersona(t *testing.T) {
	s := testStore(t)

	marker, err := s.Setting(seedAssignmentsMarker)
	if err != nil {
		t.Fatal(err)
	}
	if marker == "" {
		t.Errorf("a fresh store carries no %s marker", seedAssignmentsMarker)
	}

	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Purpose != "persona" || list[0].ModelID != "qwen3.6-35b-a3b-q4_k_m" {
		t.Errorf("assignments = %+v, want persona -> qwen3.6-35b-a3b-q4_k_m alone", list)
	}
}

// The box's side of the same fact: a new box's resolved roster carries the
// factory model, not merely an assignment row.
func TestResolvedModelsAnswersTheFactoryPersona(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("fresh-box", "Fresh", "", "")
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ResolvedModels(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got["persona"]
	if !ok {
		t.Fatalf("a fresh box resolves no persona: %+v", got)
	}
	if m.ID != "qwen3.6-35b-a3b-q4_k_m" {
		t.Errorf("persona resolves to %q, want the factory pin", m.ID)
	}
}

// An installation that already had a persona chosen — the live hub — keeps it.
// The seed fills a purpose; it does not take one over.
func TestEnsureSeedAssignmentsLeavesAnAdminChoiceAlone(t *testing.T) {
	s := testStore(t)
	if _, err := s.SetAssignment("persona", "qwen3.5-4b-q4_k_m"); err != nil {
		t.Fatal(err)
	}
	// The state an installation is in when it upgrades into this code: an
	// assignment in place and no marker, so the seed pass runs.
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, seedAssignmentsMarker); err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureSeedAssignments(); err != nil {
		t.Fatal(err)
	}
	a, err := s.AssignmentFor("persona")
	if err != nil {
		t.Fatal(err)
	}
	if a == nil || a.ModelID != "qwen3.5-4b-q4_k_m" {
		t.Errorf("persona = %+v, want the admin's qwen3.5-4b-q4_k_m left in place", a)
	}
}

// A cleared purpose stays cleared. Bringing it back on the next restart would
// silently undo the admin — the failure this seed is shaped to avoid.
func TestEnsureSeedAssignmentsDoesNotResurrectAClearedPurpose(t *testing.T) {
	s := testStore(t)
	if removed, err := s.ClearAssignment("persona"); err != nil || !removed {
		t.Fatalf("ClearAssignment = (%v, %v), want the seeded row removed", removed, err)
	}

	if err := s.EnsureSeedAssignments(); err != nil {
		t.Fatal(err)
	}
	if a, err := s.AssignmentFor("persona"); err != nil || a != nil {
		t.Errorf("persona = (%+v, %v), want it left cleared", a, err)
	}
}

// The seeder bumps the boxes when it assigns, and then never again: a second
// pass over an applied installation writes nothing and bumps nothing.
func TestEnsureSeedAssignmentsBumpsBoxesOnce(t *testing.T) {
	s := testStore(t)
	// Simulate the live installation: no assignments, no marker.
	if _, err := s.db.Exec(`DELETE FROM model_assignments`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, seedAssignmentsMarker); err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("seed-box", "Seed", "", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureSeedAssignments(); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox = (%v, %v), want the box", got, err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version = %d, want 2 (one seed bump)", got.ConfigVersion)
	}

	if err := s.EnsureSeedAssignments(); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox after second run = (%v, %v)", got, err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version after second run = %d, want 2 (no re-bump)", got.ConfigVersion)
	}
}

// A pin the factory cannot vouch for is skipped rather than fatal, and the
// installation still counts as seeded: the fixed decision, not a retry on
// every open.
func TestEnsureSeedAssignmentsSkipsAnUnverifiedPin(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec(`DELETE FROM model_assignments`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, seedAssignmentsMarker); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE models SET digest = '' WHERE id = 'qwen3.6-35b-a3b-q4_k_m'`); err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureSeedAssignments(); err != nil {
		t.Fatalf("an unverified factory pin must not keep the hub from opening: %v", err)
	}
	if a, err := s.AssignmentFor("persona"); err != nil || a != nil {
		t.Errorf("persona = (%+v, %v), want no assignment for an unverified pin", a, err)
	}
	marker, err := s.Setting(seedAssignmentsMarker)
	if err != nil {
		t.Fatal(err)
	}
	if marker == "" {
		t.Errorf("marker = %q, want the installation marked as seeded anyway", marker)
	}
}
