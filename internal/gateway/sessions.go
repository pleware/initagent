package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/connectorops"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/protocol"
)

const sessionRPCTimeout = 30 * time.Second

func (g *Gateway) liveConn(w http.ResponseWriter, r *http.Request) *agentConn {
	projectID, ok := g.resolveProject(w, r)
	if !ok {
		return nil
	}
	connectorID := r.PathValue("id")
	if !id.Is(id.Connector, connectorID) {
		httpError(w, http.StatusBadRequest, ErrBadConnectorID.Error())
		return nil
	}
	c := g.connForProject(projectID, connectorID)
	if c == nil {
		httpError(w, http.StatusServiceUnavailable, "connector is offline")
		return nil
	}
	return c
}

func (g *Gateway) handleListSessions(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sessionRPCTimeout)
	defer cancel()
	var res protocol.SessionsListResult
	if err := c.callInto(ctx, protocol.TypeSessionsList, nil, &res); err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.Sessions == nil {
		res.Sessions = []protocol.Session{}
	}
	writeJSON(w, res.Sessions)
}

func (g *Gateway) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	var req protocol.SessionCreate
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&req); err != nil || req.Name == "" {
		httpError(w, http.StatusBadRequest, "session name required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sessionRPCTimeout)
	defer cancel()
	if err := c.callInto(ctx, protocol.TypeSessionCreate, req, nil); err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (g *Gateway) handleKillSession(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sessionRPCTimeout)
	defer cancel()
	err := c.callInto(ctx, protocol.TypeSessionKill, protocol.SessionKill{Name: r.PathValue("name")}, nil)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (g *Gateway) handleSessionInput(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	var req struct {
		Text  string `json:"text"`
		Enter bool   `json:"enter"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	name := r.PathValue("name")
	cmd := ""
	if req.Text != "" {
		cmd = fmt.Sprintf("tmux send-keys -t %s -l %s", connectorops.ShellQuote(name), connectorops.ShellQuote(req.Text))
	}
	if req.Enter {
		if cmd != "" {
			cmd += " && "
		}
		cmd += fmt.Sprintf("tmux send-keys -t %s Enter", connectorops.ShellQuote(name))
	}
	if cmd == "" {
		httpError(w, http.StatusBadRequest, "nothing to send")
		return
	}
	res, err := connectorops.Exec(r.Context(), c, cmd, "", 15)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.ExitCode != 0 {
		httpError(w, http.StatusBadGateway, strings.TrimSpace(res.Stderr+res.Stdout))
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (g *Gateway) handleSessionOutput(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	lines := 200
	if l, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && l > 0 && l <= 10000 {
		lines = l
	}
	name := r.PathValue("name")
	cmd := fmt.Sprintf("tmux capture-pane -p -t %s -S -%d", connectorops.ShellQuote(name), lines)
	res, err := connectorops.Exec(r.Context(), c, cmd, "", 15)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.ExitCode != 0 {
		httpError(w, http.StatusBadGateway, strings.TrimSpace(res.Stderr+res.Stdout))
		return
	}
	writeJSON(w, map[string]string{"output": res.Stdout})
}

func (g *Gateway) handleExec(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	var req protocol.Exec
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&req); err != nil || strings.TrimSpace(req.Command) == "" {
		httpError(w, http.StatusBadRequest, "command required")
		return
	}
	res, err := connectorops.Exec(r.Context(), c, req.Command, req.Cwd, req.TimeoutSec)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, res)
}
