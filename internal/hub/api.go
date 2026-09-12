package hub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/deviceops"
	"github.com/pleware/initagent/internal/funnel"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/protocol"
	"github.com/pleware/initagent/internal/updater"
)

// --- setup & auth ---

// handleSetup claims an unclaimed hub: the first account, in both offerings.
// The bootstrap token is what separates the operator from whoever else found
// the URL, so this endpoint is worthless without it (`26`).
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !s.loginRL.allow(s.clientAddr(r)) {
		httpError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Token    string `json:"token"`
		OrgName  string `json:"orgName"`
		Locale   string `json:"locale"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	claimed, err := s.claimed()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	creds, err := auth.Claim(auth.State{
		Offering:      s.opts.Offering,
		Claimed:       claimed,
		ExpectedToken: s.claim.expected(),
	}, auth.ClaimRequest{
		Email:    req.Email,
		Password: req.Password,
		Token:    req.Token,
		OrgName:  req.OrgName,
		Locale:   req.Locale,
	})
	if err != nil {
		httpError(w, claimStatus(err), err.Error())
		return
	}
	var account *Account
	if s.opts.Offering == offering.Hosted {
		account, err = s.store.insertPlatformAdmin(creds.Email, creds.PasswordHash, creds.Locale)
	} else {
		account, _, err = s.store.insertAccountWithOwnedOrg(creds.Email, creds.PasswordHash, creds.OrgName, true, creds.Locale)
	}
	if err != nil {
		// The partial unique index on is_admin is the arbiter, so a second
		// concurrent claim lands here rather than creating a second owner.
		if nowClaimed, checkErr := s.claimed(); checkErr == nil && nowClaimed {
			httpError(w, http.StatusConflict, auth.ErrAlreadyClaimed.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.claim.consume()
	s.issueSession(w, r, account.Id)
	writeJSON(w, map[string]bool{"ok": true})
}

// claimStatus maps a claim refusal onto a status code. A claimed hub answers
// 409 whatever token arrived, which is what stops this endpoint from being an
// oracle for guessing the token.
func claimStatus(err error) int {
	switch {
	case errors.Is(err, auth.ErrAlreadyClaimed):
		return http.StatusConflict
	case errors.Is(err, auth.ErrClaimToken):
		return http.StatusForbidden
	case errors.Is(err, auth.ErrEmailInvalid), errors.Is(err, auth.ErrPasswordWeak), errors.Is(err, auth.ErrLocale):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// handleLogin authenticates an account by email and password.
//
// A hub that was set up before accounts existed still carries upstream's
// anonymous password setting, and a request with no email is matched against
// it so an existing self-host install keeps working. Nothing writes that
// setting any more.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.loginRL.allow(s.clientAddr(r)) {
		httpError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		s.loginLegacy(w, r, req.Password)
		return
	}
	// One message for a wrong address and a wrong password, and the same work
	// spent either way: the form must not report who has an account here.
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		auth.BurnVerify(req.Password)
		httpError(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	account, err := s.store.AccountByEmail(email)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if account == nil {
		auth.BurnVerify(req.Password)
		httpError(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	if !account.VerifyPassword(req.Password) {
		httpError(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	s.recordEvent(funnel.Event{Kind: funnel.KindLogin, AccountID: account.Id})
	s.issueSession(w, r, account.Id)
	writeJSON(w, map[string]bool{"ok": true})
}

// handleRegister creates a customer account on a claimed hosted hub.
//
// Self-host answers 404: the door is not there. An unclaimed hosted hub
// answers 409 so this cannot replace setup. A session cookie is issued the
// same way login does; a bearer token is ignored on purpose (`09`, `26`).
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.registerRL.allow(s.clientAddr(r)) {
		httpError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Locale   string `json:"locale"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	claimed, err := s.claimed()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	creds, err := auth.Register(auth.State{
		Offering: s.opts.Offering,
		Claimed:  claimed,
	}, auth.RegisterRequest{Email: req.Email, Password: req.Password, Locale: req.Locale})
	if err != nil {
		httpError(w, registerStatus(err), err.Error())
		return
	}
	account, org, err := s.store.RegisterCustomer(creds.Email, creds.PasswordHash, creds.OrgName, creds.Locale)
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	orgID := ""
	if org != nil {
		orgID = org.Id
	}
	s.recordEvent(funnel.Event{Kind: funnel.KindSignup, OrgID: orgID, AccountID: account.Id})
	s.issueSession(w, r, account.Id)
	writeJSON(w, map[string]bool{"ok": true})
}

