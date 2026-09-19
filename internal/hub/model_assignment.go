package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// The factory assignment surface (the assignments wave of the model
// layer): which pinned model each purpose resolves to when a box carries
// no override of its own. The handlers gate on `admin:hub.model`; there is
// no public side — the roster resolves server-side into a box's manifest.
//
// The store is model_assignment_store.go.

// assignmentInput is the wire shape of one factory pin. ModelId rides the
// same case convention boxId does elsewhere in the hub surface.
type assignmentInput struct {
	Purpose string `json:"purpose"`
	ModelID string `json:"modelId"`
}

// handleListAssignments serves every factory pin on this installation,
// ordered by purpose (the store's order).
func (s *Server) handleListAssignments(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	assignments, err := s.store.ListAssignments()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, assignments)
}

// handleSetAssignment pins one purpose to one model. The store runs the
// full cross-validation: an unknown purpose is a 400, an unknown model a
// 404, a model of another purpose or an unverified (empty-digest) model a
// 400. Re-pinning an already assigned purpose replaces the row — one row
// per purpose.
func (s *Server) handleSetAssignment(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var in assignmentInput
	if err := readJSON(r, &in); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	purpose, err := ParsePurpose(in.Purpose)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := s.store.SetAssignment(purpose, strings.TrimSpace(in.ModelID))
	switch {
	case errors.Is(err, ErrUnknownModel):
		httpError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrModelPurposeMismatch), errors.Is(err, ErrModelUnverified):
		httpError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		httpError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, a)
	}
}

// handleClearAssignment removes a purpose's factory pin. A purpose with no
// assignment is a 404.
func (s *Server) handleClearAssignment(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	cleared, err := s.store.ClearAssignment(r.PathValue("purpose"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !cleared {
		httpError(w, http.StatusNotFound, "no such assignment")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
