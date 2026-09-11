package frontdesk

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/desk"
	"github.com/pleware/initagent/internal/deskseam"
)

const testToken = "sec-desk-token"

// configured is the smallest environment that opens a desk: one provider, one
// chat binding, a token and a port the kernel picks.
func configured(t *testing.T, extra map[string]string) desk.Config {
	t.Helper()
	env := map[string]string{
		"INITAGENT_DESK_PROVIDER_OPENAI_SHAPE":       "openai",
		"INITAGENT_DESK_PROVIDER_OPENAI_SECRET_KIND": "openai",
		"INITAGENT_OPENAI_API_KEY":                   "sk-test",
		"INITAGENT_DESK_CHAT":                        "openai/gpt-4o-mini",
		"INITAGENT_DESK_SEAM_ADDR":                   "127.0.0.1:0",
		"INITAGENT_DESK_SEAM_TOKEN":                  testToken,
	}
	for key, value := range extra {
		if value == "" {
			delete(env, key)
			continue
		}
		env[key] = value
	}
	cfg, err := desk.LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

// serving opens a desk and answers on it until the test ends.
func serving(t *testing.T, opts Options) *Desk {
	t.Helper()
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- d.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-stopped:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return after the context ended")
		}
	})
	return d
}

// TestADeskWithNoTokenDoesNotOpen is the closed default reaching the assembly:
// the port is not taken, so nothing is listening for anybody to find.
func TestADeskWithNoTokenDoesNotOpen(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{Config: configured(t, map[string]string{"INITAGENT_DESK_SEAM_TOKEN": ""})})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Open = %v, want ErrClosed", err)
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error %q does not say what is missing", err)
	}
}

// TestADeskWithNobodyToAnswerDoesNotOpen keeps a mute desk and a brainless one
// apart: no voice starts the desk anyway (desk.Silence), no chat does not.
func TestADeskWithNobodyToAnswerDoesNotOpen(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{Config: configured(t, map[string]string{"INITAGENT_DESK_CHAT": ""})})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Open = %v, want ErrClosed", err)
	}
}

// TestADialectWithNoImplementationRefusesAtStartup covers the provider choice
// failing in the assembly rather than on somebody's first sentence.
func TestADialectWithNoImplementationRefusesAtStartup(t *testing.T) {
	t.Parallel()
	cfg := configured(t, map[string]string{
		"INITAGENT_DESK_PROVIDER_OPENAI_SHAPE": "anthropic",
	})
	_, err := Open(Options{Config: cfg})
	if err == nil {
		t.Fatal("Open accepted a dialect with no chat implementation")
	}
	if errors.Is(err, ErrClosed) {
		t.Errorf("error = %v, want a dialect failure rather than an unconfigured desk", err)
	}
}

// TestStaffThatCannotBeToldApartRefuses is the roster's own rule arriving at
// startup: one form, one person.
func TestStaffThatCannotBeToldApartRefuses(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{
		Config: configured(t, nil),
		Staff: []Person{
			{ID: "psn-ania", Names: []string{"ania"}},
			{ID: "psn-adam", Names: []string{"ania"}},
		},
	})
	if !errors.Is(err, desk.ErrConfig) {
		t.Fatalf("Open = %v, want desk.ErrConfig", err)
	}
}

// TestAFloorHolderNobodyEmploysRefuses covers the runner's check through the
// assembly, since the floor is the desk's setting rather than a person's state.
func TestAFloorHolderNobodyEmploysRefuses(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{Config: configured(t, nil), Floor: "psn-nobody"})
	if !errors.Is(err, desk.ErrConfig) {
		t.Fatalf("Open = %v, want desk.ErrConfig", err)
	}
}