func registerStatus(err error) int {
	switch {
	case errors.Is(err, auth.ErrNotHosted):
		return http.StatusNotFound
	case errors.Is(err, auth.ErrNotClaimed), errors.Is(err, auth.ErrEmailTaken):
		return http.StatusConflict
	case errors.Is(err, auth.ErrEmailInvalid), errors.Is(err, auth.ErrPasswordWeak), errors.Is(err, auth.ErrLocale):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func (s *Server) loginLegacy(w http.ResponseWriter, r *http.Request, password string) {
	stored, err := s.store.Setting(legacyPasswordSetting)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stored == "" || !auth.VerifyPassword(stored, password) {
		httpError(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	// No account id: this credential predates accounts, and inventing one
	// here would attribute future actions to a person who does not exist.
	s.issueSession(w, r, "")
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, account string) {
	token := s.sessions.create(account)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil, // set when served over HTTPS (e.g. behind a TLS proxy terminating here)
		MaxAge: int(sessionTTL.Seconds()),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]bool{"ok": true})
}

// handleMe reports auth state so the UI knows which screen to show.
//
// `offering` is here because the login screen has to say which kind of hub
// this is (`26`): an operator should see what they are about to type a
// credential into, and a hosted hub that fell back to selfhost is otherwise
// invisible outside the host's own health check. `signup` is the same
// decision as the register door: claimed and hosted, nowhere else.
// `defaultOrgName` is what register writes, so boarding can tell an
// unnamed company from one the owner chose.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claimed, err := s.claimed()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The password floor travels with the state instead of being repeated in
	// the cockpit: a form that says eight while the hub demands twelve is a
	// rejection the person cannot act on.
	resp := map[string]any{
		"claimed":           claimed,
		"offering":          string(s.opts.Offering),
		"passwordMinLength": auth.MinPassword(s.opts.Offering),
		"signup":            auth.SignupOpen(s.opts.Offering, claimed),
		"defaultOrgName":    auth.DefaultOrgName,
		"authenticated":     false,
		"version":           s.opts.Version,
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		writeJSON(w, resp)
		return
	}
	account, ok := s.sessions.lookup(c.Value)
	if !ok {
		writeJSON(w, resp)
		return
	}
	resp["authenticated"] = true

	// Identity, so the cockpit knows which surfaces exist for this person:
	// the platform flag decides whether there is an administration section at
	// all, and the memberships decide whose people they may manage. The
	// cockpit hiding a section is convenience — every endpoint behind it
	// checks the same capability itself.
	actor, err := s.resolveActor(account)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp["platformAdmin"] = actor.Platform
	resp["accountId"] = actor.Account
	// The assignable roles come from here so the cockpit's dropdown cannot
	// offer a name this hub would refuse.
	roles := make([]string, 0, len(authz.Roles()))
	for _, role := range authz.Roles() {
		roles = append(roles, string(role))
	}
	resp["orgRoles"] = roles
	// The scopes a token may carry travel from here for the same reason: a
	// checkbox naming a verb this hub does not know is a token nobody can
	// mint, and the `dangerous` flag is what keeps arbitrary command
	// execution from being one careless click in the mint form (09).
	scopes := make([]map[string]any, 0, len(authz.GrantableScopes()))
	for _, scope := range authz.GrantableScopes() {
		scopes = append(scopes, map[string]any{
			"scope":     string(scope),
			"dangerous": authz.Dangerous(scope),
		})
	}
	resp["tokenScopes"] = scopes
	orgs := []Membership{}
	if actor.Account != "" {
		if a, err := s.store.AccountById(actor.Account); err == nil && a != nil {
			resp["email"] = a.Email
			resp["locale"] = a.Locale
		}
		orgs, err = s.store.ListAccountOrgs(actor.Account)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	resp["orgs"] = orgs
	writeJSON(w, resp)
}

// handlePatchMe stores the language a signed-in person just picked. Guests
// have no row to write; they keep the choice in the browser until they
// register or claim.
func (s *Server) handlePatchMe(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	actor := cred.Actor
	if actor.Account == "" {
		httpError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		Locale string `json:"locale"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.Locale) == "" {
		httpError(w, http.StatusBadRequest, auth.ErrLocale.Error())
		return
	}
	locale, err := auth.NormalizeLocale(req.Locale)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetAccountLocale(actor.Account, locale); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "locale": locale})
}

// --- enrollment ---

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Hostname string `json:"hostname"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	ok, err := s.store.ConsumeEnrollToken(req.Token)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		httpError(w, http.StatusForbidden, "enrollment token is invalid, expired, or already used — create a new one in the UI")
		return
	}
	name := req.Hostname
	if name == "" {
		name = "device"
	}
	id, token, err := s.store.CreateDevice(name, req.Hostname, req.OS, req.Arch, false)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"deviceId": id, "deviceToken": token})
}

