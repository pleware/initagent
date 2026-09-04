package hub

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/brand"
)

const sessionCookie = brand.SessionCookie

// Password hashing lives in internal/auth, which owns the credential
// decisions for both planes and is under the coverage gate.

// --- browser sessions (in-memory; hub restart = re-login) ---

// session is who a cookie belongs to.
//
// Upstream stored only an expiry, so "logged in" meant "somebody
// authenticated" and nothing could be attributed to a person (`26` recorded
// this as the gap behind audit and admin surfaces). The account travels with
// the session now, which is what lets one request resolve to an actor.
type session struct {
	account string // `acc-`; empty on a hub still using the legacy operator password
	expiry  time.Time
}

type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]session
}

const sessionTTL = 30 * 24 * time.Hour

func newSessionManager() *sessionManager {
	return &sessionManager{sessions: map[string]session{}}
}

// create issues a session for an account. An empty account is the legacy
// operator credential, which has no `acc-` to point at.
func (m *sessionManager) create(account string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	m.mu.Lock()
	m.sessions[token] = session{account: account, expiry: time.Now().Add(sessionTTL)}
	m.mu.Unlock()
	return token
}

// lookup returns the account behind a token. The second result separates "no
// such session" from "a legacy session with no account", which are different
// answers that an empty string alone would merge.
func (m *sessionManager) lookup(token string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return "", false
	}
	if time.Now().After(s.expiry) {
		delete(m.sessions, token)
		return "", false
	}
	return s.account, true
}

func (m *sessionManager) valid(token string) bool {
	_, ok := m.lookup(token)
	return ok
}

func (m *sessionManager) revoke(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// --- login rate limiting (per remote IP, fixed window) ---

type rateLimiter struct {
	mu     sync.Mutex
	counts map[string]*rateWindow
}

type rateWindow struct {
	start time.Time
	n     int
}

const (
	rateWindowLen = time.Minute
	rateMax       = 10
)

func newRateLimiter() *rateLimiter {
	return &rateLimiter{counts: map[string]*rateWindow{}}
}

func (r *rateLimiter) allow(remoteAddr string) bool {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	// Opportunistically evict expired windows so the map can't grow without
	// bound from many distinct client IPs hitting the pre-auth login endpoint.
	if len(r.counts) > 1024 {
		for k, v := range r.counts {
			if now.Sub(v.start) > rateWindowLen {
				delete(r.counts, k)
			}
		}
	}
	w := r.counts[ip]
	if w == nil || now.Sub(w.start) > rateWindowLen {
		r.counts[ip] = &rateWindow{start: now, n: 1}
		return true
	}
	w.n++
	return w.n <= rateMax
}

// --- middleware ---

// requireAuth accepts either a valid browser session cookie or an API token
// via Authorization: Bearer (used by the CLI and MCP server).
//
// This proves that somebody authenticated and nothing else. Endpoints that
// need to know *who* use requireActor instead.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil && s.sessions.valid(c.Value) {
			next(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if ok, _ := s.store.ValidApiToken(token); ok {
				next(w, r)
				return
			}
		}
		httpError(w, http.StatusUnauthorized, "not authenticated")
	}
}

// requireActor resolves who is making the request and hands the actor to the
// handler, which then asks authz what that actor may do.
//
// **A browser session only, deliberately.** An API token would also satisfy
// requireAuth, and API tokens carry no scope today: every token ever minted
// for the CLI or for MCP would silently become a platform administrator the
// moment an admin route went behind the older middleware. Scoping tokens
// along 09's three axes is the real answer and is on the backlog; until then
// the account surfaces are reachable by the credential that has a person
// behind it.
func (s *Server) requireActor(next func(http.ResponseWriter, *http.Request, authz.Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			httpError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		account, ok := s.sessions.lookup(c.Value)
		if !ok {
			httpError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		actor, err := s.resolveActor(account)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		next(w, r, actor)
	}
}

// resolveActor turns an account id into the identity the rules read.
//
// An empty account is a session issued against the legacy operator password
// on a hub claimed before accounts existed. That credential was the hub's
// only administrator, so it resolves to the platform operator — and to no org
// membership, because there is no row saying otherwise.
func (s *Server) resolveActor(account string) (authz.Actor, error) {
	if account == "" {
		return authz.Actor{Platform: true}, nil
	}
	a, err := s.store.AccountById(account)
	if err != nil {
		return authz.Actor{}, err
	}
	if a == nil {
		// The account was deleted while its cookie was still alive. Not an
		// error, and not an operator either.
		return authz.Actor{}, nil
	}
	roles, err := s.store.AccountOrgRoles(account)
	if err != nil {
		return authz.Actor{}, err
	}
	return authz.Actor{Account: a.Id, Platform: a.IsAdmin, Orgs: roles}, nil
}

// forbid maps an authorization refusal onto a status code. Everything that is
// not a decision is a 500, so a store failure cannot read as "not allowed".
func forbid(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authz.ErrForbidden):
		httpError(w, http.StatusForbidden, "not allowed")
	case errors.Is(err, authz.ErrRoleUnknown):
		httpError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, authz.ErrNotMember):
		httpError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, authz.ErrOwnerOnly), errors.Is(err, authz.ErrLastOwner):
		httpError(w, http.StatusConflict, err.Error())
	default:
		httpError(w, http.StatusInternalServerError, err.Error())
	}
}
