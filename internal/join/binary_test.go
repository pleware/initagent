package join

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func serveBinary(t *testing.T, i Installer, osName, arch string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/agent-binary", nil)
	q := url.Values{}
	if osName != "" {
		q.Set("os", osName)
	}
	if arch != "" {
		q.Set("arch", arch)
	}
	req.URL.RawQuery = q.Encode()
	rec := httptest.NewRecorder()
	i.ServeBinary(rec, req)
	return rec
}

func TestServeBinaryRequiresParams(t *testing.T) {
	tests := []struct{ osName, arch string }{
		{"", ""},
		{"linux", ""},
		{"", "amd64"},
	}
	for _, tc := range tests {
		rec := serveBinary(t, Installer{}, tc.osName, tc.arch)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("os=%q arch=%q: status = %d, want 400", tc.osName, tc.arch, rec.Code)
		}
	}
}

func TestServeBinaryRejectsPathTraversal(t *testing.T) {
	// arch=../.. would otherwise escape the binaries directory into an
	// arbitrary pre-auth file read.
	for _, bad := range []string{"../../../../etc/passwd", "linux/..", "a b", "arch;rm"} {
		rec := serveBinary(t, Installer{}, "linux", bad)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("arch=%q: status = %d, want 400", bad, rec.Code)
		}
	}
	rec := serveBinary(t, Installer{}, "../etc", "amd64")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad os: status = %d, want 400", rec.Code)
	}
}

func TestServeBinaryServesOwnExecutableForThisPlatform(t *testing.T) {
	rec := serveBinary(t, Installer{}, runtime.GOOS, runtime.GOARCH)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected the running executable as the body")
	}
}

func TestServeBinaryPrefersDroppedFileOverRelease(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "binaries"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := "fake-agent"
	if err := os.WriteFile(filepath.Join(dir, "binaries", "initagent_plan9_sparc64"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	i := Installer{DataDir: dir, GithubRepo: "pleware/initagent", Version: "v0.1.0"}
	rec := serveBinary(t, i, "plan9", "sparc64")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != payload {
		t.Errorf("body = %q, want %q", rec.Body.String(), payload)
	}
}

func TestServeBinaryRedirectsToRelease(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "release build pins its own tag",
			version: "v0.1.0",
			want:    "https://github.com/pleware/initagent/releases/download/v0.1.0/initagent_plan9_sparc64",
		},
		{
			name:    "dev build follows latest",
			version: "0.1.0-dev",
			want:    "https://github.com/pleware/initagent/releases/latest/download/initagent_plan9_sparc64",
		},
		{
			name:    "prerelease tag follows latest",
			version: "v0.2.0-rc1",
			want:    "https://github.com/pleware/initagent/releases/latest/download/initagent_plan9_sparc64",
		},
		{
			name:    "unknown version follows latest",
			version: "",
			want:    "https://github.com/pleware/initagent/releases/latest/download/initagent_plan9_sparc64",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i := Installer{DataDir: t.TempDir(), GithubRepo: "pleware/initagent", Version: tc.version}
			rec := serveBinary(t, i, "plan9", "sparc64")
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != tc.want {
				t.Errorf("Location = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestServeBinaryNotFoundWithoutRepo(t *testing.T) {
	i := Installer{DataDir: t.TempDir()}
	rec := serveBinary(t, i, "plan9", "sparc64")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	// The message has to tell an operator how to unblock an air-gapped join.
	if body := rec.Body.String(); !strings.Contains(body, "cross-compile") {
		t.Errorf("body = %q", body)
	}
}
