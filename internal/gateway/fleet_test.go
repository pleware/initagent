package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pleware/initagent/internal/protocol"
)

func TestFleetAgentsListsSessionsAcrossConnectors(t *testing.T) {
	g := openTest(t, "")
	connectorID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeSessionsList {
			return nil, errors.New("unexpected " + m.Type)
		}
		return protocol.SessionsListResult{Sessions: []protocol.Session{{Name: "term-1", Kind: "shell"}}}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var rows []fleetAgent
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one", rows)
	}
	if rows[0].Name != "term-1" || rows[0].ConnectorId != connectorID || rows[0].ConnectorName != "box" {
		t.Fatalf("row = %+v", rows[0])
	}
}

func TestFleetAgentsEmptyWhenOffline(t *testing.T) {
	g := openTest(t, "")
	if _, _, err := g.Store().CreateConnector(t.Context(), g.Project().ID, "box", "box", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var rows []fleetAgent
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}
