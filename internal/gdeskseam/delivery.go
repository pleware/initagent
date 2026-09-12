package gdeskseam

import (
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/pleware/initagent/internal/gdesk"
)

// SpeechSlot is the appendable region a reply arrives in. One slot per surface:
// two people never share a surface, because one mouth is a hard rule and a
// shared surface would let two replies interleave inside it.
const SpeechSlot SlotID = "speech"

// DefaultUnclearPrompt is what the desk asks when it cannot tell who was
// addressed. It is a format string with one %s, filled with the names it is
// choosing between.
const DefaultUnclearPrompt = "Do kogo mówisz — %s?"

// RefusalLabel is who a refusal comes from. Neither staff member said it, so
// labelling it with a name would put words in her mouth. Not configurable
// until something needs it to be.
const RefusalLabel = "Biurko"

// maxAttributions bounds the utterance-to-command memory. An utterance older
// than the last few dozen no longer needs answering: the glass has either
// resolved it or asked a person about it long ago.
const maxAttributions = 64

// Delivery turns the conversation's facts into seam events. It implements
// gdesk.Facts, so the runner writes into it directly rather than through an
// adapter that could reorder or drop.
//
// Speech travels as a surface, not as a verb of its own: it opens, text
// arrives a piece at a time, and it becomes the seam's one failure view if the
// answer does not come. That is a vocabulary the glass already reads, so this
// connector can answer a real utterance without the glass shipping first.
type Delivery struct {
	mu      sync.Mutex
	log     *Log
	names   map[gdesk.StaffID]string
	unclear string
	replies map[gdesk.UtteranceID]CommandID
	spoken  []gdesk.UtteranceID
}

// DeliveryConfig is how delivery is wired.
type DeliveryConfig struct {
	// Log numbers the events. Required.
	Log *Log
	// Names is how each staff member is written when the desk shows or speaks
	// her name. Required.
	//
	// Deliberately not gdesk.Roster: the roster holds *folded* forms, lowercase
	// and stripped of diacritics, because its job is matching what a
	// transcriber returned. Labelling a surface from it would show a person the
	// spelling a matcher needed rather than her name, and title-casing it here
	// would be this package inventing typography for a name the hub owns.
	Names map[gdesk.StaffID]string
	// Unclear is the question asked when addressing is ambiguous. Empty means
	// DefaultUnclearPrompt. It must contain one %s.
	Unclear string
}

// validate checks everything about a delivery that does not depend on a log.
//
// It is separate so a desk serving several streams can refuse a bad
// configuration when it opens rather than when somebody first connects: a
// misspelled question is a mistake in a file, not an event in a conversation.
func (cfg DeliveryConfig) validate() error {
	if len(cfg.Names) == 0 {
		return fmt.Errorf("%w: delivery needs a name for each staff member", ErrSeam)
	}
	for staff, name := range cfg.Names {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w: staff %q has no name to be shown by", ErrSeam, staff)
		}
	}
	if cfg.Unclear != "" && !strings.Contains(cfg.Unclear, "%s") {
		return fmt.Errorf("%w: the unclear question has nowhere to put the names", ErrSeam)
	}
	return nil
}