func (s *Server) handleCreateEnrollToken(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	p, ok := s.gatewayFor(w, r, cred)
	if !ok {
		return
	}
	s.proxyGateway(w, r, p, http.MethodPost, "/api/enroll-tokens", gatewayProxyTimeout)
}

// --- devices ---

type deviceView struct {
	Device
	Online          bool            `json:"online"`
	Stats           *protocol.Stats `json:"stats,omitempty"`
	Tmux            bool            `json:"tmux"`
	AgentVersion    string          `json:"agentVersion,omitempty"`
	Platform        string          `json:"platform,omitempty"`
	PlatformVersion string          `json:"platformVersion,omitempty"`
	KernelVersion   string          `json:"kernelVersion,omitempty"`
}

func (s *Server) deviceViews() ([]deviceView, error) {
	devices, err := s.store.ListDevices()
	if err != nil {
		return nil, err
	}
	out := make([]deviceView, 0, len(devices))
	for _, d := range devices {
		v := deviceView{Device: d}
		if c := s.registry.get(d.Id); c != nil {
			v.Online = true
			v.Tmux = c.hello.Tmux
			v.AgentVersion = c.hello.Version
			v.Platform = c.hello.Platform
			v.PlatformVersion = c.hello.PlatformVersion
			v.KernelVersion = c.hello.KernelVersion
			c.mu.Lock()
			v.Stats = c.stats
			c.mu.Unlock()
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	// The flag still decides whether this hub has a gateway plane at all;
	// without one the inherited single-plane path below is the answer.
	if s.opts.GatewayURL != "" {
		p, ok := s.gatewayFor(w, r, cred)
		if !ok {
			return
		}
		s.proxyGateway(w, r, p, http.MethodGet, "/api/devices", gatewayProxyTimeout)
		return
	}
	views, err := s.deviceViews()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	visible, err := s.deviceFilter(cred)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	mine := make([]deviceView, 0, len(views))
	for _, v := range views {
		if visible(v.Id) {
			mine = append(mine, v)
		}
	}
	writeJSON(w, mine)
}

func (s *Server) handleRenameDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpError(w, http.StatusBadRequest, "name required")
		return
	}
	if err := s.store.RenameDevice(r.PathValue("id"), strings.TrimSpace(req.Name)); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.store.DeviceById(id)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		httpError(w, http.StatusNotFound, "no such device")
		return
	}
	if d.IsHub {
		httpError(w, http.StatusBadRequest, "cannot remove the hub itself")
		return
	}
	if err := s.store.DeleteDevice(id); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c := s.registry.get(id); c != nil {
		c.ws.Close() // token is gone; reconnect will be refused
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// --- software updates ---

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	status := s.updates.snapshot()
	// Hub plane: count the devices this installation holds directly.
	views, err := s.deviceViews()
	if err == nil {
		for _, device := range views {
			if device.IsHub {
				continue
			}
			status.FleetTotal++
			if device.AgentVersion == "" || updater.IsNewer(s.opts.Version, device.AgentVersion) {
				status.FleetOutdated++
			}
		}
	}
	// Gateway plane: each reachable gateway owns its own devices, so count
	// those too (16).
	targets, err := s.gatewayTargets(cred)
	if err == nil {
		for _, p := range targets {
			var gwViews []struct {
				IsHub        bool   `json:"isHub"`
				AgentVersion string `json:"agentVersion"`
			}
			if s.getGatewayJSON(r.Context(), p, "/api/devices", &gwViews) != nil {
				continue
			}
			for _, d := range gwViews {
				if d.IsHub {
					continue
				}
				status.FleetTotal++
				if d.AgentVersion == "" || updater.IsNewer(s.opts.Version, d.AgentVersion) {
					status.FleetOutdated++
				}
			}
		}
	}
	writeJSON(w, status)
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.updates.check(ctx, false); err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, s.updates.snapshot())
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoUpdate *bool `json:"autoUpdate"`
	}
	if err := readJSON(r, &req); err != nil || req.AutoUpdate == nil {
		httpError(w, http.StatusBadRequest, "autoUpdate is required")
		return
	}
	if err := s.updates.setAuto(*req.AutoUpdate); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleUpdateInstall(w http.ResponseWriter, r *http.Request) {
	status := s.updates.snapshot()
	if !status.Managed {
		httpError(w, http.StatusConflict, "this hub is not running as a managed service; run `"+brand.Binary+" update` on the host")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_ = s.updates.install(ctx)
	}()
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleUpdateRollback(w http.ResponseWriter, r *http.Request) {
	status := s.updates.snapshot()
	if !status.Managed {
		httpError(w, http.StatusConflict, "this hub is not running as a managed service; run `"+brand.Binary+" rollback` on the host")
		return
	}
	if status.RollbackVersion == "" {
		httpError(w, http.StatusConflict, "no previous version is available")
		return
	}
	go func() { _ = s.updates.rollback() }()
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]bool{"ok": true})
}

