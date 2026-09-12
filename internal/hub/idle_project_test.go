package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/mailer"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

func TestCreateProjectSeedsActivity(t *testing.T) {
	s := testStore(t)
	org, err := s.CreateOrg("Free")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(org.Id, "Storefront", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.projectIdle(p.Id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ActivityAt == 0 || row.ActivityAt != p.CreatedAt {
		t.Fatalf("activity_at = %d, want created_at %d", row.ActivityAt, p.CreatedAt)
	}
	if row.IdleWarnedAt != 0 {
		t.Fatalf("idle_warned_at = %d, want 0", row.IdleWarnedAt)
	}
}

func TestTouchProjectActivityClearsWarning(t *testing.T) {
	s := testStore(t)
	org, err := s.CreateOrg("Free")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(org.Id, "Storefront", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if _, err := s.db.Exec(`UPDATE projects SET activity_at = ?, idle_warned_at = ? WHERE id = ?`,
		now.Add(-46*24*time.Hour).Unix(), now.Add(-1*time.Hour).Unix(), p.Id); err != nil {
		t.Fatal(err)
	}
	if err := s.TouchProjectActivity(p.Id, now); err != nil {
		t.Fatal(err)
	}
	row, err := s.projectIdle(p.Id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ActivityAt != now.Unix() {
		t.Fatalf("activity_at = %d, want %d", row.ActivityAt, now.Unix())
	}
	if row.IdleWarnedAt != 0 {
		t.Fatalf("idle_warned_at = %d after touch, want 0", row.IdleWarnedAt)
	}
	if err := s.TouchProjectActivity(p.Id, now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	again, err := s.projectIdle(p.Id)
	if err != nil {
		t.Fatal(err)
	}
	if again.ActivityAt != now.Unix() {
		t.Fatal("debounce must skip a stamp inside one minute")
	}
}

func TestDeleteProjectDropsTaskOutputs(t *testing.T) {
	s := testStore(t)
	org, err := s.CreateOrg("Free")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(org.Id, "Storefront", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTaskOutput(TaskOutput{TaskID: "task-1", OrgID: org.Id, ProjectID: p.Id, Stdout: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(p.Id); err != nil {
		t.Fatal(err)
	}
	got, err := s.TaskOutputByID("task-1")
	if err != nil || got != nil {
		t.Fatalf("task output after delete = (%v, %v), want gone", got, err)
	}
}

func idleOrgProject(t *testing.T, s *Store) (orgID, projectID, ownerEmail string) {
	t.Helper()
	org, err := s.CreateOrg("Free")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	account, err := s.CreateAccount("ada@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrgMember(org.Id, account.Id, authz.RoleOwner); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(org.Id, "Storefront", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return org.Id, p.Id, account.Email
}

func TestRetainIdleProjectsWarnsThenDeletes(t *testing.T) {
	s := testStore(t)
	s.setOffering(offering.Hosted)
	_, projectID, ownerEmail := idleOrgProject(t, s)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if _, err := s.db.Exec(`UPDATE projects SET activity_at = ? WHERE id = ?`,
		now.Add(-46*24*time.Hour).Unix(), projectID); err != nil {
		t.Fatal(err)
	}
	srv := &Server{store: s, opts: Options{TLSDomain: "app.example"}}
	warned, deleted, err := srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 1 || deleted != 0 {
		t.Fatalf("warn pass = (%d, %d, %v), want 1 warned", warned, deleted, err)
	}
	row, err := s.projectIdle(projectID)
	if err != nil || row.IdleWarnedAt == 0 {
		t.Fatalf("warned stamp missing: %+v %v", row, err)
	}
	var kind, to, text string
	if err := s.db.QueryRow(`SELECT kind, to_addr, text_body FROM mail_outbox WHERE kind = ?`,
		mailer.KindIdleWarning).Scan(&kind, &to, &text); err != nil {
		t.Fatalf("warning mail missing: %v", err)
	}
	if kind != mailer.KindIdleWarning || to != ownerEmail {
		t.Fatalf("mail kind=%q to=%q", kind, to)
	}
	if !containsAll(text, "Storefront", "https://app.example/code/"+projectID, "14") {
		t.Fatalf("mail text = %q", text)
	}

	warned, deleted, err = srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 0 || deleted != 0 {
		t.Fatalf("second warn pass must be a no-op: %d %d %v", warned, deleted, err)
	}

	if _, err := s.db.Exec(`UPDATE projects SET activity_at = ? WHERE id = ?`,
		now.Add(-60*24*time.Hour).Unix(), projectID); err != nil {
		t.Fatal(err)
	}
	warned, deleted, err = srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 0 || deleted != 1 {
		t.Fatalf("delete pass = (%d, %d, %v), want 1 deleted", warned, deleted, err)
	}
	gone, err := s.ProjectById(projectID)
	if err != nil || gone != nil {
		t.Fatalf("project after idle delete = (%v, %v)", gone, err)
	}
}

func TestRetainIdleProjectsDeletesWithoutPriorWarn(t *testing.T) {
	s := testStore(t)
	s.setOffering(offering.Hosted)
	_, projectID, _ := idleOrgProject(t, s)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if _, err := s.db.Exec(`UPDATE projects SET activity_at = ? WHERE id = ?`,
		now.Add(-61*24*time.Hour).Unix(), projectID); err != nil {
		t.Fatal(err)
	}
	srv := &Server{store: s, opts: Options{TLSDomain: "app.example"}}
	warned, deleted, err := srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 0 || deleted != 1 {
		t.Fatalf("late delete = (%d, %d, %v)", warned, deleted, err)
	}
}

func TestRetainIdleProjectsSkipsSelfHostAndPaid(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	s := testStore(t)
	_, projectID, _ := idleOrgProject(t, s)
	if _, err := s.db.Exec(`UPDATE projects SET activity_at = ? WHERE id = ?`,
		now.Add(-400*24*time.Hour).Unix(), projectID); err != nil {
		t.Fatal(err)
	}
	srv := &Server{store: s}
	warned, deleted, err := srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 0 || deleted != 0 {
		t.Fatalf("self-host = (%d, %d, %v)", warned, deleted, err)
	}

	s.setOffering(offering.Hosted)
	orgs, err := s.ListOrgs()
	if err != nil || len(orgs) != 1 {
		t.Fatal(err)
	}
	if err := s.SetOrgPlan(orgs[0].Id, orgplan.Starter); err != nil {
		t.Fatal(err)
	}
	warned, deleted, err = srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 0 || deleted != 0 {
		t.Fatalf("starter = (%d, %d, %v)", warned, deleted, err)
	}
	if err := s.SetOrgPlan(orgs[0].Id, orgplan.Enterprise); err != nil {
		t.Fatal(err)
	}
	warned, deleted, err = srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 0 || deleted != 0 {
		t.Fatalf("enterprise = (%d, %d, %v)", warned, deleted, err)
	}
}

func TestRetainIdleProjectsStampsOnlineGatewayWorker(t *testing.T) {
	s := testStore(t)
	s.setOffering(offering.Hosted)
	org, err := s.CreateOrg("Free")
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devices" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"online": true}})
	}))
	t.Cleanup(gw.Close)
	p, err := s.CreateProject(org.Id, "Storefront", "", "", gw.URL, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if _, err := s.db.Exec(`UPDATE projects SET activity_at = ? WHERE id = ?`,
		now.Add(-61*24*time.Hour).Unix(), p.Id); err != nil {
		t.Fatal(err)
	}
	srv := &Server{store: s}
	warned, deleted, err := srv.retainIdleProjects(context.Background(), now)
	if err != nil || warned != 0 || deleted != 0 {
		t.Fatalf("online worker must keep the project: %d %d %v", warned, deleted, err)
	}
	row, err := s.projectIdle(p.Id)
	if err != nil || row.ActivityAt != now.Unix() {
		t.Fatalf("online worker did not stamp: %+v %v", row, err)
	}
}

func TestUpdateDeviceOnConnectStampsProject(t *testing.T) {
	s := testStore(t)
	org, err := s.CreateOrg("Free")
	if err != nil {
		t.Fatal(err)
	}
	deviceID, _, err := s.CreateDevice("studio", "studio.local", "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(org.Id, "Storefront", deviceID, "/srv", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-2 * time.Hour)
	if _, err := s.db.Exec(`UPDATE projects SET activity_at = ? WHERE id = ?`, now.Unix(), p.Id); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceOnConnect(deviceID, "studio.local", "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	row, err := s.projectIdle(p.Id)
	if err != nil || row.ActivityAt <= now.Unix() {
		t.Fatalf("connect did not stamp: %+v %v", row, err)
	}
}

func TestProjectActivityHTTP(t *testing.T) {
	f := hostedCustomer(t)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": "Storefront"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	var p Project
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour).Unix()
	if _, err := f.srv.store.db.Exec(`UPDATE projects SET activity_at = ? WHERE id = ?`, old, p.Id); err != nil {
		t.Fatal(err)
	}

	list := f.do(t, http.MethodGet, "/api/projects", nil)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list: %d", list.StatusCode)
	}
	afterList, err := f.srv.store.projectIdle(p.Id)
	if err != nil || afterList.ActivityAt != old {
		t.Fatalf("GET /api/projects must not stamp: %+v %v", afterList, err)
	}

	ping := f.do(t, http.MethodPost, "/api/projects/"+p.Id+"/activity", map[string]any{})
	if ping.StatusCode != http.StatusOK {
		t.Fatalf("activity: %d", ping.StatusCode)
	}
	var ok map[string]bool
	if err := json.NewDecoder(ping.Body).Decode(&ok); err != nil || !ok["ok"] {
		t.Fatalf("activity body = %v %v", ok, err)
	}
	afterPing, err := f.srv.store.projectIdle(p.Id)
	if err != nil || afterPing.ActivityAt <= old {
		t.Fatalf("UI ping did not stamp: %+v %v", afterPing, err)
	}

	other, err := f.srv.store.CreateOrg("Other")
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := f.srv.store.CreateProject(other.Id, "Secret", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	hiddenPing := f.do(t, http.MethodPost, "/api/projects/"+hidden.Id+"/activity", map[string]any{})
	if hiddenPing.StatusCode != http.StatusNotFound {
		t.Fatalf("other org activity: %d, want 404", hiddenPing.StatusCode)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
