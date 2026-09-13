package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/gateway"
	"github.com/pleware/initagent/internal/protocol"
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

func TestListConnectorsAsksGateway(t *testing.T) {
	gw, err := gateway.Open(gateway.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	if _, _, err := gw.Store().CreateConnector(context.Background(), gw.Project().ID, "box", "box", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(gw.Handler())
	t.Cleanup(ts.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = ts.URL
	req := httptest.NewRequest("GET", "/api/connectors", nil)
	w := httptest.NewRecorder()
	srv.handleListConnectors(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	var views []gateway.ConnectorView
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
	srv.handleCreateEnrollToken(w, req, operatorCred)
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
	srv.handleCreateEnrollToken(w, req, operatorCred)
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

func TestCreateSessionOfflineWithoutGateway(t *testing.T) {
	srv := newTestServer(t, "v0.1.0")
	req := httptest.NewRequest(http.MethodPost, "/api/connectors/connector-01/sessions", strings.NewReader(`{"name":"claude-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleCreateSession(w, req, operatorCred)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "connector is offline") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestCreateSessionAsksGateway(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.RequestURI()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	req := httptest.NewRequest(http.MethodPost, "/api/connectors/connector-01/sessions", strings.NewReader(`{"name":"claude-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleCreateSession(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/connectors/connector-01/sessions" {
		t.Fatalf("proxied %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, "claude-1") {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestListSessionsAsksGateway(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/connectors/connector-01/sessions" {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"term-1"}]`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/connector-01/sessions", nil)
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleListSessions(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "term-1") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

// projectWithConnector plants a project whose fx worker is a connector the hub's
// registry does not hold, which is exactly the self-host layout (the worker
// dials the gateway, not the hub).
func projectWithConnector(t *testing.T, srv *Server) *Project {
	t.Helper()
	org, err := srv.store.CreateOrg("Example Ops")
	if err != nil {
		t.Fatal(err)
	}
	project, err := srv.store.CreateProject(org.Id, "Storefront", "connector-01", "/srv/store", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func TestProjectExecOfflineWithoutGateway(t *testing.T) {
	srv := newTestServer(t, "v0.1.0")
	project := projectWithConnector(t, srv)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+project.Id+"/exec", strings.NewReader(`{"command":"ls"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", project.Id)
	w := httptest.NewRecorder()
	srv.handleProjectExec(w, req, operatorCred)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "project connector is offline") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestProjectExecAsksGateway(t *testing.T) {
	var gotMethod, gotPath string
	var gotExec protocol.Exec
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotExec)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"exitCode":0,"stdout":"ok"}`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	project := projectWithConnector(t, srv)
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+project.Id+"/exec", strings.NewReader(`{"command":"pwd","timeoutMs":120000}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", project.Id)
	w := httptest.NewRecorder()
	srv.handleProjectExec(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/connectors/connector-01/exec" {
		t.Fatalf("proxied %s %s", gotMethod, gotPath)
	}
	if gotExec.Command != "pwd" || gotExec.Cwd != "/srv/store" || gotExec.TimeoutSec != 120 {
		t.Fatalf("exec = %+v", gotExec)
	}
	if !strings.Contains(w.Body.String(), `"stdout":"ok"`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestFsListOfflineWithoutGateway(t *testing.T) {
	srv := newTestServer(t, "v0.1.0")
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/connector-01/fs", nil)
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleFsList(w, req, operatorCred)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "connector is offline") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestFsListAsksGateway(t *testing.T) {
	var gotPath string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"path":"/home","entries":[{"name":"a.txt"}]}`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/connector-01/fs?path=/home", nil)
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleFsList(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if gotPath != "/api/connectors/connector-01/fs?path=/home" {
		t.Fatalf("proxied %q", gotPath)
	}
	if !strings.Contains(w.Body.String(), "a.txt") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestFsDownloadAsksGateway(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/connectors/connector-01/fs/download" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="report.txt"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("file-bytes"))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/connector-01/fs/download?path=/srv/report.txt", nil)
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleFsDownload(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "file-bytes" {
		t.Fatalf("body = %q", w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "report.txt") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
}

func TestFsUploadAsksGateway(t *testing.T) {
	var gotMethod, gotPath, gotForm string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.RequestURI()
		_ = r.ParseMultipartForm(32 << 20)
		if f, _, err := r.FormFile("file"); err == nil {
			b, _ := io.ReadAll(f)
			gotForm = string(b)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"path":"/srv/up.txt"}`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "up.txt")
	_, _ = fw.Write([]byte("uploaded"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/connectors/connector-01/fs/upload?dir=/srv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleFsUpload(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/connectors/connector-01/fs/upload?dir=/srv" {
		t.Fatalf("proxied %s %s", gotMethod, gotPath)
	}
	if gotForm != "uploaded" {
		t.Fatalf("form = %q", gotForm)
	}
}

func TestSetupStatusAsksGateway(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/connectors/connector-01/setup" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"os":"linux","arch":"amd64","tools":[],"bundleCommand":""}`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/connector-01/setup", nil)
	req.SetPathValue("id", "connector-01")
	w := httptest.NewRecorder()
	srv.handleSetupStatus(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"os":"linux"`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}
