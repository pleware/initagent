package gdeskseam

import (
	"fmt"
	"sync"
	"time"

	"github.com/pleware/initagent/internal/gdesk"
)

// Views is one desk as several people see it.
//
// A stream is one connection's numbered view; a conversation is the person
// (docs/DESK-SCOPES.md). Each stream keeps its own log, so each device detects
// its own gaps and resumes on its own, and a fact reaches only the streams
// bound to the conversation it belongs to.
//
// Filtering one shared log per reader instead would break the seam's own
// contract: the glass finds a gap by arithmetic, so a reader that is skipped
// past somebody else's events sees 1, 2, 5 and asks for a replay of facts that
// were never hers to see.
type Views struct {
	mu     sync.Mutex
	byName map[StreamID]*View
	cfg    ViewsConfig
}

// View is one stream: its log, its readers, and the delivery that writes into
// it. It is handed to the socket serving that connection.
type View struct {
	conv     gdesk.ConversationID
	log      *Log
	feed     *Feed
	delivery *Delivery
}

// ViewsConfig is what every stream's log and delivery is built from.
type ViewsConfig struct {
	// Names is how each staff member is written. Required, see DeliveryConfig.
	Names map[gdesk.StaffID]string
	// Unclear is the question asked when addressing is ambiguous. Empty means
	// DefaultUnclearPrompt.
	Unclear string
	// Max is how many events each stream keeps for replay. Zero means
	// MaxLogEvents. It is per stream because replay is.
	Max int
	// Now defaults to time.Now.
	Now func() time.Time
}

// NewViews opens a desk with nobody looking at it yet.
//
// Everything a stream's delivery needs is checked here, so a desk that could
// not answer refuses at startup rather than on somebody's first sentence.
func NewViews(cfg ViewsConfig) (*Views, error) {
	if err := (DeliveryConfig{Names: cfg.Names, Unclear: cfg.Unclear}).validate(); err != nil {
		return nil, err
	}
	return &Views{byName: make(map[StreamID]*View), cfg: cfg}, nil
}

// Bind gives a connection its view.
//
// Reconnecting with the same stream returns the same view, history included —
// that is what makes a dropped connection a gap to be replayed rather than a
// desk that starts over. Binding a stream to a second conversation is refused:
// a stream that changed hands would hand its history to the wrong person.
func (v *Views) Bind(stream StreamID, conv gdesk.ConversationID) (*View, error) {
	if stream == "" {
		return nil, fmt.Errorf("%w: a connection needs a stream", ErrSeam)
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if existing, ok := v.byName[stream]; ok {
		if existing.conv != conv {
			return nil, fmt.Errorf("%w: stream %q already belongs to another conversation", ErrSeam, stream)
		}
		return existing, nil
	}

	view, err := v.open(stream, conv)
	if err != nil {
		return nil, err
	}
	v.byName[stream] = view
	return view, nil
}

// open builds one stream's log, fan-out and delivery.
func (v *Views) open(stream StreamID, conv gdesk.ConversationID) (*View, error) {
	feed := NewFeed()
	log, err := NewLog(LogConfig{
		Stream: stream,
		Max:    v.cfg.Max,
		Now:    v.cfg.Now,
		Notify: feed.Publish,
	})
	if err != nil {
		return nil, err
	}
	delivery, err := NewDelivery(DeliveryConfig{
		Log:     log,
		Names:   v.cfg.Names,
		Unclear: v.cfg.Unclear,
	})
	if err != nil {
		return nil, err
	}
	return &View{conv: conv, log: log, feed: feed, delivery: delivery}, nil
}

// Record carries one fact to every stream looking at that conversation.
//
// It implements gdesk.Facts, so the runner writes here and this package decides
// who may see it. A fact for a conversation no stream is bound to goes nowhere:
// nobody has ever connected as that person, so there is no log for it to be
// numbered into.
//
// Attribution is deliberately not fanned out the same way. An utterance arrives
// on one connection, so only that stream may answer a command — naming it on
// her other device would tell the glass a command it never sent came back.
func (v *Views) Record(conv gdesk.ConversationID, fact gdesk.Fact) {
	for _, view := range v.watching(conv) {
		view.delivery.Record(fact)
	}
}

// watching snapshots the streams bound to one conversation, so a fact is never
// delivered under this lock.
func (v *Views) watching(conv gdesk.ConversationID) []*View {
	v.mu.Lock()
	defer v.mu.Unlock()

	var out []*View
	for _, view := range v.byName {
		if view.conv == conv {
			out = append(out, view)
		}
	}
	return out
}

// Streams counts the bound connections.
func (v *Views) Streams() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.byName)
}

// Conversation is whose view this is.
func (v *View) Conversation() gdesk.ConversationID { return v.conv }

// Log numbers this stream's events.
func (v *View) Log() *Log { return v.log }

// Feed hands this stream's events to its open connections.
func (v *View) Feed() *Feed { return v.feed }

// Delivery turns facts into this stream's events.
func (v *View) Delivery() *Delivery { return v.delivery }
