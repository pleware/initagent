package gdeskseam

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/gdesk"
)

func testNames() map[gdesk.StaffID]string {
	return map[gdesk.StaffID]string{"staff-ania": "Ania", "staff-adam": "Adam"}
}

func newTestDelivery(t *testing.T) (*Delivery, *Log) {
	t.Helper()
	log := newTestLog(t, 0, nil)
	delivery, err := NewDelivery(DeliveryConfig{Log: log, Names: testNames()})
	if err != nil {
		t.Fatal(err)
	}
	return delivery, log
}

// payloadOf renders an event the way the glass will read it, so a test asserts
// on the wire rather than on the struct that produced it.
func payloadOf(t *testing.T, event Event) map[string]any {
	t.Helper()
	raw, err := event.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	return got.Payload
}

func TestNewDeliveryRefusesADeliveryThatCannotName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cfg    DeliveryConfig
		detail string
	}{
		{"no log", DeliveryConfig{Names: testNames()}, "needs a log"},
		{"no names", DeliveryConfig{Log: newTestLog(t, 0, nil)}, "needs a name for each"},
		{"a blank name", DeliveryConfig{Log: newTestLog(t, 0, nil), Names: map[gdesk.StaffID]string{"staff-ania": " "}}, "no name to be shown by"},
		{"a question with nowhere for the names", DeliveryConfig{Log: newTestLog(t, 0, nil), Names: testNames(), Unclear: "kto?"}, "nowhere to put the names"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDelivery(tc.cfg)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.detail) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.detail)
			}
		})
	}
}

func TestNewDeliveryDefaultsTheQuestion(t *testing.T) {
	delivery, _ := newTestDelivery(t)
	if delivery.unclear != DefaultUnclearPrompt {
		t.Fatalf("unclear = %q", delivery.unclear)
	}
}

func TestNewDeliveryKeepsItsOwnCopyOfTheNames(t *testing.T) {
	names := testNames()
	delivery, err := NewDelivery(DeliveryConfig{Log: newTestLog(t, 0, nil), Names: names})
	if err != nil {
		t.Fatal(err)
	}
	delete(names, "staff-ania")
	if got := delivery.name("staff-ania"); got != "Ania" {
		t.Fatalf("name = %q, want the caller's later edit not to rename her", got)
	}
}

func TestRecordOpensASurfaceInHerName(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.TurnOpened{Turn: "turn-1", Staff: "staff-ania", Utterance: "utt-1", Text: "sprawdź to"})

	events := log.Since(0)
	if len(events) != 1 || events[0].Kind != EventSurfaceOpened {
		t.Fatalf("events = %+v", events)
	}
	payload := payloadOf(t, events[0])
	surface, ok := payload["surface"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %+v", payload)
	}
	if surface["id"] != "surface-turn-1" {
		t.Fatalf("id = %v, want it derived from the turn", surface["id"])
	}
	if surface["label"] != "Ania" {
		t.Fatalf("label = %v, want her name", surface["label"])
	}
	if surface["attention"] != "primary" || surface["chrome"] != "bordered" {
		t.Fatalf("surface = %+v", surface)
	}
	placement, _ := surface["placement"].(map[string]any)
	if placement["at"] != "focus" {
		t.Fatalf("placement = %+v", placement)
	}
	view, _ := surface["view"].(map[string]any)
	if view["kind"] != "panel" {
		t.Fatalf("view = %+v", view)
	}
	blocks, _ := view["blocks"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %+v", blocks)
	}
	block, _ := blocks[0].(map[string]any)
	if block["kind"] != "text" || block["slot"] != "speech" || block["text"] != "" {
		t.Fatalf("block = %+v", block)
	}
	if block["streaming"] != true {
		t.Fatal("an opened reply must say more is coming, or silence reads as the end")
	}
}

func TestRecordAppendsEachPieceSheSays(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.Said{Turn: "turn-1", Staff: "staff-ania", Text: "sprawdzam"})
	delivery.Record(gdesk.Said{Turn: "turn-1", Staff: "staff-ania", Text: " opony"})

	events := log.Since(0)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	for i, want := range []string{"sprawdzam", " opony"} {
		if events[i].Kind != EventSurfaceAppended {
			t.Fatalf("event %d = %q", i, events[i].Kind)
		}
		payload := payloadOf(t, events[i])
		if payload["chunk"] != want {
			t.Fatalf("chunk %d = %v, want %q", i, payload["chunk"], want)
		}
		if payload["id"] != "surface-turn-1" || payload["slot"] != "speech" {
			t.Fatalf("payload = %+v", payload)
		}
	}
}

func TestRecordClosesTheSlotWithoutRepeatingHerLastWords(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.Said{Turn: "turn-1", Staff: "staff-ania", Text: "gotowe"})
	delivery.Record(gdesk.Said{Turn: "turn-1", Staff: "staff-ania", Final: true})

	events := log.Since(0)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	payload := payloadOf(t, events[1])
	if payload["done"] != true {
		t.Fatalf("payload = %+v, want it done", payload)
	}
	if payload["chunk"] != "" {
		t.Fatalf("chunk = %v, want the closing fact to add no words", payload["chunk"])
	}
}

func TestRecordSplitsALineTooLongForTheGlass(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.Said{Turn: "turn-1", Text: strings.Repeat("a", MaxChunkChars+1)})

	events := log.Since(0)
	if len(events) != 2 {
		t.Fatalf("events = %d, want the line split in two", len(events))
	}
	first := payloadOf(t, events[0])["chunk"].(string)
	second := payloadOf(t, events[1])["chunk"].(string)
	if len(first) != MaxChunkChars || len(second) != 1 {
		t.Fatalf("pieces = %d and %d", len(first), len(second))
	}
}

