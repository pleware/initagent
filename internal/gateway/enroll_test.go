package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/protocol"
)

func TestCreateEnrollTokenHTTPUsesRequestHost(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/enroll-tokens", nil)
	req.Host = "gateway.example:4201"
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var offer EnrollOffer
	if err := json.NewDecoder(rec.Body).Decode(&offer); err != nil {
		t.Fatal(err)
	}
	if offer.Token == "" || offer.ProjectID != g.Project().ID {
		t.Fatalf("offer = %+v", offer)
	}
	if !strings.Contains(offer.Command, "http://gateway.example:4201/install/"+offer.Token+".sh") {
		t.Fatalf("command still not gateway: %q", offer.Command)
	}
	if strings.Contains(offer.Command, ":4200") {
		t.Fatalf("command leaked a hub port: %q", offer.Command)
	}
}

func TestEnrollConsumesTokenOnce(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	token, err := g.Store().CreateEnrollToken(ctx, g.Project().ID, EnrollTTL)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{
		"token": token, "hostname": "box", "os": "linux", "arch": "amd64",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/enroll", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		DeviceID    string `json:"deviceId"`
		DeviceToken string `json:"deviceToken"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !id.Is(id.Device, got.DeviceID) || got.DeviceToken == "" {
		t.Fatalf("got %+v", got)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/enroll", bytes.NewReader(body))
	rec2 := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("reuse status = %d", rec2.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	listRec := httptest.NewRecorder()
	g.Handler().ServeHTTP(listRec, listReq)
	var views []DeviceView
	if err := json.NewDecoder(listRec.Body).Decode(&views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].ID != got.DeviceID || views[0].Online {
		t.Fatalf("views = %+v", views)
	}
}

func TestEnrollRejectsExpiredToken(t *testing.T) {
	g := openTest(t, "")
	token, err := g.Store().CreateEnrollToken(context.Background(), g.Project().ID, -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"token": token, "hostname": "box"})
	req := httptest.NewRequest(http.MethodPost, "/api/enroll", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestEnrollRejectsBadJSON(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/enroll", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestInstallScriptEmbedsGateway(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/install/abc123.sh", nil)
	req.Host = "gw.example:4201"
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "HUB=\"http://gw.example:4201\"") {
		t.Fatalf("script = %s", body)
	}
	if !strings.Contains(body, "agent enroll --hub") {
		t.Fatal("script should enroll against the baked URL")
	}
}

func TestInstallScriptRejectsMetacharacters(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/install/x.sh", nil)
	req.URL.Path = `/install/x".sh`
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("quoted token should be rejected")
	}
}

func TestWindowsInstallScript(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/install/abc123.ps1", nil)
	req.Host = "gw.example:4201"
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `$Hub = "http://gw.example:4201"`) {
		t.Fatalf("script = %s", body)
	}
}

