package gdeskseam

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/gdesk"
)

const testToken = "sec-desk-token"

// echoAnswerer answers by recording one fact, so a test can see a turn ran
// without waiting on a provider.
type echoAnswerer struct {
	mu    sync.Mutex
	views *Views
	conv  gdesk.ConversationID
	heard []gdesk.Utterance
}

func (a *echoAnswerer) Answer(_ context.Context, spoken gdesk.Utterance) error {
	a.mu.Lock()
	a.heard = append(a.heard, spoken)
	a.mu.Unlock()

	a.views.Record(spoken.Conversation, gdesk.TurnOpened{
		Turn:      "turn-1",
		Staff:     "staff-ania",
		Utterance: spoken.ID,
	})
	return nil
}

func (a *echoAnswerer) spoken() []gdesk.Utterance {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]gdesk.Utterance(nil), a.heard...)
}

// newTestListener serves one desk over a real socket.
func newTestListener(t *testing.T, conv gdesk.ConversationID) (*httptest.Server, *Views, *echoAnswerer) {
	t.Helper()
	views := newTestViews(t)
	answerer := &echoAnswerer{views: views, conv: conv}
	listener, err := NewListener(ListenConfig{
		Views:        views,
		Answerer:     answerer,
		Token:        testToken,
		Conversation: conv,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(listener)
	t.Cleanup(server.Close)
	return server, views, answerer
}

// dial connects the way the glass would: the token in the query, because a
// browser cannot set a header on a WebSocket.
func dial(t *testing.T, server *httptest.Server, stream StreamID, token string) *websocket.Conn {
	t.Helper()
	ws, resp, err := websocket.DefaultDialer.Dial(socketURL(server, stream, token), nil)
	if err != nil {
		t.Fatalf("dial: %v (status %v)", err, statusOf(resp))
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}

func socketURL(server *httptest.Server, stream StreamID, token string) string {
	u, _ := url.Parse(server.URL)
	u.Scheme = "ws"
	q := u.Query()
	if stream != "" {
		q.Set("stream", string(stream))
	}
	if token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func statusOf(resp *http.Response) string {
	if resp == nil {
		return "none"
	}
	return resp.Status
}

// readEvent waits for one event the way the glass reads it.
func readEvent(t *testing.T, ws *websocket.Conn) map[string]any {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("event is not JSON: %v", err)
	}
	return event
}

func say(t *testing.T, ws *websocket.Conn, stream StreamID, cmd CommandID, id, text string) {
	t.Helper()
	raw := frame(t, stream, cmd, CommandUtterance, map[string]any{
		"utteranceId": id, "text": text, "source": "typed",
	})
	if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestNewListenerRefusesAListenerThatCannotTellWhoIsCalling(t *testing.T) {
	views := newTestViews(t)
	for _, tc := range []struct {
		name   string
		cfg    ListenConfig
		detail string
	}{
		{"no views", ListenConfig{Answerer: &stubAnswerer{}, Token: testToken}, "desk's views"},
		{"nobody to answer", ListenConfig{Views: views, Token: testToken}, "somebody to answer"},
		{"no token", ListenConfig{Views: views, Answerer: &stubAnswerer{}, Token: "  "}, "needs a token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewListener(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.detail) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.detail)
			}
		})
	}
}

func TestAConnectionWithoutTheTokenIsRefusedBeforeTheUpgrade(t *testing.T) {
	server, views, _ := newTestListener(t, "person")

	for _, token := range []string{"", "sec-wrong"} {
		_, resp, err := websocket.DefaultDialer.Dial(socketURL(server, "gdesk:local", token), nil)
		if err == nil {
			t.Fatal("want the dial refused")
		}
		if resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %v, want 401", statusOf(resp))
		}
	}
	if views.Streams() != 0 {
		t.Fatalf("streams = %d, want a refused caller to bind nothing", views.Streams())
	}
}

func TestTheTokenMayArriveInAHeaderToo(t *testing.T) {
	server, _, _ := newTestListener(t, "person")

	header := http.Header{"Authorization": []string{"Bearer " + testToken}}
	ws, resp, err := websocket.DefaultDialer.Dial(socketURL(server, "gdesk:local", ""), header)
	if err != nil {
		t.Fatalf("dial: %v (status %v)", err, statusOf(resp))
	}
	ws.Close()
}

func TestAConnectionWithoutAStreamIsRefused(t *testing.T) {
	server, _, _ := newTestListener(t, "person")

	_, resp, err := websocket.DefaultDialer.Dial(socketURL(server, "", testToken), nil)
	if err == nil {
		t.Fatal("want the dial refused")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %v, want 400", statusOf(resp))
	}
}

func TestAStreamSomebodyElseIsOnIsRefusedWithAStatus(t *testing.T) {
	server, views, _ := newTestListener(t, "person")
	if _, err := views.Bind("gdesk:local", "somebody-else"); err != nil {
		t.Fatal(err)
	}

	_, resp, err := websocket.DefaultDialer.Dial(socketURL(server, "gdesk:local", testToken), nil)
	if err == nil {
		t.Fatal("want the dial refused")
	}
	if resp == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %v, want 409", statusOf(resp))
	}
}

