package hub

import (
	"errors"
	"testing"
)

// The generation-limits layer: set, clear, validate, resolve. The store is
// model_limits_store.go; the seed is model_limits_seed.go.

func TestSetLimitRoundTrip(t *testing.T) {
	s := testStoreNoAssignments(t)

	l, err := s.SetLimit(" Worker ", 256, 30)
	if err != nil {
		t.Fatal(err)
	}
	if l.Purpose != "worker" || l.MaxTokens != 256 || l.TimeoutSeconds != 30 {
		t.Errorf("limit = %+v, want canonical purpose and the values", l)
	}

	got, err := s.LimitFor("worker")
	if err != nil || got == nil {
		t.Fatalf("LimitFor = (%v, %v), want the row", got, err)
	}
	if got.MaxTokens != 256 || got.TimeoutSeconds != 30 {
		t.Errorf("LimitFor = %+v, want 256/30", got)
	}

	list, err := s.ListLimits()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Purpose != "worker" {
		t.Errorf("ListLimits = %+v, want the one worker row", list)
	}

	cleared, err := s.ClearLimit("worker")
	if err != nil || !cleared {
		t.Fatalf("ClearLimit = (%v, %v), want the row removed", cleared, err)
	}
	if l, err := s.LimitFor("worker"); err != nil || l != nil {
		t.Errorf("LimitFor after clear = (%+v, %v), want nil", l, err)
	}
	// Clearing a purpose with no row is (false, nil), not an error.
	if cleared, err := s.ClearLimit("worker"); err != nil || cleared {
		t.Errorf("second ClearLimit = (%v, %v), want (false, nil)", cleared, err)
	}
}

func TestSetLimitValidation(t *testing.T) {
	s := testStoreNoAssignments(t)

	if _, err := s.SetLimit("worker", -1, 0); !errors.Is(err, ErrLimitsNegative) {
		t.Errorf("negative maxTokens = %v, want ErrLimitsNegative", err)
	}
	if _, err := s.SetLimit("worker", 0, -1); !errors.Is(err, ErrLimitsNegative) {
		t.Errorf("negative timeoutSeconds = %v, want ErrLimitsNegative", err)
	}
	if _, err := s.SetLimit("worker", maxTokensCap+1, 0); !errors.Is(err, ErrMaxTokensTooLarge) {
		t.Errorf("over-cap maxTokens = %v, want ErrMaxTokensTooLarge", err)
	}
	if _, err := s.SetLimit("chat", 10, 0); err == nil {
		t.Errorf("unknown purpose = %v, want an error", err)
	}
	// The cap boundary itself is valid, and 0/0 is "no limit", not an error.
	if _, err := s.SetLimit("worker", maxTokensCap, 0); err != nil {
		t.Errorf("cap-boundary maxTokens = %v, want nil", err)
	}
	if _, err := s.SetLimit("worker", 0, 0); err != nil {
		t.Errorf("zeroed limits = %v, want nil (no limit)", err)
	}
}

