package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// recordingGateway remembers which project and secret the hub presented.
type recordingGateway struct {
	project string
	secret  string
	hits    int
}

func (g *recordingGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.hits++
	g.project = r.Header.Get("X-Initagent-Project")
	g.secret = r.Header.Get("Authorization")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": "tsk-1", "state": "done"})
}

// project inserts a project row placed on gatewayURL and returns its prj-.
func addProject(t *testing.T, srv *Server, name, gatewayURL string) string {
	t.Helper()
	org, err := srv.store.CreateOrg(name + " org")
	if err != nil {
		t.Fatal(err)
	}
	p, err := srv.store.CreateProject(org.Id, name, "", "", gatewayURL, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return p.Id
}

// Placement is the project's column, not the hub's flag: a project on a
// second gateway must be reached there, without a provisioner (18).
func TestTaskGoesToTheProjectsOwnGateway(t *testing.T) {
	first := &recordingGateway{}
	firstTS := httptest.NewServer(first)
	t.Cleanup(firstTS.Close)
	second := &recordingGateway{}
	secondTS := httptest.NewServer(second)
	t.Cleanup(secondTS.Close)

	srv, ts, token := newTaskHub(t, firstTS.URL)
	addProject(t, srv, "one", firstTS.URL)
	elsewhere := addProject(t, srv, "two", secondTS.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks?project="+elsewhere,
		[]byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if second.hits != 1 || first.hits != 0 {
		t.Fatalf("hits: second = %d, first = %d; the flag must not win over the column", second.hits, first.hits)
	}
	if second.project != elsewhere {
		t.Fatalf("project header = %q, want %q", second.project, elsewhere)
	}
}

// Two projects and no explicit one is a 400. Guessing would run a command
// against the wrong company's machine.
func TestTwoProjectsWithoutAnExplicitOneIsRefused(t *testing.T) {
	gw := &recordingGateway{}
	gwTS := httptest.NewServer(gw)
	t.Cleanup(gwTS.Close)

	srv, ts, token := newTaskHub(t, gwTS.URL)
	addProject(t, srv, "one", gwTS.URL)
	addProject(t, srv, "two", gwTS.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks", []byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if gw.hits != 0 {
		t.Fatal("refused request still reached a gateway")
	}
}

// One project needs no query parameter, which is what keeps the free plan
// and self-host working without a cockpit change.
func TestSoleProjectNeedsNoQueryParameter(t *testing.T) {
	gw := &recordingGateway{}
	gwTS := httptest.NewServer(gw)
	t.Cleanup(gwTS.Close)

	srv, ts, token := newTaskHub(t, "http://unused.invalid")
	only := addProject(t, srv, "only", gwTS.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks", []byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gw.project != only {
		t.Fatalf("project header = %q, want %q", gw.project, only)
	}
}

// An unknown prj- answers 404 rather than falling back to the flag, so a
// typo cannot silently run somewhere else.
func TestUnknownProjectIsNotFound(t *testing.T) {
	gw := &recordingGateway{}
	gwTS := httptest.NewServer(gw)
	t.Cleanup(gwTS.Close)

	srv, ts, token := newTaskHub(t, gwTS.URL)
	addProject(t, srv, "one", gwTS.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks?project=prj-doesnotexist",
		[]byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if gw.hits != 0 {
		t.Fatal("unknown project still reached a gateway")
	}
}

// A project row written before placement was read has an empty column; the
// flag is the fallback so those rows keep working.
func TestEmptyColumnFallsBackToTheFlag(t *testing.T) {
	gw := &recordingGateway{}
	gwTS := httptest.NewServer(gw)
	t.Cleanup(gwTS.Close)

	srv, ts, token := newTaskHub(t, gwTS.URL)
	legacy := addProject(t, srv, "legacy", "")

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks?project="+legacy,
		[]byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gw.project != legacy {
		t.Fatalf("project header = %q, want %q", gw.project, legacy)
	}
}

// The secret is the hub proving it is the hub on the gateway's control
// routes. Unset means today's open single-box behavior.
func TestGatewaySecretIsPresentedWhenSet(t *testing.T) {
	gw := &recordingGateway{}
	gwTS := httptest.NewServer(gw)
	t.Cleanup(gwTS.Close)

	srv, ts, token := newTaskHub(t, gwTS.URL)
	srv.opts.GatewaySecret = "shared-secret"
	addProject(t, srv, "one", gwTS.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks", []byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gw.secret != "Bearer shared-secret" {
		t.Fatalf("Authorization = %q", gw.secret)
	}
}

// A hub with the flag but no project row yet is a self-host box before
// anyone created a project: no header, and the gateway answers for the
// project it was started with.
func TestNoProjectRowStillUsesTheFlag(t *testing.T) {
	gw := &recordingGateway{}
	gwTS := httptest.NewServer(gw)
	t.Cleanup(gwTS.Close)

	_, ts, token := newTaskHub(t, gwTS.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/tasks", []byte(`{"command":"echo hi"}`), token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gw.project != "" {
		t.Fatalf("project header = %q, want none", gw.project)
	}
}

// Enroll must target the project's gateway, because the worker dials the URL
// baked into the command it is given (10).
func TestEnrollTokenGoesToTheProjectsGateway(t *testing.T) {
	first := &recordingGateway{}
	firstTS := httptest.NewServer(first)
	t.Cleanup(firstTS.Close)
	second := &recordingGateway{}
	secondTS := httptest.NewServer(second)
	t.Cleanup(secondTS.Close)

	srv, ts, token := newTaskHub(t, firstTS.URL)
	addProject(t, srv, "one", firstTS.URL)
	elsewhere := addProject(t, srv, "two", secondTS.URL)

	resp := authedRequest(t, http.MethodPost, ts.URL+"/api/enroll-tokens?project="+elsewhere, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if second.hits != 1 || second.project != elsewhere {
		t.Fatalf("second gateway: hits = %d, project = %q", second.hits, second.project)
	}
}
