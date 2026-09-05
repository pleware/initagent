package hub

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/mailer"
)

// handlePasswordForgot queues a reset mail when the address belongs to an
// account. Unknown addresses still answer 200, so the form cannot be used to
// enumerate who has an account here.
func (s *Server) handlePasswordForgot(w http.ResponseWriter, r *http.Request) {
	if !s.loginRL.allow(r.RemoteAddr) {
		httpError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	account, err := s.store.AccountByEmail(email)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if account == nil {
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	origin, err := publicOrigin(r, s.opts.TLSDomain)
	if err != nil {
		httpError(w, http.StatusBadRequest, "cannot build reset link")
		return
	}
	secret, err := auth.NewResetToken()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	if err := s.store.CreatePasswordReset(account.Id, hashToken(secret), now, now.Add(auth.ResetTTL)); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	link := origin + "/reset?token=" + url.QueryEscape(secret)
	subject, text, htmlBody := mailer.PasswordReset(link, account.Locale)
	if _, err := s.EnqueueMail(mailer.KindPasswordReset, account.Email, subject, text, htmlBody); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handlePasswordReset sets a new password from a one-time mail token and
// signs the browser in. Confirm-password stays on the form.
func (s *Server) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	if !s.loginRL.allow(r.RemoteAddr) {
		httpError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	secret := strings.TrimSpace(req.Token)
	if secret == "" {
		httpError(w, http.StatusBadRequest, auth.ErrResetToken.Error())
		return
	}
	now := time.Now()
	peek, err := s.store.accountByResetHash(hashToken(secret), now)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if peek == nil {
		httpError(w, http.StatusBadRequest, auth.ErrResetToken.Error())
		return
	}
	if err := auth.CheckPassword(s.opts.Offering, peek.Email, req.Password); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	account, err := s.store.ResetAccountPassword(hashToken(secret), hash, now)
	if err != nil {
		if errors.Is(err, auth.ErrResetToken) {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sessions.revokeAccount(account.Id)
	s.issueSession(w, r, account.Id)
	writeJSON(w, map[string]bool{"ok": true})
}

// publicOrigin is the hub URL baked into a reset link. TLSDomain wins so a
// forged Host header cannot point the mail at an attacker's site. Behind
// Caddy, X-Forwarded-* is used only when no domain was configured.
func publicOrigin(r *http.Request, tlsDomain string) (string, error) {
	if d := strings.TrimSpace(tlsDomain); d != "" {
		if !validPublicHost(d) {
			return "", errBadOrigin
		}
		return "https://" + d, nil
	}
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if !validPublicHost(host) {
		return "", errBadOrigin
	}
	proto := strings.ToLower(firstHeaderValue(r.Header.Get("X-Forwarded-Proto")))
	if proto != "http" && proto != "https" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + host, nil
}

var errBadOrigin = errors.New("hub: unusable public origin")

func firstHeaderValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func validPublicHost(host string) bool {
	if host == "" || strings.ContainsAny(host, "/@ \t") {
		return false
	}
	letter := false
	for _, r := range host {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			letter = true
		case r == '.' || r == ':' || r == '-' || r == '[' || r == ']':
		default:
			return false
		}
	}
	return letter
}
