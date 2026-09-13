// Package gdeskseam carries desk facts to the glass over the seam the glass
// already speaks.
//
// The contract lives in the glass (`initagent-glass`,
// `app/src/seam/contract.ts`) because the glass owns no verbs and the
// connector owns `product:gdesk-seam`: one side writes the shapes down, the
// other mirrors them. Nothing here invents a message the glass has to guess
// at, and nothing here is a second implementation of the loop — the
// conversation is `internal/desk`, this is the wire it is heard over.
//
// Two directions and they are not symmetrical. A command arrives as untrusted
// text and is classified rather than trusted; an event leaves as a numbered
// fact that must survive being read twice.
package gdeskseam

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pleware/initagent/internal/gdesk"
)

// Version is the seam's envelope version (`SEAM_VERSION`). It moves when the
// envelope changes, not when a payload gains a field — a bump every release
// would make the glass refuse facts it could have read.
//
// It moved to 2 when the desk became the `gdesk` context (workspace draft 05),
// because a glass still saying `desk.utterance` would be answered
// `unrecognised` — the rollout rule working as designed, and on a monitor that
// reads as a sentence going nowhere. The bump does not turn that silence into a
// message on the glass (a caller on version 1 discards a version 2 event, so
// there is nowhere to put one); what it buys is a diagnosis: the operator ring
// names both versions instead of shrugging at an unknown verb.
const Version = 2

// ErrSeam is the class of every refusal in this package, so a caller can tell
// a malformed message from a failure further in.
var ErrSeam = errors.New("gdesk seam")

// CommandID and StreamID are the glass's branded ids arriving as text. They
// are opaque here: minting them is the glass's business, and parsing them into
// something structured would quietly make their format ours.
type (
	CommandID string
	StreamID  string
)

// SurfaceID and SlotID name a surface and one appendable region inside it.
type (
	SurfaceID string
	SlotID    string
)

// The command verbs this connector answers. The glass may send more (it has
// catalogue and offers verbs of its own); an unknown verb is `unrecognised`
// rather than an error, for the same reason the glass tolerates an unknown
// event kind — a producer that is ahead of a consumer must degrade, not break.
const (
	CommandUtterance = "gdesk.utterance"
	CommandResync    = "stream.resync"
)

// The event kinds this connector emits. A reply is a surface: it opens, text
// arrives in it a piece at a time, and it turns into the seam's one failure
// view if the answer does not arrive. There is deliberately no `desk.said`
// kind — the glass would count it as unrecognised, and a fact nobody can read
// is worse than one carried in the vocabulary that already exists.
const (
	EventSurfaceOpened     = "surface.opened"
	EventSurfacePatched    = "surface.patched"
	EventSurfaceAppended   = "surface.appended"
	EventSurfaceClosed     = "surface.closed"
	EventAttendanceChanged = "gdesk.attendance.changed"
)

// MaxChunkChars mirrors `SURFACE_LIMITS.chunkChars`. The glass rejects a
// larger chunk outright, so splitting here is the difference between a long
// sentence arriving and a turn going silent.
const MaxChunkChars = 8_192

// Header is what every command carries regardless of verb.
type Header struct {
	V      int       `json:"v"`
	CmdID  CommandID `json:"cmdId"`
	Stream StreamID  `json:"stream"`
	Kind   string    `json:"kind"`
}

// InboundKind is how a received message was classified. The four outcomes
// mirror the glass's own parser, in the other direction: a message we cannot
// place is reported rather than dropped, because a connector that silently
// ignores half a protocol is indistinguishable from a broken one.
type InboundKind string

const (
	// InboundCommand is a verb we answer, with a payload we could read.
	InboundCommand InboundKind = "command"
	// InboundUnrecognised is a well-formed command in a verb we do not answer.
	InboundUnrecognised InboundKind = "unrecognised"
	// InboundInvalid is a verb we answer carrying a payload we could not read.
	InboundInvalid InboundKind = "invalid"
	// InboundMalformed is not a command at all: the envelope itself is wrong.
	InboundMalformed InboundKind = "malformed"
)

// Inbound is one classified message.
//
// Which payload field is meaningful follows from Header.Kind, and only for
// InboundCommand. Two verbs do not pay for a sealed union — when the
// catalogue and offers verbs land here that is the moment to seal them, not
// before.
type Inbound struct {
	Kind   InboundKind
	Header Header
	// Spoken is set when Header.Kind is CommandUtterance.
	Spoken gdesk.Utterance
	// FromSeq is set when Header.Kind is CommandResync.
	FromSeq int64
	// Detail says what was wrong, for InboundInvalid and InboundMalformed.
	Detail string
}

type wireCommand struct {
	Header
	Payload json.RawMessage `json:"payload"`
}

type wireUtterance struct {
	UtteranceID string `json:"utteranceId"`
	Text        string `json:"text"`
	Source      string `json:"source"`
}

type wireResync struct {
	FromSeq *int64 `json:"fromSeq"`
}