func TestPublicURLOverridesHost(t *testing.T) {
	g, err := Open(Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201", PublicURL: "https://join.example"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	req := httptest.NewRequest(http.MethodPost, "/api/enroll-tokens", nil)
	req.Host = "127.0.0.1:4201"
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	var offer EnrollOffer
	if err := json.NewDecoder(rec.Body).Decode(&offer); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(offer.Command, "https://join.example/install/") {
		t.Fatalf("command = %q", offer.Command)
	}
}

func TestAgentBinaryRejectsTraversal(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/agent-binary?os=linux&arch=..", nil)
	q := req.URL.Query()
	q.Set("os", "linux")
	q.Set("arch", "../etc")
	req.URL.RawQuery = q.Encode()
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code == http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAgentBinaryRedirectsOtherPlatform(t *testing.T) {
	g, err := Open(Options{
		DataDir:    t.TempDir(),
		Version:    "v0.1.0",
		GithubRepo: "pleware/initagent",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	req := httptest.NewRequest(http.MethodGet, "/api/agent-binary?os=plan9&arch=sparc64", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	want := "https://github.com/pleware/initagent/releases/download/v0.1.0/initagent_plan9_sparc64"
	if loc != want {
		t.Fatalf("Location = %q", loc)
	}
}

func TestAgentBinaryRequiresParams(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/agent-binary", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAgentWSHelloMarksOnline(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	_, token, err := g.Store().CreateDevice(ctx, g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	dev, err := g.Store().DeviceByToken(ctx, token)
	if err != nil || dev == nil {
		t.Fatalf("device: %v %+v", err, dev)
	}

	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/agent"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	hello, err := protocol.NewMsg(protocol.TypeHello, 0, 0, protocol.Hello{
		Hostname: "box", OS: "linux", Arch: "amd64", Tmux: true, Version: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatal(err)
	}
	var welcome protocol.Msg
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}
	if welcome.Type != protocol.TypeWelcome {
		t.Fatalf("type = %s", welcome.Type)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		rec := httptest.NewRecorder()
		g.Handler().ServeHTTP(rec, req)
		var views []DeviceView
		if err := json.NewDecoder(rec.Body).Decode(&views); err != nil {
			t.Fatal(err)
		}
		if len(views) == 1 && views[0].Online && views[0].Tmux {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	conn.Close()
	t.Fatal("device did not appear online")
}

func TestAgentWSRejectsUnknownToken(t *testing.T) {
	g := openTest(t, "")
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/agent"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
	})
	if err == nil {
		t.Fatal("expected reject")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestAgentWSRejectsMissingBearer(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/ws/agent", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeviceByIDAndBadID(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	did, _, err := g.Store().CreateDevice(ctx, g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.Store().DeviceByID(ctx, did)
	if err != nil || got == nil || got.ID != did {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := g.Store().DeviceByID(ctx, "tsk-nope"); err == nil {
		t.Fatal("expected bad id")
	}
	missing, err := id.New(id.Device)
	if err != nil {
		t.Fatal(err)
	}
	got, err = g.Store().DeviceByID(ctx, missing)
	if err != nil || got != nil {
		t.Fatalf("missing: %+v %v", got, err)
	}
}

// A device credential has to answer which project it belongs to: one gateway
// process serves many projects, so the socket cannot inherit one (18).
func TestDeviceCarriesItsProject(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	did, token, err := g.Store().CreateDevice(ctx, g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	byToken, err := g.Store().DeviceByToken(ctx, token)
	if err != nil || byToken == nil {
		t.Fatalf("by token: %+v %v", byToken, err)
	}
	if byToken.ProjectID != g.Project().ID {
		t.Fatalf("token project = %q, want %q", byToken.ProjectID, g.Project().ID)
	}
	byID, err := g.Store().DeviceByID(ctx, did)
	if err != nil || byID == nil {
		t.Fatalf("by id: %+v %v", byID, err)
	}
	if byID.ProjectID != g.Project().ID {
		t.Fatalf("id project = %q", byID.ProjectID)
	}
	listed, err := g.Store().ListDevices(ctx, g.Project().ID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed = %+v %v", listed, err)
	}
	if listed[0].ProjectID != g.Project().ID {
		t.Fatalf("listed project = %q", listed[0].ProjectID)
	}
}

func TestCreateDeviceRejectsBadProject(t *testing.T) {
	g := openTest(t, "")
	if _, _, err := g.Store().CreateDevice(context.Background(), "not-prj", "n", "", "", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestListDevicesRejectsBadProject(t *testing.T) {
	g := openTest(t, "")
	if _, err := g.Store().ListDevices(context.Background(), "nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnrollDefaultDeviceName(t *testing.T) {
	g := openTest(t, "")
	token, err := g.Store().CreateEnrollToken(context.Background(), g.Project().ID, EnrollTTL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"token": token})
	req := httptest.NewRequest(http.MethodPost, "/api/enroll", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	devices, err := g.Store().ListDevices(context.Background(), g.Project().ID)
	if err != nil || len(devices) != 1 || devices[0].Name != "device" {
		t.Fatalf("devices = %+v %v", devices, err)
	}
}

func TestEnrollAfterClose(t *testing.T) {
	g := openTest(t, "")
	token, err := g.Store().CreateEnrollToken(context.Background(), g.Project().ID, EnrollTTL)
	if err != nil {
		t.Fatal(err)
	}
	g.Close()
	body, _ := json.Marshal(map[string]string{"token": token, "hostname": "box"})
	req := httptest.NewRequest(http.MethodPost, "/api/enroll", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateEnrollTokenAfterClose(t *testing.T) {
	g := openTest(t, "")
	g.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/enroll-tokens", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListDevicesAfterClose(t *testing.T) {
	g := openTest(t, "")
	g.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAgentBinaryServesDroppedFile(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "binaries")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("fake-agent")
	if err := os.WriteFile(filepath.Join(binDir, "initagent_plan9_sparc64"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	req := httptest.NewRequest(http.MethodGet, "/api/agent-binary?os=plan9&arch=sparc64", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != string(payload) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestAgentBinaryNotFoundWithoutRepo(t *testing.T) {
	g := openTest(t, "")
	g.joiner.GithubRepo = ""
	req := httptest.NewRequest(http.MethodGet, "/api/agent-binary?os=plan9&arch=sparc64", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAgentBinaryCurrentPlatform(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/agent-binary?os="+runtime.GOOS+"&arch="+runtime.GOARCH, nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAgentWSStatsMarkPresence(t *testing.T) {
	g := openTest(t, "")
	ctx := context.Background()
	_, token, err := g.Store().CreateDevice(ctx, g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/agent"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	hello, _ := protocol.NewMsg(protocol.TypeHello, 0, 0, protocol.Hello{Hostname: "box", OS: "linux"})
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatal(err)
	}
	var welcome protocol.Msg
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}
	stats, _ := protocol.NewMsg(protocol.TypeStats, 0, 0, protocol.Stats{CPUCores: 4})
	if err := conn.WriteJSON(stats); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		rec := httptest.NewRecorder()
		g.Handler().ServeHTTP(rec, req)
		var views []DeviceView
		if err := json.NewDecoder(rec.Body).Decode(&views); err != nil {
			t.Fatal(err)
		}
		if len(views) == 1 && views[0].Online && views[0].Stats != nil && views[0].Stats.CPUCores == 4 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("stats did not land")
}

func TestAgentWSRejectsNonHello(t *testing.T) {
	g := openTest(t, "")
	_, token, err := g.Store().CreateDevice(context.Background(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/agent"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, _ := protocol.NewMsg(protocol.TypeStats, 0, 0, protocol.Stats{})
	_ = conn.WriteJSON(stats)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	var msg protocol.Msg
	err = conn.ReadJSON(&msg)
	if err == nil {
		t.Fatal("expected close after bad hello")
	}
	_ = conn.Close()
}
