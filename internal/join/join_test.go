package join

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSafeToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"alphanumeric", "abc123", true},
		{"separators", "abc-123_v0.1", true},
		{"empty", "", false},
		{"too long", strings.Repeat("a", 65), false},
		{"at length limit", strings.Repeat("a", 64), true},
		{"double quote", `x"y`, false},
		{"path traversal", "../etc", false},
		{"space", "a b", false},
		{"shell separator", "arch;rm", false},
		{"slash", "linux/amd64", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeToken(tc.in); got != tc.want {
				t.Errorf("SafeToken(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCommandsPointAtBaseURL(t *testing.T) {
	unix, windows := Commands("http://gw.example:4201/", "abc")
	if want := "curl -fsSL http://gw.example:4201/install/abc.sh | sh"; unix != want {
		t.Errorf("unix = %q, want %q", unix, want)
	}
	if !strings.Contains(windows, "irm http://gw.example:4201/install/abc.ps1 | iex") {
		t.Errorf("windows = %q", windows)
	}
}

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		tls       bool
		want      string
	}{
		{"from request host", "", false, "http://gw.example:4201"},
		{"https when the request is TLS", "", true, "https://gw.example:4201"},
		{"public URL wins over host", "https://join.example/", false, "https://join.example"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/install/abc.sh", nil)
			req.Host = "gw.example:4201"
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			got := Installer{PublicURL: tc.publicURL}.BaseURL(req)
			if got != tc.want {
				t.Errorf("BaseURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func serveScript(t *testing.T, i Installer, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/install/placeholder.sh", nil)
	req.URL.Path = path
	req.Host = "gw.example:4201"
	rec := httptest.NewRecorder()
	i.ServeScript(rec, req)
	return rec
}

func TestServeScriptUnixEmbedsBaseURLAndToken(t *testing.T) {
	rec := serveScript(t, Installer{}, "/install/abc123.sh")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`HUB="http://gw.example:4201"`,
		`TOKEN="abc123"`,
		"curl -fSL", // the binary download follows redirects to the release
		"agent enroll --hub",
		"agent install-service",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("script missing %q:\n%s", want, body)
		}
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/x-shellscript") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestServeScriptWindowsEmbedsBaseURLAndToken(t *testing.T) {
	rec := serveScript(t, Installer{}, "/install/abc123.ps1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`$Hub = "http://gw.example:4201"`,
		`$Token = "abc123"`,
		"Invoke-WebRequest",
		"os=windows&arch=$Arch",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("script missing %q:\n%s", want, body)
		}
	}
}

func TestServeScriptRejectsUnusableName(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		// A quote would break out of the shell assignment.
		{"quoted token", `/install/x".sh`},
		{"traversal", "/install/../../etc/passwd.sh"},
		{"unknown extension", "/install/abc123.bat"},
		{"no extension", "/install/abc123"},
		{"empty token", "/install/.sh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveScript(t, Installer{}, tc.path)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404 for %q", rec.Code, tc.path)
			}
		})
	}
}
