package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/protocol"
)

func echoRPC(t *testing.T, agent *websocket.Conn, handle func(protocol.Msg) (any, error)) {
	t.Helper()
	go func() {
		for {
			var m protocol.Msg
			if err := agent.ReadJSON(&m); err != nil {
				return
			}
			payload, err := handle(m)
			reply, _ := protocol.NewMsg(protocol.TypeResult, m.Id, 0, payload)
			if err != nil {
				reply.Error = err.Error()
				reply.Data = nil
			}
			_ = agent.WriteJSON(reply)
		}
	}()
}

func TestSessionCreateOffline(t *testing.T) {
	g := openTest(t, "")
	deviceID, _, err := g.Store().CreateDevice(t.Context(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.NewReader(`{"name":"claude-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions", body)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "device is offline") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestSessionCreateRejectsBadDeviceID(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/devices/not-a-device/sessions", strings.NewReader(`{"name":"x"}`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionCreateForwardsToAgent(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	var got protocol.SessionCreate
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeSessionCreate {
			return nil, errors.New("unexpected " + m.Type)
		}
		if err := json.Unmarshal(m.Data, &got); err != nil {
			return nil, err
		}
		return nil, nil
	})

	body := strings.NewReader(`{"name":"claude-1","kind":"claude","command":"claude"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions", body)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if got.Name != "claude-1" || got.Kind != "claude" {
		t.Fatalf("forwarded = %+v", got)
	}
}

func TestSessionCreateWrongProjectIsOffline(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("should not reach agent")
	})
	other, err := id.New(id.Project)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions", strings.NewReader(`{"name":"x"}`))
	req.Header.Set(brand.ProjectHeader, other)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionListForwardsToAgent(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeSessionsList {
			return nil, errors.New("unexpected " + m.Type)
		}
		return protocol.SessionsListResult{Sessions: []protocol.Session{{Name: "term-1"}}}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/devices/"+deviceID+"/sessions", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var sessions []protocol.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Name != "term-1" {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestSessionKillForwardsToAgent(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	var killed string
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeSessionKill {
			return nil, errors.New("unexpected " + m.Type)
		}
		var req protocol.SessionKill
		if err := json.Unmarshal(m.Data, &req); err != nil {
			return nil, err
		}
		killed = req.Name
		return nil, nil
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/devices/"+deviceID+"/sessions/claude-1", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if killed != "claude-1" {
		t.Fatalf("killed = %q", killed)
	}
}

func TestExecForwardsToAgent(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeExec {
			return nil, errors.New("unexpected " + m.Type)
		}
		return protocol.ExecResult{ExitCode: 0, Stdout: "hi"}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/exec", strings.NewReader(`{"command":"echo hi"}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var res protocol.ExecResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "hi" {
		t.Fatalf("result = %+v", res)
	}
}

func TestExecRequiresCommand(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("should not reach agent")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/exec", strings.NewReader(`{"command":"  "}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionInputForwardsExec(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	var cmd string
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeExec {
			return nil, errors.New("unexpected " + m.Type)
		}
		var req protocol.Exec
		if err := json.Unmarshal(m.Data, &req); err != nil {
			return nil, err
		}
		cmd = req.Command
		return protocol.ExecResult{ExitCode: 0}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions/term-1/input", strings.NewReader(`{"text":"ls","enter":true}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(cmd, "tmux send-keys") || !strings.Contains(cmd, "term-1") {
		t.Fatalf("command = %q", cmd)
	}
}

func TestSessionInputNothingToSend(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("should not reach agent")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions/term-1/input", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionOutputForwardsExec(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(m protocol.Msg) (any, error) {
		if m.Type != protocol.TypeExec {
			return nil, errors.New("unexpected " + m.Type)
		}
		return protocol.ExecResult{ExitCode: 0, Stdout: "pane"}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/devices/"+deviceID+"/sessions/term-1/output?lines=10", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["output"] != "pane" {
		t.Fatalf("out = %+v", out)
	}
}

func TestSessionCreateRequiresName(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("should not reach agent")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionCreateAgentError(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("tmux missing")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions", strings.NewReader(`{"name":"x"}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionListEmptySessions(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return protocol.SessionsListResult{}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/devices/"+deviceID+"/sessions", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	raw, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "[]" && !strings.Contains(string(raw), "[]") {
		t.Fatalf("body = %s", raw)
	}
}

func TestSessionInputBadJSON(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("should not reach agent")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions/term-1/input", strings.NewReader(`{`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionInputNonzeroExit(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return protocol.ExecResult{ExitCode: 1, Stderr: "no tmux"}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/sessions/term-1/input", strings.NewReader(`{"enter":true}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSessionOutputNonzeroExit(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return protocol.ExecResult{ExitCode: 1, Stderr: "missing"}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/api/devices/"+deviceID+"/sessions/term-1/output", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestKillSessionAgentError(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("no such session")
	})
	req := httptest.NewRequest(http.MethodDelete, "/api/devices/"+deviceID+"/sessions/x", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestListSessionsAgentError(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("list failed")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/devices/"+deviceID+"/sessions", nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestExecAgentError(t *testing.T) {
	g := openTest(t, "")
	deviceID, agent, ts := connectAgentWS(t, g)
	echoRPC(t, agent, func(protocol.Msg) (any, error) {
		return nil, errors.New("exec failed")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+deviceID+"/exec", strings.NewReader(`{"command":"true","timeoutSec":5}`))
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}
