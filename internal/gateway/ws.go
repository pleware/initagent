package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/deviceops"
	"github.com/pleware/initagent/internal/protocol"
	"github.com/pleware/initagent/internal/updater"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == ""
	},
}

type termChannel struct {
	onBinary  func([]byte)
	onControl func(protocol.Msg)
}

type agentConn struct {
	ws          *websocket.Conn
	writeMu     sync.Mutex
	mu          sync.Mutex
	nextID      uint64
	pending     map[uint64]chan protocol.Msg
	channels    map[uint32]*termChannel
	nextChannel atomic.Uint32
}

func newAgentConn(ws *websocket.Conn) *agentConn {
	return &agentConn{
		ws:       ws,
		pending:  map[uint64]chan protocol.Msg{},
		channels: map[uint32]*termChannel{},
	}
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
	ac := newAgentConn(ws)
	g.attachConn(device.ID, device.ProjectID, h, ac)
	defer g.markOffline(device.ID)

	welcome, err := protocol.NewMsg(protocol.TypeWelcome, 0, 0, protocol.Welcome{
		DeviceId: device.ID,
		Version:  g.joiner.Version,
		Repo:     g.joiner.GithubRepo,
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
		switch msgType {
		case websocket.TextMessage:
			var m protocol.Msg
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			switch {
			case m.Type == protocol.TypeStats:
				var st protocol.Stats
				if err := json.Unmarshal(m.Data, &st); err == nil {
					g.setStats(device.ID, &st)
				}
			case m.Type == protocol.TypeResult:
				ac.deliver(m)
			case m.Channel != 0:
				ac.control(m)
			}
		case websocket.BinaryMessage:
			ch, payload, err := protocol.DecodeFrame(data)
			if err != nil {
				continue
			}
			ac.binary(ch, payload)
		}
	}
}

func (c *agentConn) deliver(m protocol.Msg) {
	c.mu.Lock()
	ch, ok := c.pending[m.Id]
	c.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- m:
	default:
	}
}

func (c *agentConn) closePending() {
	c.mu.Lock()
	pending := c.pending
	c.pending = map[uint64]chan protocol.Msg{}
	channels := c.channels
	c.channels = map[uint32]*termChannel{}
	c.mu.Unlock()
	msg := protocol.Msg{Type: protocol.TypeResult, Error: "device disconnected"}
	for _, ch := range pending {
		select {
		case ch <- msg:
		default:
		}
	}
	exit := protocol.Msg{Type: protocol.TypeTermExit, Error: "device disconnected"}
	for _, h := range channels {
		if h.onControl != nil {
			h.onControl(exit)
		}
	}
}

func (c *agentConn) binary(channel uint32, payload []byte) {
	c.mu.Lock()
	h := c.channels[channel]
	c.mu.Unlock()
	if h != nil && h.onBinary != nil {
		h.onBinary(payload)
	}
}

func (c *agentConn) control(m protocol.Msg) {
	c.mu.Lock()
	h := c.channels[m.Channel]
	c.mu.Unlock()
	if h != nil && h.onControl != nil {
		h.onControl(m)
	}
}

func (c *agentConn) openChannel(h *termChannel) uint32 {
	id := c.nextChannel.Add(1)
	c.mu.Lock()
	c.channels[id] = h
	c.mu.Unlock()
	return id
}

func (c *agentConn) closeChannel(id uint32) {
	c.mu.Lock()
	delete(c.channels, id)
	c.mu.Unlock()
}

func (c *agentConn) sendJSON(m protocol.Msg) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteJSON(m)
}

func (c *agentConn) sendBinary(channel uint32, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteMessage(websocket.BinaryMessage, protocol.EncodeFrame(channel, payload))
}

func (c *agentConn) call(ctx context.Context, typ string, payload any) (protocol.Msg, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan protocol.Msg, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	m, err := protocol.NewMsg(typ, id, 0, payload)
	if err != nil {
		return protocol.Msg{}, err
	}
	c.writeMu.Lock()
	c.ws.SetWriteDeadline(time.Now().Add(30 * time.Second))
	err = c.ws.WriteJSON(m)
	c.writeMu.Unlock()
	if err != nil {
		return protocol.Msg{}, err
	}

	select {
	case <-ctx.Done():
		return protocol.Msg{}, ctx.Err()
	case reply := <-ch:
		if reply.Error != "" {
			return reply, errors.New(reply.Error)
		}
		return reply, nil
	}
}

func (c *agentConn) callInto(ctx context.Context, typ string, payload, out any) error {
	reply, err := c.call(ctx, typ, payload)
	if err != nil {
		return err
	}
	if out != nil && len(reply.Data) > 0 {
		return json.Unmarshal(reply.Data, out)
	}
	return nil
}

// The exported methods below adapt *agentConn to deviceops.Conn so the
// gateway and the hub share one implementation of exec, fs, and setup
// probing (10/16).

func (c *agentConn) Call(ctx context.Context, typ string, payload, out any) error {
	return c.callInto(ctx, typ, payload, out)
}

func (c *agentConn) OpenChannel(h *deviceops.Channel) uint32 {
	return c.openChannel(&termChannel{onBinary: h.OnBinary, onControl: h.OnControl})
}

func (c *agentConn) CloseChannel(id uint32) { c.closeChannel(id) }

func (c *agentConn) SendJSON(m protocol.Msg) error { return c.sendJSON(m) }

func (c *agentConn) SendBinary(channel uint32, payload []byte) error {
	return c.sendBinary(channel, payload)
}

func (g *Gateway) attachConn(id, projectID string, hello protocol.Hello, c *agentConn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.online[id]
	p.projectID = projectID
	p.hello = hello
	p.conn = c
	p.draining = updater.IsNewer(g.joiner.Version, hello.Version)
	g.online[id] = p
}

func (g *Gateway) markOffline(id string) {
	g.mu.Lock()
	p, ok := g.online[id]
	delete(g.online, id)
	g.mu.Unlock()
	if ok && p.conn != nil {
		p.conn.closePending()
	}
}

func (g *Gateway) connFor(id string) *agentConn {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.online[id].conn
}

// connForProject returns the socket only when the device belongs to
// projectID. Without the check a caller naming another project's dev- would
// reach that machine, which is the isolation guarantee in 01.
func (g *Gateway) connForProject(projectID, id string) *agentConn {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.online[id]
	if p.projectID != projectID {
		return nil
	}
	return p.conn
}

// firstOnlineID picks a connected machine belonging to projectID that is
// not draining. An outdated connector stays attached so its current run can
// finish, but it takes no new work (`10`, `11`).
func (g *Gateway) firstOnlineID(projectID string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, p := range g.online {
		if p.conn != nil && p.projectID == projectID && !p.draining {
			return id
		}
	}
	return ""
}

func (g *Gateway) draining(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.online[id].draining
}

func (g *Gateway) hasDraining(projectID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range g.online {
		if p.conn != nil && p.projectID == projectID && p.draining {
			return true
		}
	}
	return false
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