// resolveLiveDevice returns the hub-registry connection, or the placement to
// proxy to when the worker lives on the gateway (self-host enroll, 10/16).
// A false ok means the request was already answered (an error was written)
// and the caller must stop.
func (s *Server) resolveLiveDevice(w http.ResponseWriter, r *http.Request, cred authz.Credential) (*agentConn, placement, bool) {
	id := r.PathValue("id")
	if c := s.registry.get(id); c != nil {
		return c, placement{}, true
	}
	if strings.TrimSpace(s.opts.GatewayURL) == "" {
		httpError(w, http.StatusServiceUnavailable, "device is offline")
		return nil, placement{}, false
	}
	p, ok := s.gatewayFor(w, r, cred)
	if !ok {
		return nil, placement{}, false
	}
	return nil, p, true
}

// liveDevice returns a hub-registry connection, or answers the request via
// the project gateway when the worker lives there. A nil return means the
// caller must stop: either an error was written or the gateway answered.
func (s *Server) liveDevice(w http.ResponseWriter, r *http.Request, cred authz.Credential) *agentConn {
	c, p, ok := s.resolveLiveDevice(w, r, cred)
	if !ok {
		return nil
	}
	if c == nil {
		s.proxyGateway(w, r, p, r.Method, r.URL.RequestURI(), deviceProxyTimeout)
		return nil
	}
	return c
}

// --- sessions ---

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	ctx, cancel := deviceCtx()
	defer cancel()
	var res protocol.SessionsListResult
	if err := c.requestInto(ctx, protocol.TypeSessionsList, nil, &res); err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.Sessions == nil {
		res.Sessions = []protocol.Session{}
	}
	writeJSON(w, res.Sessions)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	var req protocol.SessionCreate
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		httpError(w, http.StatusBadRequest, "session name required")
		return
	}
	ctx, cancel := deviceCtx()
	defer cancel()
	if err := c.requestInto(ctx, protocol.TypeSessionCreate, req, nil); err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.events.publish(event{Type: "sessions.changed", DeviceId: c.deviceId})
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleKillSession(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	ctx, cancel := deviceCtx()
	defer cancel()
	err := c.requestInto(ctx, protocol.TypeSessionKill, protocol.SessionKill{Name: r.PathValue("name")}, nil)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.events.publish(event{Type: "sessions.changed", DeviceId: c.deviceId})
	writeJSON(w, map[string]bool{"ok": true})
}

