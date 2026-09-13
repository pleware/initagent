package gdesksensor

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// Sensors is what this desk currently believes about who is at it, and what it
// could not read.
//
// One of these per desk, not per sensor: the readings arrive on one pipe from
// each child and are grouped by the sensor that produced them, which is the
// grouping the contract asks a consumer to make.
type Sensors struct {
	mu       sync.Mutex
	entries  map[string]entry
	unusable map[Outcome]int
	last     string

	// since is time.Since in production and a fixed answer in a test. A field
	// rather than a call at the point of use, so that age can be asserted
	// without a sleep.
	since func(time.Time) time.Duration
}

type entry struct {
	fact     Attendance
	received time.Time
}

// New returns empty state: nobody seen, nothing skipped.
func New() *Sensors {
	return &Sensors{
		entries:  make(map[string]entry),
		unusable: make(map[Outcome]int),
		since:    time.Since,
	}
}

// Observe folds one line into what the desk knows.
//
// It takes a Reading rather than an Attendance on purpose: a line the reader
// could not use is still something an operator has to be able to see, and
// dropping it at the call site is how a camera that stopped working comes to
// look exactly like an empty room.
//
// An unusable line does not clear the last good reading — one bad line is not a
// room emptying — which is why Seen.Age exists to say whether that reading is
// still worth anything.
func (s *Sensors) Observe(r Reading) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Outcome == OutcomeFact {
		s.entries[r.Fact.Sensor] = entry{fact: r.Fact, received: time.Now()}
		return
	}
	s.unusable[r.Outcome]++
	s.last = r.Detail
}

// Seen is one sensor's last good reading, and how long ago it arrived.
type Seen struct {
	Attendance Attendance

	// Age is how long ago this reading arrived, on the monotonic clock.
	//
	// A sensor that died still describes a room full of people, and the only
	// thing that tells that apart from a room that really is full is how old the
	// reading is. Deliberately not computed from Attendance.At, which is wall
	// clock: an NTP correction there would make a fresh reading look stale, or a
	// stale one look fresh, and the second direction is the dangerous one.
	Age time.Duration
}

// State is everything this reader knows, as one value nothing can mutate behind
// its back.
type State struct {
	// Seen holds one entry per sensor that has produced a good reading, ordered
	// by sensor name so that two calls are comparable.
	Seen []Seen

	// The lines this reader could not use, counted since the desk started.
	// Three numbers and not one, because they call for three different repairs:
	// an upgrade here, a bug fixed at the sensor, and the wrong process on the
	// pipe.
	Unrecognised int
	Invalid      int
	Malformed    int

	// Last is the detail of the most recent unusable line, and empty while every
	// line has been readable.
	Last string
}

// State takes a copy of what the desk knows.
func (s *Sensors) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := State{
		Seen:         make([]Seen, 0, len(s.entries)),
		Unrecognised: s.unusable[OutcomeUnrecognised],
		Invalid:      s.unusable[OutcomeInvalid],
		Malformed:    s.unusable[OutcomeMalformed],
		Last:         s.last,
	}
	for _, e := range s.entries {
		out.Seen = append(out.Seen, Seen{Attendance: e.fact, Age: s.since(e.received)})
	}
	slices.SortFunc(out.Seen, func(a, b Seen) int {
		return cmp.Compare(a.Attendance.Sensor, b.Attendance.Sensor)
	})
	return out
}
