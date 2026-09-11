package deskseam

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

// TraceLine is one operator line. Seq is this ring's count, not a stream seq.
type TraceLine struct {
	Seq   int64  `json:"seq"`
	At    string `json:"at"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// TraceDump is what GET /desk/logs returns.
type TraceDump struct {
	V     int         `json:"v"`
	Lines []TraceLine `json:"lines"`
}

// Trace is the connector's operator ring. Stdout still gets every line;
// this is the copy the glass's back-office pane polls.
type Trace struct {
	mu   sync.Mutex
	seq  int64
	kept []TraceLine
	max  int
	now  func() time.Time
}

// TraceConfig opens an operator ring. Every field is optional.
type TraceConfig struct {
	// Max is how many lines to keep. Zero means MaxTraceLines.
	Max int
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
	return &Trace{max: max, now: now}
}

// Record keeps one line. A nil receiver is a silent no-op so a test socket
// that never asked for a ring does not have to invent one.
func (t *Trace) Record(level, text string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	t.kept = append(t.kept, TraceLine{
		Seq:   t.seq,
		At:    t.now().UTC().Format(time.RFC3339Nano),
		Level: level,
		Text:  text,
	})
	if len(t.kept) > t.max {
		t.kept = append(t.kept[:0], t.kept[len(t.kept)-t.max:]...)
	}
}

// Dump is a copy of what is still held, oldest first.
func (t *Trace) Dump() TraceDump {
	if t == nil {
		return TraceDump{V: Version}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return TraceDump{V: Version, Lines: slices.Clone(t.kept)}
}

// noteTrace writes to the process log and, when a ring is present, keeps
// the same line for the operator pane.
func noteTrace(t *Trace, level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	t.Record(level, msg)
}