func TestWhatSheSaysIsAnsweredInTheConversationHerTokenNames(t *testing.T) {
	server, _, answerer := newTestListener(t, "person")
	ws := dial(t, server, "gdesk:local", testToken)

	// The payload carries no conversation and could not be trusted with one:
	// the listener stamps it from the credential.
	say(t, ws, "gdesk:local", "cmd-1", "utt-1", "sprawdź opony")

	event := readEvent(t, ws)
	if event["kind"] != EventSurfaceOpened {
		t.Fatalf("kind = %v, want a reply to open", event["kind"])
	}
	if event["inReplyTo"] != "cmd-1" {
		t.Fatalf("inReplyTo = %v, want the command she sent", event["inReplyTo"])
	}

	heard := answerer.spoken()
	if len(heard) != 1 {
		t.Fatalf("heard %d utterances, want 1", len(heard))
	}
	if heard[0].Conversation != "person" {
		t.Fatalf("conversation = %q, want the one her token names", heard[0].Conversation)
	}
}

func TestAResyncReplaysToThisConnectionOnly(t *testing.T) {
	server, views, _ := newTestListener(t, "person")
	first := dial(t, server, "gdesk:phone", testToken)

	say(t, first, "gdesk:phone", "cmd-1", "utt-1", "cześć")
	readEvent(t, first) // her reply, numbered 1 on this stream

	// A second device of the same person numbers on its own, so its resync
	// replays its own history rather than the phone's.
	second := dial(t, server, "gdesk:glass", testToken)
	raw := frame(t, "gdesk:glass", "cmd-2", CommandResync, map[string]any{"fromSeq": 0})
	if err := second.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Nothing to replay on the glass, so the next thing it reads is the fact
	// that arrives after it connected.
	views.Record("person", gdesk.TurnOpened{Turn: "turn-9", Staff: "staff-adam", Utterance: "utt-9"})
	event := readEvent(t, second)
	if event["seq"] != float64(1) {
		t.Fatalf("seq = %v, want the glass to number from 1", event["seq"])
	}
}

func TestACommandForAnotherStreamIsRefusedOnTheWire(t *testing.T) {
	server, _, answerer := newTestListener(t, "person")
	ws := dial(t, server, "gdesk:local", testToken)

	say(t, ws, "gdesk:somebody-else", "cmd-9", "utt-1", "cześć")

	event := readEvent(t, ws)
	if event["kind"] != EventSurfaceOpened || event["inReplyTo"] != "cmd-9" {
		t.Fatalf("event = %v, want a refusal on the command she sent", event)
	}
	if got := answerer.spoken(); len(got) != 0 {
		t.Fatalf("heard %v, want nothing answered for another stream", got)
	}
}

