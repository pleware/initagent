package desk

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// typedIn is a typed utterance from one person's conversation, which is what
// the connector attaches once more than one device is awake.
func typedIn(conv ConversationID, id, text string) Utterance {
	u := typed(id, text)
	u.Conversation = conv
	return u
}

// --- one person's state --------------------------------------------------

func TestRememberKeepsTheMostRecentLines(t *testing.T) {
	c := newConversation(DefaultConversation, adam, testFloor().Since)

	for i := range MaxThreadLines + 5 {
		c.remember(ania, Line{From: LineFromPerson, Text: fmt.Sprint(i)})
	}
	c.remember(ania, Line{From: LineFromStaff, Text: ""})

	lines := c.thread(ania)
	if len(lines) != MaxThreadLines {
		t.Fatalf("lines = %d, want the bound %d", len(lines), MaxThreadLines)
	}
	if lines[0].Text != "5" {
		t.Errorf("first line = %q, want the oldest kept line %q", lines[0].Text, "5")
	}
	if last := lines[len(lines)-1].Text; last != fmt.Sprint(MaxThreadLines+4) {
		t.Errorf("last line = %q, want the newest", last)
	}
}

func TestThreadIsACopy(t *testing.T) {
	c := newConversation(DefaultConversation, adam, testFloor().Since)
	if got := c.thread(ania); got != nil {
		t.Fatalf("thread = %+v, want nil before she has spoken", got)
	}

	c.remember(ania, Line{From: LineFromPerson, Text: "raz"})
	lines := c.thread(ania)
	lines[0].Text = "mutated"

	if got := c.thread(ania)[0].Text; got != "raz" {
		t.Errorf("thread = %q, want the conversation's copy untouched", got)
	}
}

// --- two people at one desk ----------------------------------------------

func TestConversationsDoNotShareTheFloor(t *testing.T) {
	d := newDesk(t, says("już"))

	if err := d.runner.Answer(context.Background(), typedIn("phone", "u1", "Aniu, sprawdź to")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	if got := d.runner.Floor("phone").Holder; got != ania {
		t.Errorf("floor of the phone = %q, want %q", got, ania)
	}
	// The other person never named anybody, so her unnamed sentence must not
	// land on whoever the first person happened to address.
	if got := d.runner.Floor("glass").Holder; got != adam {
		t.Errorf("floor of the glass = %q, want it untouched at %q", got, adam)
	}
}

func TestConversationsDoNotShareATranscript(t *testing.T) {
	d := newDesk(t, says("tak"), says("tak"))

	if err := d.runner.Answer(context.Background(), typedIn("phone", "u1", "Aniu, hasło do banku to jeden dwa trzy")); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if err := d.runner.Answer(context.Background(), typedIn("glass", "u2", "Aniu, co dalej")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	if d.chat.calls() != 2 {
		t.Fatalf("chat calls = %d, want 2", d.chat.calls())
	}
	second := d.chat.request(1)
	if len(second.Lines) != 1 {
		t.Fatalf("lines = %+v, want only this person's words", second.Lines)
	}
	for _, line := range second.Lines {
		if line.Text == "hasło do banku to jeden dwa trzy" {
			t.Error("the second person's request carries the first person's words")
		}
	}
}

func TestTheSameUtteranceIDInTwoConversationsIsTwoTurns(t *testing.T) {
	d := newDesk(t, says("raz"), says("dwa"))

	for _, conv := range []ConversationID{"phone", "glass"} {
		// Two devices, each numbering from one, each saying the same words.
		if err := d.runner.Answer(context.Background(), typedIn(conv, "utt-1", "Aniu, sprawdź to")); err != nil {
			t.Fatalf("Answer(%q): %v", conv, err)
		}
	}

	if got := d.chat.calls(); got != 2 {
		t.Errorf("chat calls = %d, want 2: the second person's sentence was taken for a re-delivery of the first person's", got)
	}
}

func TestEveryFactIsRecordedWithWhoseItIs(t *testing.T) {
	d := newDesk(t, says("raz"), says("dwa"))

	if err := d.runner.Answer(context.Background(), typedIn("phone", "u1", "Aniu, sprawdź to")); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if err := d.runner.Answer(context.Background(), typedIn("glass", "u2", "sprawdź to")); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	// Whose a fact is decides whose screen it may reach, so a fact that
	// arrives unlabelled is one the connector has to guess about.
	for i, conv := range d.facts.conversations() {
		if conv != "phone" && conv != "glass" {
			t.Fatalf("fact %d (%s) belongs to %q, want one of the two conversations", i, d.facts.all()[i].Kind(), conv)
		}
	}
	if got := d.facts.conversations()[0]; got != "phone" {
		t.Errorf("first fact belongs to %q, want the person who spoke first", got)
	}
}

func TestAWordNobodyCanActOnIsRefusedToTheRightPerson(t *testing.T) {
	d := newDesk(t)

	if err := d.runner.Answer(context.Background(), typedIn("glass", "u1", "   ")); err == nil {
		t.Fatal("Answer = nil, want a refusal")
	}

	// The refusal is said out loud, and saying it to the other person would
	// be the desk answering somebody who did not speak.
	convos := d.facts.conversations()
	if len(convos) != 1 || convos[0] != "glass" {
		t.Errorf("refusals recorded for %+v, want only the conversation that asked", convos)
	}
}

func TestFloorOfAConversationNobodyOpenedDoesNotOpenIt(t *testing.T) {
	d := newDesk(t)

	floor := d.runner.Floor("glass")
	if floor.Holder != adam || floor.Reason != FloorDefault {
		t.Errorf("floor = %+v, want the desk's opening floor", floor)
	}
	// Reading is not arriving. Otherwise anything that can name a
	// conversation can grow the map.
	if got := len(d.runner.conversations); got != 1 {
		t.Errorf("conversations = %d, want only the desk's own", got)
	}
}

func TestConversationsAreAnsweredAtTheSameTime(t *testing.T) {
	const talking = 2

	// The provider does not answer until both people have reached it, so a
	// desk that serialises across conversations cannot finish this test.
	arrived := make(chan struct{}, talking)
	release := make(chan struct{})
	chat := chatFunc(func(_ context.Context, _ ChatRequest) (ChatStream, error) {
		arrived <- struct{}{}
		<-release
		return func(yield func(ChatDelta, error) bool) {
			yield(ChatDelta{Text: "już"}, nil)
		}, nil
	})

	runner, err := NewRunner(RunnerConfig{
		Roster:   testRoster(t),
		Chat:     chat,
		Personas: staticPersonas{},
		Facts:    &factLog{},
		Model:    "m",
		Floor:    adam,
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	var wg sync.WaitGroup
	for _, conv := range []ConversationID{"phone", "glass"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			utt := typedIn(conv, "u-"+string(conv), "sprawdź to")
			if err := runner.Answer(context.Background(), utt); err != nil {
				t.Errorf("Answer(%q): %v", conv, err)
			}
		}()
	}

	for reached := range talking {
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			close(release)
			wg.Wait()
			t.Fatalf("%d of %d conversations reached the provider: the second person is waiting for the first person's model", reached, talking)
		}
	}
	close(release)
	wg.Wait()
}

func TestWordsNobodyCanActOnOpenNoConversation(t *testing.T) {
	d := newDesk(t)

	if err := d.runner.Answer(context.Background(), typedIn("glass", "u1", "   ")); err == nil {
		t.Fatal("Answer = nil, want a refusal")
	}

	if got := len(d.runner.conversations); got != 1 {
		t.Errorf("conversations = %d, want the refusal to have opened none", got)
	}
}
