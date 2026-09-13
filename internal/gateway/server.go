package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/join"
	"github.com/pleware/initagent/internal/scheduler"
)

// Health is the JSON body of GET /health.
type Health struct {
	OK        bool   `json:"ok"`
	ProjectID string `json:"project_id"`
	Addr      string `json:"addr"`
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// fromHub guards the control routes — the ones only the hub calls, which are
// also the ones that now name a project. A worker's own routes are not on
// this list: they authenticate with an enroll token or a device credential,
// and the install script and health have to answer an unauthenticated
// caller. An empty secret leaves the routes open, which is the single-box
// self-host default; ops sets it wherever the gateway is reachable (18).
func (g *Gateway) fromHub(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if g.hubSecret == "" {
			next(w, r)
			return
		}
		presented, hasScheme := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !hasScheme || subtle.ConstantTimeCompare([]byte(presented), []byte(g.hubSecret)) != 1 {
			httpError(w, http.StatusUnauthorized, "gateway control routes require the hub secret")
			return
		}
		next(w, r)
	}
}

// Handler serves health, enroll, devices, binaries, the agent websocket,
// the terminal hop the hub proxies (16), session/exec HTTP the hub
// proxies the same way, and task enqueue/dispatch.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Health{OK: true, ProjectID: g.project.ID, Addr: g.addr})
	})
	mux.HandleFunc("POST /api/enroll-tokens", g.fromHub(g.handleCreateEnrollToken))
	mux.HandleFunc("POST /api/enroll", g.handleEnroll)
	mux.HandleFunc("GET /api/connectors", g.fromHub(g.handleListConnectors))
	mux.HandleFunc("GET /api/agents", g.fromHub(g.handleFleetAgents))
	mux.HandleFunc("GET /install/", g.joiner.ServeScript)
	mux.HandleFunc("GET /api/agent-binary", g.joiner.ServeBinary)
	mux.HandleFunc("GET /api/ws/agent", g.handleAgentWS)
	mux.HandleFunc("GET /api/ws/term", g.fromHub(g.handleTermWS))
	mux.HandleFunc("GET /api/connectors/{id}/sessions", g.fromHub(g.handleListSessions))
	mux.HandleFunc("POST /api/connectors/{id}/sessions", g.fromHub(g.handleCreateSession))
	mux.HandleFunc("DELETE /api/connectors/{id}/sessions/{name}", g.fromHub(g.handleKillSession))
	mux.HandleFunc("POST /api/connectors/{id}/sessions/{name}/input", g.fromHub(g.handleSessionInput))
	mux.HandleFunc("GET /api/connectors/{id}/sessions/{name}/output", g.fromHub(g.handleSessionOutput))
	mux.HandleFunc("POST /api/connectors/{id}/exec", g.fromHub(g.handleExec))
	mux.HandleFunc("GET /api/connectors/{id}/setup", g.fromHub(g.handleSetupStatus))
	mux.HandleFunc("GET /api/connectors/{id}/fs", g.fromHub(g.handleFsList))
	mux.HandleFunc("GET /api/connectors/{id}/fs/download", g.fromHub(g.handleFsDownload))
	mux.HandleFunc("POST /api/connectors/{id}/fs/upload", g.fromHub(g.handleFsUpload))
	mux.HandleFunc("POST /api/tasks", g.fromHub(g.handleCreateTask))
	mux.HandleFunc("GET /api/tasks/{id}", g.fromHub(g.handleGetTask))
	return mux
}

// projectFor resolves which project a hub-facing request acts on.
//
// The header is the hub's placement decision (18). Its absence means the
// project this process was started with, which is what keeps a self-host
// gateway a single flag and makes this change invisible to OSS. A named
// project is admitted on demand, because the foreign keys need a row.
func (g *Gateway) projectFor(ctx context.Context, r *http.Request) (string, error) {
	requested := strings.TrimSpace(r.Header.Get(brand.ProjectHeader))
	if requested == "" {
		return g.project.ID, nil
	}
	if !id.Is(id.Project, requested) {
		return "", fmt.Errorf("%w: %s", ErrBadProjectID, requested)
	}
	if _, err := g.store.EnsureProject(ctx, requested, g.addr); err != nil {
		return "", err
	}
	return requested, nil
}

// resolveProject answers the request itself on failure so handlers stay flat.
func (g *Gateway) resolveProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	projectID, err := g.projectFor(r.Context(), r)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, ErrBadProjectID) {
			code = http.StatusBadRequest
		}
		httpError(w, code, err.Error())
		return "", false
	}
	return projectID, true
}

func (g *Gateway) handleCreateEnrollToken(w http.ResponseWriter, r *http.Request) {
	projectID, ok := g.resolveProject(w, r)
	if !ok {
		return
	}
	token, err := g.store.CreateEnrollToken(r.Context(), projectID, EnrollTTL)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	unix, windows := join.Commands(g.joiner.BaseURL(r), token)
	writeJSON(w, EnrollOffer{
		Token:          token,
		Command:        unix,
		WindowsCommand: windows,
		ProjectID:      projectID,
	})
}

