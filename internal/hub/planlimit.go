package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/orgplan"
)

// ErrPlanLimit is the sentinel for a hosted catalogue wall (draft 48).
// Self-host never returns it: Caps ignores the org plan there.
var ErrPlanLimit = errors.New("plan limit")

type planLimitError struct {
	Wall  string
	Limit int
}

func (e planLimitError) Error() string {
	return planLimitMessage(e.Wall, e.Limit)
}

func (e planLimitError) Unwrap() error { return ErrPlanLimit }

func planLimitMessage(wall string, limit int) string {
	switch wall {
	case "projects":
		if limit == 1 {
			return "This plan allows 1 project. Open Plans to add another."
		}
		return fmt.Sprintf("This plan allows %d projects. Open Plans to add another.", limit)
	case "people":
		if limit == 1 {
			return "This plan is for 1 person. Open Plans to add someone."
		}
		return fmt.Sprintf("This plan allows %d people. Open Plans to add someone.", limit)
	case "machines":
		if limit == 1 {
			return "This plan allows 1 of your machines on this project. Open Plans to add another."
		}
		return fmt.Sprintf("This plan allows %d of your machines on this project. Open Plans to add another.", limit)
	default:
		return "This plan does not allow that. Open Plans to continue."
	}
}

func writePlanLimit(w http.ResponseWriter, wall string, limit int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": planLimitMessage(wall, limit),
		"code":  "plan_limit",
		"wall":  wall,
		"limit": limit,
	})
}

func (s *Server) handleListPlans(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, orgplan.Catalogue())
}

func (s *Server) orgCaps(orgId string) (orgplan.Limits, error) {
	org, err := s.store.OrgById(orgId)
	if err != nil {
		return orgplan.Limits{}, err
	}
	if org == nil {
		return orgplan.Limits{}, fmt.Errorf("organization %q does not exist", orgId)
	}
	return orgplan.Caps(s.opts.Offering, orgplan.ID(org.Plan)), nil
}

func (s *Server) refuseAnotherProject(w http.ResponseWriter, orgId string) bool {
	caps, err := s.orgCaps(orgId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return true
	}
	n, err := s.store.CountProjects(orgId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return true
	}
	if orgplan.AllowsAnother(n, caps.Projects) {
		return false
	}
	writePlanLimit(w, "projects", caps.Projects)
	return true
}

func (s *Server) refuseAnotherMachine(w http.ResponseWriter, orgId, existingDevice, nextDevice string) bool {
	nextDevice = strings.TrimSpace(nextDevice)
	existingDevice = strings.TrimSpace(existingDevice)
	if nextDevice == "" || nextDevice == existingDevice || existingDevice != "" {
		// Empty is no bind. Replacing the one slot is not adding a machine.
		return false
	}
	caps, err := s.orgCaps(orgId)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return true
	}
	if orgplan.AllowsAnother(0, caps.WorkersPerProject) {
		return false
	}
	writePlanLimit(w, "machines", caps.WorkersPerProject)
	return true
}
