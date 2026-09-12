package gdeskfront

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

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdeskconsole"
	"github.com/pleware/initagent/internal/gdeskseam"
)

const testToken = "sec-desk-token"

// configured is the smallest environment that opens a desk: one provider, one
// chat binding, a token and a port the kernel picks.
func configured(t *testing.T, extra map[string]string) gdesk.Config {
	t.Helper()
	env := map[string]string{
		"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE":       "openai",
		"INITAGENT_GDESK_PROVIDER_OPENAI_SECRET_KIND": "openai",
		"INITAGENT_OPENAI_API_KEY":                    "sk-test",
		"INITAGENT_GDESK_CHAT":                        "openai/gpt-4o-mini",
		"INITAGENT_GDESK_SEAM_ADDR":                   "127.0.0.1:0",
		"INITAGENT_GDESK_SEAM_TOKEN":                  testToken,
	}
	for key, value := range extra {
		if value == "" {
			delete(env, key)
			continue
		}
		env[key] = value
	}
	cfg, err := gdesk.LoadConfig(env)
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
	_, err := Open(Options{Config: configured(t, map[string]string{"INITAGENT_GDESK_SEAM_TOKEN": ""})})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Open = %v, want ErrClosed", err)
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error %q does not say what is missing", err)
	}
}

// TestADeskWithNobodyToAnswerDoesNotOpen keeps a mute desk and a brainless one
// apart: no voice starts the desk anyway (gdesk.Silence), no chat does not.
func TestADeskWithNobodyToAnswerDoesNotOpen(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{Config: configured(t, map[string]string{"INITAGENT_GDESK_CHAT": ""})})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Open = %v, want ErrClosed", err)
	}
}

// TestADialectWithNoImplementationRefusesAtStartup covers the provider choice
// failing in the assembly rather than on somebody's first sentence.
func TestADialectWithNoImplementationRefusesAtStartup(t *testing.T) {
	t.Parallel()
	cfg := configured(t, map[string]string{
		"INITAGENT_GDESK_PROVIDER_OPENAI_SHAPE": "anthropic",
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
	if !errors.Is(err, gdesk.ErrConfig) {
		t.Fatalf("Open = %v, want gdesk.ErrConfig", err)
	}
}

// TestAFloorHolderNobodyEmploysRefuses covers the runner's check through the
// assembly, since the floor is the desk's setting rather than a person's state.
func TestAFloorHolderNobodyEmploysRefuses(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{Config: configured(t, nil), Floor: "psn-nobody"})
	if !errors.Is(err, gdesk.ErrConfig) {
		t.Fatalf("Open = %v, want gdesk.ErrConfig", err)
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

	floor := d.Runner().Floor(gdesk.DefaultConversation)
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
		"INITAGENT_GDESK_SEAM_ADDR": first.Addr(),
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
	d.Views().Record(gdesk.DefaultConversation, gdesk.TurnOpened{
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
	if event.V != gdeskseam.Version {
		t.Errorf("v = %d, want %d", event.V, gdeskseam.Version)
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

// TestTheOperatorLogRequiresTheSameToken keeps the dump behind the local
// secret: guessing the port is not enough to read what she typed.
func TestTheOperatorLogRequiresTheSameToken(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	resp, err := http.Get("http://" + d.Addr() + gdeskseam.LogsPath)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestTheOperatorLogShowsTheDeskIsListening is the hop the back-office pane
// polls: the same token as the websocket, and a line that says the port is
// ours, so "nothing happened" is no longer an empty column.
func TestTheOperatorLogShowsTheDeskIsListening(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+d.Addr()+gdeskseam.LogsPath, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var dump gdeskseam.TraceDump
	if err := json.NewDecoder(resp.Body).Decode(&dump); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dump.V != gdeskseam.Version || len(dump.Lines) == 0 {
		t.Fatalf("dump = %+v, want at least the listening line", dump)
	}
	if !strings.Contains(dump.Lines[0].Text, "listening") {
		t.Fatalf("first line %q, want listening", dump.Lines[0].Text)
	}
}

// TestTheHatchOpensWithoutTheGlass is the console's whole reason for being
// here: the page comes from the connector, so it is readable when the glass is
// broken, the shell is stopped, or no dev server is running at all.
func TestTheHatchOpensWithoutTheGlass(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	resp, err := http.Get("http://" + d.Addr() + gdeskconsole.Path)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want HTML", got)
	}
}

// TestTheConsoleLinkNamesThisDeskAndItsKey is why the assembly says the link:
// it is the only place that holds both halves.
func TestTheConsoleLinkNamesThisDeskAndItsKey(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	link := d.ConsoleURL()
	if !strings.Contains(link, d.Addr()) {
		t.Errorf("console link %q does not name this desk (%s)", link, d.Addr())
	}
	if !strings.Contains(link, testToken) {
		t.Errorf("console link %q carries no key, so it opens nothing", link)
	}
	address, _, ok := strings.Cut(link, "#")
	if !ok {
		t.Fatalf("console link %q has no fragment", link)
	}
	// The key rides in the fragment, which a browser does not send.
	if strings.Contains(address, testToken) {
		t.Errorf("console link %q puts the key in the request", link)
	}
}

// TestAPathThatIsNotTheSeamIs404 is the mount, not the seam: three routes and
// nothing else, so a typo in the glass's URL fails loudly instead of hanging.
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

func readEvent(t *testing.T, ws *websocket.Conn) gdeskseam.Envelope {
	t.Helper()
	if err := ws.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("read deadline: %v", err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var envelope gdeskseam.Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return envelope
}
