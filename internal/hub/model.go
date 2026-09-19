package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// The model registry surface (the models wave of the model layer): the hub
// pins model identity and provenance — id, source, quant, digest, licence,
// purpose — and holds no weights. The admin handlers gate on
// `admin:hub.model`; the public catalog serves the pins pre-auth, the same
// shape the public skill catalog follows, so a box installer can resolve
// what the installation offers.
//
// The store is model_store.go.

// modelInput is the editable shape of a model pin, shared by create and
// update. The update handler takes the id from the path, so an id in the
// body is ignored there.
type modelInput struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Quant   string `json:"quant"`
	Digest  string `json:"digest"`
	Licence string `json:"licence"`
	Purpose string `json:"purpose"`
}

// handleListModels serves every pinned model on this installation, ordered
// by id (the store's order).
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	models, err := s.store.ListModels()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, models)
}

// handleCreateModel registers a new model pin. The id is required and is
// the pin's key; the purpose must parse (ParsePurpose). An empty digest is
// accepted — it means unverified — and the admin fills it after checking
// the pinned artifact. A duplicate id is a 409.
func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var in modelInput
	if err := readJSON(r, &in); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		httpError(w, http.StatusBadRequest, "id is required")
		return
	}
	purpose, err := ParsePurpose(in.Purpose)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := s.store.CreateModel(in.ID, strings.TrimSpace(in.Source), strings.TrimSpace(in.Quant),
		strings.TrimSpace(in.Digest), strings.TrimSpace(in.Licence), purpose)
	if errors.Is(err, ErrModelIDTaken) {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, m)
}

// handleUpdateModel replaces the editable fields of a model pin. A missing
// pin is a 404; a bad purpose is a 400.
func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var in modelInput
	if err := readJSON(r, &in); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	purpose, err := ParsePurpose(in.Purpose)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := s.store.UpdateModel(r.PathValue("id"), strings.TrimSpace(in.Source), strings.TrimSpace(in.Quant),
		strings.TrimSpace(in.Digest), strings.TrimSpace(in.Licence), purpose)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		httpError(w, http.StatusNotFound, "no such model")
		return
	}
	writeJSON(w, m)
}

// handleDeleteModel removes a model pin. A missing pin is a 404; a pin
// still referenced by an assignment or override is a 409 (ErrModelInUse).
func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	deleted, err := s.store.DeleteModel(r.PathValue("id"))
	if errors.Is(err, ErrModelInUse) {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		httpError(w, http.StatusNotFound, "no such model")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleListModelsPublic serves the model pin registry pre-auth: the
// catalogue a box installer resolves against. No middleware, like the
// public skill catalog it feeds.
func (s *Server) handleListModelsPublic(w http.ResponseWriter, r *http.Request) {
	models, err := s.store.ListModels()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, models)
}
