package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/protocol"
)

// handleTermWS bridges a hub (or test) client to a connector session.
// Draft 16: the browser stays on the hub origin; this is the gateway hop.
func (g *Gateway) handleTermWS(w http.ResponseWriter, r *http.Request) {
	projectID, ok := g.resolveProject(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	deviceID := q.Get("device")
	session := q.Get("session")
	cols := clampInt(q.Get("cols"), 80, 10, 500)
	rows := clampInt(q.Get("rows"), 24, 5, 300)
	if deviceID == "" || session == "" {
		httpError(w, http.StatusBadRequest, "device and session required")
		return
	}
	c := g.connForProject(projectID, deviceID)
	if c == nil {
		httpError(w, http.StatusServiceUnavailable, "device is offline")
		return
	}
	client, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer client.Close()

	var clientMu sync.Mutex
	sendClient := func(msgType int, data []byte) error {
		clientMu.Lock()
		defer clientMu.Unlock()
		return client.WriteMessage(msgType, data)
	}
	exit := make(chan string, 2)

	ch := c.openChannel(&termChannel{
		onBinary: func(p []byte) {
			if err := sendClient(websocket.BinaryMessage, p); err != nil {
				select {
				case exit <- "":
				default:
				}
			}
		},
		onControl: func(m protocol.Msg) {
			if m.Type == protocol.TypeTermExit {
				select {
				case exit <- m.Error:
				default:
				}
			}
		},
	})
	defer c.closeChannel(ch)
	defer func() {
		closeMsg, _ := protocol.NewMsg(protocol.TypeTermClose, 0, ch, nil)
		_ = c.sendJSON(closeMsg)
	}()

	open, _ := protocol.NewMsg(protocol.TypeTermOpen, 0, ch, protocol.TermOpen{Session: session, Cols: cols, Rows: rows})
	if err := c.sendJSON(open); err != nil {
		_ = sendClient(websocket.TextMessage, exitJSON("device connection lost"))
		return
	}

	go func() {
		for {
			msgType, data, err := client.ReadMessage()
			if err != nil {
				select {
				case exit <- "":
				default:
				}
				return
			}
			switch msgType {
			case websocket.BinaryMessage:
				if err := c.sendBinary(ch, data); err != nil {
					return
				}
			case websocket.TextMessage:
				var m struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if json.Unmarshal(data, &m) == nil && m.Type == "resize" {
					resize, _ := protocol.NewMsg(protocol.TypeTermResize, 0, ch, protocol.TermResize{Cols: m.Cols, Rows: m.Rows})
					_ = c.sendJSON(resize)
				}
			}
		}
	}()

	errMsg := <-exit
	_ = sendClient(websocket.TextMessage, exitJSON(errMsg))
}

func exitJSON(errMsg string) []byte {
	b, _ := json.Marshal(map[string]string{"type": "exit", "error": errMsg})
	return b
}

func clampInt(s string, def, lo, hi int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return min(hi, max(lo, n))
}
