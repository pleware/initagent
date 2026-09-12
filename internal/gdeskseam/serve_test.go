package gdeskseam

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/gdesk"
)

// stubAnswerer stands in for the runner, which waits on a provider.
type stubAnswerer struct {
	heard []gdesk.Utterance
	err   error
}

func (s *stubAnswerer) Answer(_ context.Context, spoken gdesk.Utterance) error {
	s.heard = append(s.heard, spoken)
	return s.err
}

func newTestSocket(t *testing.T) (*Socket, *Log, *stubAnswerer) {
	t.Helper()
	socket, view, answerer := newTestSocketOf(t, "gdesk:local", DefaultTestConversation)
	return socket, view.Log(), answerer
}

// DefaultTestConversation is whose talk a test socket serves, so a test that
// does not care about several people does not have to name one.
const DefaultTestConversation gdesk.ConversationID = "person"

func newTestSocketOf(t *testing.T, stream StreamID, conv gdesk.ConversationID) (*Socket, *View, *stubAnswerer) {
	t.Helper()
	view, err := newTestViews(t).Bind(stream, conv)
	if err != nil {
		t.Fatal(err)
	}
	answerer := &stubAnswerer{}
	socket, err := NewSocket(SocketConfig{View: view, Answerer: answerer})
	if err != nil {
		t.Fatal(err)
	}
	return socket, view, answerer
}

// frame writes a command the way the glass would.
func frame(t *testing.T, stream StreamID, cmd CommandID, kind string, payload any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"v":       Version,
		"cmdId":   cmd,
		"stream":  stream,
		"kind":    kind,
		"payload": payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestNewSocketRefusesASocketThatCannotAnswer(t *testing.T) {
	view, err := newTestViews(t).Bind("gdesk:local", DefaultTestConversation)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		cfg    SocketConfig
		detail string
	}{
		{"no stream", SocketConfig{Answerer: &stubAnswerer{}}, "the stream it serves"},
		{"nobody to answer", SocketConfig{View: view}, "somebody to answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSocket(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.detail) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.detail)
			}
		})
	}
}

func TestStreamIsTheOneTheLogNumbers(t *testing.T) {
	socket, log, _ := newTestSocket(t)
	if socket.Stream() != log.Stream() {
		t.Fatalf("stream = %q, want %q", socket.Stream(), log.Stream())
	}
}

func TestDispatchHandsAnUtteranceOnToBeAnswered(t *testing.T) {
	socket, log, _ := newTestSocket(t)
	raw := frame(t, log.Stream(), "cmd-1", CommandUtterance, map[string]any{
		"utteranceId": "utt-1",
		"text":        "sprawdź opony",
		"source":      "typed",
	})

	intent := socket.Dispatch(raw)
	if intent.Answer == nil {
		t.Fatal("want an utterance to answer")
	}
	if intent.Answer.ID != "utt-1" || intent.Answer.Text != "sprawdź opony" {
		t.Fatalf("answer = %+v", *intent.Answer)
	}
	if intent.Replay != nil {
		t.Fatalf("replay = %+v, want an utterance to say nothing back yet", intent.Replay)
	}
}

func TestDispatchWritesTheUtteranceOntoTheOperatorRing(t *testing.T) {
	view, err := newTestViews(t).Bind("gdesk:local", DefaultTestConversation)
	if err != nil {
		t.Fatal(err)
	}
	trace := NewTrace(TraceConfig{})
	socket, err := NewSocket(SocketConfig{View: view, Answerer: &stubAnswerer{}, Trace: trace})
	if err != nil {
		t.Fatal(err)
	}
	raw := frame(t, "gdesk:local", "cmd-1", CommandUtterance, map[string]any{
		"utteranceId": "utt-1", "text": "cześć", "source": "typed",
	})
	if intent := socket.Dispatch(raw); intent.Answer == nil {
		t.Fatal("want a turn")
	}
	dump := trace.Dump()
	if len(dump.Lines) != 1 || !strings.Contains(dump.Lines[0].Text, "utt-1") {
		t.Fatalf("trace = %+v", dump)
	}
}

func TestDispatchAttributesTheAnswerToTheCommandItArrivedOn(t *testing.T) {
	socket, log, _ := newTestSocket(t)
	raw := frame(t, log.Stream(), "cmd-7", CommandUtterance, map[string]any{
		"utteranceId": "utt-1", "text": "cześć", "source": "typed",
	})
	socket.Dispatch(raw)

	socket.view.Delivery().Record(gdesk.TurnOpened{Turn: "trn-1", Staff: "psn-ania", Utterance: "utt-1"})
	if got := log.Since(0)[0].InReplyTo; got != "cmd-7" {
		t.Fatalf("inReplyTo = %q, want the command she is answering", got)
	}
}

