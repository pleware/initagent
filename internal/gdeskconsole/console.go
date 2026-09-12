// Package gdeskconsole serves the desk's service hatch: one page the connector
// hands out itself, for the person maintaining the box.
//
// It lives beside the seam rather than inside the glass on purpose. The glass
// is the desk's face — what a person walking up to the machine sees — and a
// hatch bolted to the face cannot be opened when the face is dark. A page
// served here survives a broken glass, a stopped shell and a dead dev server,
// which is exactly when somebody needs to look inside.
//
// It is deliberately small: one embedded file, no build step, no framework.
// Every value the page needs about the wire — the routes, the envelope
// version, the ceiling on a sentence — is injected from Go, so the page cannot
// drift from the seam it talks to.
//
// The page holds no secret. The token arrives in the URL fragment, which a
// browser never sends to a server (`Link`), and the page keeps it for that tab.
package gdeskconsole

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdeskseam"
)

//go:embed console.html
var files embed.FS

// Path is where the hatch is mounted on the connector's local address.
const Path = "/console"

// Stream is the console's own numbered view of the desk.
//
// Its own, not the glass's: the seam gives each stream separate numbering and
// separate gap detection, and a fact reaches every stream bound to the
// conversation (`Views.Record`). So the console sees what the glass sees
// without standing in its path, and a sentence typed here reaches the
// connector directly rather than through the face.
const Stream gdeskseam.StreamID = "gdesk:console"

// tokenFragment is how the token is handed to the page. A fragment rather than
// a query because a browser does not send it to the server: it stays out of
// access logs and out of anything that records request lines.
const tokenFragment = "t="

// settings is what the page is told about the wire. Injected rather than
// written into the HTML so each value keeps one owning definition.
type settings struct {
	SeamPath      string `json:"seamPath"`
	LogsPath      string `json:"logsPath"`
	Version       int    `json:"version"`
	Stream        string `json:"stream"`
	UtteranceKind string `json:"utteranceKind"`
	ResyncKind    string `json:"resyncKind"`
	Source        string `json:"source"`
	MaxChars      int    `json:"maxChars"`
	LogMaxAgeMs   int64  `json:"logMaxAgeMs"`
}

func wire() settings {
	return settings{
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
}

// Page is the rendered hatch, ready to serve.
type Page struct {
	body []byte
}

// NewPage renders the page once.
//
// At startup rather than per request, so a template this package broke refuses
// the desk instead of handing a stack trace to whoever opens the hatch.
func NewPage() (*Page, error) {
	raw, err := files.ReadFile("console.html")
	if err != nil {
		return nil, fmt.Errorf("gdeskconsole: page: %w", err)
	}
	return newPage(string(raw))
}

// newPage renders one source.
//
// Split from NewPage so the ways a template can fail are reachable from a test.
// The embedded file is, by construction, the one source that never has them —
// which is exactly why testing through it would prove nothing.
//
// The settings are handed to the template as a value, not as pre-rendered
// bytes: in a script context html/template writes a Go value as JSON itself, so
// there is one fewer step that could disagree with the struct's tags.
func newPage(source string) (*Page, error) {
	tmpl, err := template.New("console").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("gdeskconsole: page: %w", err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, wire()); err != nil {
		return nil, fmt.Errorf("gdeskconsole: page: %w", err)
	}
	return &Page{body: []byte(out.String())}, nil
}

// ServeHTTP hands out the page.
//
// Without a token, and that is not an oversight: the page is a document, and
// everything it does afterwards — the socket, the log dump — the seam
// authenticates itself. Serving it behind the token instead would put the
// secret in the request line of whatever fetched it.
func (p *Page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page ships inside the binary, so a cached copy outliving an upgrade
	// is a page talking to a seam that has moved on.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(p.body)
}

// Link is the address to print at startup: the hatch, with the token in the
// fragment.
//
// The whole link rather than a bare route, because the operator pastes this
// into a browser, and a person assembling a host, a route and a credential by
// hand is three chances to get it wrong.
//
// It carries the desk's key, so the line it is printed on is a secret: a
// startup log pasted into a ticket is a desk handed over.
func Link(addr, token string) string {
	u := url.URL{
		Scheme:   "http",
		Host:     addr,
		Path:     Path,
		Fragment: tokenFragment + token,
	}
	return u.String()
}
