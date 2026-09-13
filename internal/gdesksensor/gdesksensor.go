// Package gdesksensor reads what a sensing process on this box saw.
//
// The contract is `os:desk-facts`, owned by the PWare OS umbrella and
// implemented in Python by `pware-os-facts`: a sensor writes one JSON object
// per line on stdout, flushed, and this connector reads the pipe as the parent
// that spawned it. Nothing listens — no socket and no port — so there is
// nothing to secure and a sensor dies with its parent. A named pipe becomes the
// right answer only when a sensor has to outlive a connector restart, and
// nothing needs that yet.
//
// A sensor fact is not a fact on the seam. Next door in gdeskseam an outbound
// event is a numbered fact, meaning something the glass may read twice safely;
// here a fact is one reading of a room. Both senses are load-bearing, so this
// package says "sensor fact" wherever the difference could matter.
//
// What a sensor may say is far narrower than what a camera can see. No
// identity, and no track id — a stable pseudonym across visits is
// re-identification. No inferred emotion, no age, no gender, and no frame ever
// crosses this boundary. The first ones are the product's decision; emotion
// inference in a workplace is a prohibited practice under the EU AI Act, so
// there the answer is not to do it carefully. This reader has no field to put
// any of it in, which is the only enforcement that survives a new sensor
// written by somebody who has not read the drafts.
//
// That narrowness is one edition's, not the connector's. PWare OS ships five
// editions over one engine (pware-os-workspace/drafts/08), and the Care edition
// reads a fall and a vital sign from the same camera. Widening this reader is
// therefore an edition decision arriving from outside this repository, and the
// narrow struct above is deliberately the thing that has to change for it — a
// reader that already had the fields would let a capability arrive with nobody
// deciding it should.
//
// The Assist edition is the other new one and it does not belong here at all: it
// acts on somebody's behalf, and this package reads. A command is not a sensor
// fact, so an actuator is a separate path with its own gate — do not grow one out
// of this reader because the connector happens to already supervise processes.
package gdesksensor

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrSensor is the class of every refusal in this package, so a caller can tell
// a sensor that misbehaved from a failure further in.
var ErrSensor = errors.New("desk sensor")

// Protocol is the envelope version of `os:desk-facts` this reader speaks.
//
// It moves for an envelope change, never for a new field and never for a new
// kind — the same discipline as gdeskseam.Version, and for the same reason: a
// consumer forced to choose between two live versions of one protocol has a
// diamond rather than a contract.
const Protocol = 2

// LegacyProtocol is what a line carrying no `p` means. Protocol 1 had no
// version field at all, so its absence is evidence rather than corruption, and
// reading it as 1 is what lets a mismatch be named instead of counted as an
// unknown kind.
const LegacyProtocol = 1

// The sensor fact kinds this reader knows about.
const (
	// KindAttendance is the whole vocabulary today. Hearing will bring a
	// second, and that is the moment to consider a union — not before.
	KindAttendance = "gdesk.attendance.changed"

	// KindPeople was protocol 1's attendance, and this reader never accepts it.
	// It is named so a version mismatch can say which string arrived rather
	// than reporting an unknown kind: `desk.` on a producer while the product
	// had already moved to `gdesk.` is the drift this project's own name
	// registry cites as its cautionary example.
	KindPeople = "desk.people.changed"
)

// Range is which band a face is in. The bands have a margin at the sensor, so a
// person standing exactly on the line does not flicker between them.
type Range string

const (
	RangeNear Range = "near"
	RangeFar  Range = "far"
)

// Gaze is which way a face is turned, and only the largest face carries it.
type Gaze string

const (
	GazeLeft   Gaze = "left"
	GazeCenter Gaze = "center"
	GazeRight  Gaze = "right"
)

// Face is one face: where it comes in the ordering, how far away it is, and
// whether it is turned this way.
type Face struct {
	// Rank orders faces by apparent size and does nothing else. It is not a
	// name and it does not survive the next reading — the same person is rank 1
	// and then rank 2 when somebody closer walks in. Treating it as an identity
	// is exactly the re-identification this contract exists to prevent.
	Rank int

	Range Range

	// Gaze is empty on every face but the first, because the sensor reports it
	// only for the largest.
	Gaze Gaze
}

