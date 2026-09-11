package deskseam

import (
	"testing"

	"github.com/pleware/initagent/internal/desk"
)

func newTestViews(t *testing.T) *Views {
	t.Helper()
	views, err := NewViews(ViewsConfig{Names: map[desk.StaffID]string{
		"psn-ania": "Ania",
		"psn-adam": "Adam",
	}})
	if err != nil {
		t.Fatalf("NewViews: %v", err)
	}
	return views
}

func TestViewsNeedsANameForEachStaffMember(t *testing.T) {
	if _, err := NewViews(ViewsConfig{}); err == nil {
		t.Fatal("NewViews = nil error, want a refusal")
	}
}

func TestAMisspelledQuestionIsRefusedBeforeAnybodyConnects(t *testing.T) {
	// The alternative is a desk that opens, serves, and then fails on the
	// first sentence nobody could attribute.
	_, err := NewViews(ViewsConfig{
		Names:   map[desk.StaffID]string{"psn-ania": "Ania"},
		Unclear: "Do kogo mówisz?",
	})
	if err == nil {
		t.Fatal("NewViews = nil error, want a refusal")
	}
}

func TestBindNeedsAStream(t *testing.T) {
	views := newTestViews(t)
	if _, err := views.Bind("", "phone"); err == nil {
		t.Fatal("Bind = nil error, want a refusal")
	}
}

func TestBindingTheSameStreamTwiceKeepsItsHistory(t *testing.T) {
	views := newTestViews(t)

	first, err := views.Bind("desk:local", "phone")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	views.Record("phone", desk.TurnOpened{Turn: "trn-1", Staff: "psn-ania", Utterance: "utt-1"})

	// A dropped connection comes back as the same stream, and the events it
	// missed must still be there to replay.
	again, err := views.Bind("desk:local", "phone")
	if err != nil {
		t.Fatalf("Bind again: %v", err)
	}
	if again != first {
		t.Fatal("reconnecting opened a second view, so the gap can no longer be replayed")
	}
	if got := again.Log().Seq(); got != 1 {
		t.Errorf("seq = %d, want the event from before the drop", got)
	}
	if got := views.Streams(); got != 1 {
		t.Errorf("streams = %d, want 1", got)
	}
}

func TestAStreamCannotChangeConversation(t *testing.T) {
	views := newTestViews(t)
	if _, err := views.Bind("desk:local", "phone"); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// Otherwise a second person connecting with the first person's stream id
	// inherits her transcript.
	if _, err := views.Bind("desk:local", "glass"); err == nil {
		t.Fatal("Bind = nil error, want a refusal")
	}
}

func TestAFactReachesOnlyItsOwnConversation(t *testing.T) {
	views := newTestViews(t)
	mine, err := views.Bind("stream-mine", "phone")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	hers, err := views.Bind("stream-hers", "glass")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	views.Record("phone", desk.TurnOpened{Turn: "trn-1", Staff: "psn-ania", Utterance: "utt-1"})
	views.Record("phone", desk.Said{Turn: "trn-1", Staff: "psn-ania", Text: "hasło to jeden dwa trzy"})

	if got := mine.Log().Seq(); got != 2 {
		t.Errorf("my seq = %d, want both events", got)
	}
	if got := hers.Log().Seq(); got != 0 {
		t.Fatalf("the other person's stream numbered %d events of a conversation that is not hers", got)
	}
}

func TestEveryDeviceOfOnePersonIsNumberedOnItsOwn(t *testing.T) {
	views := newTestViews(t)
	phone, err := views.Bind("stream-phone", "person")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	// She walks to the desk and opens the glass, mid-conversation, so this
	// stream starts at one while the phone is already further along.
	views.Record("person", desk.TurnOpened{Turn: "trn-1", Staff: "psn-ania", Utterance: "utt-1"})

	glass, err := views.Bind("stream-glass", "person")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	views.Record("person", desk.Said{Turn: "trn-1", Staff: "psn-ania", Text: "robi się"})

	if got := phone.Log().Seq(); got != 2 {
		t.Errorf("phone seq = %d, want both events", got)
	}
	if got := glass.Log().Seq(); got != 1 {
		t.Errorf("glass seq = %d, want only what arrived after it opened", got)
	}
	if phone.Conversation() != glass.Conversation() {
		t.Error("two devices of one person landed in different conversations")
	}
}

func TestAFactNobodyIsWatchingGoesNowhere(t *testing.T) {
	views := newTestViews(t)
	watching, err := views.Bind("stream-mine", "phone")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	views.Record("nobody-here", desk.Said{Turn: "trn-1", Text: "cześć"})

	if got := watching.Log().Seq(); got != 0 {
		t.Errorf("seq = %d, want the fact to have reached nobody", got)
	}
}

func TestAViewCarriesTheFeedItsConnectionReads(t *testing.T) {
	views := newTestViews(t)
	view, err := views.Bind("stream-mine", "phone")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	reader := view.Feed().Subscribe()
	defer reader.Close()
	view.Delivery().Record(desk.Said{Turn: "trn-1", Text: "cześć"})

	if got := len(reader.Drain()); got != 1 {
		t.Errorf("drained %d events, want the one just delivered", got)
	}
}