// ParseCommand classifies one message from the glass. It never returns an
// error: every outcome is a fact about the message, and the caller decides
// which ones deserve an answer.
func ParseCommand(raw []byte) Inbound {
	var cmd wireCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return Inbound{Kind: InboundMalformed, Detail: "not JSON"}
	}
	if cmd.CmdID == "" || cmd.Stream == "" || cmd.Kind == "" {
		return Inbound{Kind: InboundMalformed, Detail: "bad command"}
	}
	if cmd.V != Version {
		// Still dropped rather than refused: a caller on another envelope
		// version cannot read our answer either, because the glass discards an
		// event whose `v` is not its own. So the mismatch is named here instead
		// of counted — the operator ring is the one place both sides can still
		// be compared.
		return Inbound{
			Kind:   InboundMalformed,
			Header: cmd.Header,
			Detail: fmt.Sprintf("seam version %d, this desk speaks %d", cmd.V, Version),
		}
	}
	switch cmd.Kind {
	case CommandUtterance:
		spoken, err := parseUtterance(cmd.Payload)
		if err != nil {
			return Inbound{Kind: InboundInvalid, Header: cmd.Header, Detail: err.Error()}
		}
		return Inbound{Kind: InboundCommand, Header: cmd.Header, Spoken: spoken}
	case CommandResync:
		from, err := parseResync(cmd.Payload)
		if err != nil {
			return Inbound{Kind: InboundInvalid, Header: cmd.Header, Detail: err.Error()}
		}
		return Inbound{Kind: InboundCommand, Header: cmd.Header, FromSeq: from}
	default:
		return Inbound{Kind: InboundUnrecognised, Header: cmd.Header}
	}
}

func parseUtterance(payload json.RawMessage) (gdesk.Utterance, error) {
	var got wireUtterance
	if err := json.Unmarshal(payload, &got); err != nil {
		return gdesk.Utterance{}, fmt.Errorf("%w: utterance payload is not an object", ErrSeam)
	}
	if got.UtteranceID == "" {
		return gdesk.Utterance{}, fmt.Errorf("%w: utterance has no id", ErrSeam)
	}
	if strings.TrimSpace(got.Text) == "" {
		return gdesk.Utterance{}, fmt.Errorf("%w: utterance has no words", ErrSeam)
	}
	if utf8.RuneCountInString(got.Text) > gdesk.MaxUtteranceChars {
		return gdesk.Utterance{}, fmt.Errorf("%w: utterance is longer than %d characters", ErrSeam, gdesk.MaxUtteranceChars)
	}
	source := gdesk.UtteranceSource(got.Source)
	if source != gdesk.UtteranceVoice && source != gdesk.UtteranceTyped {
		return gdesk.Utterance{}, fmt.Errorf("%w: utterance source %q is not spoken or typed", ErrSeam, got.Source)
	}
	return gdesk.Utterance{
		ID:     gdesk.UtteranceID(got.UtteranceID),
		Text:   got.Text,
		Source: source,
	}, nil
}

func parseResync(payload json.RawMessage) (int64, error) {
	var got wireResync
	if err := json.Unmarshal(payload, &got); err != nil {
		return 0, fmt.Errorf("%w: resync payload is not an object", ErrSeam)
	}
	if got.FromSeq == nil {
		return 0, fmt.Errorf("%w: resync has no starting point", ErrSeam)
	}
	if *got.FromSeq < 0 {
		return 0, fmt.Errorf("%w: resync starts before the first fact", ErrSeam)
	}
	return *got.FromSeq, nil
}

// Envelope is what every event carries. Seq is monotonic and gapless per
// stream; At is for display and audit, never for ordering.
type Envelope struct {
	V         int       `json:"v"`
	Seq       int64     `json:"seq"`
	At        string    `json:"at"`
	Stream    StreamID  `json:"stream"`
	Kind      string    `json:"kind"`
	InReplyTo CommandID `json:"inReplyTo,omitempty"`
}

// Event is one numbered fact on its way to the glass. The payload stays a Go
// value rather than bytes so the log holds facts rather than renderings; the
// encoding happens at the edge, where a failure has somewhere to go.
type Event struct {
	Envelope
	Payload any
}

type wireEvent struct {
	Envelope
	Payload any `json:"payload"`
}

// Encode renders the event as the glass reads it: the envelope's fields and a
// payload, in one object.
func (e Event) Encode() ([]byte, error) {
	raw, err := json.Marshal(wireEvent{Envelope: e.Envelope, Payload: e.Payload})
	if err != nil {
		return nil, fmt.Errorf("%w: event %s cannot be encoded: %w", ErrSeam, e.Kind, err)
	}
	return raw, nil
}

// SplitChunk cuts text into pieces the glass will accept. It splits on runes,
// because a chunk cut through a character arrives as a broken one.
func SplitChunk(text string) []string {
	if utf8.RuneCountInString(text) <= MaxChunkChars {
		return []string{text}
	}
	var (
		pieces []string
		piece  []rune
	)
	for _, r := range text {
		piece = append(piece, r)
		if len(piece) == MaxChunkChars {
			pieces = append(pieces, string(piece))
			piece = piece[:0]
		}
	}
	if len(piece) > 0 {
		pieces = append(pieces, string(piece))
	}
	return pieces
}