// Attendance is who is at a glass at one moment: a snapshot, never a difference
// from the last one.
//
// "Somebody arrived" is the difference between two of these, and the consumer
// holds the previous one. A producer that sent the difference instead would
// make a dropped line read as a departure and a re-delivered one double a
// count; a snapshot is idempotent by construction, for the same reason
// `surface.patched` replaces a view rather than merging into it.
//
// Registered as `initagent.gdesk.attendance`, which mints nothing: a reading
// replaces the last one and nothing ever refers back to one, so there is no
// identifier to hand out — and keeping a series of them per person is the
// re-identification above.
type Attendance struct {
	// At is wall clock, for display and audit. Never for ordering, and never
	// for measuring how long somebody has been standing there: durations inside
	// a sensor are monotonic so that an NTP correction cannot un-greet a
	// visitor, and this field is the one a person has to recognise in a log.
	At time.Time

	// Sensor names the peripheral this reading came from — not the glass, and
	// not a process id. Two cameras on one box are two sensors and still one
	// fact stream, so a consumer groups by this field.
	//
	// A plain string, unlike the branded ids in gdeskseam: `camera-0` is a
	// device name, and the entity that would own it belongs to an `os` context
	// the registry does not have yet. Branding it now would put a shape on the
	// wire ahead of that decision.
	Sensor string

	// Source is which kind of sensing produced this — `vision` today, hearing
	// next. Not required: nothing here switches on it, and refusing a whole
	// reading over a label would trade a greeting for tidiness.
	Source string

	Total int
	Near  int
	Far   int

	// Faces may be shorter than Total, and that is not a broken reading: a face
	// is a person the sensor could see a face on, so somebody turned away is
	// counted without appearing here.
	Faces []Face
}

// Outcome is how one line came out.
//
// Four and not two, because the causes are different and an operator standing
// at a box in a customer's building is asking which one it is. A sensor ahead of
// this reader (OutcomeUnrecognised) needs an upgrade here; a sensor sending
// nonsense (OutcomeInvalid) is a bug there; a stream that is not this contract
// at all (OutcomeMalformed) is usually the wrong process on the pipe. It mirrors
// gdeskseam.InboundKind in the other direction, deliberately.
type Outcome string

const (
	// OutcomeFact is a kind we accept, carrying a reading we could read.
	OutcomeFact Outcome = "fact"
	// OutcomeUnrecognised is a well-formed line in a kind we do not accept.
	OutcomeUnrecognised Outcome = "unrecognised"
	// OutcomeInvalid is a kind we accept carrying a reading we could not read.
	OutcomeInvalid Outcome = "invalid"
	// OutcomeMalformed is not a sensor fact at all: the envelope itself is
	// wrong, or it belongs to another protocol version.
	OutcomeMalformed Outcome = "malformed"
)

// Reading is one line, classified.
//
// Parse never returns an error: every outcome is a fact about the line, and one
// bad line from a child process must not be able to stop a desk.
type Reading struct {
	Outcome Outcome

	// Protocol is what the line claimed, LegacyProtocol when it claimed
	// nothing, and zero when the line was not JSON so it claimed nothing at
	// all. Kept even on a refusal, because it is the first thing worth knowing
	// about a stream that stopped working.
	Protocol int

	// Kind is the wire string, when the line carried one.
	Kind string

	// Fact is set when Outcome is OutcomeFact, and is otherwise zero.
	Fact Attendance

	// Detail says what was wrong, for every outcome but OutcomeFact.
	Detail string
}

type wireEnvelope struct {
	P    *int   `json:"p"`
	Kind string `json:"kind"`
}

// The counts are pointers because an empty room is a real reading. Absent and
// zero are different facts here: `total: 0` says nobody is there, while a line
// with no `total` says the sensor is broken, and a plain int would report the
// second as the first.
type wireAttendance struct {
	At     string     `json:"at"`
	Source string     `json:"source"`
	Sensor string     `json:"sensor"`
	Total  *int       `json:"total"`
	Near   *int       `json:"near"`
	Far    *int       `json:"far"`
	Faces  []wireFace `json:"faces"`
}

type wireFace struct {
	Rank  *int   `json:"rank"`
	Range string `json:"range"`
	Gaze  string `json:"gaze"`
}

