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

func TestTermWSBridgesAgentOutput(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)

	go func() {
		for {
			var m protocol.Msg
			if err := agent.ReadJSON(&m); err != nil {
				return
			}
			if m.Type != protocol.TypeTermOpen {
				continue
			}
			frame := protocol.EncodeFrame(m.Channel, []byte("hello-term"))
			if err := agent.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
			exit, err := protocol.NewMsg(protocol.TypeTermExit, 0, m.Channel, nil)
			if err != nil {
				return
			}
			_ = agent.WriteJSON(exit)
		}
	}()

	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/term?device=" + deviceID + "&session=term-1&cols=80&rows=24"
	client, resp, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial term: %v (resp=%v)", err, resp)
	}
	t.Cleanup(func() { _ = client.Close() })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = client.SetReadDeadline(time.Now().Add(time.Second))
		msgType, data, err := client.ReadMessage()
		if err != nil {
			break
		}
		if msgType == websocket.BinaryMessage && string(data) == "hello-term" {
			return
		}
	}
	t.Fatal("browser never received agent output")
}

func TestTermWSOfflineStatus(t *testing.T) {
	g := openTest(t, "")
	deviceID, _, err := g.Store().CreateDevice(t.Context(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/term?device=" + deviceID + "&session=term-1"
	_, resp, err := websocket.DefaultDialer.Dial(u, nil)
	if err == nil {
		t.Fatal("wanted offline device to refuse the socket")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %v, want 503", resp)
	}
}
