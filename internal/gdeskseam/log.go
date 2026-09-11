package gdeskseam

import (
	"fmt"
	"sync"
	"time"
)

// MaxLogEvents is how much history one stream keeps for replay. It has to
// outlive a reconnect — a lid closed mid-sentence and opened again — and
// nothing more: the log is a replay buffer, not a transcript. The store is the
// place a fact goes to be remembered.
const MaxLogEvents = 512

// Log is one stream's numbered facts.
//
// Numbering is the whole point. The glass detects a gap by arithmetic and asks
// for a replay, so the sequence must be handed out by exactly one place, and
// that place must also be what keeps the history — a counter in one object and
// a buffer in another can disagree, and the disagreement looks to the glass
// like data loss.
type Log struct {
	mu     sync.Mutex
	stream StreamID
	seq    int64
	kept   []Event
	max    int
	now    func() time.Time
	notify func(Event)
}

// LogConfig is how a stream's log is opened.
type LogConfig struct {
	// Stream is the glass's stream id. Numbering is per stream.
	Stream StreamID
	// Max is how many events to keep for replay. Zero means MaxLogEvents.
	Max int
	// Now defaults to time.Now.
	Now func() time.Time
	// Notify, when set, is called with each appended event **while the log is
	// held**, which is what keeps notifications in sequence order. It must not
	// block: hand the event to a buffer and let a reader that falls behind be
	// resynced, because a socket write under this lock stops the desk.
	Notify func(Event)
}

// NewLog opens a log for one stream.
func NewLog(cfg LogConfig) (*Log, error) {
	if cfg.Stream == "" {
		return nil, fmt.Errorf("%w: a log needs a stream", ErrSeam)
	}
	max := cfg.Max
	if max <= 0 {
		max = MaxLogEvents
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Log{stream: cfg.Stream, max: max, now: now, notify: cfg.Notify}, nil
}

// Stream is the stream this log numbers.
func (l *Log) Stream() StreamID { return l.stream }

// Seq is the sequence number of the last appended event, or zero before the
// first. The first event is 1, so zero is "nothing yet" rather than a fact.
func (l *Log) Seq() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seq
}

// Append numbers a fact, keeps it for replay and returns it.
//
// It cannot fail. A fact that already happened must not be lost because a
// window closed or a buffer filled, so refusing here would push a decision
// nobody can make back onto the conversation.
func (l *Log) Append(kind string, payload any, inReplyTo CommandID) Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	event := Event{
		Envelope: Envelope{
			V:         Version,
			Seq:       l.seq,
			At:        l.now().UTC().Format(time.RFC3339Nano),
			Stream:    l.stream,
			Kind:      kind,
			InReplyTo: inReplyTo,
		},
		Payload: payload,
	}
	l.kept = append(l.kept, event)
	if len(l.kept) > l.max {
		l.kept = append(l.kept[:0], l.kept[len(l.kept)-l.max:]...)
	}
	if l.notify != nil {
		l.notify(event)
	}
	return event
}

// Since answers a resync: every kept fact numbered fromSeq or later, oldest
// first.
//
// A request from before the oldest fact still held is answered with everything
// there is rather than with silence or an error. The glass takes the first
// event after a resync as its new baseline, so the worst case is a person
// missing what they already saw scroll past — while refusing would leave a
// desk that never speaks again.
func (l *Log) Since(fromSeq int64) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Event, 0, len(l.kept))
	for _, event := range l.kept {
		if event.Seq >= fromSeq {
			out = append(out, event)
		}
	}
	return out
}
