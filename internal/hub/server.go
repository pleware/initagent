// Package hub implements the initagent hub: web UI host, REST API, and the
// rendezvous point every device agent dials into.
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/pleware/initagent/internal/agent"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/join"
	"github.com/pleware/initagent/internal/offering"
)

// Options configures a hub server.
type Options struct {
	Addr       string // listen address, e.g. ":4200" (ignored when TLSDomain is set)
	DataDir    string // where the hub SQLite file and binaries/ live
	Version    string
	GithubRepo string // "owner/name" used to fetch agent binaries for other platforms
	UI         fs.FS  // embedded web UI (nil = API only)

	// GatewayURL is the project gateway the cockpit enrolls workers into.
	// Empty means enroll-token minting refuses rather than baking r.Host.
	GatewayURL string

	// DatabaseURL, when set, points the store at Postgres instead of the
	// default SQLite file under DataDir. Empty = SQLite (self-host / OSS).
	DatabaseURL string

	// Offering is how this installation was started (draft 18), resolved by
	// internal/offering before the server is built. It sets the password
	// floor at first-run and is reported to the login screen. The zero value
	// is treated as the stricter (hosted) case rather than the laxer one.
	Offering offering.Kind

	// TLSDomain enables automatic HTTPS via Let's Encrypt for this exact
	// domain. When set, the hub serves HTTPS on :443 and runs an HTTP server on
	// :80 for the ACME challenge + a redirect to HTTPS. TLSEmail is sent to the
	// CA for expiry notices.
	TLSDomain string
	TLSEmail  string
}

// Server is the hub.
type Server struct {
	opts Options
	// installer serves /install/<token> and the agent binary. Enroll belongs
	// to the gateway (draft 10); this is the inherited single-plane path, and
	// it shares the gateway's implementation rather than carrying a copy.
	installer     join.Installer
	store         *Store
	claim         *bootstrapClaim
	sessions      *sessionManager
	loginRL       *rateLimiter
	registerRL    *rateLimiter
	events        *eventBus
	registry      *registry
	mux           *http.ServeMux
	updates       *updateManager
	updateApplied atomic.Bool

	// internalURL is a loopback-only plain-HTTP address serving the same mux.
	// The embedded agent and the MCP endpoint use it so they work identically
	// whether the public listener is HTTP or HTTPS.
	internalURL string
}

func NewServer(opts Options) (*Server, error) {
	if opts.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		opts.DataDir = filepath.Join(home, brand.ConfigDir)
	}
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return nil, err
	}
	if opts.GithubRepo == "" {
		opts.GithubRepo = brand.ReleaseSource
	}
	var store *Store
	var err error
	if opts.DatabaseURL != "" {
		store, err = OpenStorePostgres(opts.DatabaseURL)
	} else {
		store, err = OpenStore(filepath.Join(opts.DataDir, brand.DBFile))
	}
	if err != nil {
		return nil, err
	}
	events := newEventBus()
	s := &Server{
		opts: opts,
		installer: join.Installer{
			DataDir:    opts.DataDir,
			GithubRepo: opts.GithubRepo,
			Version:    opts.Version,
		},
		store:      store,
		sessions:   newSessionManager(),
		loginRL:    newRateLimiter(),
		registerRL: newRateLimiter(),
		events:     events,
		registry:   newRegistry(events),
		mux:        http.NewServeMux(),
	}
	claimed, err := s.claimed()
	if err != nil {
		store.Close()
		return nil, err
	}
	if s.claim, err = newBootstrapClaim(opts.DataDir, claimed); err != nil {
		store.Close()
		return nil, err
	}
	// A hub claimed before organizations existed has an owner and nothing to
	// own. First-run never runs twice, so this is the only moment left to
	// give it the org that claiming creates today.
	org, err := store.BackfillOperatorOrg()
	if err != nil {
		store.Close()
		return nil, err
	}
	if org != nil {
		log.Printf("created organization %q (%s) for this hub's existing admin account", org.Name, org.Id)
	}
	s.routes()
	s.updates = newUpdateManager(store, opts.Version, opts.GithubRepo)
	return s, nil
}

// newHTTPServer builds an http.Server for the shared mux with sane timeouts.
// ReadTimeout/WriteTimeout are intentionally unset: they would break
// long-lived WebSockets and large transfers (hijacked conns manage their own
// deadlines via gorilla).
func (s *Server) newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// Run serves until ctx is cancelled. It also starts the embedded local agent
// so the hub machine itself shows up as a device.
func (s *Server) Run(ctx context.Context) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	s.updates.start(runCtx, func() {
		s.updateApplied.Store(true)
		cancelRun()
	})

	// Internal loopback listener: always plain HTTP, used by the embedded agent
	// and the MCP endpoint's in-process client regardless of public TLS.
	internalLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.internalURL = "http://" + internalLn.Addr().String()
	internalSrv := s.newHTTPServer(s.mux)
	go internalSrv.Serve(internalLn)

	go func() {
		<-runCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		internalSrv.Shutdown(shutdownCtx)
	}()
	go s.runEmbeddedAgent(runCtx)

	if s.opts.TLSDomain != "" {
		err := s.runTLS(runCtx)
		if s.updateApplied.Load() {
			return nil
		}
		return err
	}
	err = s.runPlain(runCtx)
	if s.updateApplied.Load() {
		return nil
	}
	return err
}

