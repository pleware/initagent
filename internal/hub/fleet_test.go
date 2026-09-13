package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFleetAgentsAsksGateway(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agents" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"term-1","kind":"shell","connectorId":"connector-gw","connectorName":"gwbox"}]`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	w := httptest.NewRecorder()
	srv.handleFleetAgents(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	var rows []fleetSession
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ConnectorName != "gwbox" || rows[0].Name != "term-1" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestFleetAgentsEmptyWithoutGateway(t *testing.T) {
	srv := newTestServer(t, "v0.1.0")
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	w := httptest.NewRecorder()
	srv.handleFleetAgents(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	var rows []fleetSession
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}

func TestUpdateStatusCountsGatewayDevices(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/connectors" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"isHub":false,"agentVersion":"v0.0.9"},{"isHub":false,"agentVersion":"v0.1.0"}]`))
	}))
	t.Cleanup(gw.Close)

	srv := newTestServer(t, "v0.1.0")
	srv.opts.GatewayURL = gw.URL
	req := httptest.NewRequest(http.MethodGet, "/api/updates", nil)
	w := httptest.NewRecorder()
	srv.handleUpdateStatus(w, req, operatorCred)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body.String())
	}
	var status updateStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.FleetTotal != 2 {
		t.Fatalf("fleetTotal = %d, want 2", status.FleetTotal)
	}
	if status.FleetOutdated != 1 {
		t.Fatalf("fleetOutdated = %d, want 1", status.FleetOutdated)
	}
}
