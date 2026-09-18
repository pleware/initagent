package hub

import (
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/authz"
)

// An organization's staff view (24/28 walk-up). The canonical rows live at
// the installation boundary (admin_staff.go); here an org reads its
// effective roster — every canonical row with this org's overrides applied —
// and an org admin writes or clears those overrides. The read is every
// member's (ReadOrg at the org boundary), the writes are admin and above
// (AdminStaff at the org boundary).

// handleListOrgStaff serves the staff roster as this organization sees it:
// each overridable field is the org's override where one exists, the
// canonical value otherwise.
func (s *Server) handleListOrgStaff(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	orgID := r.PathValue("id")
	if !cred.Can(authz.ReadOrg, orgID, "") {
		hideOrRefuse(w, cred, authz.ReadOrg, "no such organization", bound{org: orgID})
		return
	}
	staff, err := s.store.StaffForOrg(orgID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, staff)
}

// handleSetOrgStaffOverride writes this org's tuning of one staff member.
// Only the overridable fields travel — name, age, soul override, voice,
// BigFive, brief, model, word budget — and a nil field means "inherit the
// canonical row". The upsert replaces a previous override in place.
func (s *Server) handleSetOrgStaffOverride(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	orgID := r.PathValue("id")
	if !cred.Can(authz.AdminStaff, orgID, "") {
		hideOrRefuse(w, cred, authz.AdminStaff, "no such organization", bound{org: orgID})
		return
	}
	var req struct {
		Name         *string    `json:"name"`
		Age          *int       `json:"age"`
		SoulOverride *string    `json:"soulOverride"`
		Voice        *string    `json:"voice"`
		BigFive      *Character `json:"bigFive"`
		Brief        *string    `json:"brief"`
		Model        *string    `json:"model"`
		WordBudget   *int       `json:"wordBudget"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if len(trimmed) < 2 {
			httpError(w, http.StatusBadRequest, "name must be at least two characters")
			return
		}
		req.Name = &trimmed
	}
	if req.Age != nil && *req.Age < 0 {
		httpError(w, http.StatusBadRequest, "age must be zero or more")
		return
	}
	if err := s.store.SetOrgStaffOverride(orgID, r.PathValue("staffId"), req.Name, req.Age, req.SoulOverride, req.Voice, req.BigFive, req.Brief, req.Model, req.WordBudget); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// handleClearOrgStaffOverride drops this org's tuning of one staff member,
// which returns that member to the canonical row. Clearing a missing
// override is not an error.
func (s *Server) handleClearOrgStaffOverride(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	orgID := r.PathValue("id")
	if !cred.Can(authz.AdminStaff, orgID, "") {
		hideOrRefuse(w, cred, authz.AdminStaff, "no such organization", bound{org: orgID})
		return
	}
	if err := s.store.ClearOrgStaffOverride(orgID, r.PathValue("staffId")); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
