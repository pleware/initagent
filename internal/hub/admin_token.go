package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// --- admin tokens ---

// The operator's installation-scoped credentials (09). They are the sibling
// of the org tokens above, minted by the platform operator rather than an
// org owner, and their boundary is the installation itself. The gate and the
// scope parser are the narrower ones on purpose: a token handed to a machine
// may never carry account administration, and ParseInstallationScopes is the
// list that holds that line at the handler as well as the store.

// handleListAdminTokens serves the installation's operator credentials. There
// is no account filter — this is the operator's view of the hub they run, and
// the Administration screen is the only place that may show it.
func (s *Server) handleListAdminTokens(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminAccounts, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	tokens, err := s.store.ListAdminTokens()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tokens == nil {
		tokens = []ApiToken{}
	}
	writeJSON(w, tokens)
}

// handleCreateAdminToken mints an installation-scoped credential. The scopes
// are parsed against the installation-grantable list, so a request naming an
// org verb or an account verb is refused rather than silently narrowed.
func (s *Server) handleCreateAdminToken(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminAccounts, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	if cred.Requester.Account == "" {
		httpError(w, http.StatusConflict, errNoSubject)
		return
	}
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpError(w, http.StatusBadRequest, "name required")
		return
	}
	scopes, err := authz.ParseInstallationScopes(strings.Join(req.Scopes, " "))
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(scopes) == 0 {
		httpError(w, http.StatusBadRequest, "at least one scope is required")
		return
	}
	secret, row, err := s.store.CreateAdminToken(strings.TrimSpace(req.Name), cred.Requester.Account, scopes)
	if err != nil {
		if errors.Is(err, ErrAdminTokenInvalid) {
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

// handleDeleteAdminToken revokes an installation-scoped credential. The
// installation guard in the store keeps an org token out of reach even when
// its id is known.
func (s *Server) handleDeleteAdminToken(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminAccounts, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	revoked, err := s.store.RevokeAdminToken(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !revoked {
		httpError(w, http.StatusNotFound, "token not found")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
