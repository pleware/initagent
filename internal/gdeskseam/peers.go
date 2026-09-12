package gdeskseam

import (
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
)

// Peer is one caller on the seam right now, and where its page came from when
// the caller is a browser.
//
// The desk cannot be told the desktop's address: in development it is a Vite
// port that moves, and a packaged glass has no address at all. What a browser
// does say, on every WebSocket handshake, is the origin its page was loaded
// from — so an address here is evidence about a live connection rather than a
// setting, which is why it is recorded only after the upgrade (fromThisMachine
// has run by then) and forgotten when the socket closes.
//
// Origin is empty for a caller that is not a browser: the shell's own client, a
// test, a CLI. That is a connection with no address, not a connection to hide —
// it still counts on the seam's row in the console's service list.
type Peer struct {
	Stream StreamID `json:"stream"`
	Origin string   `json:"origin,omitempty"`
}

// peers is who is on the seam right now, one entry per stream.
//
// One per stream and not a list, because Views.Bind already refuses a second
// connection on a bound stream: two entries would describe a state the seam
// does not allow.
type peers struct {
	mu sync.Mutex
	by map[StreamID]string
}

// note records an accepted connection, with its origin when that origin is
// something this desk would render as a link. An address it would not render is
// dropped rather than kept as text — the connection is still counted.
func (p *peers) note(stream StreamID, origin string) {
	safe, _ := browserOrigin(origin)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.by == nil {
		p.by = make(map[StreamID]string)
	}
	p.by[stream] = safe
}

// forget drops a stream when its socket goes. A link to a desktop that has
// exited is worse than no link: it looks like the glass is up.
func (p *peers) forget(stream StreamID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.by, stream)
}

// count is how many callers are on the seam. It is the number the console puts
// against the port, so it counts every connection and not only the ones that
// came with an address.
func (p *peers) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.by)
}

// list is who is on the seam, ordered by stream so a poll every second does
// not reshuffle what somebody is reading.
func (p *peers) list() []Peer {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Peer, 0, len(p.by))
	for stream, origin := range p.by {
		out = append(out, Peer{Stream: stream, Origin: origin})
	}
	slices.SortFunc(out, func(a, b Peer) int { return strings.Compare(string(a.Stream), string(b.Stream)) })
	return out
}

// browserOrigin narrows a claimed Origin to something safe to hand back as a
// link, and rebuilds it from the parsed parts so nothing else rides along.
//
// Two refusals matter. A scheme that is not http or https can be `javascript:`,
// and a link the operator clicks is the one place where that would run. A host
// that is not this machine cannot be a desktop of ours whatever it claims —
// the same rule the upgrade already applies, repeated here because this value
// leaves the process.
func browserOrigin(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	host := u.Hostname()
	if !strings.EqualFold(host, "localhost") {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return "", false
		}
	}
	return u.Scheme + "://" + u.Host, true
}
