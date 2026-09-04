package hub

import (
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
)

// The hub's own surfaces, which are two and not one (drafts 17, 25).
//
// The platform operator administers the installation: every account, every
// organization. An org's owner or admin administers their own people. These
// are the same question — "who is here, and what may they do" — asked at two
// boundaries, so they share one authorization vocabulary and one JSON shape
// rather than one merged screen that has to remember which caller it has.
//
// Collections answer with a bare JSON array, like every other list this hub
// serves. No envelope and no paging: the cockpit ships inside the same binary
// as this API, so adding a page parameter later moves both together, and an
// envelope nobody paginates is ceremony that makes the fifth list look
// different from the first four.

// handleListAccounts serves every account on the installation.
//
// Deliberately not scoped by org: this is the operator's view of the hub they
// run (`admin:hub.account`, 09). An org owner asking the same question uses
// their org's member list, which is a different boundary and a different
// answer.
func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	if !actor.Can(authz.AdminAccounts, "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	accounts, err := s.store.ListAccounts()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, accounts)
}

// handleListAllOrgs enumerates the organizations on this hub with their
// roster sizes.
//
// The size is where this stops. Reading *who* is in a customer's org is an
// org-boundary capability that the platform flag does not grant, because 09
// has not decided whether a hub admin has any path into a customer's data and
// answering it here by accident is the wrong way to find out.
func (s *Server) handleListAllOrgs(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	if !actor.Can(authz.ReadOrg, "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	orgs, err := s.store.ListOrgs()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, orgs)
}

// handleListOrgMembers serves one organization's roster to its own people.
func (s *Server) handleListOrgMembers(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	orgId := r.PathValue("id")
	if !actor.Can(authz.ReadOrg, orgId) {
		// 404 rather than 403 for an org the caller is not in: on a hub with
		// many customers, "you may not see this" confirms the org exists.
		httpError(w, http.StatusNotFound, "no such organization")
		return
	}
	members, err := s.store.ListOrgMembers(orgId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, members)
}

// handleRenameOrg renames an organization.
//
// It exists because first-run guesses the name from the operator's email
// domain, and a guess with no way to correct it is a permanent typo on every
// screen.
func (s *Server) handleRenameOrg(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	orgId := r.PathValue("id")
	if !actor.Can(authz.AdminOrg, orgId) {
		httpError(w, http.StatusNotFound, "no such organization")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httpError(w, http.StatusBadRequest, "name is required")
		return
	}
	// The email argument is unused for a non-empty name; this is the same
	// trimming and length rule first-run applies, in one place.
	if err := s.store.RenameOrg(orgId, auth.CheckOrgName(req.Name)); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleSetOrgMemberRole changes one person's role in an organization.
func (s *Server) handleSetOrgMemberRole(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	orgId := r.PathValue("id")
	target := r.PathValue("accountId")
	var req struct {
		Role string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	role, err := authz.ParseRole(req.Role)
	if err != nil {
		forbid(w, err)
		return
	}
	roster, err := s.store.OrgRoster(orgId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := authz.AuthorizeRoleChange(actor, roster, target, role); err != nil {
		forbid(w, err)
		return
	}
	if err := s.store.SetOrgMemberRole(orgId, target, role); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleRemoveOrgMember removes somebody from an organization, or lets them
// leave it themselves.
func (s *Server) handleRemoveOrgMember(w http.ResponseWriter, r *http.Request, actor authz.Actor) {
	orgId := r.PathValue("id")
	target := r.PathValue("accountId")
	roster, err := s.store.OrgRoster(orgId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := authz.AuthorizeRemoval(actor, roster, target); err != nil {
		forbid(w, err)
		return
	}
	if err := s.store.RemoveOrgMember(orgId, target); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