// Parse classifies one line from a sensor.
func Parse(line []byte) Reading {
	var env wireEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Reading{Outcome: OutcomeMalformed, Detail: "not JSON"}
	}
	protocol := LegacyProtocol
	if env.P != nil {
		protocol = *env.P
	}
	if env.Kind == "" {
		return Reading{Outcome: OutcomeMalformed, Protocol: protocol, Detail: "line names no kind"}
	}
	if protocol != Protocol {
		return Reading{
			Outcome:  OutcomeMalformed,
			Protocol: protocol,
			Kind:     env.Kind,
			Detail:   mismatch(protocol, env.Kind),
		}
	}
	switch env.Kind {
	case KindAttendance:
		fact, err := parseAttendance(line)
		if err != nil {
			return Reading{Outcome: OutcomeInvalid, Protocol: protocol, Kind: env.Kind, Detail: err.Error()}
		}
		return Reading{Outcome: OutcomeFact, Protocol: protocol, Kind: env.Kind, Fact: fact}
	default:
		return Reading{
			Outcome:  OutcomeUnrecognised,
			Protocol: protocol,
			Kind:     env.Kind,
			Detail:   fmt.Sprintf("kind %q is not one this reader accepts", env.Kind),
		}
	}
}

func mismatch(protocol int, kind string) string {
	detail := fmt.Sprintf("sensor speaks protocol %d, this reader speaks %d", protocol, Protocol)
	if kind == KindPeople {
		detail += fmt.Sprintf("; %s was protocol %d's attendance", KindPeople, LegacyProtocol)
	}
	return detail
}

func parseAttendance(line []byte) (Attendance, error) {
	var got wireAttendance
	if err := json.Unmarshal(line, &got); err != nil {
		return Attendance{}, fmt.Errorf("%w: attendance is not the shape of one: %w", ErrSensor, err)
	}
	at, err := time.Parse(time.RFC3339, got.At)
	if err != nil {
		return Attendance{}, fmt.Errorf("%w: %q is not a time", ErrSensor, got.At)
	}
	if got.Sensor == "" {
		return Attendance{}, fmt.Errorf("%w: reading names no sensor", ErrSensor)
	}
	if got.Total == nil || got.Near == nil || got.Far == nil {
		return Attendance{}, fmt.Errorf("%w: reading is missing a count", ErrSensor)
	}
	total, near, far := *got.Total, *got.Near, *got.Far
	if total < 0 || near < 0 || far < 0 {
		return Attendance{}, fmt.Errorf("%w: a count is negative (total %d, near %d, far %d)", ErrSensor, total, near, far)
	}
	// The bands may account for fewer people than the count, never for more.
	// Equality is what today's camera emits, and requiring it here would refuse
	// a legitimate future sensor that counts a body it cannot place in a band.
	if near+far > total {
		return Attendance{}, fmt.Errorf("%w: %d near and %d far is more than %d people", ErrSensor, near, far, total)
	}
	if len(got.Faces) > total {
		return Attendance{}, fmt.Errorf("%w: %d faces among %d people", ErrSensor, len(got.Faces), total)
	}
	faces := make([]Face, 0, len(got.Faces))
	for i, gf := range got.Faces {
		face, err := parseFace(gf)
		if err != nil {
			return Attendance{}, fmt.Errorf("%w: face %d: %w", ErrSensor, i+1, err)
		}
		faces = append(faces, face)
	}
	return Attendance{
		At:     at,
		Sensor: got.Sensor,
		Source: got.Source,
		Total:  total,
		Near:   near,
		Far:    far,
		Faces:  faces,
	}, nil
}

func parseFace(got wireFace) (Face, error) {
	if got.Rank == nil || *got.Rank < 1 {
		return Face{}, errors.New("rank is not a place in an ordering")
	}
	band := Range(got.Range)
	if band != RangeNear && band != RangeFar {
		return Face{}, fmt.Errorf("range %q is neither near nor far", got.Range)
	}
	look := Gaze(got.Gaze)
	if got.Gaze != "" && look != GazeLeft && look != GazeCenter && look != GazeRight {
		return Face{}, fmt.Errorf("gaze %q is not left, center or right", got.Gaze)
	}
	return Face{Rank: *got.Rank, Range: band, Gaze: look}, nil
}
