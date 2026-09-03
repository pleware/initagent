// Package join serves the one-paste device joiner: the install script with an
// enrollment token baked in, and the agent binary that script downloads.
//
// Two planes serve the same joiner — a project gateway, which is where enroll
// belongs (draft 10), and the inherited single-plane hub — so the script, the
// token guard, and the binary lookup live here once instead of in each.
package join

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Installer renders join scripts and serves agent binaries for one plane.
type Installer struct {
	// DataDir holds a binaries/ subdirectory. A binary dropped there wins over
	// a release download, which is what makes a same-LAN air-gapped join work.
	DataDir string
	// GithubRepo is "owner/name". Empty disables the release redirect, leaving
	// DataDir as the only source.
	GithubRepo string
	// Version is this process's version. A release tag pins the asset to that
	// release; anything else (a dev build) follows latest.
	Version string
	// PublicURL is baked into the script instead of the request host. Needed
	// when the plane sits behind a proxy that rewrites Host.
	PublicURL string
}

// BaseURL is the URL a joined device dials back: PublicURL when set, otherwise
// the scheme and host of the request that asked for the script.
func (i Installer) BaseURL(r *http.Request) string {
	if i.PublicURL != "" {
		return strings.TrimRight(i.PublicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// Commands returns the unix and Windows one-paste joiners for baseURL.
func Commands(baseURL, token string) (unix, windows string) {
	base := strings.TrimRight(baseURL, "/")
	unix = fmt.Sprintf("curl -fsSL %s/install/%s.sh | sh", base, token)
	windows = fmt.Sprintf("powershell -NoProfile -ExecutionPolicy Bypass -Command \"irm %s/install/%s.ps1 | iex\"", base, token)
	return unix, windows
}

// SafeToken reports whether s is a short alphanumeric token (letters, digits,
// and a couple of safe separators). Values that reach a filesystem path, a
// shell script served for `| sh`, or a redirect URL go through here first:
// a quote would break out of the shell assignment, and "../.." would escape
// the binaries directory into an arbitrary pre-auth file read.
func SafeToken(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

// ServeScript handles GET /install/<token>.sh and GET /install/<token>.ps1.
//
// The token is validated at enroll time rather than here — keeping this
// endpoint dumb costs one round trip and a bogus token merely yields a script
// that fails to enroll.
func (i Installer) ServeScript(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/install/")
	token, ok := strings.CutSuffix(name, ".sh")
	format := "sh"
	if !ok {
		token, ok = strings.CutSuffix(name, ".ps1")
		format = "ps1"
	}
	if !ok || !SafeToken(token) {
		httpError(w, http.StatusNotFound, "not found")
		return
	}
	base := i.BaseURL(r)
	if format == "ps1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, windowsScript, base, token)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	fmt.Fprintf(w, unixScript, base, token)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