// handleSessionInput types text into a tmux session (used by MCP/CLI to steer
// agents without holding a live terminal open).
func (s *Server) handleSessionInput(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	var req struct {
		Text  string `json:"text"`
		Enter bool   `json:"enter"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	name := r.PathValue("name")
	cmd := ""
	if req.Text != "" {
		cmd = fmt.Sprintf("tmux send-keys -t %s -l %s", deviceops.ShellQuote(name), deviceops.ShellQuote(req.Text))
	}
	if req.Enter {
		if cmd != "" {
			cmd += " && "
		}
		cmd += fmt.Sprintf("tmux send-keys -t %s Enter", deviceops.ShellQuote(name))
	}
	if cmd == "" {
		httpError(w, http.StatusBadRequest, "nothing to send")
		return
	}
	res, err := s.execOnDevice(c, cmd, "", 15)
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

// handleSessionOutput captures the last N lines of a tmux session's pane.
func (s *Server) handleSessionOutput(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	lines := 200
	if l, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && l > 0 && l <= 10000 {
		lines = l
	}
	name := r.PathValue("name")
	cmd := fmt.Sprintf("tmux capture-pane -p -t %s -S -%d", deviceops.ShellQuote(name), lines)
	res, err := s.execOnDevice(c, cmd, "", 15)
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

// --- exec ---

func (s *Server) execOnDevice(c *agentConn, command, cwd string, timeoutSec int) (protocol.ExecResult, error) {
	return deviceops.Exec(context.Background(), c, command, cwd, timeoutSec)
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	var req protocol.Exec
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Command) == "" {
		httpError(w, http.StatusBadRequest, "command required")
		return
	}
	res, err := s.execOnDevice(c, req.Command, req.Cwd, req.TimeoutSec)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, res)
}

// --- fleet-wide agent sessions ---

type fleetSession struct {
	protocol.Session
	DeviceId   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
}

func (s *Server) handleFleetAgents(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	devices, err := s.store.ListDevices()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The registry is installation-wide, so this view has to be narrowed
	// before it is fanned out: without it a member of one organization would
	// see every terminal session running on the hub.
	visible, err := s.deviceFilter(cred)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nameById := map[string]string{}
	for _, d := range devices {
		nameById[d.Id] = d.Name
	}

	var mu sync.Mutex
	var out []fleetSession
	var wg sync.WaitGroup

	// Hub plane: fan out the live registry.
	for _, c := range s.registry.all() {
		if !visible(c.deviceId) {
			continue
		}
		wg.Add(1)
		go func(c *agentConn) {
			defer wg.Done()
			ctx, cancel := contextWithTimeout(10 * time.Second)
			defer cancel()
			var res protocol.SessionsListResult
			if err := c.requestInto(ctx, protocol.TypeSessionsList, nil, &res); err != nil {
				return
			}
			mu.Lock()
			for _, sess := range res.Sessions {
				out = append(out, fleetSession{Session: sess, DeviceId: c.deviceId, DeviceName: nameById[c.deviceId]})
			}
			mu.Unlock()
		}(c)
	}

	// Gateway plane: ask every reachable gateway for its project's sessions
	// and merge. A worker that dialed a gateway rather than the hub lives on
	// that plane, so the registry above cannot see it (16).
	targets, err := s.gatewayTargets(cred)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, p := range targets {
		wg.Add(1)
		go func(p placement) {
			defer wg.Done()
			var rows []fleetSession
			if err := s.getGatewayJSON(r.Context(), p, "/api/agents", &rows); err != nil {
				return
			}
			mu.Lock()
			out = append(out, rows...)
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		if out[i].DeviceName != out[j].DeviceName {
			return out[i].DeviceName < out[j].DeviceName
		}
		return out[i].Name < out[j].Name
	})
	if out == nil {
		out = []fleetSession{}
	}
	writeJSON(w, out)
}

// --- presets ---

func (s *Server) handleListPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := s.store.ListPresets()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, presets)
}

func (s *Server) handleCreatePreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Command string `json:"command"`
		Kind    string `json:"kind"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpError(w, http.StatusBadRequest, "name required")
		return
	}
	if req.Kind == "" {
		req.Kind = strings.ToLower(strings.Fields(req.Command + " shell")[0])
	}
	id, err := s.store.CreatePreset(req.Name, req.Command, req.Kind)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]int64{"id": id})
}

