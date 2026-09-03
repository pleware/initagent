package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/id"
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

func (g *Gateway) baseURL(r *http.Request) string {
	if g.publicURL != "" {
		return g.publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// Handler serves health, enroll, devices, binaries, the agent websocket,
// and task enqueue/dispatch.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Health{OK: true, ProjectID: g.project.ID, Addr: g.addr})
	})
	mux.HandleFunc("POST /api/enroll-tokens", g.handleCreateEnrollToken)
	mux.HandleFunc("POST /api/enroll", g.handleEnroll)
	mux.HandleFunc("GET /api/devices", g.handleListDevices)
	mux.HandleFunc("GET /install/", g.handleInstallScript)
	mux.HandleFunc("GET /api/agent-binary", g.handleAgentBinary)
	mux.HandleFunc("GET /api/ws/agent", g.handleAgentWS)
	mux.HandleFunc("POST /api/tasks", g.handleCreateTask)
	mux.HandleFunc("GET /api/tasks/{id}", g.handleGetTask)
	return mux
}

func (g *Gateway) handleCreateEnrollToken(w http.ResponseWriter, r *http.Request) {
	token, err := g.store.CreateEnrollToken(r.Context(), g.project.ID, EnrollTTL)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	unix, windows := InstallCommands(g.baseURL(r), token)
	writeJSON(w, EnrollOffer{
		Token:          token,
		Command:        unix,
		WindowsCommand: windows,
		ProjectID:      g.project.ID,
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
		name = "device"
	}
	id, token, err := g.store.CreateDevice(r.Context(), projectID, name, req.Hostname, req.OS, req.Arch)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"deviceId": id, "deviceToken": token})
}

func (g *Gateway) handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := g.store.ListDevices(r.Context(), g.project.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]DeviceView, 0, len(devices))
	for _, d := range devices {
		v := DeviceView{Device: d}
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

func (g *Gateway) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/install/")
	token, ok := strings.CutSuffix(name, ".sh")
	format := "sh"
	if !ok {
		token, ok = strings.CutSuffix(name, ".ps1")
		format = "ps1"
	}
	if !ok || !isSimpleToken(token) {
		httpError(w, http.StatusNotFound, "not found")
		return
	}
	base := g.baseURL(r)
	if format == "ps1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, windowsInstallScript, base, token)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	fmt.Fprintf(w, unixInstallScript, base, token)
}

func (g *Gateway) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command  string `json:"command"`
		DeviceID string `json:"deviceId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		httpError(w, http.StatusBadRequest, ErrEmptyCommand.Error())
		return
	}
	worker := req.DeviceID
	if worker == "" {
		worker = g.firstOnlineID()
	}
	if worker == "" {
		httpError(w, http.StatusServiceUnavailable, ErrDeviceOffline.Error())
		return
	}
	if !id.Is(id.Device, worker) {
		httpError(w, http.StatusBadRequest, ErrBadDeviceID.Error())
		return
	}
	if g.connFor(worker) == nil {
		httpError(w, http.StatusServiceUnavailable, ErrDeviceOffline.Error())
		return
	}
	if _, err := g.store.Enqueue(r.Context(), scheduler.Task{
		ProjectID: g.project.ID,
		Command:   req.Command,
	}); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, err := g.RunQueued(r.Context(), worker)
	if err != nil {
		code := http.StatusInternalServerError
		switch {
		case errors.Is(err, scheduler.ErrNoFreeSlot):
			code = http.StatusConflict
		case errors.Is(err, ErrDeviceOffline):
			code = http.StatusServiceUnavailable
		}
		httpError(w, code, err.Error())
		return
	}
	writeJSON(w, view)
}

func (g *Gateway) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task, err := g.store.Task(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, scheduler.ErrTaskNotFound) || errors.Is(err, ErrBadTaskID) {
			httpError(w, http.StatusNotFound, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, viewTask(task, "", ""))
}

// Serve listens on addr until ctx is cancelled.
func (g *Gateway) Serve(ctx context.Context, addr string) error {
	if addr == "" {
		addr = g.addr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	g.addr = ln.Addr().String()

	srv := &http.Server{Handler: g.Handler()}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
		return ctx.Err()
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
