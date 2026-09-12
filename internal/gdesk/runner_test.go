package gdesk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fakes ---------------------------------------------------------------

type factLog struct {
	mu     sync.Mutex
	facts  []Fact
	convos []ConversationID
}

func (l *factLog) Record(conv ConversationID, f Fact) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.facts = append(l.facts, f)
	l.convos = append(l.convos, conv)
}

// conversations names whose each recorded fact was, in order.
func (l *factLog) conversations() []ConversationID {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]ConversationID, len(l.convos))
	copy(out, l.convos)
	return out
}

func (l *factLog) all() []Fact {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Fact, len(l.facts))
	copy(out, l.facts)
	return out
}

// kinds names every fact in order, which is also what calls Kind on each type.
func (l *factLog) kinds() []FactKind {
	var out []FactKind
	for _, f := range l.all() {
		out = append(out, f.Kind())
	}
	return out
}

func (l *factLog) said() string {
	var b strings.Builder
	for _, f := range l.all() {
		if said, ok := f.(Said); ok {
			b.WriteString(said.Text)
		}
	}
	return b.String()
}

func (l *factLog) opened() []TurnOpened {
	var out []TurnOpened
	for _, f := range l.all() {
		if opened, ok := f.(TurnOpened); ok {
			out = append(out, opened)
		}
	}
	return out
}

func (l *factLog) failures() []Failed {
	var out []Failed
	for _, f := range l.all() {
		if failed, ok := f.(Failed); ok {
			out = append(out, failed)
		}
	}
	return out
}

type chatFunc func(ctx context.Context, req ChatRequest) (ChatStream, error)

func (f chatFunc) Turn(ctx context.Context, req ChatRequest) (ChatStream, error) {
	return f(ctx, req)
}

// recordingChat answers from a script, one entry per call, and keeps every
// request it was given. Locked because two conversations are answered at the
// same time.
type recordingChat struct {
	mu       sync.Mutex
	answers  []func() (ChatStream, error)
	requests []ChatRequest
}

func (c *recordingChat) Turn(_ context.Context, req ChatRequest) (ChatStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.requests = append(c.requests, req)
	if len(c.requests) > len(c.answers) {
		return nil, fmt.Errorf("chat called %d times, script has %d", len(c.requests), len(c.answers))
	}
	return c.answers[len(c.requests)-1]()
}

func (c *recordingChat) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func (c *recordingChat) request(i int) ChatRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests[i]
}

func says(texts ...string) func() (ChatStream, error) {
	return func() (ChatStream, error) {
		return func(yield func(ChatDelta, error) bool) {
			for _, text := range texts {
				if !yield(ChatDelta{Text: text}, nil) {
					return
				}
			}
		}, nil
	}
}

func saysThenFails(err error, texts ...string) func() (ChatStream, error) {
	return func() (ChatStream, error) {
		return func(yield func(ChatDelta, error) bool) {
			for _, text := range texts {
				if !yield(ChatDelta{Text: text}, nil) {
					return
				}
			}
			yield(ChatDelta{}, err)
		}, nil
	}
}

func refuses(err error) func() (ChatStream, error) {
	return func() (ChatStream, error) { return nil, err }
}

type staticPersonas struct {
	brief string
	words int
	err   error
}

func (p staticPersonas) Persona(_ context.Context, staff StaffID) (Persona, error) {
	if p.err != nil {
		return Persona{}, p.err
	}
	return Persona{Brief: p.brief + string(staff), MaxWords: p.words}, nil
}

// --- harness -------------------------------------------------------------

type harness struct {
	runner *Runner
	facts  *factLog
	chat   *recordingChat
	waited []time.Duration
}

