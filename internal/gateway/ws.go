package gateway

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/protocol"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == ""
	},
}

func (g *Gateway) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		httpError(w, http.StatusUnauthorized, "missing device token")
		return
	}
	device, err := g.store.DeviceByToken(r.Context(), strings.TrimPrefix(auth, "Bearer "))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if device == nil {
		httpError(w, http.StatusForbidden, "unknown device token")
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	ws.SetReadDeadline(time.Now().Add(15 * time.Second))
	var hello protocol.Msg
	if err := ws.ReadJSON(&hello); err != nil || hello.Type != protocol.TypeHello {
		return
	}
	var h protocol.Hello
	_ = json.Unmarshal(hello.Data, &h)
	ws.SetReadDeadline(time.Time{})

	_ = g.store.UpdateDeviceOnConnect(r.Context(), device.ID, h.Hostname, h.OS, h.Arch)
	g.markOnline(device.ID, h)
	defer g.markOffline(device.ID)

	welcome, err := protocol.NewMsg(protocol.TypeWelcome, 0, 0, protocol.Welcome{
		DeviceId: device.ID,
		Version:  g.version,
		Repo:     g.githubRepo,
	})
	if err != nil {
		return
	}
	if err := ws.WriteJSON(welcome); err != nil {
		return
	}
	log.Printf("device %s (%s) connected", device.Name, device.ID)

	ws.SetReadLimit(16 * 1024 * 1024)
	for {
		msgType, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage {
			continue
		}
		var m protocol.Msg
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if m.Type == protocol.TypeStats {
			var st protocol.Stats
			if err := json.Unmarshal(m.Data, &st); err == nil {
				g.setStats(device.ID, &st)
			}
		}
	}
}

func (g *Gateway) markOnline(id string, hello protocol.Hello) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.online[id]
	p.hello = hello
	g.online[id] = p
}

func (g *Gateway) markOffline(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.online, id)
}

func (g *Gateway) setStats(id string, st *protocol.Stats) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.online[id]
	p.stats = st
	g.online[id] = p
}

func (g *Gateway) presence(id string) (presence, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p, ok := g.online[id]
	return p, ok
}
