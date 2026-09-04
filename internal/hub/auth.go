package hub

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pleware/initagent/internal/brand"
)

const sessionCookie = brand.SessionCookie

// Password hashing lives in internal/auth, which owns the credential
// decisions for both planes and is under the coverage gate.

// --- browser sessions (in-memory; hub restart = re-login) ---

type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
}

const sessionTTL = 30 * 24 * time.Hour

func newSessionManager() *sessionManager {
	return &sessionManager{sessions: map[string]time.Time{}}
}

func (m *sessionManager) create() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	m.mu.Lock()
	m.sessions[token] = time.Now().Add(sessionTTL)
	m.mu.Unlock()
	return token
}

func (m *sessionManager) valid(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(m.sessions, token)
		return false
	}
	return true
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
