package gdeskseam

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/gdesk"
)

func TestAnOriginIsNarrowedToWhatMayBeOfferedAsALink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		origin string
		want   string
	}{
		{"the desktop in development", "http://localhost:5178", "http://localhost:5178"},
		{"loopback by address", "http://127.0.0.1:4202", "http://127.0.0.1:4202"},
		{"loopback in six", "https://[::1]:8443", "https://[::1]:8443"},
		{"a path is not part of an origin", "http://localhost:5178/desk?x=1#f", "http://localhost:5178"},
		{"credentials do not survive", "http://user:secret@localhost:5178", "http://localhost:5178"},
		{"no header at all", "", ""},
		{"a packaged glass", "file://", ""},
		{"the one scheme a link must never carry", "javascript:alert(1)", ""},
		{"a page on the open web", "https://example.test", ""},
		{"a name that only looks local", "http://localhost.example.test", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := browserOrigin(tc.origin)
			if tc.want == "" {
				if ok {
					t.Fatalf("browserOrigin(%q) = %q, want refused", tc.origin, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("browserOrigin(%q) = %q, %v; want %q", tc.origin, got, ok, tc.want)
			}
		})
	}
}

// TestTheConsoleLearnsWhereTheDesktopIsFromItsHandshake is the whole reason
// peers exist: nothing configures the desktop's address here, so the only
// place it can come from is the browser that joined.
func TestTheConsoleLearnsWhereTheDesktopIsFromItsHandshake(t *testing.T) {
	t.Parallel()
	listener, server := listenerWithLogs(t)

	ws := dialFrom(t, server, "gdesk:local", "http://localhost:5178")

	peers := peersInDump(t, listener)
	if len(peers) != 1 {
		t.Fatalf("peers = %#v, want one", peers)
	}
	if peers[0].Stream != "gdesk:local" || peers[0].Origin != "http://localhost:5178" {
		t.Fatalf("peer = %#v", peers[0])
	}

	// And gone when the socket goes: a link to a desktop that has exited reads
	// as a desktop that is up.
	ws.Close()
	deadline := time.Now().Add(2 * time.Second)
	for len(peersInDump(t, listener)) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("peer outlived its socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAClientWithNoAddressStillCounts is the difference between the two
// questions the dump answers. "Where is the desktop" needs an address and only
// a browser has one; "how many callers are on this port" counts the shell's own
// client, a test and a CLI too, or the number under-reports the desk.
func TestAClientWithNoAddressStillCounts(t *testing.T) {
	t.Parallel()
	listener, server := listenerWithLogs(t)

	dialFrom(t, server, "gdesk:local", "")

	peers := peersInDump(t, listener)
	if len(peers) != 1 || peers[0].Origin != "" {
		t.Fatalf("peers = %#v, want one connection with no address", peers)
	}
	if got := listener.Clients(); got != 1 {
		t.Fatalf("Clients() = %d, want 1", got)
	}
}

// TestAnAddressTheDeskWouldNotOfferIsDroppedAndTheCallerStillCounts keeps the
// two apart in the one case that matters: a caller that claimed something this
// desk must never hand back as a link.
func TestAnAddressTheDeskWouldNotOfferIsDroppedAndTheCallerStillCounts(t *testing.T) {
	t.Parallel()
	listener := listenerOnly(t)

	listener.peers.note("gdesk:local", "javascript:alert(1)")

	peers := listener.peers.list()
	if len(peers) != 1 || peers[0].Origin != "" {
		t.Fatalf("peers = %#v, want one counted connection with no address", peers)
	}
}

// TestADeskWithNoInventoryReportsNone is the nil case: a listener nobody handed
// a service list to answers without one rather than crashing a poll.
func TestADeskWithNoInventoryReportsNone(t *testing.T) {
	t.Parallel()
	listener, _ := listenerWithLogs(t)

	if dump := dumpOf(t, listener); len(dump.Services) != 0 {
		t.Fatalf("services = %#v, want none", dump.Services)
	}
}

// TestTheInventoryIsReadWhenTheDumpIsTaken is why Services is a function: a
// list read once at boot would report a client count and a silence from
// whenever the desk started.
func TestTheInventoryIsReadWhenTheDumpIsTaken(t *testing.T) {
	t.Parallel()
	views := newTestViews(t)
	conv := gdesk.ConversationID("conversation-inventory")
	asked := 0
	listener, err := NewListener(ListenConfig{
		Views:        views,
		Answerer:     &echoAnswerer{views: views, conv: conv},
		Token:        testToken,
		Conversation: conv,
		Services: func() []Service {
			asked++
			return []Service{{Name: "gdesk seam", Where: "127.0.0.1:4202/gdesk", State: ServiceListening}}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := dumpOf(t, listener)
	second := dumpOf(t, listener)

	if asked != 2 {
		t.Fatalf("inventory asked %d times, want one per dump", asked)
	}
	if len(first.Services) != 1 || first.Services[0].Name != "gdesk seam" {
		t.Fatalf("services = %#v", first.Services)
	}
	if len(second.Services) != 1 {
		t.Fatalf("second dump lost the inventory: %#v", second.Services)
	}
}

// listenerWithLogs is one desk reachable two ways, the way the connector mounts
// it: a socket to dial, and the dump the console polls.
func listenerWithLogs(t *testing.T) (*Listener, *httptest.Server) {
	t.Helper()
	views := newTestViews(t)
	conv := gdesk.ConversationID("conversation-peers")
	listener, err := NewListener(ListenConfig{
		Views:        views,
		Answerer:     &echoAnswerer{views: views, conv: conv},
		Token:        testToken,
		Conversation: conv,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(listener)
	t.Cleanup(server.Close)
	return listener, server
}

// dialFrom connects claiming to be a page at origin. An empty origin is a
// client that is not a browser at all.
func dialFrom(t *testing.T, server *httptest.Server, stream StreamID, origin string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	ws, resp, err := websocket.DefaultDialer.Dial(socketURL(server, stream, testToken), header)
	if err != nil {
		t.Fatalf("dial: %v (status %v)", err, statusOf(resp))
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}

// listenerOnly is a desk with no socket to dial, for a question about the book
// itself rather than about a connection.
func listenerOnly(t *testing.T) *Listener {
	t.Helper()
	views := newTestViews(t)
	conv := gdesk.ConversationID("conversation-book")
	listener, err := NewListener(ListenConfig{
		Views:        views,
		Answerer:     &echoAnswerer{views: views, conv: conv},
		Token:        testToken,
		Conversation: conv,
	})
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func peersInDump(t *testing.T, listener *Listener) []Peer {
	t.Helper()
	return dumpOf(t, listener).Peers
}

func dumpOf(t *testing.T, listener *Listener) TraceDump {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, LogsPath+"?token="+testToken, nil)
	w := httptest.NewRecorder()
	listener.ServeLogs(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("logs status = %d", w.Code)
	}
	var dump TraceDump
	if err := json.Unmarshal(w.Body.Bytes(), &dump); err != nil {
		t.Fatalf("dump is not JSON: %v", err)
	}
	return dump
}
