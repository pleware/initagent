package gdesksensor

import (
	"testing"
	"time"
)

func factFrom(t *testing.T, sensor string, total int) Reading {
	t.Helper()
	return Reading{
		Outcome:  OutcomeFact,
		Protocol: Protocol,
		Kind:     KindAttendance,
		Fact:     Attendance{At: fixtureAt, Sensor: sensor, Source: "vision", Total: total},
	}
}

func TestStateStartsEmpty(t *testing.T) {
	got := New().State()
	if len(got.Seen) != 0 {
		t.Errorf("seen = %d sensors, want none", len(got.Seen))
	}
	if got.Last != "" {
		t.Errorf("last = %q, want empty", got.Last)
	}
}

// Two cameras on one box are two sensors and one fact stream, so the grouping is
// by sensor and the order is by name — a state that reorders itself between two
// calls is one nobody can diff.
func TestStateGroupsBySensorAndOrdersByName(t *testing.T) {
	s := New()
	s.Observe(factFrom(t, "camera-1", 1))
	s.Observe(factFrom(t, "camera-0", 2))

	got := s.State()
	if len(got.Seen) != 2 {
		t.Fatalf("seen = %d sensors, want 2", len(got.Seen))
	}
	if got.Seen[0].Attendance.Sensor != "camera-0" || got.Seen[1].Attendance.Sensor != "camera-1" {
		t.Errorf("order = %q then %q", got.Seen[0].Attendance.Sensor, got.Seen[1].Attendance.Sensor)
	}
	if got.Seen[0].Attendance.Total != 2 {
		t.Errorf("camera-0 read %d people, want 2", got.Seen[0].Attendance.Total)
	}
}

func TestAReadingReplacesTheLastOneFromThatSensor(t *testing.T) {
	s := New()
	s.Observe(factFrom(t, "camera-0", 2))
	s.Observe(factFrom(t, "camera-0", 0))

	got := s.State()
	if len(got.Seen) != 1 {
		t.Fatalf("seen = %d sensors, want 1", len(got.Seen))
	}
	// The room emptying is a whole new snapshot, not a decrement of the old one.
	if got.Seen[0].Attendance.Total != 0 {
		t.Errorf("total = %d, want the newer reading's 0", got.Seen[0].Attendance.Total)
	}
}

// Three counters, because they call for three different repairs. The most recent
// detail is kept so an operator has something to read besides a number.
func TestUnusableLinesAreCountedApart(t *testing.T) {
	s := New()
	s.Observe(Reading{Outcome: OutcomeUnrecognised, Detail: "kind we do not accept"})
	s.Observe(Reading{Outcome: OutcomeInvalid, Detail: "missing a count"})
	s.Observe(Reading{Outcome: OutcomeMalformed, Detail: "not JSON"})
	s.Observe(Reading{Outcome: OutcomeMalformed, Detail: "line names no kind"})

	got := s.State()
	if got.Unrecognised != 1 || got.Invalid != 1 || got.Malformed != 2 {
		t.Errorf("counts = %d unrecognised, %d invalid, %d malformed; want 1, 1, 2",
			got.Unrecognised, got.Invalid, got.Malformed)
	}
	if got.Last != "line names no kind" {
		t.Errorf("last = %q, want the most recent detail", got.Last)
	}
}

// A camera that stops sending leaves its last reading behind, and one bad line
// is not a room emptying. Age is the only thing that tells a busy reception from
// a dead sensor, which is why an unusable line must not clear a good reading.
func TestABadLineDoesNotEmptyTheRoom(t *testing.T) {
	s := New()
	s.Observe(factFrom(t, "camera-0", 2))
	s.Observe(Reading{Outcome: OutcomeMalformed, Detail: "not JSON"})

	got := s.State()
	if len(got.Seen) != 1 || got.Seen[0].Attendance.Total != 2 {
		t.Fatalf("seen = %+v, want the last good reading intact", got.Seen)
	}
	if got.Malformed != 1 {
		t.Errorf("malformed = %d, want 1", got.Malformed)
	}
}

func TestAgeIsMeasuredOnTheMonotonicClock(t *testing.T) {
	s := New()
	// Standing in for time.Since so the age of a reading can be asserted
	// without sleeping through it.
	s.since = func(time.Time) time.Duration { return 90 * time.Second }
	s.Observe(factFrom(t, "camera-0", 1))

	got := s.State()
	if len(got.Seen) != 1 {
		t.Fatalf("seen = %d sensors, want 1", len(got.Seen))
	}
	if got.Seen[0].Age != 90*time.Second {
		t.Errorf("age = %s, want 1m30s", got.Seen[0].Age)
	}
}