// NewDelivery refuses a delivery that cannot say who is speaking.
func NewDelivery(cfg DeliveryConfig) (*Delivery, error) {
	if cfg.Log == nil {
		return nil, fmt.Errorf("%w: delivery needs a log", ErrSeam)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	unclear := cfg.Unclear
	if unclear == "" {
		unclear = DefaultUnclearPrompt
	}
	return &Delivery{
		log:     cfg.Log,
		names:   maps.Clone(cfg.Names),
		unclear: unclear,
		replies: make(map[gdesk.UtteranceID]CommandID),
	}, nil
}

// Attribute remembers which command an utterance arrived on, so the facts it
// produces can answer it. Without this the glass keeps the command in flight
// and, after a dropped connection, hands a person a question they cannot
// answer — a typed sentence is not repeatable, so nobody may retry it for
// them.
func (d *Delivery) Attribute(utterance gdesk.UtteranceID, cmd CommandID) {
	if utterance == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, seen := d.replies[utterance]; !seen {
		d.spoken = append(d.spoken, utterance)
	}
	d.replies[utterance] = cmd
	for len(d.spoken) > maxAttributions {
		delete(d.replies, d.spoken[0])
		d.spoken = d.spoken[1:]
	}
}

// Record carries one fact to the glass.
func (d *Delivery) Record(fact gdesk.Fact) {
	switch got := fact.(type) {
	case gdesk.TurnOpened:
		d.open(got)
	case gdesk.Said:
		d.say(got)
	case gdesk.AddressingUnclear:
		d.ask(got)
	case gdesk.Failed:
		d.failed(got)
	}
	// gdesk.FloorMoved stays here. Who holds the floor is the connector's
	// bookkeeping; the glass draws no floor, so an event for it would be one
	// the glass counts as unrecognised — noise on a channel whose gaps mean
	// data loss.
}

func (d *Delivery) open(fact gdesk.TurnOpened) {
	surface := surfaceOpened{Surface: surfaceSpec{
		ID:    surfaceForTurn(fact.Turn),
		Label: d.name(fact.Staff),
		View: panelView{
			Kind: "panel",
			Blocks: []any{textBlock{
				Kind:      "text",
				Slot:      SpeechSlot,
				Text:      "",
				Streaming: true,
			}},
		},
		Placement: placementSpec{At: "focus"},
		Attention: "primary",
		Chrome:    "bordered",
	}}
	d.log.Append(EventSurfaceOpened, surface, d.replyTo(fact.Utterance))
}

func (d *Delivery) say(fact gdesk.Said) {
	id := surfaceForTurn(fact.Turn)
	if fact.Final {
		d.log.Append(EventSurfaceAppended, surfaceAppended{
			ID:   id,
			Slot: SpeechSlot,
			Done: true,
		}, "")
		return
	}
	for _, chunk := range SplitChunk(fact.Text) {
		d.log.Append(EventSurfaceAppended, surfaceAppended{
			ID:    id,
			Slot:  SpeechSlot,
			Chunk: chunk,
		}, "")
	}
}

func (d *Delivery) ask(fact gdesk.AddressingUnclear) {
	names := make([]string, 0, len(fact.Candidates))
	for _, candidate := range fact.Candidates {
		names = append(names, d.name(candidate))
	}
	surface := surfaceOpened{Surface: surfaceSpec{
		ID:    SurfaceID("surface-" + string(fact.Utterance)),
		Label: strings.Join(names, " / "),
		View: panelView{
			Kind: "panel",
			Blocks: []any{textBlock{
				Kind: "text",
				Slot: SpeechSlot,
				Text: fmt.Sprintf(d.unclear, strings.Join(names, " czy ")),
			}},
		},
		Placement: placementSpec{At: "focus"},
		Attention: "primary",
		Chrome:    "bordered",
	}}
	d.log.Append(EventSurfaceOpened, surface, d.replyTo(fact.Utterance))
}

func (d *Delivery) failed(fact gdesk.Failed) {
	if fact.Turn == "" {
		// A failure before any turn opened has no surface to patch, and
		// patching "surface-" would name a surface the glass never saw. That
		// failure belongs to the command the words arrived on, and Refuse is
		// what answers it — with an id the glass can resolve.
		return
	}
	d.log.Append(EventSurfacePatched, surfacePatched{
		ID:   surfaceForTurn(fact.Turn),
		View: viewOf(fact.Failure),
	}, "")
}

// Refuse answers a command the desk will not act on.
//
// A command needs an answer even when it is nonsense, because the two the
// glass sends are not repeatable: an unanswered `gdesk.utterance` becomes
// unresolved and waits for a person. So a refusal opens its own surface,
// keyed on the command rather than on a turn that never existed.
func (d *Delivery) Refuse(cmd CommandID, failure gdesk.Failure) {
	if cmd == "" {
		// Nothing to answer, and nothing to key a surface on.
		return
	}
	d.log.Append(EventSurfaceOpened, surfaceOpened{Surface: surfaceSpec{
		ID:        SurfaceID("surface-" + string(cmd)),
		Label:     RefusalLabel,
		View:      viewOf(failure),
		Placement: placementSpec{At: "focus"},
		Attention: "primary",
		Chrome:    "bordered",
	}}, cmd)
}

func viewOf(failure gdesk.Failure) failedView {
	return failedView{
		Kind: "failed",
		Failure: failureSpec{
			Code:    string(failure.Code),
			Message: failure.Message,
		},
	}
}

func (d *Delivery) name(staff gdesk.StaffID) string {
	if name, ok := d.names[staff]; ok {
		return name
	}
	// A staff member the roster does not know cannot be labelled with a name,
	// and labelling her with nothing would produce a surface the glass rejects.
	return string(staff)
}

func (d *Delivery) replyTo(utterance gdesk.UtteranceID) CommandID {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.replies[utterance]
}

func surfaceForTurn(turn gdesk.TurnID) SurfaceID {
	return SurfaceID("surface-" + string(turn))
}

type textBlock struct {
	Kind      string `json:"kind"`
	Slot      SlotID `json:"slot"`
	Text      string `json:"text"`
	Streaming bool   `json:"streaming,omitempty"`
}

type panelView struct {
	Kind   string `json:"kind"`
	Blocks []any  `json:"blocks"`
}

type failureSpec struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type failedView struct {
	Kind    string      `json:"kind"`
	Failure failureSpec `json:"failure"`
}

type placementSpec struct {
	At string `json:"at"`
}

type surfaceSpec struct {
	ID        SurfaceID     `json:"id"`
	Label     string        `json:"label"`
	View      any           `json:"view"`
	Placement placementSpec `json:"placement"`
	Attention string        `json:"attention"`
	Chrome    string        `json:"chrome"`
}

type surfaceOpened struct {
	Surface surfaceSpec `json:"surface"`
}

type surfacePatched struct {
	ID   SurfaceID `json:"id"`
	View any       `json:"view"`
}

type surfaceAppended struct {
	ID    SurfaceID `json:"id"`
	Slot  SlotID    `json:"slot"`
	Chunk string    `json:"chunk"`
	Done  bool      `json:"done,omitempty"`
}