func (g *Gateway) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Hostname string `json:"hostname"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	projectID, ok, err := g.store.ConsumeEnrollToken(r.Context(), req.Token)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		httpError(w, http.StatusForbidden, "enrollment token is invalid, expired, or already used")
		return
	}
	name := req.Hostname
	if name == "" {
		name = "connector"
	}
	id, token, err := g.store.CreateConnector(r.Context(), projectID, name, req.Hostname, req.OS, req.Arch)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"connectorId": id, "connectorToken": token})
}

func (g *Gateway) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	projectID, ok := g.resolveProject(w, r)
	if !ok {
		return
	}
	devices, err := g.store.ListConnectors(r.Context(), projectID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]ConnectorView, 0, len(devices))
	for _, d := range devices {
		v := ConnectorView{Connector: d}
		if p, ok := g.presence(d.ID); ok {
			v.Online = true
			v.Tmux = p.hello.Tmux
			v.AgentVersion = p.hello.Version
			v.Platform = p.hello.Platform
			v.PlatformVersion = p.hello.PlatformVersion
			v.KernelVersion = p.hello.KernelVersion
			v.Stats = p.stats
		}
		out = append(out, v)
	}
	writeJSON(w, out)
}

func (g *Gateway) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	projectID, ok := g.resolveProject(w, r)
	if !ok {
		return
	}
	var req struct {
		Command     string `json:"command"`
		ConnectorID string `json:"connectorId"`
		Launch      string `json:"launch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		httpError(w, http.StatusBadRequest, ErrEmptyCommand.Error())
		return
	}
	launch, err := scheduler.NormalizeLaunch(req.Launch)
	if err != nil {
		httpError(w, http.StatusBadRequest, ErrUnknownLaunch.Error())
		return
	}
	worker := req.ConnectorID
	if worker == "" {
		worker = g.firstOnlineID(projectID)
		if worker == "" {
			if g.hasDraining(projectID) {
				httpError(w, http.StatusConflict, ErrConnectorDraining.Error())
				return
			}
			httpError(w, http.StatusServiceUnavailable, ErrConnectorOffline.Error())
			return
		}
	}
	if !id.Is(id.Connector, worker) {
		httpError(w, http.StatusBadRequest, ErrBadConnectorID.Error())
		return
	}
	// Scoped, not just online: a named connector- from another project must not
	// be reachable through this project's task surface (01).
	if g.connForProject(projectID, worker) == nil {
		httpError(w, http.StatusServiceUnavailable, ErrConnectorOffline.Error())
		return
	}
	if g.draining(worker) {
		httpError(w, http.StatusConflict, ErrConnectorDraining.Error())
		return
	}
	if _, err := g.store.Enqueue(r.Context(), scheduler.Task{
		ProjectID:  projectID,
		Command:    req.Command,
		LaunchMode: launch,
	}); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, err := g.RunQueued(r.Context(), projectID, worker)
	if err != nil {
		code := http.StatusInternalServerError
		switch {
		case errors.Is(err, scheduler.ErrNoFreeSlot):
			code = http.StatusConflict
		case errors.Is(err, ErrConnectorDraining):
			code = http.StatusConflict
		case errors.Is(err, ErrConnectorOffline):
			code = http.StatusServiceUnavailable
		}
		httpError(w, code, err.Error())
		return
	}
	writeJSON(w, view)
}

func (g *Gateway) handleGetTask(w http.ResponseWriter, r *http.Request) {
	projectID, ok := g.resolveProject(w, r)
	if !ok {
		return
	}
	task, err := g.store.Task(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, scheduler.ErrTaskNotFound) || errors.Is(err, ErrBadTaskID) {
			httpError(w, http.StatusNotFound, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A task- from another project answers 404, not the row: otherwise one
	// project's task ids are readable from another project's surface.
	if task.ProjectID != projectID {
		httpError(w, http.StatusNotFound, scheduler.ErrTaskNotFound.Error())
		return
	}
	writeJSON(w, viewTask(task, "", ""))
}

// Start binds addr and serves in the background until ctx is cancelled.
// The returned URL is what a hub should put in --gateway-url.
func (g *Gateway) Start(ctx context.Context, addr string) (string, error) {
	if addr == "" {
		addr = g.addr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	g.addr = ln.Addr().String()

	srv := &http.Server{Handler: g.Handler()}
	go func() {
		_ = srv.Serve(ln)
	}()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	go g.runEnrollPurge(ctx)
	return "http://" + g.addr, nil
}

func (g *Gateway) runEnrollPurge(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	purgeOnce := func() {
		n, err := g.store.PurgeEnrollTokens(context.Background(), time.Now())
		if err != nil {
			log.Printf("enroll token purge: %v", err)
			return
		}
		if n > 0 {
			log.Printf("enroll tokens: purged %d spent or expired rows older than %s", n, EnrollRetainFor)
		}
	}
	purgeOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			purgeOnce()
		}
	}
}

// Serve listens on addr until ctx is cancelled.
func (g *Gateway) Serve(ctx context.Context, addr string) error {
	if _, err := g.Start(ctx, addr); err != nil {
		return err
	}
	<-ctx.Done()
	if err := ctx.Err(); err == context.Canceled {
		return err
	}
	return ctx.Err()
}
