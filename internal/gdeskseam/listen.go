package gdeskseam

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/gdesk"
)

// upgrader matches the house sizes (internal/hub/ws.go). The buffers are large
// because a reply arrives as many small chunks and a resync as a burst.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin:     fromThisMachine,
}

// Path is where the glass-desk seam is mounted on the connector's local address.
// One route, because the seam is one socket: a second path would be a second
// contract for the glass to learn.
const Path = "/gdesk"

// LogsPath is the operator ring. It is not a second seam — nobody talks to the
// desk here — and it is what the operator's console polls once a second.
const LogsPath = Path + "/logs"

const (
	// writeDeadline is how long one frame may take to leave, matching the
	// connector's agent link (internal/agent/agent.go).
	writeDeadline = 30 * time.Second

	// pingEvery keeps a quiet desk open through anything counting idle
	// sockets, and is how a half-open connection is noticed at all.
	pingEvery = 30 * time.Second

	// maxFrame bounds one inbound frame. An utterance is capped in
	// gdesk.MaxUtteranceChars; this is that plus room for an envelope, so a
	// caller cannot make the connector hold a megabyte to be told it is
	// malformed.
	maxFrame = 64 * 1024

	// MaxTurnsWaiting is how many sentences may wait for the desk on one
	// connection before it stops reading.
	//
	// Bounded rather than unbounded because each waiting turn is a goroutine's
	// worth of state the caller chose; small because the desk answers a
	// conversation one turn at a time, so a deep queue is a person talking
	// into a backlog she cannot see.
	MaxTurnsWaiting = 32
)

