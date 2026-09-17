package hub

import (
	"net/http"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/orgplan"
)

// Organization lifecycle administration: the installation boundary of
// AdminOrg (08). The operator — or an installation token carrying
// admin:hub.org — sees every organization and may suspend, resume, or re-plan
// one. A customer's AdminOrg is scoped to their own org, so it fails the
// empty boundary and gets a plain 403. The org row is loaded only after the
// gate, so the missing-row answer is 404, never a hiding refusal.

// orgOr404 loads an organization for a lifecycle handler, or writes the 404
// itself. Every caller passed the installation gate first, so "no such
// organization" is the honest answer, not a way to conceal a customer's
// existence from somebody who may see them all.
func (s *Server) orgOr404(w http.ResponseWriter, orgId string) (*Org, bool) {
	org, err := s.store.OrgById(orgId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if org == nil {
		httpError(w, http.StatusNotFound, "no such organization")
		return nil, false
	}
	return org, true
}

// handleGetOrgAdmin returns one organization's full row to the operator,
// including its status and mode.
func (s *Server) handleGetOrgAdmin(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminOrg, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	org, ok := s.orgOr404(w, r.PathValue("id"))
	if !ok {
		return
	}
	writeJSON(w, org)
}

// handleSuspendOrg blocks an organization from using the hub. Like the mode
// and status writers it is an operator action — the gate keeps a customer
// from suspending themselves or anybody else.
func (s *Server) handleSuspendOrg(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminOrg, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	orgId := r.PathValue("id")
	if _, ok := s.orgOr404(w, orgId); !ok {
		return
	}
	if err := s.store.SetOrgStatus(orgId, OrgStatusSuspended); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleResumeOrg returns a suspended organization to active service.
func (s *Server) handleResumeOrg(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminOrg, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	orgId := r.PathValue("id")
	if _, ok := s.orgOr404(w, orgId); !ok {
		return
	}
	if err := s.store.SetOrgStatus(orgId, OrgStatusActive); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleSetOrgPlan moves an organization between catalogue plans. The store's
// SetOrgPlan is the single writer of the org row's plan — this handler parses
// the submission, refuses unknown ids, and reuses that writer rather than
// adding a second one.
func (s *Server) handleSetOrgPlan(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminOrg, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	orgId := r.PathValue("id")
	var req struct {
		Plan string `json:"plan"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	plan, err := orgplan.Parse(req.Plan)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	org, ok := s.orgOr404(w, orgId)
	if !ok {
		return
	}
	if err := s.store.SetOrgPlan(orgId, plan); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	org.Plan = string(plan)
	writeJSON(w, org)
}
