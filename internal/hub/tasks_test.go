package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeGateway serves canned task responses and records what the hub proxied.
type fakeGateway struct {
	path   string
	body   map[string]any
	status int
	view   map[string]any
}

func (f *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.path = r.URL.Path
	f.body = map[string]any{}
	_ = json.NewDecoder(r.Body).Decode(&f.body)
	w.Header().Set("Content-Type", "application/json")
	if f.status == 0 {
		f.status = http.StatusOK
	}
	w.WriteHeader(f.status)
	_ = json.NewEncoder(w).Encode(f.view)
}

func newTaskHub(t *testing.T, gatewayURL string) (*Server, *httptest.Server, string) {
	t.Helper()
	srv, err := NewServer(Options{Addr: "127.0.0.1:0", DataDir: t.TempDir(), GatewayURL: gatewayURL})
	if err != nil {
		t.Fatal(err)
	}
	// The hub store keeps its SQLite handle open; close it so t.TempDir can
	// unlink the file on Windows.
	t.Cleanup(func() { _ = srv.store.Close() })
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	token, err := srv.store.CreateApiToken("test")
	if err != nil {
		t.Fatal(err)
	}
	return srv, ts, token
}

func authedRequest(t *testing.T, method, url string, body []byte, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestCreateTaskProxyForwardsBody(t *testing.T) {
	fake := &fakeGateway{view: map[string]any{
		"id": "tsk-1", "state": "done", "command": "echo hi",
		"exitCode": 0, "reason": "exec", "stdout": "hi\n",
	}}
	gateway := httptest.NewServer(fake)
	t.Cleanup(gateway.Close)

	_, ts, token := newTaskHub(t, gateway.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks", []byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var view map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view["state"] != "done" || view["reason"] != "exec" || view["stdout"] != "hi\n" {
		t.Fatalf("view = %+v", view)
	}
	if fake.path != "/api/tasks" {
		t.Fatalf("proxy path = %q, want /api/tasks", fake.path)
	}
	if fake.body["command"] != "echo hi" {
		t.Fatalf("forwarded body = %+v, want command", fake.body)
	}
}

func TestGetTaskProxy(t *testing.T) {
	fake := &fakeGateway{view: map[string]any{"id": "tsk-9", "state": "failed", "exitCode": 1}}
	gateway := httptest.NewServer(fake)
	t.Cleanup(gateway.Close)

	_, ts, token := newTaskHub(t, gateway.URL)

	resp := authedRequest(t, http.MethodGet, ts.URL+"/api/tasks/tsk-9", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if fake.path != "/api/tasks/tsk-9" {
		t.Fatalf("proxy path = %q, want /api/tasks/tsk-9", fake.path)
	}
}

func TestCreateTaskRequiresGatewayURL(t *testing.T) {
	_, ts, token := newTaskHub(t, "")

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks", []byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

func TestCreateTaskRequiresAuth(t *testing.T) {
	fake := &fakeGateway{view: map[string]any{}}
	gateway := httptest.NewServer(fake)
	t.Cleanup(gateway.Close)

	_, ts, _ := newTaskHub(t, gateway.URL)

	resp, err := http.Post(ts.URL+"/api/tasks", "application/json", bytes.NewReader([]byte(`{"command":"echo hi"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
