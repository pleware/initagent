package hub

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// The box surface (58): the PWare OS appliances this installation
// configures. A box (`initagent.fleet.box`) is the logical appliance a
// `fleet.host` machine runs; each box carries its organizations, its
// narrator and its staff overrides, and syncs them down to the machine.
// The box store is box_store.go.
//
// Box administration is the platform admin's, split on the box's two
// verbs: the read handlers gate on `read:fleet.box` and the mutations on
// `admin:fleet.box`. The gate lives in the handler, so an installation
// token carrying the verb is admitted at the middleware and refused — or
// admitted — here, the same shape the staff, skill and org admin
// surfaces use.

// boxSlugRe is the box slug's shape (58): lowercase letters, digits and
// dashes. The cockpit filters the input to this alphabet; the server refuses
// anything else so a non-browser client cannot mint a slug with capitals,
// underscores or spaces.
var boxSlugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// boxEditions names the appliance classes a box can be (58).
var boxEditions = map[string]bool{
	"company": true,
	"home":    true,
	"assist":  true,
	"care":    true,
	"lite":    true,
}

// ParseEdition accepts an edition name from the wire, trimmed and
// case-insensitive. The empty string is the lite default — the zero-value
// appliance a fresh box starts as. An unknown name is refused rather than
// defaulted, the same convention ParseRole follows: a typo that silently
// became `lite` would mint the wrong appliance.
func ParseEdition(s string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(s))
	if e == "" {
		return "lite", nil
	}
	if !boxEditions[e] {
		return "", fmt.Errorf("edition %q: want company, home, assist, care or lite", s)
	}
	return e, nil
}

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

// handleCreateBox mints a new box. The slug is the box's unique key in
// [a-z0-9-] form; the store refuses a collision with ErrBoxSlugTaken, and
// hostId is optional and may be bound in a later PATCH.
func (s *Server) handleCreateBox(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	var req struct {
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		HostID  string `json:"hostId"`
		Edition string `json:"edition"`
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
	if !boxSlugRe.MatchString(req.Slug) {
		httpError(w, http.StatusBadRequest, "slug must contain only lowercase letters, digits and dashes")
		return
	}
	edition, err := ParseEdition(req.Edition)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	box, err := s.store.CreateBox(req.Slug, req.Name, req.HostID, edition)
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
	if !cred.Can(authz.ReadBox, "", "") {
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
	if !cred.Can(authz.ReadBox, "", "") {
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
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	boxID := r.PathValue("id")
	var req struct {
		Name    string `json:"name"`
		HostID  string `json:"hostId"`
		Edition string `json:"edition"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httpError(w, http.StatusBadRequest, "name is required")
		return
	}
	edition, err := ParseEdition(req.Edition)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	box, err := s.store.UpdateBox(boxID, req.Name, req.HostID, edition)
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

// handleDeleteBox removes a box and everything bound to it. A missing box
// is a 404.
func (s *Server) handleDeleteBox(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	deleted, err := s.store.DeleteBox(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		httpError(w, http.StatusNotFound, "no such box")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleSetBoxOrgs replaces the organization set bound to a box. The body
// carries the whole new set; duplicate ids collapse and an absent orgIds
// clears the box, matching the store's replace semantics.
func (s *Server) handleSetBoxOrgs(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
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

// handleListBoxOrgs serves the organization set bound to a box, in the same
// {"orgIds":[...]} shape handleSetBoxOrgs accepts, so the operator reads back
// exactly what a PUT wrote. A missing box is a 404.
func (s *Server) handleListBoxOrgs(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.ReadBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	orgIDs, err := s.store.ListBoxOrgs(box.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string][]string{"orgIds": orgIDs})
}

// handleGetBoxNarrator serves the box's narrator — its one box-scoped staff
// member (58). A box whose narrator has not been seeded yet answers 404
// with the reason, so the cockpit can tell "nothing to show" from "not
// authorized".
func (s *Server) handleGetBoxNarrator(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.ReadBox, "", "") {
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

// handleUpdateBoxNarrator edits the box's narrator — its one box-scoped
// staff member (58). The write upserts the row (CreateBox seeds it, so the
// normal path is an update) and bumps the box's config_version in the same
// transaction, so the connector's next sync picks the edited narrator up.
// A missing box is a 404; a blank name and a negative age or word budget
// are refused with 400.
func (s *Server) handleUpdateBoxNarrator(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminBox, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	box, ok := s.boxOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	var req struct {
		Name       string    `json:"name"`
		Locale     string    `json:"locale"`
		Age        int       `json:"age"`
		WordBudget int       `json:"wordBudget"`
		Model      string    `json:"model"`
		Voice      string    `json:"voice"`
		BigFive    Character `json:"bigFive"`
		Brief      string    `json:"brief"`
		SoulCore   string    `json:"soulCore"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Age < 0 {
		httpError(w, http.StatusBadRequest, "age cannot be negative")
		return
	}
	if req.WordBudget < 0 {
		httpError(w, http.StatusBadRequest, "word budget cannot be negative")
		return
	}
	staff, err := s.store.UpdateBoxNarrator(box.ID, req.Name, req.Locale, req.Model, req.Brief, req.SoulCore, req.Voice, req.Age, req.WordBudget, req.BigFive)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, staff)
}
