package hub

import (
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// The installation's canonical staff catalogue (24/28 walk-up). The staff
// rows live at the installation boundary — the hub's own named teammates —
// and every organization inherits them, overriding only the fields its own
// rows name. This file is the platform operator's surface: list, create and
// update the canonical rows. An org's effective view and its overrides are
// org_staff.go.

// handleListStaff serves the installation's canonical staff catalogue.
func (s *Server) handleListStaff(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	staff, err := s.store.ListStaff()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, staff)
}

// handleGetStaff serves one canonical staff member by id. A missing id is
// a 404.
func (s *Server) handleGetStaff(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	st, err := s.store.StaffById(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if st == nil {
		httpError(w, http.StatusNotFound, "no such staff member")
		return
	}
	writeJSON(w, st)
}

// handleUpsertStaff writes one canonical staff member and answers with the
// full row. POST creates and PATCH updates; the store's UpsertStaff keys on
// slug and does both, so one handler serves the two routes and the {id} in
// the PATCH path is not consulted.
func (s *Server) handleUpsertStaff(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var req struct {
		Slug          string    `json:"slug"`
		Name          string    `json:"name"`
		Locale        string    `json:"locale"`
		AvatarModel3D string    `json:"avatarModel3d"`
		Brief         string    `json:"brief"`
		Age           int       `json:"age"`
		WordBudget    int       `json:"wordBudget"`
		SoulCore      string    `json:"soulCore"`
		Voice         string    `json:"voice"`
		BigFive       Character `json:"bigFive"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	req.Slug = strings.TrimSpace(req.Slug)
	req.Name = strings.TrimSpace(req.Name)
	if req.Slug == "" || req.Name == "" {
		httpError(w, http.StatusBadRequest, "slug and name are required")
		return
	}
	st, err := s.store.UpsertStaff(req.Slug, req.Name, req.Locale, req.AvatarModel3D, req.Brief, req.SoulCore, req.Voice, "org", "", req.Age, req.WordBudget, req.BigFive)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, st)
}
