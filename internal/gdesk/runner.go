package gdesk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"
)

// MaxThreadLines bounds what one staff member has heard inside one
// conversation (docs/GDESK-SCOPES.md).
//
// A working day's tail, not a memory: cross-day recall of what was discussed
// is deliberately not built (docs/GDESK-CONVERSATION.md §12), and an unbounded
// thread makes every later turn slower and more expensive than the one before
// it while adding nothing a person at the desk would notice.
const MaxThreadLines = 40

// FailureCode is the seam's closed set of failure codes. These cross to the
// glass verbatim (initagent-glass app/src/seam/contract.ts GDESK_FAILURE_CODES),
// so a code added here without adding it there is a fact the glass will count
// as unrecognised.
type FailureCode string

const (
	// FailureInvalidRequest is a request we refused to act on: the caller's
	// mistake, and the same answer every time it is repeated.
	FailureInvalidRequest FailureCode = "INVALID_REQUEST"

	// FailureUnauthorized is a provider refusing our credentials.
	FailureUnauthorized FailureCode = "UNAUTHORIZED"

	// FailureNotFound is a model or route the provider does not have.
	FailureNotFound FailureCode = "NOT_FOUND"

	// FailureConflict is a refusal on the merits — a request that cannot be
	// served as asked, such as a context window already exceeded.
	FailureConflict FailureCode = "CONFLICT"

	// FailureTimeout is a turn that ran out of time.
	FailureTimeout FailureCode = "TIMEOUT"

	// FailureUpstreamFailed is the provider failing or being unreachable.
	FailureUpstreamFailed FailureCode = "UPSTREAM_FAILED"

	// FailureInternal is our own defect: a misconfigured desk, or a dialect
	// this build does not speak.
	FailureInternal FailureCode = "INTERNAL"
)

// Failure is the one shape every failure on this seam takes.
//
// A failure is a fact, not an exception thrown across the wire: Ania has to
// say what went wrong out loud, and a dropped promise has nothing to say it
// with. Message is technical and is what a log wants; turning it into a
// spoken line is the loop's job, not this struct's.
type Failure struct {
	Code    FailureCode
	Message string
}

// FactKind names one fact the desk publishes. The list is the contract with
// the glass; a kind it has never seen is counted as unrecognised rather than
// dropped, because deploys are producer-first.
type FactKind string

const (
	FactFloorMoved        FactKind = "gdesk.floor.moved"
	FactTurnOpened        FactKind = "gdesk.turn.opened"
	FactSaid              FactKind = "gdesk.said"
	FactAddressingUnclear FactKind = "gdesk.addressing.unclear"
	FactFailed            FactKind = "gdesk.failed"
)

// Fact is something that happened at the desk.
//
// The set is closed on purpose — the unexported method means a new kind is a
// change to this file and to the glass's contract in the same breath, rather
// than something a call site can invent locally and half-deliver.
type Fact interface {
	Kind() FactKind
	fact()
}

// FloorMoved says who hears the next sentence.
type FloorMoved struct {
	Staff  StaffID
	Reason FloorReason
}

// TurnOpened says a staff member has taken a turn, before any words arrive.
// It is published for a granted claim only: a replayed or refused duplicate
// opens nothing.
type TurnOpened struct {
	Turn      TurnID
	Staff     StaffID
	Utterance UtteranceID
	Index     int
	Text      string
}

// Said is a piece of a reply, chunked as it arrives.
//
// The closing fact carries Final with no text rather than marking the last
// delta, because a delta is only known to be last once the stream ends: to
// label it we would have to hold it, and a provider that stalls before
// closing would then be holding her last words.
type Said struct {
	Turn  TurnID
	Staff StaffID
	Text  string
	Final bool
}

// AddressingUnclear is a question, not a route. It carries whoever was
// mentioned so the glass can ask, and never a staff member to act.
type AddressingUnclear struct {
	Utterance  UtteranceID
	Candidates []StaffID
	Text       string
}

// Failed is a turn that could not be answered. Turn is empty when the
// utterance failed before any turn was opened.
type Failed struct {
	Turn    TurnID
	Failure Failure
}

func (FloorMoved) Kind() FactKind        { return FactFloorMoved }
func (TurnOpened) Kind() FactKind        { return FactTurnOpened }
func (Said) Kind() FactKind              { return FactSaid }
func (AddressingUnclear) Kind() FactKind { return FactAddressingUnclear }
func (Failed) Kind() FactKind            { return FactFailed }

func (FloorMoved) fact()        {}
func (TurnOpened) fact()        {}
func (Said) fact()              {}
func (AddressingUnclear) fact() {}
func (Failed) fact()            {}

