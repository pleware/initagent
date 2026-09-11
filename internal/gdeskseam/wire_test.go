package gdeskseam

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/gdesk"
)

func TestParseCommandClassifiesWhatCannotBeACommand(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		want   InboundKind
		detail string
	}{
		{"not json", "{", InboundMalformed, "not JSON"},
		{"another envelope version", `{"v":3,"cmdId":"c1","stream":"gdesk:local","kind":"gdesk.utterance","payload":{}}`, InboundMalformed, "bad command"},
		{"no command id", `{"v":2,"cmdId":"","stream":"gdesk:local","kind":"gdesk.utterance","payload":{}}`, InboundMalformed, "bad command"},
		{"no stream", `{"v":2,"cmdId":"c1","stream":"","kind":"gdesk.utterance","payload":{}}`, InboundMalformed, "bad command"},
		{"no verb", `{"v":2,"cmdId":"c1","stream":"gdesk:local","kind":"","payload":{}}`, InboundMalformed, "bad command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCommand([]byte(tc.raw))
			if got.Kind != tc.want {
				t.Fatalf("kind = %q, want %q", got.Kind, tc.want)
			}
			if got.Detail != tc.detail {
				t.Fatalf("detail = %q, want %q", got.Detail, tc.detail)
			}
		})
	}
}

func TestParseCommandKeepsAVerbItDoesNotAnswer(t *testing.T) {
	got := ParseCommand([]byte(`{"v":2,"cmdId":"c1","stream":"gdesk:local","kind":"catalog.open","payload":{"path":[]}}`))
	if got.Kind != InboundUnrecognised {
		t.Fatalf("kind = %q, want %q", got.Kind, InboundUnrecognised)
	}
	if got.Header.CmdID != "c1" || got.Header.Kind != "catalog.open" {
		t.Fatalf("header = %+v", got.Header)
	}
}

func TestParseCommandReadsAnUtterance(t *testing.T) {
	got := ParseCommand([]byte(`{"v":2,"cmdId":"c1","stream":"gdesk:local","kind":"gdesk.utterance","payload":{"utteranceId":"utt-1","text":"ania sprawdź to","source":"typed"}}`))
	if got.Kind != InboundCommand {
		t.Fatalf("kind = %q, detail = %q", got.Kind, got.Detail)
	}
	want := gdesk.Utterance{ID: "utt-1", Text: "ania sprawdź to", Source: gdesk.UtteranceTyped}
	if got.Spoken != want {
		t.Fatalf("spoken = %+v, want %+v", got.Spoken, want)
	}
}

func TestParseCommandRefusesAnUtteranceNobodyCanAct(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		detail  string
	}{
		{"not an object", `5`, "utterance payload is not an object"},
		{"no id", `{"text":"cześć","source":"typed"}`, "utterance has no id"},
		{"no words", `{"utteranceId":"utt-1","text":"   ","source":"typed"}`, "utterance has no words"},
		{"too long", `{"utteranceId":"utt-1","text":"` + strings.Repeat("a", gdesk.MaxUtteranceChars+1) + `","source":"typed"}`, "utterance is longer than"},
		{"unknown source", `{"utteranceId":"utt-1","text":"cześć","source":"telepathy"}`, "is not spoken or typed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCommand([]byte(`{"v":2,"cmdId":"c1","stream":"gdesk:local","kind":"gdesk.utterance","payload":` + tc.payload + `}`))
			if got.Kind != InboundInvalid {
				t.Fatalf("kind = %q, want %q", got.Kind, InboundInvalid)
			}
			if !strings.Contains(got.Detail, tc.detail) {
				t.Fatalf("detail = %q, want it to mention %q", got.Detail, tc.detail)
			}
			if got.Header.CmdID != "c1" {
				t.Fatalf("header lost the command id: %+v", got.Header)
			}
		})
	}
}

func TestParseCommandReadsAResync(t *testing.T) {
	got := ParseCommand([]byte(`{"v":2,"cmdId":"c1","stream":"gdesk:local","kind":"stream.resync","payload":{"fromSeq":7}}`))
	if got.Kind != InboundCommand || got.FromSeq != 7 {
		t.Fatalf("got = %+v", got)
	}
}