func TestRecordAsksWhoWasMeant(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.AddressingUnclear{
		Utterance:  "utt-1",
		Candidates: []gdesk.StaffID{"staff-ania", "staff-adam"},
		Text:       "sprawdź to",
	})

	events := log.Since(0)
	if len(events) != 1 || events[0].Kind != EventSurfaceOpened {
		t.Fatalf("events = %+v", events)
	}
	surface := payloadOf(t, events[0])["surface"].(map[string]any)
	if surface["id"] != "surface-utt-1" {
		t.Fatalf("id = %v, want it derived from the utterance", surface["id"])
	}
	if surface["label"] != "Ania / Adam" {
		t.Fatalf("label = %v", surface["label"])
	}
	view := surface["view"].(map[string]any)
	blocks := view["blocks"].([]any)
	block := blocks[0].(map[string]any)
	if block["text"] != fmt.Sprintf(DefaultUnclearPrompt, "Ania czy Adam") {
		t.Fatalf("text = %v", block["text"])
	}
	if _, streaming := block["streaming"]; streaming {
		t.Fatal("a question is finished when it is asked")
	}
}

func TestRecordTurnsTheSurfaceIntoTheSeamsFailureView(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.Failed{
		Turn:    "turn-1",
		Failure: gdesk.Failure{Code: gdesk.FailureUpstreamFailed, Message: "provider said no"},
	})

	events := log.Since(0)
	if len(events) != 1 || events[0].Kind != EventSurfacePatched {
		t.Fatalf("events = %+v", events)
	}
	payload := payloadOf(t, events[0])
	if payload["id"] != "surface-turn-1" {
		t.Fatalf("id = %v", payload["id"])
	}
	view := payload["view"].(map[string]any)
	if view["kind"] != "failed" {
		t.Fatalf("view = %+v", view)
	}
	failure := view["failure"].(map[string]any)
	if failure["code"] != "UPSTREAM_FAILED" || failure["message"] != "provider said no" {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestRecordSaysNothingAboutTheFloor(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.FloorMoved{Staff: "staff-adam", Reason: gdesk.FloorNamed})

	if got := log.Since(0); len(got) != 0 {
		t.Fatalf("events = %+v, want none - the glass draws no floor", got)
	}
}

func TestRecordAnswersTheCommandTheUtteranceArrivedOn(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Attribute("utt-1", "cmd-1")
	delivery.Record(gdesk.TurnOpened{Turn: "turn-1", Staff: "staff-ania", Utterance: "utt-1"})
	delivery.Record(gdesk.Said{Turn: "turn-1", Text: "robi się"})

	events := log.Since(0)
	if events[0].InReplyTo != "cmd-1" {
		t.Fatalf("inReplyTo = %q, want the command", events[0].InReplyTo)
	}
	if events[1].InReplyTo != "" {
		t.Fatalf("inReplyTo = %q, want each piece to be unsolicited", events[1].InReplyTo)
	}
}

func TestRecordCarriesNoAnswerForAnUtteranceItWasNotToldAbout(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.TurnOpened{Turn: "turn-1", Staff: "staff-ania", Utterance: "utt-9"})

	if got := log.Since(0)[0].InReplyTo; got != "" {
		t.Fatalf("inReplyTo = %q, want nothing invented", got)
	}
}

func TestAttributeIgnoresAnUtteranceWithNoId(t *testing.T) {
	delivery, _ := newTestDelivery(t)
	delivery.Attribute("", "cmd-1")
	if len(delivery.replies) != 0 {
		t.Fatalf("replies = %+v", delivery.replies)
	}
}

func TestAttributeKeepsTheSameUtteranceOnce(t *testing.T) {
	delivery, _ := newTestDelivery(t)
	delivery.Attribute("utt-1", "cmd-1")
	delivery.Attribute("utt-1", "cmd-2")
	if len(delivery.spoken) != 1 {
		t.Fatalf("spoken = %v, want one entry", delivery.spoken)
	}
	if delivery.replies["utt-1"] != "cmd-2" {
		t.Fatalf("replies = %+v, want the latest command", delivery.replies)
	}
}

func TestAttributeForgetsTheOldestPastItsCeiling(t *testing.T) {
	delivery, _ := newTestDelivery(t)
	for i := range maxAttributions + 1 {
		delivery.Attribute(gdesk.UtteranceID(fmt.Sprintf("utt-%d", i)), CommandID(fmt.Sprintf("cmd-%d", i)))
	}
	if len(delivery.replies) != maxAttributions {
		t.Fatalf("replies = %d, want %d", len(delivery.replies), maxAttributions)
	}
	if _, remembered := delivery.replies["utt-0"]; remembered {
		t.Fatal("the oldest utterance should have been forgotten")
	}
	if delivery.replies["utt-1"] != "cmd-1" {
		t.Fatalf("replies = %+v, want the rest kept", delivery.replies)
	}
}

func TestRecordLabelsAStrangerWithWhatItHas(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.TurnOpened{Turn: "turn-1", Staff: "staff-nobody"})

	surface := payloadOf(t, log.Since(0)[0])["surface"].(map[string]any)
	if surface["label"] != "staff-nobody" {
		t.Fatalf("label = %v, want the id rather than an empty label", surface["label"])
	}
}

func TestRecordIgnoresAFactItHasNoSurfaceFor(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.FloorMoved{Staff: "staff-ania"})
	delivery.Record(gdesk.Said{Turn: "turn-1", Text: "cześć"})

	if got := log.Since(0); len(got) != 1 {
		t.Fatalf("events = %+v, want only the spoken piece", seqsOf(got))
	}
}
