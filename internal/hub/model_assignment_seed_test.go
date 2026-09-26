package hub

import "testing"

// testStoreNoAssignments opens a fresh store and clears its factory defaults —
// the assignments and the generation limits — so a test examines the layer's
// own behaviour with no seeded row beside it. The seeded persona and worker
// would otherwise be a second row in every assertion about that layer, and
// rewriting those assertions around it would hide what they are testing. The
// markers stay, which is exactly the state of an installation whose admin has
// cleared everything: the seeds must not fill those purposes again.
func testStoreNoAssignments(t *testing.T) *Store {
	t.Helper()
	s := testStore(t)
	if _, err := s.db.Exec(`DELETE FROM model_assignments`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM model_limits`); err != nil {
		t.Fatal(err)
	}
	return s
}

// The factory defaults are part of a fresh installation: a store opened for the
// first time resolves all six purposes while nobody has touched the admin.
func TestOpenStoreSeedsFactoryAssignments(t *testing.T) {
	s := testStore(t)

	marker, err := s.Setting(seedAssignmentsMarker)
	if err != nil {
		t.Fatal(err)
	}
	if marker == "" {
		t.Errorf("a fresh store carries no %s marker", seedAssignmentsMarker)
	}

	// The expected pairs are written out rather than read back from
	// factoryAssignments: changing the factory set is a decision about what
	// every new box resolves, so it has to be taken here as well as there.
	want := map[string]string{
		"persona":   "qwen3.6-35b-a3b-q4_k_m",
		"worker":    "kat-coder-v2.5-dev-q4_k_m",
		"embedding": "bge-m3",
		"stt":       "faster-whisper-medium",
		"vad":       "silero-vad",
		"tts":       "pl_PL-gosia-medium",
	}

	list, err := s.ListAssignments()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(want) {
		t.Fatalf("assignments = %+v, want the %d factory defaults", list, len(want))
	}
	for _, a := range list {
		if want[a.Purpose] != a.ModelID {
			t.Errorf("%s -> %q, want %q", a.Purpose, a.ModelID, want[a.Purpose])
		}
	}
}

// Every default the factory ships has to name a pin the store can actually
// assign. A pin without a digest is skipped by the seed, so a typo in the
// factory set would read as an unassigned purpose rather than as an error —
// this is the test that turns that silence into a failure.
func TestFactoryAssignmentsNameVerifiedPins(t *testing.T) {
	s := testStore(t)

	if len(factoryAssignments()) == 0 {
		t.Fatal("the factory names no defaults at all")
	}
	for _, want := range factoryAssignments() {
		purpose, err := ParsePurpose(want.purpose)
		if err != nil {
			t.Errorf("factory default %q: %v", want.purpose, err)
			continue
		}
		m, err := s.GetModel(want.modelID)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case m == nil:
			t.Errorf("factory default %s -> %q: no such seeded pin", purpose, want.modelID)
		case m.Purpose != purpose:
			t.Errorf("factory default %s -> %q: that pin's purpose is %s", purpose, want.modelID, m.Purpose)
		case m.Digest == "":
			t.Errorf("factory default %s -> %q: the pin carries no digest, so it is not assignable", purpose, want.modelID)
		}
	}
}

// The box's side of the same fact: a new box's resolved roster carries all six
// factory models, not merely the assignment rows.
func TestResolvedModelsAnswersTheFactoryDefaults(t *testing.T) {
	s := testStore(t)
	box, err := s.CreateBox("fresh-box", "Fresh", "", "")
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ResolvedModels(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"persona":   "qwen3.6-35b-a3b-q4_k_m",
		"worker":    "kat-coder-v2.5-dev-q4_k_m",
		"embedding": "bge-m3",
		"stt":       "faster-whisper-medium",
		"vad":       "silero-vad",
		"tts":       "pl_PL-gosia-medium",
	}
	if len(got) != len(want) {
		t.Errorf("a fresh box resolves %d purposes, want %d: %+v", len(got), len(want), got)
	}
	for purpose, wantID := range want {
		m, ok := got[purpose]
		if !ok {
			t.Errorf("a fresh box resolves no %s", purpose)
			continue
		}
		if m.ID != wantID {
			t.Errorf("%s resolves to %q, want %q", purpose, m.ID, wantID)
		}
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
