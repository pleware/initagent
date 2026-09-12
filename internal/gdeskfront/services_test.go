package gdeskfront

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/gdeskconsole"
	"github.com/pleware/initagent/internal/gdeskseam"
)

// TestTheServiceListSaysWhatHoldsWhichPort is the console's table: every route
// this desk serves, on the address it actually took, with the state that says
// whether a port here is ours.
func TestTheServiceListSaysWhatHoldsWhichPort(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	// The test desk asks for `:0`, so the port it holds is one nobody wrote
	// down. That is the case the column exists for, and it is why this is read
	// back from the listener instead of compared with a constant.
	_, held, err := net.SplitHostPort(d.Addr())
	if err != nil {
		t.Fatalf("the desk's address is not host:port: %v", err)
	}
	port, err := strconv.Atoi(held)
	if err != nil {
		t.Fatalf("port %q is not a number: %v", held, err)
	}

	byName := servicesOf(t, d)
	for _, want := range []struct {
		name  string
		where string
		state string
	}{
		{"gdesk seam", d.Addr() + gdeskseam.Path, gdeskseam.ServiceListening},
		{"operator console", d.Addr() + gdeskconsole.Path, gdeskseam.ServiceListening},
		{"operator log", d.Addr() + gdeskseam.LogsPath, gdeskseam.ServiceListening},
	} {
		got, ok := byName[want.name]
		if !ok {
			t.Fatalf("no row for %q, so the console cannot show it", want.name)
		}
		if got.Where != want.where {
			t.Errorf("%s is at %q, want %q", want.name, got.Where, want.where)
		}
		if got.State != want.state {
			t.Errorf("%s is %q, want %q", want.name, got.State, want.state)
		}
		if got.Port != port {
			t.Errorf("%s reports port %d, want %d", want.name, got.Port, port)
		}
	}
}

// TestAnOutboundRowCarriesThePortItWouldReach is the half of the column that is
// not ours. A provider URL usually names no port and still reaches 443, so an
// empty cell there would read as "the desk calls nowhere"; a local model on a
// chosen port has to show that port instead.
func TestAnOutboundRowCarriesThePortItWouldReach(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		base string
		want int
	}{
		{"https with no port in it", "", 443},
		{"a local model on its own port", "http://127.0.0.1:11434/v1", 11434},
		{"plain http", "http://models.example.com/v1", 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := serving(t, Options{Config: configured(t, map[string]string{
				"INITAGENT_GDESK_PROVIDER_OPENAI_BASE_URL": tc.base,
			})})

			chat, ok := servicesOf(t, d)["gdesk.chat"]
			if !ok {
				t.Fatal("no row for the voice that answers")
			}
			if chat.Port != tc.want {
				t.Errorf("port = %d for %q, want %d", chat.Port, chat.Where, tc.want)
			}
		})
	}
}

// TestARowWithNoPortSaysSoWithAZero guards the other end: a role nothing is
// bound to reaches nothing, and facts on a pipe have no port by design. A number
// invented for either would be the same lie the `declared` state prevents.
func TestARowWithNoPortSaysSoWithAZero(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	byName := servicesOf(t, d)
	for _, name := range []string{"gdesk.stt", "local sensing"} {
		got, ok := byName[name]
		if !ok {
			t.Fatalf("no row for %q in %v", name, names(byName))
		}
		if got.Port != 0 {
			t.Errorf("%s reports port %d, and it reaches nothing", name, got.Port)
		}
	}
}