// Facts is where the desk writes what happened.
//
// Recording cannot fail, because this is the connector's own fact log and not
// the wire: delivery to the glass has its own sequence numbers, gap detection
// and resume (draft 53), and a turn that already happened must not be undone
// because a window is closed. Implementations must be safe for concurrent
// use — the desk serialises replies inside one conversation, not across them.
//
// Every fact belongs to exactly one conversation, and it is recorded with it.
// Without that the connector could not tell whose fact it is, and one person's
// transcript would reach another person's screen (docs/GDESK-SCOPES.md).
type Facts interface {
	Record(conv ConversationID, fact Fact)
}

// Persona is who a staff member is at the moment she answers: the hub's
// baseline and prose rules, plus the mood offset the connector keeps in
// memory, already rendered to text. Rendering happens outside this package
// so mood decay lives in one place instead of being re-derived per turn.
//
// There is deliberately no model here. Which brain is behind a staff member
// is still open (draft 99, §53), and the MVP binds one desk.chat provider for
// the pair; when a per-staff brain is decided, this is where it lands.
type Persona struct {
	Brief    string
	MaxWords int
}

// Personas answers who is speaking.
//
// A lookup rather than a field on Staff, because a brief changes between
// turns — mood decays in minutes — while the forms a person is addressed by
// do not. It takes a context because the hub is the source of truth and this
// call may cross the network.
type Personas interface {
	Persona(ctx context.Context, staff StaffID) (Persona, error)
}

// UtteranceSource says how the words arrived. The desk may read a mistyped
// word differently from a misheard one.
type UtteranceSource string

const (
	UtteranceVoice UtteranceSource = "voice"
	UtteranceTyped UtteranceSource = "typed"
)

// Utterance is what a person said once — one press-and-release, or one
// textarea submit. ID is minted where the words were captured and is reused
// on every re-delivery, which is the only reason a duplicate is recognisable.
//
// The turn key derives from ID alone, so ID must be unique across the desk
// and not merely inside one device: two devices numbering their own
// utterances from one would make the second person's sentence look like a
// re-delivery of the first person's and be silently dropped. Carrying the
// origin is the connector's job, not the caller's promise
// (docs/GDESK-SCOPES.md).
type Utterance struct {
	ID     UtteranceID
	Text   string
	Source UtteranceSource

	// Conversation is whose talk this belongs to. The zero value is the
	// desk's only conversation, which is what one person at one desk means.
	// It is attached by the connector from the connection, never read from
	// what the caller sent (see ConversationID).
	Conversation ConversationID
}

func (u Utterance) validate() error {
	if u.ID == "" {
		return fmt.Errorf("%w: utterance has no id", ErrRequest)
	}
	if trimSegment(u.Text) == "" {
		return fmt.Errorf("%w: utterance has no words", ErrRequest)
	}
	// The glass enforces the exact UTF-16 bound where the words are captured
	// and refuses rather than cutting; this is the backstop that keeps a
	// pasted megabyte from becoming a provider request.
	if n := utf8.RuneCountInString(u.Text); n > MaxUtteranceChars {
		return fmt.Errorf("%w: utterance is %d characters, over %d", ErrRequest, n, MaxUtteranceChars)
	}
	return nil
}

// RunnerConfig is what a Runner needs. Every field but Now and Claims is
// required.
type RunnerConfig struct {
	Roster   Roster
	Chat     Chat
	Personas Personas
	Facts    Facts

	// Model is the desk.chat binding's model, from configuration.
	Model string

	// Floor is who holds the voice before anybody has spoken. The floor is
	// never empty (docs/GDESK-CONVERSATION.md §2), so this is required. Every
	// conversation opens with it, so it is the desk's setting rather than one
	// person's state.
	Floor StaffID

	// Claims defaults to a fresh store. A Runner without one would duplicate
	// work on every re-delivery, so it is made rather than left nil.
	Claims *TurnClaims

	// Now defaults to time.Now.
	Now func() time.Time
}

// Runner turns an utterance into facts.
//
// It owns the conversations and serialises turns, because "one mouth" cannot
// be enforced by a caller that might be holding two copies of who has the
// voice: two half sentences from one person is noise, not a busy desk. Audio
// and surfaces are not serialised here — a chart appearing while somebody
// talks is normal — and the mouth itself is the speaker's problem.
//
// It holds no socket. Facts go to the connector's log and the transport reads
// from there, so the same runner serves a local WebSocket, a test, and
// whatever replaces the transport later.
type Runner struct {
	// mu guards the conversation map and nothing else. Serialising a turn is
	// each conversation's own job, or the second person would wait for the
	// first person's provider (docs/GDESK-SCOPES.md).
	mu sync.Mutex

	// conversations is keyed by an id the connector attaches, so its size is
	// bounded by the conversations the desk admitted rather than by anything
	// a caller can name.
	conversations map[ConversationID]*conversation

	opening  StaffID
	roster   Roster
	chat     Chat
	personas Personas
	facts    Facts
	claims   *TurnClaims
	model    string
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
}