func TestParseCommandReadsAResyncFromTheBeginning(t *testing.T) {
	got := ParseCommand([]byte(`{"v":2,"cmdId":"c1","stream":"gdesk:local","kind":"stream.resync","payload":{"fromSeq":0}}`))
	if got.Kind != InboundCommand || got.FromSeq != 0 {
		t.Fatalf("got = %+v", got)
	}
}

func TestParseCommandRefusesAResyncItCannotHonour(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		detail  string
	}{
		{"not an object", `"seven"`, "resync payload is not an object"},
		{"no starting point", `{}`, "resync has no starting point"},
		{"before the first fact", `{"fromSeq":-1}`, "starts before the first fact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCommand([]byte(`{"v":2,"cmdId":"c1","stream":"gdesk:local","kind":"stream.resync","payload":` + tc.payload + `}`))
			if got.Kind != InboundInvalid {
				t.Fatalf("kind = %q, want %q", got.Kind, InboundInvalid)
			}
			if !strings.Contains(got.Detail, tc.detail) {
				t.Fatalf("detail = %q, want it to mention %q", got.Detail, tc.detail)
			}
		})
	}
}

func TestEncodeWritesTheEnvelopeAndPayloadInOneObject(t *testing.T) {
	event := Event{
		Envelope: Envelope{
			V:         Version,
			Seq:       3,
			At:        "2026-09-11T03:00:00Z",
			Stream:    "gdesk:local",
			Kind:      EventSurfaceAppended,
			InReplyTo: "c1",
		},
		Payload: surfaceAppended{ID: "sur-1", Slot: SpeechSlot, Chunk: "cześć"},
	}
	raw, err := event.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["v"] != float64(Version) || got["seq"] != float64(3) || got["kind"] != EventSurfaceAppended {
		t.Fatalf("envelope = %+v", got)
	}
	if got["inReplyTo"] != "c1" || got["stream"] != "gdesk:local" || got["at"] != "2026-09-11T03:00:00Z" {
		t.Fatalf("envelope = %+v", got)
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", got["payload"])
	}
	if payload["id"] != "sur-1" || payload["slot"] != "speech" || payload["chunk"] != "cześć" {
		t.Fatalf("payload = %+v", payload)
	}
	if _, present := payload["done"]; present {
		t.Fatal("an unfinished append should not claim to be done")
	}
}

func TestEncodeLeavesOutAnAnswerToNothing(t *testing.T) {
	raw, err := Event{Envelope: Envelope{V: Version, Seq: 1, Kind: EventSurfaceClosed}}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "inReplyTo") {
		t.Fatalf("unsolicited event carries an answer: %s", raw)
	}
}

func TestEncodeReportsAPayloadItCannotWrite(t *testing.T) {
	_, err := Event{
		Envelope: Envelope{V: Version, Seq: 1, Kind: EventSurfaceOpened},
		Payload:  make(chan int),
	}.Encode()
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), ErrSeam.Error()) {
		t.Fatalf("err = %v, want it to be a seam failure", err)
	}
}

func TestSplitChunkKeepsWhatTheGlassAccepts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		text  string
		want  int
		first int
	}{
		{"a sentence", "cześć", 1, 5},
		{"exactly the ceiling", strings.Repeat("a", MaxChunkChars), 1, MaxChunkChars},
		{"one over", strings.Repeat("a", MaxChunkChars+1), 2, MaxChunkChars},
		{"two full pieces", strings.Repeat("a", MaxChunkChars*2), 2, MaxChunkChars},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pieces := SplitChunk(tc.text)
			if len(pieces) != tc.want {
				t.Fatalf("pieces = %d, want %d", len(pieces), tc.want)
			}
			if len([]rune(pieces[0])) != tc.first {
				t.Fatalf("first piece = %d runes, want %d", len([]rune(pieces[0])), tc.first)
			}
			if strings.Join(pieces, "") != tc.text {
				t.Fatal("the pieces do not add up to what was said")
			}
		})
	}
}

func TestSplitChunkDoesNotCutThroughACharacter(t *testing.T) {
	text := strings.Repeat("ą", MaxChunkChars+10)
	pieces := SplitChunk(text)
	if strings.Join(pieces, "") != text {
		t.Fatal("a character was broken in half")
	}
	for i, piece := range pieces {
		if !utf8Valid(piece) {
			t.Fatalf("piece %d is not valid text", i)
		}
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}
