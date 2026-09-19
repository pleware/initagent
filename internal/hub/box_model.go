package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// The per-box model surface (the overrides wave of the model layer): one
// box's model roster and the pins that shadow the factory assignments for
// that box alone. Every handler gates on `admin:fleet.box`, the box
// administration verb, and loads the box first — a missing box is a 404
// before anything else.
//
// The store is box_model_override_store.go.

// handleListBoxModels serves the box's resolved roster: for each purpose
// the override wins when the box has one, the factory assignment answers
// when not, and a purpose with neither is omitted.
func (s *Server) handleListBoxModels(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	roster, err := s.store.ResolvedModels(box.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, roster)
}

// handleSetBoxModelOverride pins one purpose of one box to a model. The
// store runs the full cross-validation: an unknown box is answered by
// boxOr404, an unknown model is a 404, and a model of another purpose or
// an unverified (empty-digest) model is a 400. Re-pinning an already
// overridden purpose replaces the row.
func (s *Server) handleSetBoxModelOverride(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
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
	o, err := s.store.SetBoxModelOverride(box.ID, purpose, strings.TrimSpace(in.ModelID))
	switch {
	case errors.Is(err, ErrUnknownModel):
		httpError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrModelPurposeMismatch), errors.Is(err, ErrModelUnverified):
		httpError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		httpError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, o)
	}
}

// handleClearBoxModelOverride removes a box's per-box pin for one purpose,
// so the purpose falls back to the factory assignment. An override that
// does not exist is a 404.
func (s *Server) handleClearBoxModelOverride(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	cleared, err := s.store.ClearBoxModelOverride(box.ID, r.PathValue("purpose"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !cleared {
		httpError(w, http.StatusNotFound, "no such override")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