// NewRunner refuses a desk that cannot answer anybody.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	switch {
	case len(cfg.Roster.Staff()) == 0:
		return nil, fmt.Errorf("%w: runner has no roster", ErrConfig)
	case cfg.Chat == nil:
		return nil, fmt.Errorf("%w: runner has no chat seam", ErrConfig)
	case cfg.Personas == nil:
		return nil, fmt.Errorf("%w: runner has no personas", ErrConfig)
	case cfg.Facts == nil:
		return nil, fmt.Errorf("%w: runner has no fact log", ErrConfig)
	case cfg.Model == "":
		return nil, fmt.Errorf("%w: runner has no chat model", ErrConfig)
	case !cfg.Roster.Has(cfg.Floor):
		return nil, fmt.Errorf("%w: floor holder %q is not on the roster", ErrConfig, cfg.Floor)
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	claims := cfg.Claims
	if claims == nil {
		claims = NewTurnClaims()
	}

	return &Runner{
		// The desk's own conversation exists from the start, so a floor can
		// be read before anybody has spoken.
		conversations: map[ConversationID]*conversation{
			DefaultConversation: newConversation(DefaultConversation, cfg.Floor, now()),
		},
		opening:  cfg.Floor,
		roster:   cfg.Roster,
		chat:     cfg.Chat,
		personas: cfg.Personas,
		facts:    cfg.Facts,
		claims:   claims,
		model:    cfg.Model,
		now:      now,
		wait:     sleep,
	}, nil
}

// Floor reports who hears the next sentence in one conversation. A
// conversation nobody has opened yet answers with the desk's opening floor,
// and reading does not start one.
func (r *Runner) Floor(conv ConversationID) Floor {
	r.mu.Lock()
	c, open := r.conversations[conv]
	r.mu.Unlock()

	if !open {
		return NewFloor(r.opening, r.now())
	}
	return c.currentFloor()
}

// conversationFor returns one person's state, opening it on her first words.
func (r *Runner) conversationFor(conv ConversationID) *conversation {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.conversations[conv]
	if !ok {
		c = newConversation(conv, r.opening, r.now())
		r.conversations[conv] = c
	}
	return c
}

// Answer resolves who was addressed and answers as them.
//
// A re-delivered utterance is a deliberate no-op: the claim already ran or is
// still running, and the facts to replay are in the log the transport reads.
// So a nil error means "handled", not "answered now".
//
// Segments are independent. "Adam zrób to, Ania zrób tamto" is two requests
// to two people, so one failing does not cancel the other, and both failures
// come back joined.
//
// Two conversations are answered at the same time; two sentences into the
// same conversation are not, because one person hears one reply at a time.
func (r *Runner) Answer(ctx context.Context, u Utterance) error {
	if err := u.validate(); err != nil {
		r.fail(u.Conversation, "", err)
		return err
	}

	// After validation, so words nobody could act on do not open a
	// conversation.
	c := r.conversationFor(u.Conversation)

	c.ear.Lock()
	defer c.ear.Unlock()

	addressing := ResolveAddressing(u.Text, c.currentFloor(), r.roster)
	switch addressing.Kind {
	case AddressedUnclear:
		r.facts.Record(c.id, AddressingUnclear{
			Utterance:  u.ID,
			Candidates: addressing.Candidates,
			Text:       u.Text,
		})
		return nil
	case AddressedSummon:
		// Summoning carries no work: it moves the floor and nothing else, or
		// "Aniu?" would book a commitment nobody asked for.
		r.moveFloor(c, addressing)
		return nil
	}

	// The floor moves first. Addressing succeeded even if answering does not,
	// and a provider outage must not leave her conversation pointed at
	// whoever spoke last time.
	r.moveFloor(c, addressing)

	var failures error
	for _, segment := range addressing.Segments {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		failures = errors.Join(failures, r.turn(ctx, c, u, segment))
	}
	return failures
}

// turn runs one segment under its own claim.
func (r *Runner) turn(ctx context.Context, c *conversation, u Utterance, segment Segment) error {
	turn := NewTurnID(u.Conversation, u.ID, segment.Index)

	outcome, err := r.claims.Claim(turn, segment.Text, r.now())
	if err != nil {
		r.fail(c.id, turn, err)
		return err
	}
	if outcome != ClaimGranted {
		// in-flight is refused and replay is already in the log. Recording
		// anything here would either duplicate her answer or announce a turn
		// that is not ours to run.
		return nil
	}

	r.facts.Record(c.id, TurnOpened{
		Turn:      turn,
		Staff:     segment.Staff,
		Utterance: u.ID,
		Index:     segment.Index,
		Text:      segment.Text,
	})
	c.remember(segment.Staff, Line{From: LineFromPerson, Text: segment.Text})

	spoken, err := r.say(ctx, c, turn, segment)
	if err == nil {
		return r.claims.Settle(turn, r.now())
	}

	r.fail(c.id, turn, err)
	if spoken == 0 {
		// Nothing reached the scene, so nothing needs replaying and the same
		// utterance may be asked again.
		r.claims.Release(turn)
		return err
	}
	// Words are already out. Settling keeps a re-delivery from saying the
	// first half of the sentence a second time.
	return errors.Join(err, r.claims.Settle(turn, r.now()))
}

