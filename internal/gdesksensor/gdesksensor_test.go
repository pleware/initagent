package gdesksensor

import (
	"strings"
	"testing"
	"time"
)

// The exact bytes the producer writes, captured from `pware-os-facts`
// (facts.writer.as_line) rather than written here by hand.
//
// A hand-written fixture tests this parser against its author's memory of
// another repository. These came off the producer, so a field this reader has
// wrong fails here instead of on a box in a customer's building. Nothing
// refreshes them automatically — the producer is another language on another
// remote — so regenerating them is a deliberate edit, the same discipline the
// name registry's pinned copy already lives under.
const (
	fixtureCrowd = `{"at":"2026-03-08T20:00:00.000+00:00","faces":[{"gaze":"center","range":"near","rank":1},{"range":"far","rank":2}],"far":1,"kind":"gdesk.attendance.changed","near":1,"p":2,"sensor":"camera-0","source":"vision","total":2}`
	fixtureEmpty = `{"at":"2026-03-08T20:00:00.000+00:00","faces":[],"far":0,"kind":"gdesk.attendance.changed","near":0,"p":2,"sensor":"camera-0","source":"vision","total":0}`
)

var fixtureAt = time.Date(2026, 3, 8, 20, 0, 0, 0, time.UTC)

func TestParseReadsAProducedLine(t *testing.T) {
	got := Parse([]byte(fixtureCrowd))
	if got.Outcome != OutcomeFact {
		t.Fatalf("outcome = %q (%s), want %q", got.Outcome, got.Detail, OutcomeFact)
	}
	if got.Protocol != Protocol {
		t.Errorf("protocol = %d, want %d", got.Protocol, Protocol)
	}
	if got.Kind != KindAttendance {
		t.Errorf("kind = %q, want %q", got.Kind, KindAttendance)
	}
	fact := got.Fact
	if !fact.At.Equal(fixtureAt) {
		t.Errorf("at = %s, want %s", fact.At, fixtureAt)
	}
	if fact.Sensor != "camera-0" {
		t.Errorf("sensor = %q, want camera-0", fact.Sensor)
	}
	if fact.Source != "vision" {
		t.Errorf("source = %q, want vision", fact.Source)
	}
	if fact.Total != 2 || fact.Near != 1 || fact.Far != 1 {
		t.Errorf("counts = %d total, %d near, %d far; want 2, 1, 1", fact.Total, fact.Near, fact.Far)
	}
	if len(fact.Faces) != 2 {
		t.Fatalf("faces = %d, want 2", len(fact.Faces))
	}
	if want := (Face{Rank: 1, Range: RangeNear, Gaze: GazeCenter}); fact.Faces[0] != want {
		t.Errorf("first face = %+v, want %+v", fact.Faces[0], want)
	}
	// The second face carries no gaze, because the sensor reports it only for
	// the largest. An empty Gaze here is the contract, not a dropped field.
	if want := (Face{Rank: 2, Range: RangeFar}); fact.Faces[1] != want {
		t.Errorf("second face = %+v, want %+v", fact.Faces[1], want)
	}
}

// An empty room is a reading, not a missing one. This is the case a plain int
// would get wrong: `total: 0` and no `total` at all would arrive here as the
// same value, and the desk would treat a broken sensor as a quiet reception.
func TestParseReadsAnEmptyRoom(t *testing.T) {
	got := Parse([]byte(fixtureEmpty))
	if got.Outcome != OutcomeFact {
		t.Fatalf("outcome = %q (%s), want %q", got.Outcome, got.Detail, OutcomeFact)
	}
	if got.Fact.Total != 0 || len(got.Fact.Faces) != 0 {
		t.Errorf("empty room read as %d people and %d faces", got.Fact.Total, len(got.Fact.Faces))
	}
}

// attendance builds a line whose envelope is right, so a case can be wrong in
// exactly one way.
func attendance(fields string) string {
	return `{"p":2,"kind":"gdesk.attendance.changed","at":"2026-03-08T20:00:00.000+00:00","sensor":"camera-0",` + fields + `}`
}

