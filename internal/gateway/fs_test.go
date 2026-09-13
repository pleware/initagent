package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pleware/initagent/internal/protocol"
)

func TestFsListForwardsToAgent(t *testing.T) {
	g := openTest(t, "")
	connectorID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeFsList {
			return nil, errors.New("unexpected " + m.Type)
		}
		return protocol.FsListResult{Path: "/home", Entries: []protocol.FsEntry{{Name: "a.txt", Size: 3}}}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/"+connectorID+"/fs?path=/home", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var res protocol.FsListResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Name != "a.txt" {
		t.Fatalf("res = %+v", res)
	}
}

func TestFsListOffline(t *testing.T) {
	g := openTest(t, "")
	connectorID, _, err := g.Store().CreateConnector(t.Context(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/"+connectorID+"/fs", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestFsDownloadMissingPath(t *testing.T) {
	g := openTest(t, "")
	connectorID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("should not reach agent")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/"+connectorID+"/fs/download", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSetupStatusForwardsToAgent(t *testing.T) {
	g := openTest(t, "")
	connectorID, agent, ts := connectAgent(t, g, protocol.Hello{Hostname: "box", OS: "linux", Arch: "amd64"})
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeExec {
			return nil, errors.New("unexpected " + m.Type)
		}
		return protocol.ExecResult{ExitCode: 0, Stdout: "installed\n1.2.3\n"}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/connectors/"+connectorID+"/setup", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var over map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &over); err != nil {
		t.Fatal(err)
	}
	if over["os"] != "linux" || over["arch"] != "amd64" {
		t.Fatalf("over = %+v", over)
	}
}