func TestSetBoxLimitRoundTrip(t *testing.T) {
	s := testStoreNoAssignments(t)
	box := testBox(t, s, "limits-box")

	l, err := s.SetBoxLimit(box.ID, "worker", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if l.BoxID != box.ID || l.Purpose != "worker" || l.MaxTokens != 100 {
		t.Errorf("box limit = %+v, want the box/purpose/values", l)
	}

	// A box that does not exist is refused.
	if _, err := s.SetBoxLimit("box-00000000-0000-0000-0000-000000000000", "worker", 1, 0); !errors.Is(err, ErrUnknownBox) {
		t.Errorf("unknown box = %v, want ErrUnknownBox", err)
	}

	list, err := s.ListBoxLimits(box.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].MaxTokens != 100 {
		t.Errorf("ListBoxLimits = %+v, want the one worker row", list)
	}

	cleared, err := s.ClearBoxLimit(box.ID, "worker")
	if err != nil || !cleared {
		t.Fatalf("ClearBoxLimit = (%v, %v), want the row removed", cleared, err)
	}
	if list, err := s.ListBoxLimits(box.ID); err != nil || len(list) != 0 {
		t.Errorf("ListBoxLimits after clear = (%+v, %v), want empty", list, err)
	}
}

// A box resolves a limit the same way it resolves a model: the box's override
// wins over the factory limit, and a box without an override answers the
// factory.
func TestResolvedLimitsOverrideWinsOverFactory(t *testing.T) {
	s := testStoreNoAssignments(t)
	if _, err := s.SetLimit("worker", 512, 0); err != nil {
		t.Fatal(err)
	}

	overrideBox := testBox(t, s, "limits-override-box")
	if _, err := s.SetBoxLimit(overrideBox.ID, "worker", 8192, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.ResolvedLimits(overrideBox.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got["worker"].MaxTokens != 8192 {
		t.Errorf("override box worker limit = %+v, want 8192", got["worker"])
	}

	factoryBox := testBox(t, s, "limits-factory-box")
	got, err = s.ResolvedLimits(factoryBox.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got["worker"].MaxTokens != 512 {
		t.Errorf("factory box worker limit = %+v, want 512", got["worker"])
	}
}

// The factory limits are part of a fresh installation: the three generative
// slots resolve a runaway ceiling while nobody has touched the admin.
func TestOpenStoreSeedsFactoryLimits(t *testing.T) {
	s := testStore(t)

	marker, err := s.Setting(seedLimitsMarker)
	if err != nil {
		t.Fatal(err)
	}
	if marker == "" {
		t.Errorf("a fresh store carries no %s marker", seedLimitsMarker)
	}

	want := map[string]int{
		"persona":  512,
		"worker":   4096,
		"narrator": 512,
	}
	list, err := s.ListLimits()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(want) {
		t.Fatalf("limits = %+v, want the %d factory defaults", list, len(want))
	}
	for _, l := range list {
		if want[l.Purpose] != l.MaxTokens {
			t.Errorf("%s -> maxTokens %d, want %d", l.Purpose, l.MaxTokens, want[l.Purpose])
		}
		if l.TimeoutSeconds != 0 {
			t.Errorf("%s -> timeoutSeconds %d, want 0 (no factory timeout)", l.Purpose, l.TimeoutSeconds)
		}
	}
}

// An installation that already had a ceiling chosen keeps it: the seed fills a
// purpose, it does not take one over.
func TestEnsureSeedLimitsLeavesAnAdminChoiceAlone(t *testing.T) {
	s := testStore(t)
	if _, err := s.SetLimit("worker", 111, 0); err != nil {
		t.Fatal(err)
	}
	// The state an installation is in when it upgrades into this code: a limit
	// in place and no marker, so the seed pass runs.
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, seedLimitsMarker); err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureSeedLimits(); err != nil {
		t.Fatal(err)
	}
	l, err := s.LimitFor("worker")
	if err != nil {
		t.Fatal(err)
	}
	if l == nil || l.MaxTokens != 111 {
		t.Errorf("worker = %+v, want the admin's 111 left in place", l)
	}
}

// A cleared limit stays cleared: the marker is what makes the seed a one-time
// act, so a purpose the admin zeroed must not come back on the next restart.
func TestEnsureSeedLimitsDoesNotResurrectAClearedLimit(t *testing.T) {
	s := testStore(t)
	if removed, err := s.ClearLimit("persona"); err != nil || !removed {
		t.Fatalf("ClearLimit = (%v, %v), want the seeded row removed", removed, err)
	}

	if err := s.EnsureSeedLimits(); err != nil {
		t.Fatal(err)
	}
	if l, err := s.LimitFor("persona"); err != nil || l != nil {
		t.Errorf("persona = (%+v, %v), want it left cleared", l, err)
	}
}
