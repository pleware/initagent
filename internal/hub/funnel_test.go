package hub

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pleware/initagent/internal/funnel"
	"github.com/pleware/initagent/internal/offering"
)

func TestRecordFunnelEventRejectsUnknownKind(t *testing.T) {
	s := testStore(t)
	if err := s.RecordFunnelEvent(funnel.Event{Kind: "nope"}); err == nil {
		t.Fatal("unknown kind must fail")
	}
}

func TestFunnelFactsFromLiveRows(t *testing.T) {
	s := testStore(t)
	account, org, err := s.RegisterCustomer("ada@example.com", "hash", "Ada", "en")
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(org.Id, "Desk", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateConnector("box", "box", "linux", "amd64", false); err != nil {
		t.Fatal(err)
	}
	devices, err := s.ListConnectors()
	if err != nil || len(devices) == 0 {
		t.Fatalf("devices: %v %v", devices, err)
	}
	if _, err := s.AttachProjectConnector(project.Id, devices[0].Id); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTaskOutput(TaskOutput{
		TaskID: "task-1", OrgID: org.Id, ProjectID: project.Id,
	}); err != nil {
		t.Fatal(err)
	}

	facts, err := s.FunnelFacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Accounts) != 1 || facts.Accounts[0].ID != account.Id {
		t.Fatalf("accounts = %+v", facts.Accounts)
	}
	if len(facts.Orgs) != 1 || !facts.Orgs[0].HasProject || !facts.Orgs[0].HasConnector || facts.Orgs[0].FirstTaskAt.IsZero() {
		t.Fatalf("orgs = %+v", facts.Orgs)
	}
}

func noFollow(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestCTARecordsAndRedirects(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	client := noFollow(t)

	resp := requestJSON(t, f.ts, client, http.MethodGet, "/r/cta/open_app?lng=pl", nil)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("open_app: %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/?lng=pl" {
		t.Fatalf("Location = %q, want /?lng=pl", loc)
	}

	resp = requestJSON(t, f.ts, client, http.MethodGet, "/r/cta/self_host?to=https://evil.example/x", nil)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("self_host: %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != funnel.DefaultSelfHostURL {
		t.Fatalf("evil to = %q, want default developers page", loc)
	}

	resp = requestJSON(t, f.ts, client, http.MethodGet, "/r/cta/nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown which: %d, want 404", resp.StatusCode)
	}

	events, err := f.srv.store.ListFunnelEvents()
	if err != nil {
		t.Fatal(err)
	}
	var open, self int
	for _, ev := range events {
		switch ev.Kind {
		case funnel.KindCTAOpenApp:
			open++
		case funnel.KindCTASelfHost:
			self++
		}
	}
	if open != 1 || self != 1 {
		t.Fatalf("cta events open=%d self=%d, want 1 and 1", open, self)
	}
}

func TestAdminKPIsRequirePlatform(t *testing.T) {
	cust := hostedCustomer(t)
	resp := cust.do(t, http.MethodGet, "/api/admin/kpis", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("customer KPIs: %d, want 403", resp.StatusCode)
	}

	ops := claimedHub(t, offering.Hosted)
	// hostedCustomer already recorded a signup on a different hub; this
	// operator hub should still answer an empty-but-shaped snapshot.
	resp = ops.do(t, http.MethodGet, "/api/admin/kpis", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("operator KPIs: %d, want 200", resp.StatusCode)
	}
	var snap funnel.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.Conversion.MRRAvailable {
		t.Fatal("MRR must stay unavailable until Stripe")
	}
}

func TestRegisterWritesSignupEvent(t *testing.T) {
	f := hostedCustomer(t)
	events, err := f.srv.store.ListFunnelEvents()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		if ev.Kind == funnel.KindSignup && ev.AccountID == f.ownerId && ev.OrgID == f.orgId {
			found = true
		}
	}
	if !found {
		t.Fatalf("signup event missing: %+v", events)
	}
}

func TestLoginWritesLoginEvent(t *testing.T) {
	f := hostedCustomer(t)
	f.signIn(t, "ada@example.com", "correct-horse-battery")
	events, err := f.srv.store.ListFunnelEvents()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Kind == funnel.KindLogin && ev.AccountID == f.ownerId {
			return
		}
	}
	t.Fatalf("login event missing: %+v", events)
}
