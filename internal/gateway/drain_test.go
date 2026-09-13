package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/protocol"
)

func TestOutdatedConnectorTakesNoNewTask(t *testing.T) {
	g, err := Open(Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201", Version: "v0.3.9"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })

	connectorID, _, ts := connectAgent(t, g, protocol.Hello{
		Hostname: "box", OS: "linux", Version: "v0.3.8",
	})
	if !g.draining(connectorID) {
		t.Fatal("outdated hello should drain")
	}

	rec := postTask(t, ts, map[string]string{"command": "echo hi", "connectorId": connectorID})
	if rec.Code != 409 {
		t.Fatalf("named drain status = %d %s", rec.Code, rec.Body.String())
	}

	rec = postTask(t, ts, map[string]string{"command": "echo hi"})
	if rec.Code != 409 {
		t.Fatalf("auto pick drain status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestMatchingVersionIsClaimable(t *testing.T) {
	g, err := Open(Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201", Version: "v0.3.9"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })

	connectorID, conn, ts := connectAgent(t, g, protocol.Hello{
		Hostname: "box", OS: "linux", Version: "v0.3.9",
	})
	if g.draining(connectorID) {
		t.Fatal("current hello should not drain")
	}
	replyExec(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "echo hi", "connectorId": connectorID})
	if rec.Code != 200 {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestFirstOnlineSkipsDrainingConnector(t *testing.T) {
	g, err := Open(Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201", Version: "v0.3.9"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })

	oldID, _, _ := connectAgent(t, g, protocol.Hello{
		Hostname: "old", OS: "linux", Version: "v0.3.8",
	})
	currentID, conn, ts := connectAgent(t, g, protocol.Hello{
		Hostname: "new", OS: "linux", Version: "v0.3.9",
	})
	if oldID == currentID {
		t.Fatal("expected two connectors")
	}
	replyExec(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "echo hi"})
	if rec.Code != 200 {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	picked := g.firstOnlineID(g.Project().ID)
	if picked != currentID {
		t.Fatalf("firstOnline = %q, want current %q (old=%q)", picked, currentID, oldID)
	}
}

func TestMatchingHelloClearsDrain(t *testing.T) {
	g, err := Open(Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201", Version: "v0.3.9"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })

	connectorID, token, err := g.Store().CreateConnector(t.Context(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)

	old := dialHello(t, ts, token, protocol.Hello{Hostname: "box", OS: "linux", Version: "v0.3.8"})
	waitAttached(t, g, connectorID)
	if !g.draining(connectorID) {
		t.Fatal("outdated hello should drain")
	}

	_ = old.Close()
	waitDetached(t, g, connectorID)

	conn := dialHello(t, ts, token, protocol.Hello{Hostname: "box", OS: "linux", Version: "v0.3.9"})
	waitAttached(t, g, connectorID)
	if g.draining(connectorID) {
		t.Fatal("matching hello should clear drain")
	}
	replyExec(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "echo hi", "connectorId": connectorID})
	if rec.Code != 200 {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func dialHello(t *testing.T, ts *httptest.Server, token string, hello protocol.Hello) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/agent"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	msg, _ := protocol.NewMsg(protocol.TypeHello, 0, 0, hello)
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatal(err)
	}
	var welcome protocol.Msg
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}
	if welcome.Type != protocol.TypeWelcome {
		t.Fatalf("welcome = %+v", welcome)
	}
	return conn
}

func waitAttached(t *testing.T, g *Gateway, connectorID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if g.connFor(connectorID) != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("agent never attached")
}

func waitDetached(t *testing.T, g *Gateway, connectorID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if g.connFor(connectorID) == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("agent still attached")
}