func TestAMalformedFrameCostsNeitherTheStreamNorTheConnection(t *testing.T) {
	server, views, _ := newTestListener(t, "person")
	ws := dial(t, server, "gdesk:local", testToken)

	if err := ws.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Still on the same stream afterwards: a frame without a command id has no
	// surface to refuse onto, so silence is the documented outcome.
	views.Record("person", gdesk.TurnOpened{Turn: "turn-9", Staff: "staff-adam", Utterance: "utt-9"})
	if event := readEvent(t, ws); event["seq"] != float64(1) {
		t.Fatalf("seq = %v, want the stream to carry on from 1", event["seq"])
	}
}

func TestReconnectingWithTheSameStreamKeepsWhatSheAlreadyHeard(t *testing.T) {
	server, _, _ := newTestListener(t, "person")

	first := dial(t, server, "gdesk:local", testToken)
	say(t, first, "gdesk:local", "cmd-1", "utt-1", "cześć")
	readEvent(t, first)
	first.Close()

	second := dial(t, server, "gdesk:local", testToken)
	raw := frame(t, "gdesk:local", "cmd-2", CommandResync, map[string]any{"fromSeq": 0})
	if err := second.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("write: %v", err)
	}
	if event := readEvent(t, second); event["seq"] != float64(1) {
		t.Fatalf("seq = %v, want the reply she missed replayed", event["seq"])
	}
}

func TestAFactTheConnectorCannotEncodeCostsTheConnectionNothing(t *testing.T) {
	server, views, _ := newTestListener(t, "person")
	ws := dial(t, server, "gdesk:local", testToken)

	view, err := views.Bind("gdesk:local", "person")
	if err != nil {
		t.Fatal(err)
	}
	// A payload no encoder can render is our own defect. It is still numbered,
	// so the glass sees the gap and asks for a resync rather than being told a
	// lie about how much it has.
	view.Log().Append(EventSurfaceAppended, make(chan int), "")
	view.Log().Append(EventSurfaceAppended, surfaceAppended{ID: "surface-1"}, "")

	if event := readEvent(t, ws); event["seq"] != float64(2) {
		t.Fatalf("seq = %v, want the numbering to carry past what we could not encode", event["seq"])
	}
}

func TestATurnThatFailsLeavesTheConnectionOpen(t *testing.T) {
	views := newTestViews(t)
	listener, err := NewListener(ListenConfig{
		Views:        views,
		Answerer:     &stubAnswerer{err: errors.New("provider gone")},
		Token:        testToken,
		Conversation: "person",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(listener)
	t.Cleanup(server.Close)

	ws := dial(t, server, "gdesk:local", testToken)
	say(t, ws, "gdesk:local", "cmd-1", "utt-1", "cześć")

	// The desk records its own failure; the error the runner returns is for the
	// connector's log. She is still connected and can say the next thing.
	views.Record("person", gdesk.TurnOpened{Turn: "turn-2", Staff: "staff-ania", Utterance: "utt-2"})
	if event := readEvent(t, ws); event["kind"] != EventSurfaceOpened {
		t.Fatalf("kind = %v, want the next reply to open", event["kind"])
	}
}

func TestAPageFromTheOpenWebCannotReachTheDesk(t *testing.T) {
	for _, tc := range []struct {
		origin string
		want   bool
	}{
		{"", true},
		{"http://localhost:5178", true},
		{"http://127.0.0.1:5178", true},
		{"http://[::1]:5178", true},
		{"https://example.com", false},
		{"http://192.168.1.10:5178", false},
		{"::not a url", false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if got := fromThisMachine(r); got != tc.want {
			t.Fatalf("origin %q allowed = %v, want %v", tc.origin, got, tc.want)
		}
	}
}