func newDesk(t *testing.T, answers ...func() (ChatStream, error)) *harness {
	t.Helper()

	d := &harness{
		facts: &factLog{},
		chat:  &recordingChat{answers: answers},
	}
	runner, err := NewRunner(RunnerConfig{
		Roster:   testRoster(t),
		Chat:     d.chat,
		Personas: staticPersonas{brief: "you are ", words: 60},
		Facts:    d.facts,
		Model:    "gpt-4o-mini",
		Floor:    adam,
		Now:      func() time.Time { return testFloor().Since },
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	runner.wait = func(_ context.Context, wait time.Duration) error {
		d.waited = append(d.waited, wait)
		return nil
	}
	d.runner = runner
	return d
}

func typed(id, text string) Utterance {
	return Utterance{ID: UtteranceID(id), Text: text, Source: UtteranceTyped}
}

// --- construction --------------------------------------------------------

func TestNewRunnerRefusesADeskThatCannotAnswer(t *testing.T) {
	roster := testRoster(t)
	good := RunnerConfig{
		Roster:   roster,
		Chat:     chatFunc(nil),
		Personas: staticPersonas{},
		Facts:    &factLog{},
		Model:    "m",
		Floor:    adam,
	}

	cases := []struct {
		name string
		want string
		edit func(*RunnerConfig)
	}{
		{"no roster", "no roster", func(c *RunnerConfig) { c.Roster = Roster{} }},
		{"no chat", "no chat seam", func(c *RunnerConfig) { c.Chat = nil }},
		{"no personas", "no personas", func(c *RunnerConfig) { c.Personas = nil }},
		{"no facts", "no fact log", func(c *RunnerConfig) { c.Facts = nil }},
		{"no model", "no chat model", func(c *RunnerConfig) { c.Model = "" }},
		{"floor off the roster", "not on the roster", func(c *RunnerConfig) { c.Floor = "staff-nobody" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.edit(&cfg)

			_, err := NewRunner(cfg)
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("err = %v, want ErrConfig", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestNewRunnerDefaultsTheClockAndTheClaimStore(t *testing.T) {
	runner, err := NewRunner(RunnerConfig{
		Roster:   testRoster(t),
		Chat:     chatFunc(nil),
		Personas: staticPersonas{},
		Facts:    &factLog{},
		Model:    "m",
		Floor:    ania,
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if runner.claims == nil {
		t.Error("claims = nil, want a store: a runner without one duplicates work")
	}
	if runner.now == nil || runner.now().IsZero() {
		t.Error("now returned a zero time, want a real clock")
	}
	floor := runner.Floor(DefaultConversation)
	if floor.Holder != ania || floor.Reason != FloorDefault {
		t.Errorf("floor = %+v, want %s by default", floor, ania)
	}
}

// --- refusals ------------------------------------------------------------

func TestAnswerRefusesAnUtteranceNobodyCanAct(t *testing.T) {
	cases := []struct {
		name string
		want string
		utt  Utterance
	}{
		{"no id", "no id", Utterance{Text: "cześć"}},
		{"no words", "no words", typed("utt-1", "   \n ")},
		{"over the ceiling", "over", typed("utt-1", strings.Repeat("a", MaxUtteranceChars+1))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newDesk(t)

			err := d.runner.Answer(context.Background(), tc.utt)
			if !errors.Is(err, ErrRequest) {
				t.Fatalf("err = %v, want ErrRequest", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to mention %q", err, tc.want)
			}
			if d.chat.calls() != 0 {
				t.Errorf("chat called %d times, want 0", d.chat.calls())
			}

			failures := d.facts.failures()
			if len(failures) != 1 {
				t.Fatalf("failures = %d, want 1: a refusal must be sayable", len(failures))
			}
			if failures[0].Failure.Code != FailureInvalidRequest {
				t.Errorf("code = %q, want %q", failures[0].Failure.Code, FailureInvalidRequest)
			}
			if failures[0].Turn != "" {
				t.Errorf("turn = %q, want empty: no turn was opened", failures[0].Turn)
			}
		})
	}
}

// --- routing -------------------------------------------------------------

func TestAnswerCarriesToTheFloorHolder(t *testing.T) {
	d := newDesk(t, says("Sprawdzam", " to."))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to jeszcze raz")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	want := []FactKind{FactTurnOpened, FactSaid, FactSaid, FactSaid}
	if got := d.facts.kinds(); !slices.Equal(got, want) {
		t.Fatalf("facts = %v, want %v (no floor move: nobody was named)", got, want)
	}
	opened := d.facts.opened()
	if opened[0].Staff != adam {
		t.Errorf("staff = %q, want the floor holder %q", opened[0].Staff, adam)
	}
	if got := d.facts.said(); got != "Sprawdzam to." {
		t.Errorf("said = %q, want the deltas in order", got)
	}
	if d.runner.Floor(DefaultConversation).Holder != adam {
		t.Errorf("floor = %q, want it to stay with %q", d.runner.Floor(DefaultConversation).Holder, adam)
	}
}

func TestAnswerMovesTheFloorBeforeSheAnswers(t *testing.T) {
	d := newDesk(t, says("Już."))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "Aniu sprawdź to")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	kinds := d.facts.kinds()
	if len(kinds) == 0 || kinds[0] != FactFloorMoved {
		t.Fatalf("facts = %v, want the floor move first: addressing succeeded before answering did", kinds)
	}
	opened := d.facts.opened()
	if len(opened) != 1 || opened[0].Staff != ania {
		t.Fatalf("opened = %+v, want one turn for %q", opened, ania)
	}
	if opened[0].Text != "sprawdź to" {
		t.Errorf("segment text = %q, want the request without the address", opened[0].Text)
	}
	if d.runner.Floor(DefaultConversation).Reason != FloorNamed {
		t.Errorf("reason = %q, want %q", d.runner.Floor(DefaultConversation).Reason, FloorNamed)
	}
	if got := d.chat.request(0).Brief; !strings.Contains(got, string(ania)) {
		t.Errorf("brief = %q, want the persona of %q", got, ania)
	}
}

func TestAnswerAsksRatherThanGuessing(t *testing.T) {
	d := newDesk(t)

	if err := d.runner.Answer(context.Background(), typed("utt-1", "Adam, Ania")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	if d.chat.calls() != 0 {
		t.Fatalf("chat called %d times, want 0: an unclear address must not book work", d.chat.calls())
	}
	facts := d.facts.all()
	if len(facts) != 1 {
		t.Fatalf("facts = %v, want one question", d.facts.kinds())
	}
	unclear, ok := facts[0].(AddressingUnclear)
	if !ok {
		t.Fatalf("fact = %T, want AddressingUnclear", facts[0])
	}
	if len(unclear.Candidates) != 2 {
		t.Errorf("candidates = %v, want both mentioned", unclear.Candidates)
	}
	if unclear.Utterance != "utt-1" || unclear.Text != "Adam, Ania" {
		t.Errorf("unclear = %+v, want the utterance verbatim", unclear)
	}
	if d.runner.Floor(DefaultConversation).Holder != adam || d.runner.Floor(DefaultConversation).Reason != FloorDefault {
		t.Errorf("floor = %+v, want it untouched by a question", d.runner.Floor(DefaultConversation))
	}
}

func TestAnswerSummonsWithoutBookingWork(t *testing.T) {
	d := newDesk(t)

	if err := d.runner.Answer(context.Background(), typed("utt-1", "zawołaj Anię")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	if d.chat.calls() != 0 {
		t.Fatalf("chat called %d times, want 0: a summon is attention, not a request", d.chat.calls())
	}
	want := []FactKind{FactFloorMoved}
	if got := d.facts.kinds(); !slices.Equal(got, want) {
		t.Fatalf("facts = %v, want %v", got, want)
	}
	if floor := d.runner.Floor(DefaultConversation); floor.Holder != ania || floor.Reason != FloorSummoned {
		t.Errorf("floor = %+v, want %q summoned", floor, ania)
	}
}

func TestAnswerSplitsTwoSegmentsIntoTwoTurns(t *testing.T) {
	d := newDesk(t, says("Robię."), says("Już."))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "Adam zrób to a Ania zrób tamto")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	opened := d.facts.opened()
	if len(opened) != 2 {
		t.Fatalf("opened = %+v, want two turns", opened)
	}
	if opened[0].Staff != adam || opened[1].Staff != ania {
		t.Errorf("staff = %q then %q, want %q then %q", opened[0].Staff, opened[1].Staff, adam, ania)
	}
	if opened[0].Turn == opened[1].Turn {
		t.Errorf("both turns are %q, want a key per segment", opened[0].Turn)
	}
	if opened[0].Index == opened[1].Index {
		t.Errorf("both indexes are %d, want them to differ", opened[0].Index)
	}
	if floor := d.runner.Floor(DefaultConversation); floor.Holder != ania {
		t.Errorf("floor = %q, want the last segment's staff %q", floor.Holder, ania)
	}
}

func TestAnswerKeepsTheSecondSegmentWhenTheFirstFails(t *testing.T) {
	broken := fmt.Errorf("%w: nobody speaks this", ErrDialect)
	d := newDesk(t, refuses(broken), says("Ja mogę."))

	err := d.runner.Answer(context.Background(), typed("utt-1", "Adam zrób to a Ania zrób tamto"))
	if !errors.Is(err, ErrDialect) {
		t.Fatalf("err = %v, want the first segment's failure", err)
	}
	if d.chat.calls() != 2 {
		t.Fatalf("chat called %d times, want 2: segments are independent requests", d.chat.calls())
	}
	if got := d.facts.said(); got != "Ja mogę." {
		t.Errorf("said = %q, want the second segment answered anyway", got)
	}
	if failures := d.facts.failures(); len(failures) != 1 {
		t.Errorf("failures = %d, want 1", len(failures))
	}
}

// --- claims --------------------------------------------------------------

func TestAnswerIsANoOpForARedeliveredUtterance(t *testing.T) {
	d := newDesk(t, says("Raz."))
	utt := typed("utt-1", "sprawdź to")

	if err := d.runner.Answer(context.Background(), utt); err != nil {
		t.Fatalf("first Answer: %v", err)
	}
	before := len(d.facts.all())

	if err := d.runner.Answer(context.Background(), utt); err != nil {
		t.Fatalf("second Answer: %v", err)
	}

	if d.chat.calls() != 1 {
		t.Errorf("chat called %d times, want 1: a re-delivery replays, it does not re-ask", d.chat.calls())
	}
	if got := len(d.facts.all()); got != before {
		t.Errorf("facts grew from %d to %d, want the log untouched by a replay", before, got)
	}
}

func TestAnswerRetriesUnderTheSameTurnBeforeTheFirstWord(t *testing.T) {
	d := newDesk(t, refuses(errors.New("dial tcp: refused")), says("W końcu."))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	if d.chat.calls() != 2 {
		t.Fatalf("chat called %d times, want 2", d.chat.calls())
	}
	if len(d.waited) != 1 || d.waited[0] != Backoff(1) {
		t.Errorf("waited = %v, want one %v backoff", d.waited, Backoff(1))
	}
	if opened := d.facts.opened(); len(opened) != 1 {
		t.Errorf("opened = %d turns, want 1: a provider retry is not a second turn", len(opened))
	}
	if len(d.facts.failures()) != 0 {
		t.Errorf("failures = %v, want none: the retry answered", d.facts.failures())
	}
}

func TestAnswerStopsRetryingAtTheCeiling(t *testing.T) {
	flaky := errors.New("connection reset")
	d := newDesk(t, refuses(flaky), refuses(flaky), refuses(flaky), says("never"))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to")); !errors.Is(err, flaky) {
		t.Fatalf("err = %v, want the provider failure", err)
	}
	if d.chat.calls() != MaxTurnAttempts {
		t.Errorf("chat called %d times, want %d", d.chat.calls(), MaxTurnAttempts)
	}
	if failures := d.facts.failures(); len(failures) != 1 || failures[0].Failure.Code != FailureUpstreamFailed {
		t.Errorf("failures = %+v, want one upstream failure", failures)
	}
}

func TestAnswerDoesNotRetryOnceWordsAreOut(t *testing.T) {
	midReply := &ProviderError{Provider: "openai", Message: "stream broke"}
	d := newDesk(t, saysThenFails(midReply, "Zaczynam"), says("never"))
	utt := typed("utt-1", "sprawdź to")

	if err := d.runner.Answer(context.Background(), utt); !errors.Is(err, midReply) {
		t.Fatalf("err = %v, want the mid-reply failure", err)
	}
	if d.chat.calls() != 1 {
		t.Fatalf("chat called %d times, want 1: re-asking would say it twice", d.chat.calls())
	}
	if got := d.facts.said(); got != "Zaczynam" {
		t.Errorf("said = %q, want what was already delivered", got)
	}

	// Settled, not released: a re-delivery must not start the sentence again.
	if err := d.runner.Answer(context.Background(), utt); err != nil {
		t.Fatalf("re-delivery: %v", err)
	}
	if d.chat.calls() != 1 {
		t.Errorf("chat called %d times after a re-delivery, want 1", d.chat.calls())
	}
}

func TestAnswerReleasesATurnThatNeverSpoke(t *testing.T) {
	d := newDesk(t, refuses(fmt.Errorf("%w: bad dialect", ErrDialect)), says("Teraz mogę."))
	utt := typed("utt-1", "sprawdź to")

	if err := d.runner.Answer(context.Background(), utt); !errors.Is(err, ErrDialect) {
		t.Fatalf("err = %v, want the configuration failure", err)
	}
	if err := d.runner.Answer(context.Background(), utt); err != nil {
		t.Fatalf("re-delivery: %v", err)
	}

	if d.chat.calls() != 2 {
		t.Errorf("chat called %d times, want 2: nothing was said, so the turn was free", d.chat.calls())
	}
	if got := d.facts.said(); got != "Teraz mogę." {
		t.Errorf("said = %q, want the second attempt's answer", got)
	}
}

func TestAnswerRefusesTheSameTurnWithDifferentText(t *testing.T) {
	d := newDesk(t, says("Raz."))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to")); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź coś innego"))
	if !errors.Is(err, ErrTurnText) {
		t.Fatalf("err = %v, want ErrTurnText", err)
	}
	if d.chat.calls() != 1 {
		t.Errorf("chat called %d times, want 1", d.chat.calls())
	}
	failures := d.facts.failures()
	if len(failures) != 1 || failures[0].Failure.Code != FailureInvalidRequest {
		t.Fatalf("failures = %+v, want one invalid request", failures)
	}
	if failures[0].Turn == "" {
		t.Error("turn is empty, want the turn that was claimed")
	}
}

// --- interruption --------------------------------------------------------

func TestAnswerSaysNothingAboutACancelledTurn(t *testing.T) {
	d := newDesk(t, refuses(context.Canceled))

	err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if failures := d.facts.failures(); len(failures) != 0 {
		t.Errorf("failures = %+v, want none: being cut off is not a fault", failures)
	}
}

func TestAnswerStopsBetweenSegmentsWhenTheTurnIsAbandoned(t *testing.T) {
	d := newDesk(t, says("Robię."))
	ctx, cancel := context.WithCancel(context.Background())
	d.chat.answers[0] = func() (ChatStream, error) {
		cancel()
		return func(yield func(ChatDelta, error) bool) {
			yield(ChatDelta{Text: "Robię."}, nil)
		}, nil
	}

	err := d.runner.Answer(ctx, typed("utt-1", "Adam zrób to a Ania zrób tamto"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if d.chat.calls() != 1 {
		t.Errorf("chat called %d times, want 1: the second segment was abandoned", d.chat.calls())
	}
}

// --- the thread ----------------------------------------------------------

func TestAnswerCarriesTheThreadIntoTheNextTurn(t *testing.T) {
	d := newDesk(t, says("Sprawdzam."), says("Nadal sprawdzam."))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to")); err != nil {
		t.Fatalf("first Answer: %v", err)
	}
	if err := d.runner.Answer(context.Background(), typed("utt-2", "i jeszcze raz")); err != nil {
		t.Fatalf("second Answer: %v", err)
	}

	first := d.chat.request(0).Lines
	if len(first) != 1 || first[0].From != LineFromPerson || first[0].Text != "sprawdź to" {
		t.Fatalf("first lines = %+v, want just the person", first)
	}
	second := d.chat.request(1).Lines
	want := []Line{
		{From: LineFromPerson, Text: "sprawdź to"},
		{From: LineFromStaff, Text: "Sprawdzam."},
		{From: LineFromPerson, Text: "i jeszcze raz"},
	}
	if len(second) != len(want) {
		t.Fatalf("second lines = %+v, want %+v", second, want)
	}
	for i, line := range want {
		if second[i] != line {
			t.Errorf("line %d = %+v, want %+v", i, second[i], line)
		}
	}
}

func TestAnswerKeepsEachStaffMembersThreadApart(t *testing.T) {
	d := newDesk(t, says("Adam tu."), says("Ania tu."), says("Znowu Ania."))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "Adam zrób to a Ania zrób tamto")); err != nil {
		t.Fatalf("first Answer: %v", err)
	}
	if err := d.runner.Answer(context.Background(), typed("utt-2", "a teraz to")); err != nil {
		t.Fatalf("second Answer: %v", err)
	}

	// The floor is Ania's after the split, so the third request is hers and
	// must not carry a word Adam said.
	third := d.chat.request(2).Lines
	for _, line := range third {
		if strings.Contains(line.Text, "Adam tu.") {
			t.Fatalf("lines = %+v, want Adam's reply left out of Ania's thread", third)
		}
	}
	if len(third) != 3 {
		t.Errorf("lines = %d, want her own two plus the new request", len(third))
	}
}

// --- one mouth -----------------------------------------------------------

func TestAnswerSerialisesRepliesAcrossCallers(t *testing.T) {
	const callers = 8

	facts := &factLog{}
	chat := chatFunc(func(_ context.Context, _ ChatRequest) (ChatStream, error) {
		return func(yield func(ChatDelta, error) bool) {
			for _, text := range []string{"a", "b", "c"} {
				if !yield(ChatDelta{Text: text}, nil) {
					return
				}
			}
		}, nil
	})
	runner, err := NewRunner(RunnerConfig{
		Roster:   testRoster(t),
		Chat:     chat,
		Personas: staticPersonas{},
		Facts:    facts,
		Model:    "m",
		Floor:    adam,
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			utt := typed(fmt.Sprintf("utt-%d", i), "sprawdź to")
			if err := runner.Answer(context.Background(), utt); err != nil {
				t.Errorf("Answer: %v", err)
			}
		}()
	}
	wg.Wait()

	// Every turn's pieces must be contiguous: two half sentences from one
	// person is noise, not a busy desk.
	var current TurnID
	seen := map[TurnID]bool{}
	for _, fact := range facts.all() {
		said, ok := fact.(Said)
		if !ok {
			continue
		}
		if said.Turn == current {
			continue
		}
		if seen[said.Turn] {
			t.Fatalf("turn %q resumed after %q spoke: replies interleaved", said.Turn, current)
		}
		seen[said.Turn] = true
		current = said.Turn
	}
	if len(seen) != callers {
		t.Errorf("turns = %d, want %d", len(seen), callers)
	}
}

// --- failure mapping -----------------------------------------------------

func TestFailureOfMapsOntoTheSeamsCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want FailureCode
	}{
		{"deadline", context.DeadlineExceeded, FailureTimeout},
		{"our request", fmt.Errorf("%w: no id", ErrRequest), FailureInvalidRequest},
		{"configuration", fmt.Errorf("%w: bad entry", ErrConfig), FailureInternal},
		{"dialect", fmt.Errorf("%w: unknown", ErrDialect), FailureInternal},
		{"unauthorized", &ProviderError{Code: http.StatusUnauthorized}, FailureUnauthorized},
		{"forbidden", &ProviderError{Code: http.StatusForbidden}, FailureUnauthorized},
		{"missing model", &ProviderError{Code: http.StatusNotFound}, FailureNotFound},
		{"request timeout", &ProviderError{Code: http.StatusRequestTimeout}, FailureTimeout},
		{"gateway timeout", &ProviderError{Code: http.StatusGatewayTimeout}, FailureTimeout},
		{"context too long", &ProviderError{Code: http.StatusBadRequest}, FailureConflict},
		{"conflict", &ProviderError{Code: http.StatusConflict}, FailureConflict},
		{"payload too large", &ProviderError{Code: http.StatusRequestEntityTooLarge}, FailureConflict},
		{"rate limited", &ProviderError{Code: http.StatusTooManyRequests}, FailureUpstreamFailed},
		{"provider down", &ProviderError{Code: http.StatusBadGateway}, FailureUpstreamFailed},
		{"mid reply", &ProviderError{Message: "broke"}, FailureUpstreamFailed},
		{"unclassified", errors.New("dial tcp"), FailureUpstreamFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failure := failureOf(tc.err)
			if failure.Code != tc.want {
				t.Errorf("code = %q, want %q", failure.Code, tc.want)
			}
			if failure.Message == "" {
				t.Error("message is empty, want the error kept for the log")
			}
		})
	}
}

func TestFactKindsAreTheSeamsNames(t *testing.T) {
	facts := []Fact{
		FloorMoved{}, TurnOpened{}, Said{}, AddressingUnclear{}, Failed{},
	}
	want := []FactKind{
		FactFloorMoved, FactTurnOpened, FactSaid, FactAddressingUnclear, FactFailed,
	}
	for i, fact := range facts {
		if got := fact.Kind(); got != want[i] {
			t.Errorf("%T kind = %q, want %q", fact, got, want[i])
		}
		fact.fact()
	}
}

// --- the one edge --------------------------------------------------------

func TestSleepGivesUpWhenTheTurnIsAbandoned(t *testing.T) {
	if err := sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("sleep: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want the cancellation rather than an hour", err)
	}
}

func TestAnswerStopsWhenTheBackoffIsAbandoned(t *testing.T) {
	d := newDesk(t, refuses(errors.New("reset")), says("never"))
	d.runner.wait = func(context.Context, time.Duration) error { return context.Canceled }

	err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if d.chat.calls() != 1 {
		t.Errorf("chat called %d times, want 1", d.chat.calls())
	}
}

func TestAnswerReportsAPersonaThatCannotBeRead(t *testing.T) {
	facts := &factLog{}
	chat := &recordingChat{}
	runner, err := NewRunner(RunnerConfig{
		Roster:   testRoster(t),
		Chat:     chat,
		Personas: staticPersonas{err: errors.New("hub unreachable")},
		Facts:    facts,
		Model:    "m",
		Floor:    adam,
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	runner.wait = func(context.Context, time.Duration) error { return nil }

	if err := runner.Answer(context.Background(), typed("utt-1", "sprawdź to")); err == nil {
		t.Fatal("Answer succeeded, want the persona failure")
	}
	if chat.calls() != 0 {
		t.Errorf("chat called %d times, want 0: we do not answer as nobody", chat.calls())
	}
	if failures := facts.failures(); len(failures) != 1 {
		t.Errorf("failures = %d, want 1", len(failures))
	}
}

func TestStreamSkipsAKeepAlive(t *testing.T) {
	d := newDesk(t, says("Raz", "", "dwa"))

	if err := d.runner.Answer(context.Background(), typed("utt-1", "sprawdź to")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	said := 0
	for _, fact := range d.facts.all() {
		if _, ok := fact.(Said); ok {
			said++
		}
	}
	if said != 3 {
		t.Errorf("said facts = %d, want 2 words plus the closing one", said)
	}
	if got := d.facts.said(); got != "Razdwa" {
		t.Errorf("said = %q, want the empty delta dropped", got)
	}
}