func TestParseRefuses(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		outcome Outcome
		detail  string
	}{{
		name:    "a line that is not JSON",
		line:    `camera-0 sees two people`,
		outcome: OutcomeMalformed,
		detail:  "not JSON",
	}, {
		name:    "a fact with no kind",
		line:    `{"p":2,"total":0}`,
		outcome: OutcomeMalformed,
		detail:  "names no kind",
	}, {
		// The point of naming the old string: the operator learns the sensor is
		// behind, instead of being told a kind is unknown.
		name:    "protocol 1, which had no version field",
		line:    `{"kind":"desk.people.changed","total":1}`,
		outcome: OutcomeMalformed,
		detail:  "was protocol 1's attendance",
	}, {
		name:    "a sensor ahead of this reader",
		line:    `{"p":3,"kind":"gdesk.attendance.changed"}`,
		outcome: OutcomeMalformed,
		detail:  "sensor speaks protocol 3, this reader speaks 2",
	}, {
		name:    "a kind this reader does not accept",
		line:    `{"p":2,"kind":"gdesk.hearing.changed"}`,
		outcome: OutcomeUnrecognised,
		detail:  "not one this reader accepts",
	}, {
		name:    "a count that is a word",
		line:    attendance(`"total":"two","near":1,"far":1`),
		outcome: OutcomeInvalid,
		detail:  "not the shape of one",
	}, {
		name:    "a reading with no counts",
		line:    attendance(`"faces":[]`),
		outcome: OutcomeInvalid,
		detail:  "missing a count",
	}, {
		name:    "a negative count",
		line:    attendance(`"total":-1,"near":0,"far":0`),
		outcome: OutcomeInvalid,
		detail:  "negative",
	}, {
		name:    "bands accounting for more people than the count",
		line:    attendance(`"total":1,"near":1,"far":1`),
		outcome: OutcomeInvalid,
		detail:  "1 near and 1 far is more than 1 people",
	}, {
		name:    "more faces than people",
		line:    attendance(`"total":1,"near":1,"far":0,"faces":[{"rank":1,"range":"near"},{"rank":2,"range":"far"}]`),
		outcome: OutcomeInvalid,
		detail:  "2 faces among 1 people",
	}, {
		name:    "a face with no place in the ordering",
		line:    attendance(`"total":1,"near":1,"far":0,"faces":[{"rank":0,"range":"near"}]`),
		outcome: OutcomeInvalid,
		detail:  "face 1: rank is not a place in an ordering",
	}, {
		name:    "a band that is neither",
		line:    attendance(`"total":1,"near":1,"far":0,"faces":[{"rank":1,"range":"middling"}]`),
		outcome: OutcomeInvalid,
		detail:  `range "middling" is neither near nor far`,
	}, {
		name:    "a gaze outside the closed set",
		line:    attendance(`"total":1,"near":1,"far":0,"faces":[{"rank":1,"range":"near","gaze":"up"}]`),
		outcome: OutcomeInvalid,
		detail:  `gaze "up" is not left, center or right`,
	}, {
		name:    "a time nobody could audit",
		line:    `{"p":2,"kind":"gdesk.attendance.changed","at":"just now","sensor":"camera-0","total":0,"near":0,"far":0}`,
		outcome: OutcomeInvalid,
		detail:  `"just now" is not a time`,
	}, {
		name:    "a reading whose producer is unknown",
		line:    `{"p":2,"kind":"gdesk.attendance.changed","at":"2026-03-08T20:00:00.000+00:00","total":0,"near":0,"far":0}`,
		outcome: OutcomeInvalid,
		detail:  "names no sensor",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse([]byte(c.line))
			if got.Outcome != c.outcome {
				t.Errorf("outcome = %q, want %q", got.Outcome, c.outcome)
			}
			if !strings.Contains(got.Detail, c.detail) {
				t.Errorf("detail = %q, want it to contain %q", got.Detail, c.detail)
			}
			if got.Fact.Total != 0 || got.Fact.Sensor != "" {
				t.Errorf("a refused line still produced a reading: %+v", got.Fact)
			}
		})
	}
}

// These strings are the contract with a producer in another language on another
// remote. A rename here is a rename there, so it fails as a test rather than as
// a silent unreadable line on a box we sold.
func TestWireStringsAreTheContract(t *testing.T) {
	if Protocol != 2 {
		t.Errorf("Protocol = %d; moving it means moving facts/envelope.py", Protocol)
	}
	if LegacyProtocol != 1 {
		t.Errorf("LegacyProtocol = %d; it is what an unstamped line means and cannot change", LegacyProtocol)
	}
	if KindAttendance != "gdesk.attendance.changed" {
		t.Errorf("KindAttendance = %q; the registry entity is initagent.gdesk.attendance", KindAttendance)
	}
	if KindPeople != "desk.people.changed" {
		t.Errorf("KindPeople = %q; it is history and history does not get renamed", KindPeople)
	}
	if RangeNear != "near" || RangeFar != "far" {
		t.Errorf("bands = %q and %q", RangeNear, RangeFar)
	}
	if GazeLeft != "left" || GazeCenter != "center" || GazeRight != "right" {
		t.Errorf("gazes = %q, %q and %q", GazeLeft, GazeCenter, GazeRight)
	}
}
