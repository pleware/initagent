package gdeskseam

import (
	"fmt"
	"log"
	"slices"
	"sync"
	"time"
)

// MaxTraceLines is how much operator history one desk keeps. It is a
// scrolling pane, not an audit: the conversation log numbers facts for the
// glass, and this ring is what a developer reads when a typed sentence
// appears to vanish.
const MaxTraceLines = 80

// MaxTraceAge is how far back the console may look. A line count alone is the
// wrong bound for a desk nobody is talking to: a handful of lines an hour
// keeps an hour of history on screen, so what a person sees when they finally
// look is mostly not about what they just did. Two bounds together say it
// properly — the last ten minutes, and never more than MaxTraceLines of them.
//
// This is a scrolling pane, and forgetting is the point. Anything that has to
// survive belongs in the process log, which still gets every line.
const MaxTraceAge = 10 * time.Minute

// TraceLine is one operator line. Seq is this ring's count, not a stream seq.
type TraceLine struct {
	Seq   int64  `json:"seq"`
	At    string `json:"at"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// TraceDump is what GET /gdesk/logs returns: the ring, plus who is on the seam
// from a browser.
//
// Neither Peers nor Services is the ring's business, and the ring fills
// neither: the listener adds them on the way out, because it is the half of the
// desk that saw the handshake and the half the assembly hands its inventory to.
// They are here rather than on routes of their own for the reason given at
// ServeLogs.
type TraceDump struct {
	V        int         `json:"v"`
	Lines    []TraceLine `json:"lines"`
	Peers    []Peer      `json:"peers,omitempty"`
	Services []Service   `json:"services,omitempty"`
}

// tracked is one kept line plus the instant it was recorded. The instant is
// held as a time.Time rather than re-parsed from TraceLine.At because the age
// bound is checked on every read, and a ring that parses its whole history to
// answer one poll is a ring that gets sampled less often than it should be.
type tracked struct {
	line TraceLine
	at   time.Time
}

// Trace is the connector's operator ring. Stdout still gets every line;
// this is the copy the operator's console polls.
type Trace struct {
	mu   sync.Mutex
	seq  int64
	kept []tracked
	max  int
	age  time.Duration
	now  func() time.Time
}

// TraceConfig opens an operator ring. Every field is optional.
type TraceConfig struct {
	// Max is how many lines to keep. Zero means MaxTraceLines.
	Max int
	// MaxAge is how old a line may be. Zero means MaxTraceAge; a negative
	// value keeps every line, which is for a test that owns the clock.
	MaxAge time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
}

// NewTrace opens an operator ring.
func NewTrace(cfg TraceConfig) *Trace {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	max := cfg.Max
	if max <= 0 {
		max = MaxTraceLines
	}
	age := cfg.MaxAge
	if age == 0 {
		age = MaxTraceAge
	}
	return &Trace{max: max, age: age, now: now}
}

// Record keeps one line. A nil receiver is a silent no-op so a test socket
// that never asked for a ring does not have to invent one.
func (t *Trace) Record(level, text string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	at := t.now().UTC()
	t.seq++
	t.kept = append(t.kept, tracked{
		line: TraceLine{
			Seq:   t.seq,
			At:    at.Format(time.RFC3339Nano),
			Level: level,
			Text:  text,
		},
		at: at,
	})
	t.forget(at)
}

// Dump is a copy of what is still held, oldest first.
func (t *Trace) Dump() TraceDump {
	if t == nil {
		return TraceDump{V: Version}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	// Forgetting on the way out as well as on the way in, because the age
	// bound has to hold on a quiet desk too. A ring that only prunes when a
	// line arrives would show a stale hour precisely when nothing is
	// happening — the moment somebody goes looking for why.
	t.forget(t.now().UTC())
	lines := make([]TraceLine, 0, len(t.kept))
	for _, k := range t.kept {
		lines = append(lines, k.line)
	}
	return TraceDump{V: Version, Lines: lines}
}

// forget drops what is too old, then what is over the count. Caller holds mu.
func (t *Trace) forget(now time.Time) {
	if t.age > 0 {
		cutoff := now.Add(-t.age)
		// Recorded in order, so the first line still young ends the discard.
		// A clock that stepped backwards leaves everything in place rather
		// than emptying the column, which is the friendlier of the two wrong
		// answers.
		keep := 0
		for keep < len(t.kept) && t.kept[keep].at.Before(cutoff) {
			keep++
		}
		if keep > 0 {
			t.kept = slices.Delete(t.kept, 0, keep)
		}
	}
	if len(t.kept) > t.max {
		t.kept = slices.Delete(t.kept, 0, len(t.kept)-t.max)
	}
}

// noteTrace writes to the process log and, when a ring is present, keeps
// the same line for the operator console.
func noteTrace(t *Trace, level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	t.Record(level, msg)
}