// TestTheSeamRowCountsWhoIsOnThePort is the number the table was asked for. A
// count that only moved on a restart would answer "is anybody connected" with
// whatever was true when the desk started.
func TestTheSeamRowCountsWhoIsOnThePort(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	if got := clientsOnSeam(t, d); got != 0 {
		t.Fatalf("clients = %d before anybody dialled", got)
	}

	ws, _, err := websocket.DefaultDialer.Dial(
		"ws://"+d.Addr()+gdeskseam.Path+"?stream=gdesk:local&token="+testToken, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if got := clientsOnSeam(t, d); got != 1 {
		t.Fatalf("clients = %d with one caller on the seam", got)
	}

	ws.Close()
	waitFor(t, func() bool { return clientsOnSeam(t, d) == 0 }, "the count to drop with the socket")
}

// TestARoleNobodyBoundIsSilentAndSaysWhy keeps two afternoons apart: nothing was
// configured, versus a provider whose key has not arrived.
func TestARoleNobodyBoundIsSilentAndSaysWhy(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	byName := servicesOf(t, d)
	stt, ok := byName["gdesk.stt"]
	if !ok {
		t.Fatalf("no row for the ear, so a deaf desk looks like a working one: %v", names(byName))
	}
	if stt.State != gdeskseam.ServiceSilent {
		t.Errorf("stt is %q, want %q", stt.State, gdeskseam.ServiceSilent)
	}
	if stt.Note == "" {
		t.Error("a silent role that does not say why sends somebody reading configuration")
	}
	if stt.Clients != nil {
		t.Error("a role nothing is bound to cannot have callers")
	}

	chat, ok := byName["gdesk.chat"]
	if !ok {
		t.Fatal("no row for the voice that answers")
	}
	if chat.State != gdeskseam.ServiceOutbound {
		t.Errorf("chat is %q, want %q: the port belongs to the provider", chat.State, gdeskseam.ServiceOutbound)
	}
	if chat.Clients != nil {
		t.Error("a client count on an outbound row would be somebody else's number")
	}
	if !strings.Contains(chat.Note, "gpt-4o-mini") {
		t.Errorf("chat note %q does not name the model", chat.Note)
	}
	if strings.Contains(chat.Note+chat.Where, "sk-test") {
		t.Error("the key reached the console")
	}
}

// TestSensingIsListedAsDeclaredAndNotAsRunning is the honesty rule of this
// table. The camera process belongs on a PWare OS box, and this connector has no
// reader for its facts yet: a row that looked live would cost somebody an
// afternoon, and no row at all would read as a complete list.
func TestSensingIsListedAsDeclaredAndNotAsRunning(t *testing.T) {
	t.Parallel()
	d := serving(t, Options{Config: configured(t, nil)})

	byName := servicesOf(t, d)
	sensing, ok := byName["local sensing"]
	if !ok {
		t.Fatalf("nothing about sensing in %v", names(byName))
	}
	if sensing.State != gdeskseam.ServiceDeclared {
		t.Errorf("sensing is %q, want %q", sensing.State, gdeskseam.ServiceDeclared)
	}
	if sensing.Clients != nil {
		t.Error("a declared service cannot have callers")
	}
	if !strings.Contains(sensing.Note, sensingProducer) {
		t.Errorf("note %q does not name who produces the facts", sensing.Note)
	}
	if strings.Contains(sensing.Where, ":") {
		t.Errorf("where = %q: sensing has no port, and inventing one is the lie this test exists for", sensing.Where)
	}
}

// TestADeskThatNeverTookAPortReportsNothing is the order inside Open: the
// inventory is read at a poll, and until the port is taken there is nothing true
// to say about it.
func TestADeskThatNeverTookAPortReportsNothing(t *testing.T) {
	t.Parallel()

	if got := (&Desk{}).Services(); got != nil {
		t.Fatalf("Services() = %#v, want nothing", got)
	}
}

func servicesOf(t *testing.T, d *Desk) map[string]gdeskseam.Service {
	t.Helper()
	out := map[string]gdeskseam.Service{}
	for _, service := range d.Services() {
		if _, twice := out[service.Name]; twice {
			t.Fatalf("two rows named %q: the table cannot say which is which", service.Name)
		}
		out[service.Name] = service
	}
	return out
}

func names(byName map[string]gdeskseam.Service) []string {
	out := make([]string, 0, len(byName))
	for name := range byName {
		out = append(out, name)
	}
	return out
}

func clientsOnSeam(t *testing.T, d *Desk) int {
	t.Helper()
	seam, ok := servicesOf(t, d)["gdesk seam"]
	if !ok {
		t.Fatal("no row for the seam")
	}
	if seam.Clients == nil {
		t.Fatal("the seam row has no count, so the question cannot be answered")
	}
	return *seam.Clients
}

// waitFor polls a condition that a closed socket has to travel to reach: the
// listener forgets a caller when its handler returns, not when the client
// stops reading.
func waitFor(t *testing.T, done func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %s and it did not happen", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
