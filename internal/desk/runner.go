package desk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"
)

// MaxThreadLines bounds one staff member's conversation thread.
//
// A working day's tail, not a memory: cross-day recall of what was discussed
// is deliberately not built (docs/DESK-CONVERSATION.md §12), and an unbounded
// thread makes every later turn slower and more expensive than the one before
// it while adding nothing a person at the desk would notice.
const MaxThreadLines = 40

// FailureCode is the seam's closed set of failure codes. These cross to the
// glass verbatim (initagent-hud app/src/seam/contract.ts DESK_FAILURE_CODES),
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
	FactFloorMoved        FactKind = "desk.floor.moved"
	FactTurnOpened        FactKind = "desk.turn.opened"
	FactSaid              FactKind = "desk.said"
	FactAddressingUnclear FactKind = "desk.addressing.unclear"
	FactFailed            FactKind = "desk.failed"
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
// use — the desk serialises replies, not the log.
type Facts interface {
	Record(Fact)
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
type Utterance struct {
	ID     UtteranceID
	Text   string
	Source UtteranceSource
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
	// never empty (docs/DESK-CONVERSATION.md §2), so this is required.
	Floor StaffID

	// Claims defaults to a fresh store. A Runner without one would duplicate
	// work on every re-delivery, so it is made rather than left nil.
	Claims *TurnClaims

	// Now defaults to time.Now.
	Now func() time.Time
}

// Runner turns an utterance into facts.
//
// It owns the floor and serialises turns, because "one mouth" cannot be
// enforced by a caller that might be holding two copies of who has the
// voice: two half sentences from one person is noise, not a busy desk. Audio
// and surfaces are not serialised here — a chart appearing while somebody
// talks is normal — and the mouth itself is the speaker's problem.
//
// It holds no socket. Facts go to the connector's log and the transport reads
// from there, so the same runner serves a local WebSocket, a test, and
// whatever replaces the transport later.
type Runner struct {
	mu       sync.Mutex
	floor    Floor
	threads  map[StaffID][]Line
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
		floor:    NewFloor(cfg.Floor, now()),
		threads:  make(map[StaffID][]Line),
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

// Floor reports who hears the next sentence.
func (r *Runner) Floor() Floor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.floor
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
func (r *Runner) Answer(ctx context.Context, u Utterance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := u.validate(); err != nil {
		r.fail("", err)
		return err
	}

	addressing := ResolveAddressing(u.Text, r.floor, r.roster)
	switch addressing.Kind {
	case AddressedUnclear:
		r.facts.Record(AddressingUnclear{
			Utterance:  u.ID,
			Candidates: addressing.Candidates,
			Text:       u.Text,
		})
		return nil
	case AddressedSummon:
		// Summoning carries no work: it moves the floor and nothing else, or
		// "Aniu?" would book a commitment nobody asked for.
		r.moveFloor(addressing)
		return nil
	}

	// The floor moves first. Addressing succeeded even if answering does not,
	// and a provider outage must not leave the desk pointed at whoever spoke
	// last time.
	r.moveFloor(addressing)

	var failures error
	for _, segment := range addressing.Segments {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		failures = errors.Join(failures, r.turn(ctx, u, segment))
	}
	return failures
}

// turn runs one segment under its own claim.
func (r *Runner) turn(ctx context.Context, u Utterance, segment Segment) error {
	turn := NewTurnID(u.ID, segment.Index)

	outcome, err := r.claims.Claim(turn, segment.Text, r.now())
	if err != nil {
		r.fail(turn, err)
		return err
	}
	if outcome != ClaimGranted {
		// in-flight is refused and replay is already in the log. Recording
		// anything here would either duplicate her answer or announce a turn
		// that is not ours to run.
		return nil
	}

	r.facts.Record(TurnOpened{
		Turn:      turn,
		Staff:     segment.Staff,
		Utterance: u.ID,
		Index:     segment.Index,
		Text:      segment.Text,
	})
	r.remember(segment.Staff, Line{From: LineFromPerson, Text: segment.Text})

	spoken, err := r.say(ctx, turn, segment)
	if err == nil {
		return r.claims.Settle(turn, r.now())
	}

	r.fail(turn, err)
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
func (r *Runner) say(ctx context.Context, turn TurnID, segment Segment) (int, error) {
	persona, err := r.personas.Persona(ctx, segment.Staff)
	if err != nil {
		return 0, err
	}

	req := ChatRequest{
		Model:    r.model,
		Brief:    persona.Brief,
		Lines:    r.thread(segment.Staff),
		MaxWords: persona.MaxWords,
	}

	for attempt := 1; ; attempt++ {
		spoken, err := r.stream(ctx, turn, segment.Staff, req)
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
func (r *Runner) stream(ctx context.Context, turn TurnID, staff StaffID, req ChatRequest) (int, error) {
	stream, err := r.chat.Turn(ctx, req)
	if err != nil {
		return 0, err
	}

	spoken := 0
	var reply string
	for delta, err := range stream {
		if err != nil {
			r.remember(staff, Line{From: LineFromStaff, Text: reply})
			return spoken, err
		}
		if delta.Text == "" {
			// A keep-alive is not a word. Publishing it would make the scene
			// append nothing and the mouth speak silence.
			continue
		}
		reply += delta.Text
		spoken++
		r.facts.Record(Said{Turn: turn, Staff: staff, Text: delta.Text})
	}

	r.remember(staff, Line{From: LineFromStaff, Text: reply})
	r.facts.Record(Said{Turn: turn, Staff: staff, Final: true})
	return spoken, nil
}

// thread copies a staff member's lines, so a request being built cannot see
// the next turn's edits.
func (r *Runner) thread(staff StaffID) []Line {
	lines := r.threads[staff]
	if len(lines) == 0 {
		return nil
	}
	out := make([]Line, len(lines))
	copy(out, lines)
	return out
}

// remember appends to a staff member's thread, keeping the most recent lines.
// An empty line is dropped: a turn that failed before the first word said
// nothing, and a blank line in the thread reads as her ignoring the person.
func (r *Runner) remember(staff StaffID, line Line) {
	if line.Text == "" {
		return
	}
	lines := append(r.threads[staff], line)
	if len(lines) > MaxThreadLines {
		lines = lines[len(lines)-MaxThreadLines:]
	}
	r.threads[staff] = lines
}

// moveFloor publishes a move only when the holder or the reason changed, so a
// second sentence to the same person is not a stream of identical facts.
func (r *Runner) moveFloor(addressing Addressing) {
	next, changed := r.floor.Move(addressing, r.now())
	if !changed {
		return
	}
	r.floor = next
	r.facts.Record(FloorMoved{Staff: next.Holder, Reason: next.Reason})
}

// fail records a failure the desk can say out loud.
//
// A cancelled turn is not one of them: cutting somebody off is a person
// interrupting (docs/DESK-CONVERSATION.md §6), and an apology for it would be
// the desk treating a normal act as a fault.
func (r *Runner) fail(turn TurnID, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	r.facts.Record(Failed{Turn: turn, Failure: failureOf(err)})
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