// TestTheDeskOpensOnLoopbackAndTheFloorIsHeld is the happy path: a port on
// this machine only, and somebody already holding the voice before anybody has
// spoken.
func TestTheDeskOpensOnLoopbackAndTheFloorIsHeld(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	host, port, err := net.SplitHostPort(d.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", d.Addr(), err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Errorf("addr = %q, want a loopback address", d.Addr())
	}
	if port == "0" || port == "" {
		t.Errorf("addr = %q, want a port the kernel handed out", d.Addr())
	}

	floor := d.Runner().Floor(desk.DefaultConversation)
	if floor.Holder != DefaultStaff()[0].ID {
		t.Errorf("floor = %q, want the first of the staff", floor.Holder)
	}
	if d.Views().Streams() != 0 {
		t.Errorf("streams = %d, want none before anybody connects", d.Views().Streams())
	}
}

// TestTheSecondDeskOnThisBoxRefuses. Two desks would each answer half the
// glass's sentences, so the port is taken at startup and the failure is the
// port's.
func TestTheSecondDeskOnThisBoxRefuses(t *testing.T) {
	t.Parallel()
	first := serving(t, Options{Config: configured(t, nil)})

	_, err := Open(Options{Config: configured(t, map[string]string{
		"INITAGENT_DESK_SEAM_ADDR": first.Addr(),
	})})
	if err == nil {
		t.Fatal("Open took a port that was already taken")
	}
	if !strings.Contains(err.Error(), first.Addr()) {
		t.Errorf("error %q does not name the address", err)
	}
}

// TestTheGlassReachesTheDeskWithItsToken walks the whole assembly: dial the
// address Open chose, on the path the seam is mounted at, and get this stream's
// first numbered event.
func TestTheGlassReachesTheDeskWithItsToken(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	ws, _, err := websocket.DefaultDialer.Dial(socketURL(d, "trm-glass", testToken), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// Bound by the time the dial returns, because binding happens before the
	// upgrade. A fact recorded now therefore has a log to be numbered into,
	// which is the whole path without a provider in it.
	if d.Views().Streams() != 1 {
		t.Fatalf("streams = %d, want one", d.Views().Streams())
	}
	d.Views().Record(desk.DefaultConversation, desk.TurnOpened{
		Turn:      "trn-1",
		Staff:     DefaultStaff()[0].ID,
		Utterance: "utt-1",
	})

	event := readEvent(t, ws)
	if event.Stream != "trm-glass" {
		t.Errorf("stream = %q, want the one the glass named", event.Stream)
	}
	if event.Seq != 1 {
		t.Errorf("seq = %d, want this stream's first event", event.Seq)
	}
	if event.V != deskseam.Version {
		t.Errorf("v = %d, want %d", event.V, deskseam.Version)
	}
}

// TestADeskThatIsNotOursRefusesTheConnection keeps the local token doing its
// job through the assembly, before any upgrade.
func TestADeskThatIsNotOursRefusesTheConnection(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	_, resp, err := websocket.DefaultDialer.Dial(socketURL(d, "trm-glass", "guessed"), nil)
	if err == nil {
		t.Fatal("a connection without the token was upgraded")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
}

// TestAPathThatIsNotTheSeamIs404 is the mount, not the seam: one route, so a
// typo in the glass's URL fails loudly instead of hanging.
func TestAPathThatIsNotTheSeamIs404(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	resp, err := http.Get("http://" + d.Addr() + "/desk-seam")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestClosingTwiceIsNotAFailure is what makes `defer d.Close()` correct beside
// a Serve that already returned.
func TestClosingTwiceIsNotAFailure(t *testing.T) {
	t.Parallel()
	d, err := Open(Options{Config: configured(t, nil)})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestServeReturnsWhenTheSocketIsClosedUnderIt covers the other exit: the
// server stopping on its own rather than on a cancelled context.
func TestServeReturnsWhenTheSocketIsClosedUnderIt(t *testing.T) {
	t.Parallel()
	d, err := Open(Options{Config: configured(t, nil)})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- d.Serve(context.Background()) }()

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Serve = %v, want a clean stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

func socketURL(d *Desk, stream, token string) string {
	return d.URL() +
		"?stream=" + url.QueryEscape(stream) + "&token=" + url.QueryEscape(token)
}

func readEvent(t *testing.T, ws *websocket.Conn) deskseam.Envelope {
	t.Helper()
	if err := ws.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("read deadline: %v", err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var envelope deskseam.Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return envelope
}
