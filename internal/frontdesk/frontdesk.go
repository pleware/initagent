// Package frontdesk opens one front desk on this box.
//
// Three packages, three jobs, and this is the only one that knows all of
// them: `desk` is the vocabulary and the provider inventory, `deskseam` is the
// wire the glass reads, and `frontdesk` is the assembly — configuration in,
// a listening socket out.
//
// It exists so the wiring is testable. A binary that built a runner, a set of
// views and a listener inline would put every startup refusal — no key, a
// address that is not loopback, a port already taken — in a place no test
// reaches.
//
// The desk it opens is one person's: the caller holding the local token is the
// person at this box, and a device somewhere else joins the same desk through
// the hub as a relay rather than through this port
// (workspace docs/DESK-SCOPES.md).
package frontdesk

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/pleware/initagent/internal/desk"
	"github.com/pleware/initagent/internal/deskseam"
)

// ErrClosed is the desk this box was not told how to open: no chat provider,
// or no token for the local seam. It is a distinct error because the connector
// still serves devices and MCP without a desk, so a caller decides whether to
// carry on rather than being handed a generic configuration failure.
var ErrClosed = errors.New("frontdesk: not configured")

// shutdownGrace is how long an open connection has to finish after the
// process is asked to stop. Long enough for a reply in flight to land, short
// enough that a restart is not a wait.
const shutdownGrace = 5 * time.Second

// Options is what the assembly cannot read from the environment.
type Options struct {
	// Config is the desk's half of what this box was told, already loaded.
	Config desk.Config

	// Staff is who is at the desk. Empty means DefaultStaff — the pair the
	// product ships with, until the hub serves the roster with the Big Five
	// baseline and the voice beside it.
	Staff []Person

	// Floor is who holds the voice before anybody has spoken. Empty means the
	// first of Staff.
	Floor desk.StaffID

	// HTTP reaches the providers. Nil means desk.NewHTTPClient, which is
	// deliberately without a whole-request timeout: a streamed reply stays
	// open for as long as she is talking.
	HTTP *http.Client

	// Now defaults to time.Now.
	Now func() time.Time
}

// Desk is one open front desk: the conversation loop, the views the glass
// reads, and the socket it connects to.
type Desk struct {
	runner   *desk.Runner
	views    *deskseam.Views
	listener net.Listener
	server   *http.Server
}

// Open builds the desk and takes the port.
//
// Binding here rather than in Serve is deliberate: a port already in use is a
// second desk on this box, and finding that out at startup is the difference
// between a refusal and a glass that connects to somebody else's conversation.
func Open(opts Options) (*Desk, error) {
	seam := opts.Config.Seam()
	if !seam.Open() {
		return nil, fmt.Errorf("%w: no local seam token", ErrClosed)
	}
	chatBinding, bound := opts.Config.Binding(desk.RoleChat)
	if !bound {
		return nil, fmt.Errorf("%w: nothing answers, so there is nobody to talk to", ErrClosed)
	}
	chat, err := desk.NewChat(chatBinding, opts.HTTP)
	if err != nil {
		return nil, err
	}

	staff := opts.Staff
	if len(staff) == 0 {
		staff = DefaultStaff()
	}
	roster, err := desk.NewRoster(rosterOf(staff)...)
	if err != nil {
		return nil, err
	}
	floor := opts.Floor
	if floor == "" {
		floor = staff[0].ID
	}

	views, err := deskseam.NewViews(deskseam.ViewsConfig{
		Names: namesOf(staff),
		Now:   opts.Now,
	})
	if err != nil {
		return nil, err
	}

	runner, err := desk.NewRunner(desk.RunnerConfig{
		Roster:   roster,
		Chat:     chat,
		Personas: personasOf(staff),
		Facts:    views,
		Model:    chatBinding.Model,
		Floor:    floor,
		Now:      opts.Now,
	})
	if err != nil {
		return nil, err
	}

	// The local caller is the person at this box, which is the desk's own
	// conversation. A second person is a second credential, and that arrives
	// through the relay rather than through this address.
	socket, err := deskseam.NewListener(deskseam.ListenConfig{
		Views:        views,
		Answerer:     runner,
		Token:        seam.Token,
		Conversation: desk.DefaultConversation,
	})
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", seam.Addr)
	if err != nil {
		return nil, fmt.Errorf("desk seam cannot listen on %s: %w", seam.Addr, err)
	}

	mux := http.NewServeMux()
	mux.Handle(deskseam.Path, socket)
	return &Desk{
		runner:   runner,
		views:    views,
		listener: listener,
		server:   &http.Server{Handler: mux},
	}, nil
}

// Addr is where the glass connects. Known before Serve, because Open already
// took the port.
func (d *Desk) Addr() string { return d.listener.Addr().String() }

// URL is the whole address of the seam, mount included. The assembly chose the
// route, so the assembly is what can say it — a caller that pastes together a
// host and a path is a second copy of that decision.
func (d *Desk) URL() string { return "ws://" + d.Addr() + deskseam.Path }

// Runner is the conversation loop, for a caller that also feeds it from
// somewhere other than the socket — the audio path, later.
func (d *Desk) Runner() *desk.Runner { return d.runner }

// Views is what every connected device sees.
func (d *Desk) Views() *deskseam.Views { return d.views }

// Serve answers until the context ends, then gives open connections a moment
// to finish. It returns nil on a clean stop.
func (d *Desk) Serve(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { done <- d.server.Serve(d.listener) }()

	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		return d.Close()
	}
}

// Close stops the socket. It is safe to call twice, which is what makes
// `defer d.Close()` correct beside Serve.
func (d *Desk) Close() error {
	stopping, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := d.server.Shutdown(stopping); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
