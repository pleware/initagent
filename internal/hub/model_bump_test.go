package hub

import (
	"errors"
	"testing"
)

// boxVersions reads the config_version of every named box, failing the test
// on a store error.
func boxVersions(t *testing.T, s *Store, boxes ...*Box) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, b := range boxes {
		got, err := s.GetBox(b.ID)
		if err != nil || got == nil {
			t.Fatalf("GetBox(%s) = (%v, %v)", b.ID, got, err)
		}
		out[b.ID] = got.ConfigVersion
	}
	return out
}

// assertBumpDelta fails when any box's config_version moved by a delta
// other than the one named for it. want maps box id to the expected delta.
func assertBumpDelta(t *testing.T, before, after map[string]int64, want map[string]int64) {
	t.Helper()
	for id, got := range after {
		delta := got - before[id]
		if delta != want[id] {
			t.Errorf("box %s moved %d → %d (delta %d), want delta %d", id, before[id], got, delta, want[id])
		}
	}
}

func TestModelWriteBumpsAllBoxes(t *testing.T) {
	s := testStore(t)
	boxA := testBox(t, s, "model-bump-a")
	boxB := testBox(t, s, "model-bump-b")
	boxes := []*Box{boxA, boxB}
	both := map[string]int64{boxA.ID: 1, boxB.ID: 1}

	// Create: every box bumps.
	before := boxVersions(t, s, boxes...)
	m, err := s.CreateModel("bump-worker", "s", "", "bump-digest", "MIT", "worker")
	if err != nil {
		t.Fatal(err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), both)

	// Update of an existing pin: every box bumps.
	before = boxVersions(t, s, boxes...)
	if _, err := s.UpdateModel(m.ID, "s2", "q4_0", "bump-digest", "MIT", "worker"); err != nil {
		t.Fatal(err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), both)

	// Update of a missing pin is a no-op: no bump.
	before = boxVersions(t, s, boxes...)
	if _, err := s.UpdateModel("no-such-model", "s2", "", "", "MIT", "worker"); err != nil {
		t.Fatal(err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), map[string]int64{boxA.ID: 0, boxB.ID: 0})

	// Delete: every box bumps.
	before = boxVersions(t, s, boxes...)
	deleted, err := s.DeleteModel(m.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteModel = (%v, %v), want (true, nil)", deleted, err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), both)

	// Deleting a missing pin is a no-op: no bump.
	before = boxVersions(t, s, boxes...)
	deleted, err = s.DeleteModel(m.ID)
	if err != nil || deleted {
		t.Fatalf("second DeleteModel = (%v, %v), want (false, nil)", deleted, err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), map[string]int64{boxA.ID: 0, boxB.ID: 0})
}

func TestAssignmentWriteBumpsAllBoxes(t *testing.T) {
	s := testStore(t)
	boxA := testBox(t, s, "assignment-bump-a")
	boxB := testBox(t, s, "assignment-bump-b")
	boxes := []*Box{boxA, boxB}
	first := verifiedModel(t, s, "assignment-bump-1", "first-digest", "worker")
	second := verifiedModel(t, s, "assignment-bump-2", "second-digest", "worker")
	both := map[string]int64{boxA.ID: 1, boxB.ID: 1}
	none := map[string]int64{boxA.ID: 0, boxB.ID: 0}

	// Set: every box bumps. Box A and box B carry no override, so this is
	// the positive plan case — an assignment change reaches boxes that only
	// follow the factory.
	before := boxVersions(t, s, boxes...)
	if _, err := s.SetAssignment("worker", first.ID); err != nil {
		t.Fatal(err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), both)

	// Re-pin: every box bumps again.
	before = boxVersions(t, s, boxes...)
	if _, err := s.SetAssignment("worker", second.ID); err != nil {
		t.Fatal(err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), both)

	// Clear: every box bumps.
	before = boxVersions(t, s, boxes...)
	cleared, err := s.ClearAssignment("worker")
	if err != nil || !cleared {
		t.Fatalf("ClearAssignment = (%v, %v), want (true, nil)", cleared, err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), both)

	// A no-op clear bumps nothing.
	before = boxVersions(t, s, boxes...)
	cleared, err = s.ClearAssignment("worker")
	if err != nil || cleared {
		t.Fatalf("second ClearAssignment = (%v, %v), want (false, nil)", cleared, err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxes...), none)
}

func TestOverrideWriteBumpsOnlyThatBox(t *testing.T) {
	s := testStore(t)
	boxA := testBox(t, s, "override-bump-a")
	boxB := testBox(t, s, "override-bump-b")
	first := verifiedModel(t, s, "override-bump-1", "first-digest", "worker")
	second := verifiedModel(t, s, "override-bump-2", "second-digest", "worker")
	onlyA := map[string]int64{boxA.ID: 1, boxB.ID: 0}

	// Set on A: A bumps, B stays.
	before := boxVersions(t, s, boxA, boxB)
	if _, err := s.SetBoxModelOverride(boxA.ID, "worker", first.ID); err != nil {
		t.Fatal(err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxA, boxB), onlyA)

	// Re-pin on A: A bumps, B stays.
	before = boxVersions(t, s, boxA, boxB)
	if _, err := s.SetBoxModelOverride(boxA.ID, "worker", second.ID); err != nil {
		t.Fatal(err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxA, boxB), onlyA)

	// Clear on A: A bumps, B stays.
	before = boxVersions(t, s, boxA, boxB)
	cleared, err := s.ClearBoxModelOverride(boxA.ID, "worker")
	if err != nil || !cleared {
		t.Fatalf("ClearBoxModelOverride = (%v, %v), want (true, nil)", cleared, err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxA, boxB), onlyA)

	// A no-op clear bumps nothing.
	before = boxVersions(t, s, boxA, boxB)
	cleared, err = s.ClearBoxModelOverride(boxA.ID, "worker")
	if err != nil || cleared {
		t.Fatalf("second ClearBoxModelOverride = (%v, %v), want (false, nil)", cleared, err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, boxA, boxB), map[string]int64{boxA.ID: 0, boxB.ID: 0})
}

func TestRefusedModelWritesDoNotBump(t *testing.T) {
	s := testStore(t)
	box := testBox(t, s, "refusal-bump-box")
	unverified := verifiedModel(t, s, "refusal-unverified", "", "persona")

	// A refused assignment (empty digest) bumps nothing.
	before := boxVersions(t, s, box)
	if _, err := s.SetAssignment("persona", unverified.ID); !errors.Is(err, ErrModelUnverified) {
		t.Fatalf("SetAssignment = %v, want ErrModelUnverified", err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, box), map[string]int64{box.ID: 0})

	// A refused override (unknown box) bumps nothing.
	before = boxVersions(t, s, box)
	if _, err := s.SetBoxModelOverride("no-such-box", "persona", unverified.ID); !errors.Is(err, ErrUnknownBox) {
		t.Fatalf("SetBoxModelOverride = %v, want ErrUnknownBox", err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, box), map[string]int64{box.ID: 0})

	// A refused delete (in use) bumps nothing.
	verified := verifiedModel(t, s, "refusal-verified", "verified-digest", "persona")
	if _, err := s.SetAssignment("persona", verified.ID); err != nil {
		t.Fatal(err)
	}
	before = boxVersions(t, s, box)
	if _, err := s.DeleteModel(verified.ID); !errors.Is(err, ErrModelInUse) {
		t.Fatalf("DeleteModel = %v, want ErrModelInUse", err)
	}
	assertBumpDelta(t, before, boxVersions(t, s, box), map[string]int64{box.ID: 0})
}
