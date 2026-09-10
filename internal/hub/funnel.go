package hub

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/funnel"
	"github.com/pleware/initagent/internal/id"
)

// recordEvent writes a KPI row. Insert failure is logged and never
// returned to the caller — counting must not fail signup, login, or a wall.
func (s *Server) recordEvent(e funnel.Event) {
	if s.store == nil {
		return
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}
	if e.ID == "" {
		rowID, err := id.New(id.Event)
		if err != nil {
			log.Printf("funnel event: %v", err)
			return
		}
		e.ID = rowID
	}
	if err := s.store.RecordFunnelEvent(e); err != nil {
		log.Printf("funnel event: %v", err)
	}
}

func (s *Server) hitPlanLimit(orgID, accountID, projectID, wall string) {
	s.recordEvent(funnel.Event{
		Kind:      funnel.KindPlanLimitHit,
		OrgID:     orgID,
		AccountID: accountID,
		ProjectID: projectID,
		Wall:      wall,
	})
}

func (s *Server) reportPlanLimit(w http.ResponseWriter, err error, orgID, accountID, projectID string) bool {
	var pe planLimitError
	if !errors.As(err, &pe) {
		return false
	}
	s.hitPlanLimit(orgID, accountID, projectID, pe.Wall)
	writePlanLimit(w, pe.Wall, pe.Limit)
	return true
}

// handleCTA records a marketing click and 302s. 301 would be cached and
// the second click would never hit us.
func (s *Server) handleCTA(w http.ResponseWriter, r *http.Request) {
	kind, location, ok := funnel.ResolveCTA(r.PathValue("which"), r.URL.Query().Get("to"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.recordEvent(funnel.Event{Kind: kind})
	if kind == funnel.KindCTAOpenApp {
		if lng := strings.TrimSpace(r.URL.Query().Get("lng")); lng != "" {
			location += "?lng=" + url.QueryEscape(lng)
		}
	}
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusFound)
}

func (s *Server) handleAdminKPIs(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminAccounts, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	facts, err := s.store.FunnelFacts()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := s.store.ListFunnelEvents()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap := funnel.SnapshotFrom(facts, events, time.Now())
	if s.registry != nil {
		snap.Cost.OnlineWorkers = len(s.registry.all())
	}
	writeJSON(w, snap)
}
