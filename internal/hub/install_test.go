package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/gateway"
)

func newTestServer(t *testing.T, version string) *Server {
	t.Helper()
	srv, err := NewServer(Options{
		Addr: "127.0.0.1:0", DataDir: t.TempDir(), Version: version, GithubRepo: "pleware/initagent",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.store.Close() })
	return srv
}

// The script and binary behavior itself is owned by internal/join, which
// tests it against both planes' cases. What the hub still has to prove is
// that its own options reach that installer.

func TestAgentBinaryUsesHubVersionAndRepo(t *testing.T) {
	srv := newTestServer(t, "v0.1.0")
	// Ask for a platform the running test host almost certainly isn't.
	req := httptest.NewRequest("GET", "/api/agent-binary?os=plan9&arch=sparc64", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	want := "https://github.com/pleware/initagent/releases/download/v0.1.0/initagent_plan9_sparc64"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestInstallScriptEmbedsHubHost(t *testing.T) {
	srv := newTestServer(t, "v0.1.0")
	req := httptest.NewRequest("GET", "/install/abc123.sh", nil)
	req.Host = "hub.example:4200"
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `HUB="http://hub.example:4200"`) {
		t.Errorf("script should embed the hub URL:\n%s", body)
	}
	if !strings.Contains(body, `TOKEN="abc123"`) {
		t.Error("script should embed the enrollment token")
	}
}

func TestListDevicesAsksGateway(t *testing.T) {
	gw, err := gateway.Open(gateway.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	if _, _, err := gw.Store().CreateDevice(context.Background(), gw.Project().ID, "box", "box", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(gw.Handler())
	t.Cleanup(ts.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = ts.URL
	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()
	srv.handleListDevices(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	var views []gateway.DeviceView
	if err := json.Unmarshal(w.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Name != "box" {
		t.Fatalf("views = %+v", views)
	}
}

func TestCreateEnrollTokenRequiresGatewayURL(t *testing.T) {
	srv := newTestServer(t, "v0.1.0")
	req := httptest.NewRequest("POST", "/api/enroll-tokens", nil)
	req.Host = "hub.example:4200"
	w := httptest.NewRecorder()
	srv.handleCreateEnrollToken(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (must not bake r.Host)", w.Code)
	}
}

func TestCreateEnrollTokenAsksGateway(t *testing.T) {
	gw, err := gateway.Open(gateway.Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	ts := httptest.NewServer(gw.Handler())
	t.Cleanup(ts.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = ts.URL
	req := httptest.NewRequest("POST", "/api/enroll-tokens", nil)
	req.Host = "hub.example:4200"
	w := httptest.NewRecorder()
	srv.handleCreateEnrollToken(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["token"] == "" {
		t.Fatal("token should be returned")
	}
	if strings.Contains(got["command"], "hub.example") {
		t.Fatalf("command still points at the hub: %q", got["command"])
	}
	if !strings.Contains(got["command"], ts.URL+"/install/"+got["token"]+".sh") {
		t.Fatalf("unix command should hit the gateway, got %q", got["command"])
	}
	if !strings.Contains(got["windowsCommand"], ts.URL+"/install/"+got["token"]+".ps1") {
		t.Fatalf("windows command should hit the gateway, got %q", got["windowsCommand"])
	}
}
