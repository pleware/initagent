package hub

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/mailer"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

// activityTouchMin is how often a heartbeat may rewrite activity_at.
// Same grain as api_tokens.last_used_at: a busy CLI must not chatter.
const activityTouchMin = time.Minute

type projectIdleRow struct {
	ID           string
	Name         string
	OrgID        string
	GatewayURL   string
	ActivityAt   int64
	IdleWarnedAt int64
}

func (s *Store) TouchProjectActivity(id string, now time.Time) error {
	if id == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE projects SET activity_at = ?, idle_warned_at = 0
		WHERE id = ? AND activity_at <= ?`, now.Unix(), id, now.Add(-activityTouchMin).Unix())
	return err
}

func (s *Store) MarkProjectIdleWarned(id string, now time.Time) error {
	if id == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE projects SET idle_warned_at = ? WHERE id = ? AND idle_warned_at = 0`,
		now.Unix(), id)
	return err
}

func (s *Store) projectIdle(id string) (projectIdleRow, error) {
	var row projectIdleRow
	err := s.db.QueryRow(`SELECT id, name, org_id, gateway_url, activity_at, idle_warned_at
		FROM projects WHERE id = ?`, id).
		Scan(&row.ID, &row.Name, &row.OrgID, &row.GatewayURL, &row.ActivityAt, &row.IdleWarnedAt)
	return row, err
}

func (s *Store) listProjectIdleByOrg(orgID string) ([]projectIdleRow, error) {
	rows, err := s.db.Query(`SELECT id, name, org_id, gateway_url, activity_at, idle_warned_at
		FROM projects WHERE org_id = ? ORDER BY id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []projectIdleRow{}
	for rows.Next() {
		var row projectIdleRow
		if err := rows.Scan(&row.ID, &row.Name, &row.OrgID, &row.GatewayURL, &row.ActivityAt, &row.IdleWarnedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) touchProjectsForDevice(deviceID string, now time.Time) error {
	bounds, err := s.DeviceBoundaries(deviceID)
	if err != nil {
		return err
	}
	for _, b := range bounds {
		if err := s.TouchProjectActivity(b.ProjectId, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) runIdleProjectRetention(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	runOnce := func() {
		warned, deleted, err := s.retainIdleProjects(context.Background(), time.Now())
		if err != nil {
			log.Printf("idle project retention: %v", err)
			return
		}
		if warned > 0 || deleted > 0 {
			log.Printf("idle project: warned %d, deleted %d past the org idle window", warned, deleted)
		}
	}
	runOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			runOnce()
		}
	}
}

func (s *Server) retainIdleProjects(ctx context.Context, now time.Time) (warned, deleted int, err error) {
	if s.store.offering != offering.Hosted {
		return 0, 0, nil
	}
	orgs, err := s.store.ListOrgs()
	if err != nil {
		return 0, 0, err
	}
	for _, org := range orgs {
		idleDays := orgplan.Caps(s.store.offering, orgplan.ID(org.Plan)).IdleDays
		deleteAt, deleteOK := orgplan.IdleCutoff(now, idleDays)
		warnAt, warnOK := orgplan.IdleWarningCutoff(now, idleDays)
		if !deleteOK && !warnOK {
			continue
		}
		projects, err := s.store.listProjectIdleByOrg(org.Id)
		if err != nil {
			return warned, deleted, err
		}
		for _, p := range projects {
			if s.projectHasOnlineWorker(ctx, p) {
				if err := s.store.TouchProjectActivity(p.ID, now); err != nil {
					return warned, deleted, err
				}
				fresh, err := s.store.projectIdle(p.ID)
				if err != nil {
					return warned, deleted, err
				}
				p = fresh
			}
			if deleteOK && p.ActivityAt <= deleteAt.Unix() {
				if err := s.store.DeleteProject(p.ID); err != nil {
					return warned, deleted, err
				}
				deleted++
				continue
			}
			if warnOK && p.IdleWarnedAt == 0 && p.ActivityAt <= warnAt.Unix() {
				if err := s.warnIdleProject(org.Id, p, idleDays, now); err != nil {
					log.Printf("idle project warning %s: %v", p.ID, err)
					continue
				}
				if err := s.store.MarkProjectIdleWarned(p.ID, now); err != nil {
					return warned, deleted, err
				}
				warned++
			}
		}
	}
	return warned, deleted, nil
}

func (s *Server) projectHasOnlineWorker(ctx context.Context, p projectIdleRow) bool {
	if p.GatewayURL != "" {
		var devices []struct {
			Online bool `json:"online"`
		}
		if err := s.getGatewayJSON(ctx, placement{projectID: p.ID, gatewayURL: p.GatewayURL},
			"/api/devices", &devices); err != nil {
			return false
		}
		for _, d := range devices {
			if d.Online {
				return true
			}
		}
		return false
	}
	if s.registry == nil {
		return false
	}
	ids, err := s.store.deviceIdsByProjects([]string{p.ID})
	if err != nil {
		return false
	}
	for _, id := range ids[p.ID] {
		if s.registry.get(id) != nil {
			return true
		}
	}
	return false
}

func (s *Server) warnIdleProject(orgID string, p projectIdleRow, idleDays int, now time.Time) error {
	members, err := s.store.ListOrgMembers(orgID)
	if err != nil {
		return err
	}
	deleteAt := time.Unix(p.ActivityAt, 0).Add(time.Duration(idleDays) * 24 * time.Hour)
	daysLeft := max(1, int(deleteAt.Sub(now).Hours()/24))
	link := s.idleProjectLink(p.ID)
	queued := false
	var first error
	for _, m := range members {
		if authz.Role(m.Role) != authz.RoleOwner {
			continue
		}
		account, err := s.store.AccountById(m.AccountId)
		if err != nil {
			return err
		}
		locale := ""
		if account != nil {
			locale = account.Locale
		}
		subject, text, htmlBody := mailer.IdleWarning(p.Name, link, daysLeft, locale)
		if _, err := s.store.EnqueueMail(mailer.KindIdleWarning, m.Email, subject, text, htmlBody); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		queued = true
		s.kickMail()
	}
	if !queued {
		return first
	}
	return nil
}

func (s *Server) idleProjectLink(projectID string) string {
	if d := strings.TrimSpace(s.opts.TLSDomain); d != "" {
		return "https://" + d + "/code/" + projectID
	}
	return "/code/" + projectID
}

func (s *Server) stampProjectActivity(projectID string) {
	if projectID == "" {
		return
	}
	if err := s.store.TouchProjectActivity(projectID, time.Now()); err != nil {
		log.Printf("project activity: %v", err)
	}
}

func (s *Server) stampTokenGrant(cred authz.Credential) {
	if cred.Grant == nil {
		return
	}
	if cred.Grant.Project != "" {
		s.stampProjectActivity(cred.Grant.Project)
		return
	}
	if cred.Grant.Org == "" {
		return
	}
	projects, err := s.store.ListProjectsByOrg(cred.Grant.Org)
	if err != nil {
		log.Printf("project activity: %v", err)
		return
	}
	for _, p := range projects {
		s.stampProjectActivity(p.Id)
	}
}
