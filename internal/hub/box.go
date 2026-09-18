package hub

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// The box surface (58): the PWare OS appliances this installation
// configures. A box (`initagent.fleet.box`) is the logical appliance a
// `fleet.host` machine runs; each box carries its organizations, its
// narrator and its staff overrides, and syncs them down to the machine.
// The box store is box_store.go.
//
// Box administration is the platform admin's: the handlers share the
// canonical staff catalogue's empty-boundary gate (admin_staff.go) until
// the dedicated admin:box / read:box capabilities land. The gate lives in
// the handler, so an installation token carrying the verb is admitted at
// the middleware and refused — or admitted — here, the same shape the
// staff, skill and org admin surfaces use.

// boxOr404 loads a box for a handler, or writes the 404 itself. Every
// caller passed the installation gate first, so "no such box" is the
// honest answer, not a way to conceal a row from somebody who may see
// them all.
func (s *Server) boxOr404(w http.ResponseWriter, boxID string) (*Box, bool) {
	box, err := s.store.GetBox(boxID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if box == nil {
		httpError(w, http.StatusNotFound, "no such box")
		return nil, false
	}
	return box, true
}

// handleCreateBox mints a new box. The slug is the box's unique key, so
// the store refuses a collision with ErrBoxSlugTaken; hostId is optional
// and may be bound in a later PATCH.
func (s *Server) handleCreateBox(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var req struct {
		Slug   string `json:"slug"`
		Name   string `json:"name"`
		HostID string `json:"hostId"`
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
	box, err := s.store.CreateBox(req.Slug, req.Name, req.HostID)
	if err != nil {
		if errors.Is(err, ErrBoxSlugTaken) {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, box)
}

// handleListBoxes serves every box on this installation, ordered by slug.
func (s *Server) handleListBoxes(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	boxes, err := s.store.ListBoxes()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, boxes)
}

// handleGetBox serves one box by id. A missing box is a 404.
func (s *Server) handleGetBox(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	writeJSON(w, box)
}

// handleUpdateBox replaces the editable fields of a box: its name and its
// host binding. An empty hostId clears the binding. A missing box is a 404.
func (s *Server) handleUpdateBox(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	boxID := r.PathValue("id")
	var req struct {
		Name   string `json:"name"`
		HostID string `json:"hostId"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httpError(w, http.StatusBadRequest, "name is required")
		return
	}
	box, err := s.store.UpdateBox(boxID, req.Name, req.HostID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if box == nil {
		httpError(w, http.StatusNotFound, "no such box")
		return
	}
	writeJSON(w, box)
}

// handleSetBoxOrgs replaces the organization set bound to a box. The body
// carries the whole new set; duplicate ids collapse and an absent orgIds
// clears the box, matching the store's replace semantics.
func (s *Server) handleSetBoxOrgs(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	var req struct {
		OrgIDs []string `json:"orgIds"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := s.store.SetBoxOrgs(box.ID, req.OrgIDs); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleGetBoxNarrator serves the box's narrator — its one box-scoped staff
// member (58). A box whose narrator has not been seeded yet answers 404
// with the reason, so the cockpit can tell "nothing to show" from "not
// authorized".
func (s *Server) handleGetBoxNarrator(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminStaff, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	staff, err := s.store.StaffForBox(box.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(staff) == 0 {
		httpError(w, http.StatusNotFound, "this box has no narrator yet")
		return
	}
	writeJSON(w, staff[0])
}