// say asks the provider and publishes the reply, retrying only while nothing
// has been said yet. It reports how many pieces reached the log.
func (r *Runner) say(ctx context.Context, c *conversation, turn TurnID, segment Segment) (int, error) {
	persona, err := r.personas.Persona(ctx, segment.Staff)
	if err != nil {
		return 0, err
	}

	req := ChatRequest{
		Model:    r.model,
		Brief:    persona.Brief,
		Lines:    c.thread(segment.Staff),
		MaxWords: persona.MaxWords,
	}

	for attempt := 1; ; attempt++ {
		spoken, err := r.stream(ctx, c, turn, segment.Staff, req)
		switch {
		case err == nil:
			return spoken, nil
		case spoken > 0:
			// A failure that arrives mid-reply is never retried: re-asking
			// would say what is already on the scene a second time. The
			// repair is a line the loop chooses with the transcript in hand.
			return spoken, err
		case !Retryable(err) || !RetryChat(attempt):
			return 0, err
		}
		if err := r.wait(ctx, Backoff(attempt)); err != nil {
			return 0, err
		}
	}
}

// stream publishes one attempt's deltas.
func (r *Runner) stream(ctx context.Context, c *conversation, turn TurnID, staff StaffID, req ChatRequest) (int, error) {
	stream, err := r.chat.Turn(ctx, req)
	if err != nil {
		return 0, err
	}

	spoken := 0
	var reply string
	for delta, err := range stream {
		if err != nil {
			c.remember(staff, Line{From: LineFromStaff, Text: reply})
			return spoken, err
		}
		if delta.Text == "" {
			// A keep-alive is not a word. Publishing it would make the scene
			// append nothing and the mouth speak silence.
			continue
		}
		reply += delta.Text
		spoken++
		r.facts.Record(c.id, Said{Turn: turn, Staff: staff, Text: delta.Text})
	}

	c.remember(staff, Line{From: LineFromStaff, Text: reply})
	r.facts.Record(c.id, Said{Turn: turn, Staff: staff, Final: true})
	return spoken, nil
}

// moveFloor publishes a move only when the holder or the reason changed, so a
// second sentence to the same person is not a stream of identical facts.
func (r *Runner) moveFloor(c *conversation, addressing Addressing) {
	next, changed := c.currentFloor().Move(addressing, r.now())
	if !changed {
		return
	}
	c.setFloor(next)
	r.facts.Record(c.id, FloorMoved{Staff: next.Holder, Reason: next.Reason})
}

// fail records a failure the desk can say out loud.
//
// A cancelled turn is not one of them: cutting somebody off is a person
// interrupting (docs/GDESK-CONVERSATION.md §6), and an apology for it would be
// the desk treating a normal act as a fault.
func (r *Runner) fail(conv ConversationID, turn TurnID, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	r.facts.Record(conv, Failed{Turn: turn, Failure: failureOf(err)})
}

// failureOf maps an error onto the seam's closed set of codes.
func failureOf(err error) Failure {
	failure := Failure{Code: FailureUpstreamFailed, Message: err.Error()}

	var provider *ProviderError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		failure.Code = FailureTimeout
	case errors.Is(err, ErrRequest):
		failure.Code = FailureInvalidRequest
	case errors.Is(err, ErrConfig), errors.Is(err, ErrDialect):
		// Ours, not the provider's: a desk bound to a dialect this build
		// cannot speak is a deployment that was never going to work.
		failure.Code = FailureInternal
	case errors.As(err, &provider):
		failure.Code = providerFailure(provider.Code)
	}
	return failure
}

func providerFailure(status int) FailureCode {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return FailureUnauthorized
	case http.StatusNotFound:
		return FailureNotFound
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return FailureTimeout
	case http.StatusBadRequest, http.StatusConflict, http.StatusRequestEntityTooLarge:
		// A refusal on the merits — a context window already exceeded is the
		// same answer every time, so it reads as a conflict rather than an
		// outage.
		return FailureConflict
	}
	return FailureUpstreamFailed
}

// sleep waits, or gives up early when the turn is abandoned.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
