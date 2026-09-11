package deskseam

import (
	"context"
	"fmt"

	"github.com/pleware/initagent/internal/desk"
)

// Answerer answers what somebody said. *desk.Runner is one.
//
// Named here rather than imported as the concrete runner so the socket can be
// tested against a stand-in that returns immediately: a real turn waits on a
// provider, and a read loop is not the place to discover that.
type Answerer interface {
	Answer(ctx context.Context, spoken desk.Utterance) error
}

// Intent is what one inbound frame asks the connector to do.
//
// Returned rather than acted on, because the two actions have opposite timing:
// a replay is a handful of writes on this connection and answering waits on a
// provider for seconds. Deciding is pure and testable; the connection loop
// chooses what to run inline and what to run alongside.
type Intent struct {
	// Answer is a turn to run, or nil.
	Answer *desk.Utterance

	// Replay goes to this connection only. Every other event reaches every
	// connection through the feed.
	Replay []Event
}

// SocketConfig is what a Socket needs. Every field is required.
type SocketConfig struct {
	// Log numbers events and holds what a resync replays.
	Log *Log
	// Delivery attributes an answer to the command it arrived on, and answers
	// a command the desk refuses.
	Delivery *Delivery
	// Answerer runs a turn.
	Answerer Answerer
}

// Socket is the seam's inbound half: it reads commands and decides.
//
// It holds no connection. What it decides is the same whether the frame came
// from a WebSocket, a test, or whatever replaces the transport, which is the
// only reason the decision can be tested without one.
type Socket struct {
	log      *Log
	delivery *Delivery
	answerer Answerer
}

// NewSocket refuses a socket that cannot answer or cannot replay.
func NewSocket(cfg SocketConfig) (*Socket, error) {
	switch {
	case cfg.Log == nil:
		return nil, fmt.Errorf("%w: socket needs a log to replay from", ErrSeam)
	case cfg.Delivery == nil:
		return nil, fmt.Errorf("%w: socket needs a delivery", ErrSeam)
	case cfg.Answerer == nil:
		return nil, fmt.Errorf("%w: socket needs somebody to answer", ErrSeam)
	}
	return &Socket{log: cfg.Log, delivery: cfg.Delivery, answerer: cfg.Answerer}, nil
}

// Stream is the stream this socket numbers events on. A connection sends it
// back on every command, and a command naming another stream is not ours.
func (s *Socket) Stream() StreamID {
	return s.log.Stream()
}

// Dispatch decides what one raw frame asks for.
func (s *Socket) Dispatch(raw []byte) Intent {
	inbound := ParseCommand(raw)

	switch inbound.Kind {
	case InboundMalformed:
		// Nothing to answer to: a frame without a command id has no surface
		// the glass could resolve a refusal onto.
		return Intent{}
	case InboundUnrecognised:
		// A verb this build does not speak is tolerated, because deploys are
		// producer-first — the glass may be ahead of the connector. Silence is
		// the documented outcome for a command nobody answers.
		return Intent{}
	case InboundInvalid:
		s.delivery.Refuse(inbound.Header.CmdID, desk.Failure{
			Code:    desk.FailureInvalidRequest,
			Message: inbound.Detail,
		})
		return Intent{}
	}

	if inbound.Header.Stream != s.log.Stream() {
		// A command for another stream is not ours to run. Refusing rather
		// than ignoring, because a glass talking to the wrong connector would
		// otherwise wait on a turn nobody is running.
		s.delivery.Refuse(inbound.Header.CmdID, desk.Failure{
			Code:    desk.FailureInvalidRequest,
			Message: fmt.Sprintf("command is for stream %q, this desk is %q", inbound.Header.Stream, s.log.Stream()),
		})
		return Intent{}
	}

	switch inbound.Header.Kind {
	case CommandResync:
		return Intent{Replay: s.log.Since(inbound.FromSeq)}
	case CommandUtterance:
		// Attributed before the turn runs, so the surface she opens carries
		// the command it answers. desk.utterance is not repeatable: without
		// the attribution an in-flight command stays unresolved and waits for
		// a person.
		s.delivery.Attribute(inbound.Spoken.ID, inbound.Header.CmdID)
		spoken := inbound.Spoken
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
func (s *Socket) Answer(ctx context.Context, spoken desk.Utterance) error {
	return s.answerer.Answer(ctx, spoken)
}
