package hub

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"slices"
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
	account string // `account-`; empty on a hub still using the legacy operator password
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
// operator credential, which has no `account-` to point at.
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

// revokeAccount drops every session for this account so a password reset
// cannot leave an already-open browser signed in.
func (m *sessionManager) revokeAccount(account string) {
	if account == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for token, s := range m.sessions {
		if s.account == account {
			delete(m.sessions, token)
		}
	}
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

// credHandler is a handler that has already been told who is asking. Every
// guarded route takes one, so no handler reads a cookie or a bearer itself
// and no handler has to know which of the two it received.
type credHandler func(http.ResponseWriter, *http.Request, authz.Credential)

// plain adapts a handler that does not need to know who is asking.
//
// The middleware has already decided, and these handlers act on a target the
// URL names. Marking them explicitly is worth the noise: the ones that *do*
// take a credential are exactly the ones with something left to check or
// filter, so this reads as a list of what is already settled.
func plain(next http.HandlerFunc) credHandler {
	return func(w http.ResponseWriter, r *http.Request, _ authz.Credential) { next(w, r) }
}

// bound is one organization-or-project pair a request could be acting in.
//
// Requests often have more than one candidate: a machine attached to three
// projects is reachable by a credential holding any of them. The resolvers
// below therefore return a list, and the middleware needs a single match.
type bound struct {
	org     string
	project string
}

// atInstallation is the empty boundary — operating the hub itself rather than
// acting inside a tenant. No token reaches it, by the rule in authz.Grant.
var atInstallation = []bound{{}}

// credentialOf resolves the secret on a request into who is asking and how
// far that secret reaches.
//
// A cookie yields a session with no grant: a person is present, so their role
// is the whole answer. A bearer yields the token's row, whose subject and
// boundary are what let one handler serve both.
func (s *Server) credentialOf(r *http.Request) (authz.Credential, bool, error) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if account, ok := s.sessions.lookup(c.Value); ok {
			actor, err := s.resolveActor(account)
			if err != nil {
				return authz.Credential{}, false, err
			}
			return authz.Credential{Actor: actor}, true, nil
		}
	}
	presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || presented == "" {
		return authz.Credential{}, false, nil
	}
	t, found, err := s.store.ApiTokenAuth(presented)
	if err != nil || !found {
		return authz.Credential{}, false, err
	}
	// A subjectless token cannot be written — the store refuses — but if one
	// ever appeared it must not resolve to the legacy operator, because
	// resolveActor reads an empty account as the installation's owner.
	if t.AccountId == "" {
		return authz.Credential{}, false, nil
	}
	actor, err := s.resolveActor(t.AccountId)
	if err != nil {
		return authz.Credential{}, false, err
	}
	grant := t.Grant
	return authz.Credential{Actor: actor, Grant: &grant}, true, nil
}

// requireAt guards a route with exactly one capability, checked against the
// boundaries the request could be acting in.
//
// It replaced a middleware that proved only "somebody authenticated". That
// was enough while every credential was equal; it stopped being enough the
// moment one of the routes behind it was arbitrary command execution on an
// enrolled machine.
func (s *Server) requireAt(c authz.Capability, at func(*http.Request, authz.Credential) ([]bound, error), next credHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cred, ok, err := s.credentialOf(r)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			httpError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		bounds, err := at(r, cred)
		if err != nil {
			var notFound errNotFound
			if errors.As(err, &notFound) {
				httpError(w, http.StatusNotFound, err.Error())
				return
			}
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, b := range bounds {
			if cred.Can(c, b.org, b.project) {
				s.stampTokenGrant(cred)
				next(w, r, cred)
				return
			}
		}
		refuse(w, cred, c, bounds)
	}
}

// requireConnector guards a route whose path names the machine it acts on. The
// boundary is that machine's, which is what keeps a project-scoped token off
// another project's hardware.
func (s *Server) requireConnector(c authz.Capability, next credHandler) http.HandlerFunc {
	return s.requireAt(c, func(r *http.Request, _ authz.Credential) ([]bound, error) {
		return s.boundsForConnector(strings.TrimSpace(r.PathValue("id")))
	}, next)
}

// requireFleet guards a route that names its target by query parameter, or
// not at all: creating a task, reading presets, watching events.
//
// Separate from requireConnector on purpose. Several of these routes have an
// {id} of their own — a task, a preset — and reading that as a connector id
// would check the wrong boundary and refuse every caller.
func (s *Server) requireFleet(c authz.Capability, next credHandler) http.HandlerFunc {
	return s.requireAt(c, s.atFleet, next)
}

// requireInstallation guards a route that operates the hub rather than using
// it. Only the platform operator passes, and no token does.
func (s *Server) requireInstallation(c authz.Capability, next credHandler) http.HandlerFunc {
	return s.requireAt(c, func(*http.Request, authz.Credential) ([]bound, error) {
		return atInstallation, nil
	}, next)
}

// requireCredential admits any authenticated caller and leaves the capability
// to the handler.
//
// For the routes that load a row before they can name their own boundary — a
// project by path id, an org by path id — where checking here would mean
// fetching it twice. Those handlers call Credential.Can with the row in hand.
func (s *Server) requireCredential(next credHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cred, ok, err := s.credentialOf(r)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			httpError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		next(w, r, cred)
	}
}