func (s *Server) handleDeletePreset(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.store.DeletePreset(id); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// --- API tokens ---

// errNoSubject refuses to mint on a hub whose only credential is the legacy
// operator password. A token needs an `account-` to act as, and inventing one
// here would attribute a machine's actions to a person who does not exist.
const errNoSubject = "claim this hub with an email and password before minting tokens"

func (s *Server) handleListApiTokens(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	tokens, err := s.store.ListApiTokens(cred.Actor.Account)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tokens == nil {
		tokens = []ApiToken{}
	}
	writeJSON(w, tokens)
}

// handleCreateApiToken mints a credential narrower than the person asking.
//
// Every axis is checked against the minter rather than trusted from the
// request: a member cannot hand out an admin's verbs, and nobody can hand out
// reach into an organization they do not belong to. That check is what makes
// the intersection rule hold at rest rather than only at request time.
func (s *Server) handleCreateApiToken(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	var req struct {
		Name      string   `json:"name"`
		OrgId     string   `json:"orgId"`
		ProjectId string   `json:"projectId"`
		Scopes    []string `json:"scopes"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpError(w, http.StatusBadRequest, "name required")
		return
	}
	if cred.Actor.Account == "" {
		httpError(w, http.StatusConflict, errNoSubject)
		return
	}
	orgId, err := resolveProjectOrg(cred, req.OrgId)
	if err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			forbid(w, err)
			return
		}
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	scopes, err := authz.ParseScopes(strings.Join(req.Scopes, " "))
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(scopes) == 0 {
		httpError(w, http.StatusBadRequest, "at least one scope is required")
		return
	}
	projectId := strings.TrimSpace(req.ProjectId)
	if projectId != "" {
		project, err := s.store.ProjectById(projectId)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// A project in another org would give the token a boundary its own
		// org row contradicts, so the pair has to agree before it is stored.
		if project == nil || project.OrgId != orgId {
			httpError(w, http.StatusNotFound, "project not found")
			return
		}
	}
	for _, c := range scopes {
		if !cred.Actor.Can(c, orgId) {
			httpError(w, http.StatusForbidden, "you cannot grant "+string(c)+" in this organization")
			return
		}
	}
	secret, row, err := s.store.CreateApiToken(strings.TrimSpace(req.Name), cred.Actor.Account,
		authz.Grant{Org: orgId, Project: projectId, Scopes: scopes})
	if err != nil {
		if errors.Is(err, ErrTokenUnscoped) {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The plaintext secret is shown exactly once, beside the row so the
	// cockpit can list what was just handed out without a second request.
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"token": secret, "row": row})
}

func (s *Server) handleDeleteApiToken(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	revoked, err := s.store.RevokeApiToken(r.PathValue("id"), cred.Actor.Account)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Someone else's token answers the same 404 as a missing one, so the id
	// space cannot be probed for tokens you do not own.
	if !revoked {
		httpError(w, http.StatusNotFound, "token not found")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// --- file browser ---

func (s *Server) handleFsList(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	ctx, cancel := deviceCtx()
	defer cancel()
	res, err := deviceops.ListDir(ctx, c, r.URL.Query().Get("path"))
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.Entries == nil {
		res.Entries = []protocol.FsEntry{}
	}
	writeJSON(w, res)
}

func (s *Server) handleFsDownload(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c, p, ok := s.resolveLiveDevice(w, r, cred)
	if !ok {
		return
	}
	if c == nil {
		// Downloads can exceed the JSON proxy's 1 MiB cap, so this hop
		// streams unbounded and forwards Content-Disposition.
		s.proxyGatewayStream(w, r, p, r.Method, r.URL.RequestURI())
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		httpError(w, http.StatusBadRequest, "path required")
		return
	}
	base := path[strings.LastIndex(path, "/")+1:]
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", base))
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := deviceops.Download(c, path, w); err != nil {
		// A failure before the first byte is unreportable once the stream
		// headers are set; a dropped device surfaces as a truncated download.
		return
	}
}

func (s *Server) handleFsUpload(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	c := s.liveDevice(w, r, cred)
	if c == nil {
		return
	}
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		httpError(w, http.StatusBadRequest, "dir required")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpError(w, http.StatusBadRequest, "expected multipart upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()
	name := header.Filename
	if name == "" || strings.ContainsAny(name, "/\\") {
		httpError(w, http.StatusBadRequest, "bad filename")
		return
	}
	target := strings.TrimRight(dir, "/") + "/" + name
	if err := deviceops.Upload(c, target, file); err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, map[string]string{"path": target})
}
