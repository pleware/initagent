package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// The generation-limits surface (the limits wave of the model layer): the
// per-slot ceilings the manifest carries beside the resolved model. There is
// no public side — the limits resolve server-side into a box's manifest. The
// factory handlers gate on `admin:hub.model`, the same verb the assignment
// surface uses; the per-box handlers gate on `admin:fleet.box`.
//
// The store is model_limits_store.go.

// limitsInput is the wire shape of one generation limit, shared by the
// factory and per-box setters. The field names follow the manifest's camelCase
// convention (maxTokens, timeoutSeconds), like wordBudget and avatarModel3d.
type limitsInput struct {
	Purpose        string `json:"purpose"`
	MaxTokens      int    `json:"maxTokens"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

// handleListLimits serves every factory limit on this installation, ordered
// by purpose (the store's order).
func (s *Server) handleListLimits(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	limits, err := s.store.ListLimits()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, limits)
}

// handleSetLimit pins one purpose to a factory limit. The store runs the full
// validation: an unknown purpose is a 400, a negative limit a 400, and a
// maxTokens above the cap a 400. Re-pinning an already limited purpose
// replaces the row — one row per purpose.
func (s *Server) handleSetLimit(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var in limitsInput
	if err := readJSON(r, &in); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	in.Purpose = strings.TrimSpace(in.Purpose)
	purpose, err := ParsePurpose(in.Purpose)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	l, err := s.store.SetLimit(purpose, in.MaxTokens, in.TimeoutSeconds)
	switch {
	case errors.Is(err, ErrLimitsNegative), errors.Is(err, ErrMaxTokensTooLarge):
		httpError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		httpError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, l)
	}
}

// handleClearLimit removes a purpose's factory limit. A purpose with no limit
// is a 404.
func (s *Server) handleClearLimit(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	cleared, err := s.store.ClearLimit(r.PathValue("purpose"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !cleared {
		httpError(w, http.StatusNotFound, "no such limit")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleListBoxLimits serves the limits one box resolves against: for each
// purpose the box's override wins when present, the factory limit answers
// when not. Like handleListBoxModels, this is the effective view, not the
// raw override rows.
func (s *Server) handleListBoxLimits(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	limits, err := s.store.ResolvedLimits(box.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, limits)
}

// handleSetBoxLimit pins one purpose of one box to a limit. The store runs the
// full validation: an unknown box is answered by boxOr404, an unknown purpose
// is a 400, and a negative or over-cap limit is a 400. Re-pinning an already
// limited purpose replaces the row.
func (s *Server) handleSetBoxLimit(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	var in limitsInput
	if err := readJSON(r, &in); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	in.Purpose = strings.TrimSpace(in.Purpose)
	purpose, err := ParsePurpose(in.Purpose)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	l, err := s.store.SetBoxLimit(box.ID, purpose, in.MaxTokens, in.TimeoutSeconds)
	switch {
	case errors.Is(err, ErrLimitsNegative), errors.Is(err, ErrMaxTokensTooLarge):
		httpError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		httpError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, l)
	}
}

// handleClearBoxLimit removes a box's per-box limit for one purpose, so the
// purpose falls back to the factory limit. A limit that does not exist is a
// 404.
func (s *Server) handleClearBoxLimit(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	cleared, err := s.store.ClearBoxLimit(box.ID, r.PathValue("purpose"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !cleared {
		httpError(w, http.StatusNotFound, "no such limit")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
