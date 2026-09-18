package hub

import (
	"net/http"

	"github.com/pleware/initagent/internal/authz"
)

// --- box tokens ---

// The box's sync credential (58). A box token is a machine secret with no
// account behind it, so the surface is the platform operator's: the gate
// and the session-only routes hold the same line the admin token surface
// does, and the secret is shown exactly once at mint.

// handleCreateBoxToken mints the box's sync credential. The plaintext
// secret comes back beside the row so the cockpit can show it once
// without a second request; the next mint revokes it.
func (s *Server) handleCreateBoxToken(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	secret, row, err := s.store.CreateBoxToken(box.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"token": secret, "row": row})
}

// handleListBoxTokens serves a box's live credentials. The secret never
// travels again: only the row, so the operator sees what is active and
// when it was last used.
func (s *Server) handleListBoxTokens(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	tokens, err := s.store.ListBoxTokens(box.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tokens == nil {
		tokens = []BoxToken{}
	}
	writeJSON(w, tokens)
}

// handleRevokeBoxToken stops a box's credential. The box scoping in the
// store keeps a token id that belongs to another box out of reach, and a
// token that is not live answers 404 rather than claiming a success.
func (s *Server) handleRevokeBoxToken(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	revoked, err := s.store.RevokeBoxToken(r.PathValue("tokenId"), box.ID)
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