// runPlain serves HTTP on the configured address.
func (s *Server) runPlain(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return err
	}
	srv := s.newHTTPServer(s.mux)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Printf("%s hub listening on http://%s", brand.Name, displayAddr(ln.Addr().String()))
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// runTLS serves HTTPS with automatic Let's Encrypt certificates for the
// configured domain, plus a plain-HTTP server on :80 that answers the ACME
// challenge and redirects everything else to HTTPS.
func (s *Server) runTLS(ctx context.Context) error {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(s.opts.TLSDomain),
		Cache:      autocert.DirCache(filepath.Join(s.opts.DataDir, "certs")),
		Email:      s.opts.TLSEmail,
	}

	// :80 — serves the HTTP-01 challenge and redirects real traffic to HTTPS.
	httpSrv := s.newHTTPServer(m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+s.opts.TLSDomain+r.URL.RequestURI(), http.StatusMovedPermanently)
	})))
	httpSrv.Addr = ":80"
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http (:80) challenge server: %v", err)
		}
	}()

	httpsSrv := s.newHTTPServer(s.mux)
	httpsSrv.Addr = ":443"
	httpsSrv.TLSConfig = m.TLSConfig() // negotiates certs + acme-tls/1

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		httpsSrv.Shutdown(shutdownCtx)
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("%s hub listening on https://%s (auto-TLS via Let's Encrypt)", brand.Name, s.opts.TLSDomain)
	// Certs are supplied by TLSConfig, so no cert/key files here.
	if err := httpsSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// runEmbeddedAgent registers (once) and runs an in-process agent for the hub
// machine, connecting over the internal loopback listener like any other device.
func (s *Server) runEmbeddedAgent(ctx context.Context) {
	token, err := s.store.Setting("hub_device_token")
	if err != nil {
		log.Printf("embedded agent: %v", err)
		return
	}
	if token == "" {
		hostname, _ := os.Hostname()
		name := hostname
		if name == "" {
			name = "hub"
		}
		_, tok, err := s.store.CreateDevice(name, hostname, "", "", true)
		if err != nil {
			log.Printf("embedded agent: registering hub device: %v", err)
			return
		}
		if err := s.store.SetSetting("hub_device_token", tok); err != nil {
			log.Printf("embedded agent: %v", err)
			return
		}
		token = tok
	}
	cfg := agent.Config{HubURL: s.internalURL, Token: token}
	if err := agent.New(cfg, s.opts.Version).Run(ctx); err != nil && ctx.Err() == nil {
		log.Printf("embedded agent stopped: %v", err)
	}
}

