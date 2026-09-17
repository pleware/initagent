package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

// decodeOrg reads the full-Org JSON both the detail and the plan endpoints
// answer with.
type orgAdminOrg struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Plan    string `json:"plan"`
	Mode    string `json:"mode"`
	Status  string `json:"status"`
	Members int    `json:"members"`
}

// The operator drives one organization's whole lifecycle over HTTP: read the
// row with its status, suspend it, resume it, and move it between plans.
func TestOrgAdminLifecycle(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	customer, err := f.srv.store.CreateOrg("Customer Ltd")
	if err != nil {
		t.Fatal(err)
	}

	resp := f.do(t, http.MethodGet, "/api/admin/orgs/"+customer.Id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail: %d, want 200", resp.StatusCode)
	}
	var org orgAdminOrg
	if err := json.NewDecoder(resp.Body).Decode(&org); err != nil {
		t.Fatal(err)
	}
	if org.Id != customer.Id || org.Name != "Customer Ltd" || org.Plan != string(orgplan.Free) {
		t.Errorf("org = %+v, want the created row on the free plan", org)
	}
	if org.Status != string(OrgStatusActive) {
		t.Errorf("status = %q, want the active zero value", org.Status)
	}

	resp = f.do(t, http.MethodPost, "/api/admin/orgs/"+customer.Id+"/suspend", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("suspend: %d, want 200", resp.StatusCode)
	}
	after, err := f.srv.store.OrgById(customer.Id)
	if err != nil || after == nil {
		t.Fatal(err)
	}
	if after.Status != OrgStatusSuspended {
		t.Fatalf("status after suspend = %q, want suspended", after.Status)
	}

	resp = f.do(t, http.MethodPost, "/api/admin/orgs/"+customer.Id+"/resume", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume: %d, want 200", resp.StatusCode)
	}
	after, err = f.srv.store.OrgById(customer.Id)
	if err != nil || after == nil {
		t.Fatal(err)
	}
	if after.Status != OrgStatusActive {
		t.Fatalf("status after resume = %q, want active", after.Status)
	}

	resp = f.do(t, http.MethodPatch, "/api/admin/orgs/"+customer.Id+"/plan",
		map[string]string{"plan": "team"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("plan: %d, want 200", resp.StatusCode)
	}
	var updated orgAdminOrg
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Plan != string(orgplan.Team) {
		t.Errorf("plan in response = %q, want team", updated.Plan)
	}
	after, err = f.srv.store.OrgById(customer.Id)
	if err != nil || after == nil {
		t.Fatal(err)
	}
	if after.Plan != string(orgplan.Team) {
		t.Errorf("stored plan = %q, want team", after.Plan)
	}
}

// A plan id that is not in the catalogue is refused before anything is
// written, and the stored plan stays where it was.
func TestOrgAdminUnknownPlan(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	customer, err := f.srv.store.CreateOrg("Customer Ltd")
	if err != nil {
		t.Fatal(err)
	}

	resp := f.do(t, http.MethodPatch, "/api/admin/orgs/"+customer.Id+"/plan",
		map[string]string{"plan": "banana"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown plan: %d, want 400", resp.StatusCode)
	}
	org, err := f.srv.store.OrgById(customer.Id)
	if err != nil || org == nil {
		t.Fatal(err)
	}
	if org.Plan != string(orgplan.Free) {
		t.Errorf("stored plan = %q, want the untouched free plan", org.Plan)
	}
}

// Every lifecycle surface answers 404 for an id that is not an organization —
// after the gate, so the caller is somebody who may see them all.
func TestOrgAdminMissingOrg(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	missing := "org-missing"

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"detail", http.MethodGet, "/api/admin/orgs/" + missing, nil},
		{"suspend", http.MethodPost, "/api/admin/orgs/" + missing + "/suspend", nil},
		{"resume", http.MethodPost, "/api/admin/orgs/" + missing + "/resume", nil},
		{"plan", http.MethodPatch, "/api/admin/orgs/" + missing + "/plan", map[string]string{"plan": "team"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := requestJSON(t, f.ts, f.client, c.method, c.path, c.body)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s on a missing org: %d, want 404", c.method, resp.StatusCode)
			}
		})
	}
}

// The lifecycle is the installation boundary of AdminOrg: a customer session
// and an org-scoped token both fail the empty boundary even when the org is
// their own.
func TestOrgAdminRefusesCustomersAndOrgTokens(t *testing.T) {
	f := hostedCustomer(t)

	for _, c := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/orgs/" + f.orgId},
		{http.MethodPost, "/api/admin/orgs/" + f.orgId + "/suspend"},
		{http.MethodPost, "/api/admin/orgs/" + f.orgId + "/resume"},
		{http.MethodPatch, "/api/admin/orgs/" + f.orgId + "/plan"},
	} {
		resp := requestJSON(t, f.ts, f.client, c.method, c.path, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("customer session %s %s: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}

	orgCred := authz.Credential{
		Requester: authz.Requester{Account: f.ownerId},
		Grant:     &authz.Grant{Org: f.orgId, Scopes: []authz.Capability{authz.AdminOrg}},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orgs/"+f.orgId+"/suspend", nil)
	req.SetPathValue("id", f.orgId)
	rec := httptest.NewRecorder()
	f.srv.handleSuspendOrg(rec, req, orgCred)
	if rec.Code != http.StatusForbidden {
		t.Errorf("org token carrying admin:hub.org: %d, want 403", rec.Code)
	}
}

// An installation token carrying admin:hub.org is the machine equivalent of
// the operator: it passes the empty boundary on every lifecycle handler.
func TestOrgAdminTakesInstallationToken(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	customer, err := f.srv.store.CreateOrg("Customer Ltd")
	if err != nil {
		t.Fatal(err)
	}
	cred := authz.Credential{
		Requester: authz.Requester{Account: "account-ops", Platform: true},
		Grant:     &authz.Grant{Installation: true, Scopes: []authz.Capability{authz.AdminOrg}},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/orgs/"+customer.Id, nil)
	req.SetPathValue("id", customer.Id)
	rec := httptest.NewRecorder()
	f.srv.handleGetOrgAdmin(rec, req, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail with an installation token: %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/admin/orgs/"+customer.Id+"/suspend", nil)
	req.SetPathValue("id", customer.Id)
	rec = httptest.NewRecorder()
	f.srv.handleSuspendOrg(rec, req, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("suspend with an installation token: %d, want 200", rec.Code)
	}
	after, err := f.srv.store.OrgById(customer.Id)
	if err != nil || after == nil {
		t.Fatal(err)
	}
	if after.Status != OrgStatusSuspended {
		t.Errorf("status = %q, want suspended", after.Status)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/admin/orgs/"+customer.Id+"/resume", nil)
	req.SetPathValue("id", customer.Id)
	rec = httptest.NewRecorder()
	f.srv.handleResumeOrg(rec, req, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume with an installation token: %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/admin/orgs/"+customer.Id+"/plan",
		strings.NewReader(`{"plan":"starter"}`))
	req.SetPathValue("id", customer.Id)
	rec = httptest.NewRecorder()
	f.srv.handleSetOrgPlan(rec, req, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan with an installation token: %d, want 200", rec.Code)
	}
	after, err = f.srv.store.OrgById(customer.Id)
	if err != nil || after == nil {
		t.Fatal(err)
	}
	if after.Plan != string(orgplan.Starter) {
		t.Errorf("plan = %q, want starter", after.Plan)
	}
}