func TestDispatchReplaysFromWhereTheGlassStopped(t *testing.T) {
	socket, log, _ := newTestSocket(t)
	for range 3 {
		log.Append(EventSurfaceAppended, surfaceAppended{ID: "sur-1"}, "")
	}
	// fromSeq is the first event wanted, inclusive - the glass filters with
	// `seq >= fromSeq` (initagent-glass app/src/seam/fake-connector.ts), so
	// answering from after it would silently lose one fact.
	raw := frame(t, log.Stream(), "cmd-1", CommandResync, map[string]any{"fromSeq": 2})

	intent := socket.Dispatch(raw)
	if intent.Answer != nil {
		t.Fatal("a resync asks for facts, not for a turn")
	}
	if got := seqsOf(intent.Replay); len(got) != 2 || got[0] != 2 {
		t.Fatalf("replay = %v, want seq 2 onward", got)
	}
}

func TestDispatchSaysNothingBackToAFrameItCannotIdentify(t *testing.T) {
	socket, log, _ := newTestSocket(t)

	intent := socket.Dispatch([]byte("{"))
	if intent.Answer != nil || intent.Replay != nil {
		t.Fatalf("intent = %+v", intent)
	}
	if got := log.Since(0); len(got) != 0 {
		t.Fatalf("events = %v, want no refusal a glass could not place", seqsOf(got))
	}
}

func TestDispatchToleratesAVerbThisBuildDoesNotSpeak(t *testing.T) {
	socket, log, _ := newTestSocket(t)
	raw := frame(t, log.Stream(), "cmd-1", "gdesk.interrupt", map[string]any{})

	intent := socket.Dispatch(raw)
	if intent.Answer != nil || intent.Replay != nil {
		t.Fatalf("intent = %+v", intent)
	}
	if got := log.Since(0); len(got) != 0 {
		t.Fatalf("events = %v, want silence rather than a refusal - deploys are producer-first", seqsOf(got))
	}
}

func TestDispatchRefusesACommandNobodyCanActOn(t *testing.T) {
	socket, log, _ := newTestSocket(t)
	raw := frame(t, log.Stream(), "cmd-1", CommandUtterance, map[string]any{
		"utteranceId": "utt-1", "text": "   ", "source": "typed",
	})

	intent := socket.Dispatch(raw)
	if intent.Answer != nil {
		t.Fatal("want no turn for words nobody said")
	}
	events := log.Since(0)
	if len(events) != 1 || events[0].Kind != EventSurfaceOpened {
		t.Fatalf("events = %+v", events)
	}
	if events[0].InReplyTo != "cmd-1" {
		t.Fatalf("inReplyTo = %q, want the refusal to answer the command", events[0].InReplyTo)
	}
	surface := payloadOf(t, events[0])["surface"].(map[string]any)
	if surface["id"] != "sur-cmd-1" {
		t.Fatalf("id = %v, want it keyed on the command", surface["id"])
	}
	if surface["label"] != RefusalLabel {
		t.Fatalf("label = %v, want the refusal not to come from her", surface["label"])
	}
	view := surface["view"].(map[string]any)
	failure := view["failure"].(map[string]any)
	if view["kind"] != "failed" || failure["code"] != "INVALID_REQUEST" {
		t.Fatalf("view = %+v", view)
	}
	if !strings.Contains(failure["message"].(string), "no words") {
		t.Fatalf("message = %v", failure["message"])
	}
}

func TestDispatchRefusesACommandForAnotherDesk(t *testing.T) {
	socket, log, _ := newTestSocket(t)
	raw := frame(t, "str-somebody-else", "cmd-1", CommandUtterance, map[string]any{
		"utteranceId": "utt-1", "text": "cześć", "source": "typed",
	})

	intent := socket.Dispatch(raw)
	if intent.Answer != nil {
		t.Fatal("want no turn for a command aimed elsewhere")
	}
	events := log.Since(0)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want a refusal so the glass is not left waiting", events)
	}
	failure := payloadOf(t, events[0])["surface"].(map[string]any)["view"].(map[string]any)["failure"].(map[string]any)
	if !strings.Contains(failure["message"].(string), "str-somebody-else") {
		t.Fatalf("message = %v, want it to name the stream asked for", failure["message"])
	}
}

func TestAnswerPassesTheTurnOnAndReportsWhatWentWrong(t *testing.T) {
	socket, _, answerer := newTestSocket(t)
	answerer.err = errors.New("provider said no")

	err := socket.Answer(context.Background(), gdesk.Utterance{ID: "utt-1", Text: "cześć"})
	if err == nil || !strings.Contains(err.Error(), "provider said no") {
		t.Fatalf("err = %v", err)
	}
	if len(answerer.heard) != 1 || answerer.heard[0].ID != "utt-1" {
		t.Fatalf("heard = %+v", answerer.heard)
	}
}

func TestRefuseIgnoresACommandWithNoId(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Refuse("", gdesk.Failure{Code: gdesk.FailureInternal})
	if got := log.Since(0); len(got) != 0 {
		t.Fatalf("events = %v, want nothing to key a surface on", seqsOf(got))
	}
}

func TestRecordSaysNothingAboutAFailureWithNoTurn(t *testing.T) {
	delivery, log := newTestDelivery(t)
	delivery.Record(gdesk.Failed{Failure: gdesk.Failure{Code: gdesk.FailureInvalidRequest}})
	if got := log.Since(0); len(got) != 0 {
		t.Fatalf("events = %v, want no patch to a surface the glass never saw", seqsOf(got))
	}
}