// requireSession admits a browser session and refuses every token.
//
// Two surfaces need this. **Minting and revoking credentials**: a token that
// can mint a token launders a narrow grant into a wide one, and every scope
// check downstream becomes decoration. **The account behind a credential**:
// changing its own password would let a read-only token trade itself for a
// full session.
func (s *Server) requireSession(next credHandler) http.HandlerFunc {
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
		next(w, r, authz.Credential{Actor: actor})
	}
}

// atFleet resolves the boundaries a fleet request acts in.
//
// A named machine is the strongest signal available, so ?connector= wins: the
// projects it is attached to are its boundary. Failing that an explicit
// ?project= answers, and failing that the credential's own boundary does —
// the shape of a self-host install and of the CLI, where there is one of
// everything.
func (s *Server) atFleet(r *http.Request, cred authz.Credential) ([]bound, error) {
	q := r.URL.Query()
	if connectorId := strings.TrimSpace(q.Get("connector")); connectorId != "" {
		return s.boundsForConnector(connectorId)
	}
	if projectId := strings.TrimSpace(q.Get(projectParam)); projectId != "" {
		return s.boundsForProject(projectId)
	}
	return credentialBounds(cred), nil
}

// boundsForConnector answers where a machine lives. An unattached machine yields
// nothing, so nothing scoped can reach it — see Store.ConnectorBoundaries.
func (s *Server) boundsForConnector(connectorId string) ([]bound, error) {
	rows, err := s.store.ConnectorBoundaries(connectorId)
	if err != nil {
		return nil, err
	}
	out := make([]bound, 0, len(rows))
	for _, b := range rows {
		out = append(out, bound{org: b.OrgId, project: b.ProjectId})
	}
	return out, nil
}

// boundsForProject answers where a named project lives.
//
// A project that does not exist is a 404 rather than a refusal: the caller
// named something absent, not something forbidden. Nothing leaks, because a
// project that exists but lies outside the credential's reach answers 404
// too — see resolveProject.
func (s *Server) boundsForProject(projectId string) ([]bound, error) {
	project, err := s.store.ProjectById(projectId)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, errProjectNotFound
	}
	return []bound{{org: project.OrgId, project: project.Id}}, nil
}

// credentialBounds is the fallback for a request that names neither a connector
// nor a project: listing machines, reading presets, watching events.
//
// A session is checked in every org it belongs to, which is the reach it
// already has. A token is checked in its own boundary, so the scope list does
// the deciding and a token cannot widen itself by omitting a target.
func credentialBounds(cred authz.Credential) []bound {
	if cred.Grant != nil {
		return []bound{{org: cred.Grant.Org, project: cred.Grant.Project}}
	}
	out := make([]bound, 0, len(cred.Actor.Orgs)+1)
	for org := range cred.Actor.Orgs {
		out = append(out, bound{org: org})
	}
	// The installation is appended, not substituted, so an operator who also
	// owns an org keeps both surfaces.
	return append(out, bound{})
}

// refuse explains a refusal in terms the caller can act on.
//
// A bare 403 is the worst possible answer after this change: the holder
// cannot tell whether their credential is wrong, revoked, or simply missing
// one verb. Naming the axis that failed makes re-minting an obvious next
// step, and it discloses nothing the holder could not learn by trying.
func refuse(w http.ResponseWriter, cred authz.Credential, c authz.Capability, bounds []bound) {
	if !cred.Scoped() {
		httpError(w, http.StatusForbidden, "not allowed")
		return
	}
	if !slices.Contains(cred.Grant.Scopes, c) {
		httpError(w, http.StatusForbidden, "this token is missing the "+string(c)+" scope")
		return
	}
	inside := slices.ContainsFunc(bounds, func(b bound) bool {
		return cred.Grant.Contains(b.org, b.project)
	})
	if !inside {
		httpError(w, http.StatusForbidden, "this token is scoped to another project or organization")
		return
	}
	// Scope and boundary both hold, so the account behind the token no longer
	// has the role. That is the intersection working as intended.
	httpError(w, http.StatusForbidden, "the account behind this token no longer has this permission")
}

// hideOrRefuse answers a refusal a handler makes after loading the row it
// acts on, where a plain 403 and a plain 404 are each wrong half the time.
//
// A tenant the caller cannot see at all answers 404: on a hub with many
// customers, "you may not see this" confirms it exists. But a token already
// scoped *into* that tenant has learned nothing new, and hiding the row costs
// its holder the one thing they need — the name of the scope to re-mint with.
func hideOrRefuse(w http.ResponseWriter, cred authz.Credential, c authz.Capability, absent string, b bound) {
	if cred.Scoped() && cred.Grant.Contains(b.org, b.project) {
		refuse(w, cred, c, []bound{b})
		return
	}
	httpError(w, http.StatusNotFound, absent)
}

// resolveActor turns an account id into the identity the rules read.
//
// An empty account is a session issued against the legacy operator password
// on a hub claimed before accounts existed. That credential was the hub's
// only administrator, so it resolves to the platform operator — and to no org
// membership, because there is no row saying otherwise.
func (s *Server) resolveActor(account string) (authz.Actor, error) {
	if account == "" {
		// Such a hub may also have no organizations at all, in which case
		// there is no boundary to enforce and this operator holds the fleet.
		// The check runs only for this legacy credential, so a hub with
		// accounts never pays for it.
		orgs, err := s.store.ListOrgs()
		if err != nil {
			return authz.Actor{}, err
		}
		return authz.Actor{Platform: true, Unpartitioned: len(orgs) == 0}, nil
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
