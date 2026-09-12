package gdeskconsole

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdeskseam"
)

// injected pulls the settings block back out of the rendered page. The page is
// only ever as right as this object, so a test that read the HTML around it
// would pass while the page talked to the wrong route.
var injected = regexp.MustCompile(`(?m)^\s*const WIRE = (\{.*\})\s*$`)

func rendered(t *testing.T) *Page {
	t.Helper()
	page, err := NewPage()
	if err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	return page
}

func body(t *testing.T, page *Page) string {
	t.Helper()
	rec := httptest.NewRecorder()
	page.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

// TestThePageDoesNotKnowWhereTheDesktopIs guards how the header's link is
// found. The desktop's address is a live handshake's Origin, never a value
// written here: a development port baked into this file would ship inside a
// customer's binary and would be wrong the first time somebody moved it.
func TestThePageDoesNotKnowWhereTheDesktopIs(t *testing.T) {
	t.Parallel()

	page := body(t, rendered(t))
	if !strings.Contains(page, `id="peer"`) {
		t.Fatal("the header has nowhere to put the desktop's link")
	}
	for _, guess := range []string{"localhost", "5178", "vite", "127.0.0.1"} {
		if strings.Contains(strings.ToLower(page), guess) {
			t.Fatalf("the page names %q: the desktop is learned, not configured", guess)
		}
	}
}

// TestThePageOffersItsOwnAddressAsALinkItBuilds guards the header's second
// link: this hatch's address with the token on it, for a second browser or for
// after the tab is closed. It has to be assembled in the browser from the token
// that tab already holds, because the document itself is served to anybody who
// asks for it (ServeHTTP) — a rendered link would put the key in it.
func TestThePageOffersItsOwnAddressAsALinkItBuilds(t *testing.T) {
	t.Parallel()

	page := body(t, rendered(t))
	if !strings.Contains(page, `id="hatch"`) {
		t.Fatal("the header has nowhere to put the console's own link")
	}
	// Belt and braces with TestThePageIsServedWithoutTheToken: that one proves
	// no token is rendered, this one that no address is either, so the only
	// thing the link can be built from is the running tab.
	if strings.Contains(page, "http://") {
		t.Error("the page names an address of its own; the link must come from location")
	}
}

// TestThePageHasAWordForEveryStateTheDeskReports is the drift guard for the
// service table's last column. A state Go sends and the page cannot translate
// renders as a machine word in a Polish table; a word the page keeps for a state
// Go stopped sending is a row nobody will ever see.
func TestThePageHasAWordForEveryStateTheDeskReports(t *testing.T) {
	t.Parallel()

	page := body(t, rendered(t))
	for _, state := range gdeskseam.ServiceStates {
		// The page's map has bare keys, so a translated state is followed by a
		// colon. Adding a state to the seam therefore fails here until the page
		// has a word for it.
		if !strings.Contains(page, state+":") {
			t.Errorf("the page has no word for %q", state)
		}
	}
	if !strings.Contains(page, `id="services"`) {
		t.Fatal("the page has nowhere to put the service list")
	}
}

// TestThePortIsItsOwnColumnAndIsNeverReadOutOfTheAddress is the first question
// asked of a desk that looks dead, so it gets a column instead of hiding inside
// an address. The number arrives from Go with the row: a page that split
// `service.where` on a colon would be parsing the desk's evidence back out of a
// string, and would disagree with it the day a route changes.
func TestThePortIsItsOwnColumnAndIsNeverReadOutOfTheAddress(t *testing.T) {
	t.Parallel()

	page := body(t, rendered(t))
	if !strings.Contains(page, `<th scope="col">port</th>`) {
		t.Error("the service table has no port column")
	}
	if !strings.Contains(page, "service.port") {
		t.Error("the page does not read the port the desk sends")
	}
}

// TestThePageIsToldWhatTheSeamDecided is the drift guard: every value here has
// one owning definition in Go, and a page that hard-coded any of them would
// keep working until the day the seam moved.
func TestThePageIsToldWhatTheSeamDecided(t *testing.T) {
	t.Parallel()

	match := injected.FindStringSubmatch(body(t, rendered(t)))
	if match == nil {
		t.Fatal("the page carries no settings block")
	}
	var got settings
	if err := json.Unmarshal([]byte(match[1]), &got); err != nil {
		t.Fatalf("settings are not JSON: %v (%s)", err, match[1])
	}

	want := settings{
		SeamPath:      gdeskseam.Path,
		LogsPath:      gdeskseam.LogsPath,
		Version:       gdeskseam.Version,
		Stream:        string(Stream),
		UtteranceKind: gdeskseam.CommandUtterance,
		ResyncKind:    gdeskseam.CommandResync,
		Source:        string(gdesk.UtteranceTyped),
		MaxChars:      gdesk.MaxUtteranceChars,
		LogMaxAgeMs:   gdeskseam.MaxTraceAge.Milliseconds(),
	}
	if got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

// TestThePageIsServedWithoutTheToken is the reason the hatch may be handed out
// unauthenticated: the document itself is not a credential.
func TestThePageIsServedWithoutTheToken(t *testing.T) {
	t.Parallel()

	const token = "s3cret-desk-token"
	page := rendered(t)
	if strings.Contains(body(t, page), token) {
		t.Error("the page carries a token")
	}
	if !strings.Contains(Link("127.0.0.1:4202", token), token) {
		t.Error("the printed link does not carry the token, so nothing can open the hatch")
	}
}

func TestThePageIsHTMLAndNotCached(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	rendered(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Path, nil))

	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	// A page cached past an upgrade is a page talking to a seam that has moved.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// TestTheLinkKeepsTheTokenOutOfTheRequest is the whole reason for a fragment: a
// browser does not send one, so the key stays out of access logs.
func TestTheLinkKeepsTheTokenOutOfTheRequest(t *testing.T) {
	t.Parallel()

	link := Link("127.0.0.1:4202", "abc123")
	before, fragment, ok := strings.Cut(link, "#")
	if !ok {
		t.Fatalf("link %q has no fragment", link)
	}
	if before != "http://127.0.0.1:4202"+Path {
		t.Errorf("address = %q", before)
	}
	if fragment != "t=abc123" {
		t.Errorf("fragment = %q, want t=abc123", fragment)
	}
	if strings.Contains(before, "abc123") {
		t.Error("the token is in the part a browser sends")
	}
}

// TestAPageThatCannotBeRenderedRefusesTheDesk is why rendering happens at
// startup: a template this package broke must stop the desk opening, not wait
// to hand a stack trace to whoever finally opens the hatch.
func TestAPageThatCannotBeRenderedRefusesTheDesk(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		source string
	}{
		{"an unfinished action", "<p>{{"},
		{"a field the settings do not have", "<script>const WIRE = {{.NoSuchSetting}}</script>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := newPage(tc.source); err == nil {
				t.Error("rendered a page it should have refused")
			}
		})
	}
}

// TestALinkForATokenWithSyntaxInIt is not hypothetical: a token is opaque bytes
// chosen elsewhere, and one containing a '#' would otherwise truncate itself.
func TestALinkForATokenWithSyntaxInIt(t *testing.T) {
	t.Parallel()

	link := Link("127.0.0.1:4202", "a#b c&d")
	_, fragment, _ := strings.Cut(link, "#")
	if strings.Contains(fragment, " ") {
		t.Errorf("fragment %q carries a raw space", fragment)
	}
	if strings.Count(link, "#") != 1 {
		t.Errorf("link %q has more than one fragment marker", link)
	}
}
