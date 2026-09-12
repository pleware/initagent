package gdeskseam

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/pleware/initagent/internal/gdesk"
)

// Answerer answers what somebody said. *gdesk.Runner is one.
//
// Named here rather than imported as the concrete runner so the socket can be
// tested against a stand-in that returns immediately: a real turn waits on a
// provider, and a read loop is not the place to discover that.
type Answerer interface {
	Answer(ctx context.Context, spoken gdesk.Utterance) error
}

// Intent is what one inbound frame asks the connector to do.
//
// Returned rather than acted on, because the two actions have opposite timing:
// a replay is a handful of writes on this connection and answering waits on a
// provider for seconds. Deciding is pure and testable; the connection loop
// chooses what to run inline and what to run alongside.
type Intent struct {
	// Answer is a turn to run, or nil.
	Answer *gdesk.Utterance

	// Replay goes to this connection only. Every other event reaches every
	// connection through the feed.
	Replay []Event
}

// SocketConfig is what a Socket needs. Every field is required.
type SocketConfig struct {
	// View is this connection's stream: what numbers its events, what a
	// resync replays from, and whose conversation it belongs to.
	//
	// Deliberately the whole view rather than a log and a delivery separately.
	// The conversation has to come from the connection, and passing the parts
	// would leave a caller free to pair one person's log with another person's
	// conversation.
	View *View
	// Answerer runs a turn.
	Answerer Answerer
	// Trace is the operator ring. Nil is fine.
	Trace *Trace
}

// Socket is the seam's inbound half: it reads commands and decides.
//
// It holds no connection. What it decides is the same whether the frame came
// from a WebSocket, a test, or whatever replaces the transport, which is the
// only reason the decision can be tested without one.
type Socket struct {
	view     *View
	answerer Answerer
	trace    *Trace
}

// NewSocket refuses a socket that cannot answer or cannot replay.
func NewSocket(cfg SocketConfig) (*Socket, error) {
	switch {
	case cfg.View == nil:
		return nil, fmt.Errorf("%w: socket needs the stream it serves", ErrSeam)
	case cfg.Answerer == nil:
		return nil, fmt.Errorf("%w: socket needs somebody to answer", ErrSeam)
	}
	return &Socket{view: cfg.View, answerer: cfg.Answerer, trace: cfg.Trace}, nil
}

// Stream is the stream this socket numbers events on. A connection sends it
// back on every command, and a command naming another stream is not ours.
func (s *Socket) Stream() StreamID {
	return s.view.log.Stream()
}

// Dispatch decides what one raw frame asks for.
func (s *Socket) Dispatch(raw []byte) Intent {
	inbound := ParseCommand(raw)

	switch inbound.Kind {
	case InboundMalformed:
		// Nothing to answer to: a frame without a command id has no surface the
		// glass could resolve a refusal onto, and a frame on another envelope
		// version would discard the refusal. The detail is the whole point of
		// the log line — it is the only account of the frame anyone gets.
		noteTrace(s.trace, "warn", "gdesk: dropped malformed frame: %s", inbound.Detail)
		return Intent{}
	case InboundUnrecognised:
		// A verb this build does not speak is tolerated, because deploys are
		// producer-first — the glass may be ahead of the connector. Silence is
		// the documented outcome for a command nobody answers.
		noteTrace(s.trace, "warn", "gdesk: unrecognised command %q", inbound.Header.Kind)
		return Intent{}
	case InboundInvalid:
		s.view.delivery.Refuse(inbound.Header.CmdID, gdesk.Failure{
			Code:    gdesk.FailureInvalidRequest,
			Message: inbound.Detail,
		})
		return Intent{}
	}

	if inbound.Header.Stream != s.view.log.Stream() {
		// A command for another stream is not ours to run. Refusing rather
		// than ignoring, because a glass talking to the wrong connector would
		// otherwise wait on a turn nobody is running.
		s.view.delivery.Refuse(inbound.Header.CmdID, gdesk.Failure{
			Code:    gdesk.FailureInvalidRequest,
			Message: fmt.Sprintf("command is for stream %q, this desk is %q", inbound.Header.Stream, s.view.log.Stream()),
		})
		return Intent{}
	}

	switch inbound.Header.Kind {
	case CommandResync:
		return Intent{Replay: s.view.log.Since(inbound.FromSeq)}
	case CommandUtterance:
		// Attributed before the turn runs, so the surface she opens carries
		// the command it answers. gdesk.utterance is not repeatable: without
		// the attribution an in-flight command stays unresolved and waits for
		// a person.
		s.view.delivery.Attribute(inbound.Spoken.ID, inbound.Header.CmdID)
		spoken := inbound.Spoken
		// Whose sentence this is comes from the connection, never from the
		// payload. A caller that could name its own conversation could name
		// somebody else's and be answered inside her transcript
		// (docs/GDESK-SCOPES.md).
		spoken.Conversation = s.view.conv
		noteTrace(s.trace, "info", "gdesk: utterance %s queued (%d chars)", spoken.ID, utf8.RuneCountInString(spoken.Text))
		return Intent{Answer: &spoken}
	}
	// Unreachable today: ParseCommand classifies only those two verbs as
	// commands and everything else as unrecognised. Kept because the
	// alternative is a verb added there and forgotten here being read as an
	// utterance with no words — a refusal the person never asked for, which is
	// worse than the silence a command nobody answers already means.
	return Intent{}
}

// Answer runs one turn. Failures are already facts by the time it returns, so
// the error is for the connector's log rather than for the glass.
func (s *Socket) Answer(ctx context.Context, spoken gdesk.Utterance) error {
	return s.answerer.Answer(ctx, spoken)
}
