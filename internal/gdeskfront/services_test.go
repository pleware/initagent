package gdeskfront

import (
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