// fromThisMachine allows the glass and header-less clients, and nothing else.
//
// The hub compares Origin against its own Host, which cannot work here: the
// glass is served by Vite on another port, so every legitimate connection is
// cross-origin by that test. Loopback is the substitute, and it is what keeps
// a page on the open web from reaching a desk on somebody's machine even
// though the port is easy to guess.
func fromThisMachine(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // not a browser: the shell's own client, a test, a CLI
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Hostname(), "localhost") {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

// ListenConfig is what the local listener needs. Every field is required.
type ListenConfig struct {
	// Views is the desk as its connections see it: one numbered view each.
	Views *Views
	// Answerer runs a turn. *gdesk.Runner is one.
	Answerer Answerer

	// Token is the shared secret every connection presents.
	//
	// It comes from the environment, never from a flag: a flag lands in `ps`
	// output and in shell history, and this one opens somebody's desk.
	Token string

	// Conversation is whose talk a caller holding that token joins.
	//
	// Empty is gdesk.DefaultConversation, which is the person at this machine.
	Conversation gdesk.ConversationID

	// Trace is the operator ring the console polls. Nil is fine: stdout still
	// gets the same lines, the dump just has nothing to say.
	Trace *Trace

	// Services is asked, on every dump, what this box runs.
	//
	// A function and not a value because a list read at boot would be a
	// snapshot: a role can be silent, a client count changes by the second.
	// Nil is a desk that reports no inventory, which is what a test wants.
	Services func() []Service
}

// Listener is the connector's local half of the seam: one WebSocket per
// connection, bound to the loopback.
//
// Whose conversation a connection belongs to comes from the token it presents,
// never from the message it sends — a caller that could name its own
// conversation could name somebody else's and be answered inside her
// transcript (docs/GDESK-SCOPES.md). A second person therefore does not arrive
// as a second token here. She arrives through the hub as a relay, over the
// link the connector already dials outward, and the hub says who she is.
type Listener struct {
	views    *Views
	answerer Answerer
	token    string
	conv     gdesk.ConversationID
	trace    *Trace
	services func() []Service
	peers    peers
}

// NewListener refuses a listener that cannot serve or cannot tell who is
// calling.
func NewListener(cfg ListenConfig) (*Listener, error) {
	switch {
	case cfg.Views == nil:
		return nil, fmt.Errorf("%w: listener needs the desk's views", ErrSeam)
	case cfg.Answerer == nil:
		return nil, fmt.Errorf("%w: listener needs somebody to answer", ErrSeam)
	case strings.TrimSpace(cfg.Token) == "":
		return nil, fmt.Errorf("%w: listener needs a token", ErrSeam)
	}
	return &Listener{
		views:    cfg.Views,
		answerer: cfg.Answerer,
		token:    cfg.Token,
		conv:     cfg.Conversation,
		trace:    cfg.Trace,
		services: cfg.Services,
	}, nil
}

// ServeHTTP accepts one connection. Everything that can be refused with a
// status code is refused before the upgrade, so a caller reads a reason
// instead of a socket that closes without one.
func (l *Listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !l.admits(r) {
		// Deliberately without detail: which part of the credential was wrong
		// is not the caller's business.
		http.Error(w, "gdesk: unknown token", http.StatusUnauthorized)
		return
	}
	stream := StreamID(strings.TrimSpace(r.URL.Query().Get("stream")))
	if stream == "" {
		http.Error(w, "gdesk: stream required", http.StatusBadRequest)
		return
	}
	view, err := l.views.Bind(stream, l.conv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	socket, err := NewSocket(SocketConfig{View: view, Answerer: l.answerer, Trace: l.trace})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade has already answered the request.
	}
	// After the upgrade, so an origin this desk would have refused never
	// becomes something the console offers to open, and only for as long as
	// the socket lives.
	l.peers.note(stream, r.Header.Get("Origin"))
	defer l.peers.forget(stream)
	Talk(ws, socket, view)
}

// Clients is how many callers are on the seam. It is what the console puts
// against the port, and it is a live number: a caller that closed its socket is
// already gone from it.
func (l *Listener) Clients() int { return l.peers.count() }

// ServeLogs dumps the operator ring, who is on the seam, and what this box
// runs. Same token as the websocket, so a page on the open web that guessed the
// port still cannot read what she said.
//
// All three ride along here rather than on routes of their own because the
// console already polls this one every second: another endpoint would be
// another thing to authenticate and another thing to keep in step, for
// questions that are only ever asked beside "what has the desk been doing".
func (l *Listener) ServeLogs(w http.ResponseWriter, r *http.Request) {
	if !l.admits(r) {
		http.Error(w, "gdesk: unknown token", http.StatusUnauthorized)
		return
	}
	dump := l.trace.Dump()
	dump.Peers = l.peers.list()
	if l.services != nil {
		dump.Services = l.services()
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dump); err != nil {
		noteTrace(l.trace, "error", "gdesk: logs encode: %v", err)
	}
}

// admits compares the presented token in constant time.
//
// Two places to present it, because a browser cannot set a header on a
// WebSocket: the query string for the glass, Authorization for the shell, the
// CLI and tests.
func (l *Listener) admits(r *http.Request) bool {
	presented := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(presented, "Bearer "); ok {
		presented = after
	} else {
		presented = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(l.token)) == 1
}

// Talk runs one connection until either side stops.
//
// Three goroutines and only one of them writes, because a WebSocket has a
// single writer: events, replays and pings all leave from the loop here.
// Reading is separate so a long turn cannot stop a resync, and turns run on a
// third in the order she spoke them.
//
// Exported because the relay serves the same conversation over a different
// transport: only how the caller is authenticated differs.
func Talk(ws *websocket.Conn, socket *Socket, view *View) {
	defer ws.Close()

	reader := view.Feed().Subscribe()
	defer reader.Close()
	noteTrace(socket.trace, "info", "gdesk: stream %s connected", view.Log().Stream())

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	replays := make(chan []Event, 1)
	turns := make(chan gdesk.Utterance, MaxTurnsWaiting)

	go answerTurns(ctx, socket, turns)
	go readCommands(ctx, ws, socket, turns, replays, stop)

	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reader.Ready():
			if !writeEvents(ws, reader.Drain(), socket.trace) {
				return
			}
		case events := <-replays:
			if !writeEvents(ws, events, socket.trace) {
				return
			}
		case <-ping.C:
			ws.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// writeEvents sends what was queued and reports whether the connection lived.
func writeEvents(ws *websocket.Conn, events []Event, trace *Trace) bool {
	for _, event := range events {
		raw, err := event.Encode()
		if err != nil {
			// Our own defect, and nothing the glass could do with it. It does
			// not cost the connection: the numbering continues, and the glass
			// closes the gap with a resync.
			noteTrace(trace, "error", "%v", err)
			continue
		}
		ws.SetWriteDeadline(time.Now().Add(writeDeadline))
		if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
			return false
		}
	}
	return true
}

// readCommands classifies what arrives and hands the work on.
func readCommands(
	ctx context.Context,
	ws *websocket.Conn,
	socket *Socket,
	turns chan<- gdesk.Utterance,
	replays chan<- []Event,
	stop func(),
) {
	defer stop()

	ws.SetReadLimit(maxFrame)
	// A peer that stops answering pings is gone, and its stream would
	// otherwise stay bound to a socket nobody is on the other end of.
	ws.SetReadDeadline(time.Now().Add(2 * pingEvery))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(2 * pingEvery))
	})

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}
		// Any frame proves the peer is there, pong or not.
		ws.SetReadDeadline(time.Now().Add(2 * pingEvery))

		intent := socket.Dispatch(raw)
		if len(intent.Replay) > 0 {
			select {
			case replays <- intent.Replay:
			case <-ctx.Done():
				return
			}
		}
		if intent.Answer != nil {
			// Blocks once MaxTurnsWaiting are queued rather than refusing.
			// Refusing would be a second answer to a queue the seam already
			// bounds, and the desk answers one conversation at a time anyway:
			// what is waiting here is her own talking, not somebody else's.
			select {
			case turns <- *intent.Answer:
			case <-ctx.Done():
				return
			}
		}
	}
}

// answerTurns runs one turn at a time, in the order she spoke.
func answerTurns(ctx context.Context, socket *Socket, turns <-chan gdesk.Utterance) {
	for {
		select {
		case <-ctx.Done():
			return
		case spoken := <-turns:
			if err := socket.Answer(ctx, spoken); err != nil {
				// Already a fact on her stream: the desk records the failure
				// before returning. This line is the connector's own log.
				noteTrace(socket.trace, "error", "gdesk: utterance %s: %v", spoken.ID, err)
				continue
			}
			noteTrace(socket.trace, "info", "gdesk: utterance %s answered", spoken.ID)
		}
	}
}