func (s *Server) routes() {
	m := s.mux

	// Public (pre-auth) endpoints.
	m.HandleFunc("POST /api/setup", s.handleSetup)
	m.HandleFunc("POST /api/login", s.handleLogin)
	m.HandleFunc("POST /api/register", s.handleRegister)
	m.HandleFunc("POST /api/logout", s.handleLogout)
	m.HandleFunc("GET /api/me", s.handleMe)
	m.HandleFunc("POST /api/enroll", s.handleEnroll)
	m.HandleFunc("GET /install/", s.installer.ServeScript)
	m.HandleFunc("GET /api/agent-binary", s.installer.ServeBinary)
	m.HandleFunc("GET /api/ws/agent", s.handleAgentWS)

	// Remote MCP endpoint (does its own Bearer-token auth).
	m.HandleFunc("/mcp", s.handleMCPHTTP)

	// Authenticated API.
	m.HandleFunc("GET /api/devices", s.requireAuth(s.handleListDevices))
	m.HandleFunc("PATCH /api/devices/{id}", s.requireAuth(s.handleRenameDevice))
	m.HandleFunc("DELETE /api/devices/{id}", s.requireAuth(s.handleDeleteDevice))
	m.HandleFunc("POST /api/enroll-tokens", s.requireAuth(s.handleCreateEnrollToken))
	m.HandleFunc("GET /api/devices/{id}/sessions", s.requireAuth(s.handleListSessions))
	m.HandleFunc("POST /api/devices/{id}/sessions", s.requireAuth(s.handleCreateSession))
	m.HandleFunc("DELETE /api/devices/{id}/sessions/{name}", s.requireAuth(s.handleKillSession))
	m.HandleFunc("POST /api/devices/{id}/sessions/{name}/input", s.requireAuth(s.handleSessionInput))
	m.HandleFunc("GET /api/devices/{id}/sessions/{name}/output", s.requireAuth(s.handleSessionOutput))
	m.HandleFunc("POST /api/devices/{id}/exec", s.requireAuth(s.handleExec))
	m.HandleFunc("GET /api/devices/{id}/setup", s.requireAuth(s.handleSetupStatus))
	m.HandleFunc("GET /api/devices/{id}/fs", s.requireAuth(s.handleFsList))
	m.HandleFunc("GET /api/devices/{id}/fs/download", s.requireAuth(s.handleFsDownload))
	m.HandleFunc("POST /api/devices/{id}/fs/upload", s.requireAuth(s.handleFsUpload))
	m.HandleFunc("GET /api/projects", s.requireActor(s.handleListProjects))
	m.HandleFunc("POST /api/projects", s.requireActor(s.handleCreateProject))
	m.HandleFunc("PATCH /api/projects/{id}", s.requireActor(s.handleUpdateProject))
	m.HandleFunc("DELETE /api/projects/{id}", s.requireActor(s.handleDeleteProject))
	m.HandleFunc("POST /api/projects/{id}/exec", s.requireActor(s.handleProjectExec))
	m.HandleFunc("POST /api/tasks", s.requireAuth(s.handleCreateTask))
	m.HandleFunc("GET /api/tasks/{id}", s.requireAuth(s.handleGetTask))
	m.HandleFunc("GET /api/agents", s.requireAuth(s.handleFleetAgents))
	m.HandleFunc("GET /api/presets", s.requireAuth(s.handleListPresets))
	m.HandleFunc("POST /api/presets", s.requireAuth(s.handleCreatePreset))
	m.HandleFunc("DELETE /api/presets/{id}", s.requireAuth(s.handleDeletePreset))
	m.HandleFunc("GET /api/tokens", s.requireAuth(s.handleListApiTokens))
	m.HandleFunc("POST /api/tokens", s.requireAuth(s.handleCreateApiToken))
	m.HandleFunc("DELETE /api/tokens/{id}", s.requireAuth(s.handleDeleteApiToken))
	m.HandleFunc("GET /api/updates", s.requireAuth(s.handleUpdateStatus))
	m.HandleFunc("POST /api/updates/check", s.requireAuth(s.handleUpdateCheck))
	m.HandleFunc("PATCH /api/updates", s.requireAuth(s.handleUpdateSettings))
	m.HandleFunc("POST /api/updates/install", s.requireAuth(s.handleUpdateInstall))
	m.HandleFunc("POST /api/updates/rollback", s.requireAuth(s.handleUpdateRollback))
	m.HandleFunc("GET /api/ws/term", s.requireAuth(s.handleTermWS))
	m.HandleFunc("GET /api/ws/events", s.requireAuth(s.handleEventsWS))

	// Hub surfaces: the installation's operator, and an organization's own
	// people (17, 25). These take requireActor rather than requireAuth
	// because they need to know who is asking, and because an unscoped API
	// token must not be a way in (see requireActor).
	m.HandleFunc("GET /api/admin/accounts", s.requireActor(s.handleListAccounts))
	m.HandleFunc("GET /api/admin/orgs", s.requireActor(s.handleListAllOrgs))
	m.HandleFunc("PATCH /api/orgs/{id}", s.requireActor(s.handleRenameOrg))
	m.HandleFunc("GET /api/orgs/{id}/members", s.requireActor(s.handleListOrgMembers))
	m.HandleFunc("PATCH /api/orgs/{id}/members/{accountId}", s.requireActor(s.handleSetOrgMemberRole))
	m.HandleFunc("DELETE /api/orgs/{id}/members/{accountId}", s.requireActor(s.handleRemoveOrgMember))

	// Web UI (embedded SPA) at everything else.
	if s.opts.UI != nil {
		m.HandleFunc("/", s.handleUI)
	}
}

// handleUI serves the embedded SPA with index.html fallback for client routes.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	f, err := s.opts.UI.Open(p)
	if err != nil {
		p = "index.html"
		if f, err = s.opts.UI.Open(p); err != nil {
			httpError(w, http.StatusNotFound, "UI not built into this binary")
			return
		}
	}
	defer f.Close()
	stat, _ := f.Stat()
	if rs, ok := f.(interface {
		fs.File
		Seek(int64, int) (int64, error)
	}); ok {
		http.ServeContent(w, r, p, stat.ModTime(), rs)
	}
}

// --- small helpers used across handlers ---

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

func displayAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		return fmt.Sprintf("localhost:%s", port)
	}
	return addr
}
