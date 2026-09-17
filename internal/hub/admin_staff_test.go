package hub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

// staffBySlug returns the staff member with the given slug from a list,
// failing the test when it is absent.
func staffBySlug(t *testing.T, staff []Staff, slug string) Staff {
	t.Helper()
	for _, st := range staff {
		if st.Slug == slug {
			return st
		}
	}
	t.Fatalf("staff list has no %q: %+v", slug, staff)
	return Staff{}
}

// withTokenBody performs a request carrying a bearer and an optional JSON
// body, where a nil body sends no body at all — the body-capable sibling of
// adminFixture.withToken.
func (f *adminFixture) withTokenBody(t *testing.T, token, method, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// The platform operator lists, creates and updates the canonical staff
// catalogue through the wire, and every write answers with the full row.
func TestAdminStaffCRUD(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)

	resp := f.do(t, http.MethodGet, "/api/admin/staff", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/staff: %d, want 200", resp.StatusCode)
	}
	var seeded []Staff
	if err := json.NewDecoder(resp.Body).Decode(&seeded); err != nil {
		t.Fatal(err)
	}
	if len(seeded) != 2 {
		t.Fatalf("seeded staff = %d, want the two baseline members", len(seeded))
	}

	resp = f.do(t, http.MethodPost, "/api/admin/staff", map[string]any{
		"slug": "staff-nonbinary-00", "name": "Alex", "locale": "en", "age": 28,
		"model": "alex.glb", "brief": "curious about everything", "wordBudget": 500,
		"bigFive": map[string]float64{"openness": 0.8},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/admin/staff: %d, want 200", resp.StatusCode)
	}
	var created Staff
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Slug != "staff-nonbinary-00" || created.Name != "Alex" {
		t.Errorf("created = %+v, want a minted row for the submitted slug and name", created)
	}
	if created.Locale != "en" || created.Age != 28 || created.WordBudget != 500 ||
		created.Model != "alex.glb" || created.Brief != "curious about everything" {
		t.Errorf("created fields = %+v, want the submitted values", created)
	}

	// PATCH updates by slug and keeps the same row.
	resp = f.do(t, http.MethodPatch, "/api/admin/staff/"+created.ID, map[string]any{
		"slug": "staff-nonbinary-00", "name": "Aleks", "locale": "pl", "age": 29,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/admin/staff/%s: %d, want 200", created.ID, resp.StatusCode)
	}
	var updated Staff
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != "Aleks" || updated.Age != 29 || updated.Locale != "pl" {
		t.Errorf("updated = %+v, want the same row with the submitted fields", updated)
	}

	resp = f.do(t, http.MethodGet, "/api/admin/staff", nil)
	if err := json.NewDecoder(resp.Body).Decode(&seeded); err != nil {
		t.Fatal(err)
	}
	if got := staffBySlug(t, seeded, "staff-nonbinary-00"); got.Name != "Aleks" {
		t.Errorf("list carries name %q after the update, want Aleks", got.Name)
	}
}

// A canonical row has to be nameable and greettable: the slug and the name
// are required, and neither may be blank after trimming.
func TestAdminStaffRejectsMissingSlugOrName(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)

	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"no slug", map[string]any{"name": "Nova"}},
		{"blank slug", map[string]any{"slug": "   ", "name": "Nova"}},
		{"no name", map[string]any{"slug": "staff-nova-00"}},
		{"blank name", map[string]any{"slug": "staff-nova-00", "name": " "}},
	} {
		resp := f.do(t, http.MethodPost, "/api/admin/staff", c.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", c.name, resp.StatusCode)
		}
	}
}

// An installation token carrying admin:hub.staff is the machine equivalent
// of the operator at this boundary: the empty-org gate admits it on every
// handler, checked directly here and over the wire in
// TestAdminStaffTakesInstallationTokenOverWire.
func TestAdminStaffTakesInstallationToken(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	cred := authz.Credential{
		Requester: authz.Requester{Account: "account-ops", Platform: true},
		Grant:     &authz.Grant{Installation: true, Scopes: []authz.Capability{authz.AdminStaff}},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/staff", nil)
	rec := httptest.NewRecorder()
	f.srv.handleListStaff(rec, req, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("list with an installation token: %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/admin/staff",
		strings.NewReader(`{"slug":"staff-nova-00","name":"Nova","age":30}`))
	rec = httptest.NewRecorder()
	f.srv.handleUpsertStaff(rec, req, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("create with an installation token: %d, want 200", rec.Code)
	}
	var created Staff
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Slug != "staff-nova-00" || created.Name != "Nova" {
		t.Errorf("created = %+v, want the submitted row", created)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/admin/staff/"+created.ID,
		strings.NewReader(`{"slug":"staff-nova-00","name":"Nova Two"}`))
	rec = httptest.NewRecorder()
	f.srv.handleUpsertStaff(rec, req, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("update with an installation token: %d, want 200", rec.Code)
	}
	var updated Staff
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != "Nova Two" {
		t.Errorf("updated = %+v, want the same row renamed", updated)
	}
}

// An org token reaches the routes but cannot stand at the empty boundary,
// so the gate refuses it 403; a customer session is authenticated but not
// the platform operator, so the gate refuses it too; an anonymous caller is
// turned away at the middleware with 401.
func TestAdminStaffSurfaceRefusals(t *testing.T) {
	f := hostedCustomer(t)

	wide := f.mintToken(t, authz.Grant{Scopes: authz.GrantableScopes()})
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/staff"},
		{http.MethodPost, "/api/admin/staff"},
		{http.MethodPatch, "/api/admin/staff/staff-whatever"},
	} {
		resp := f.withToken(t, wide, c.method, c.path)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s with a token: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/staff"},
		{http.MethodPost, "/api/admin/staff"},
		{http.MethodPatch, "/api/admin/staff/staff-whatever"},
	} {
		resp := f.do(t, c.method, c.path, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as a customer: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/staff"},
		{http.MethodPost, "/api/admin/staff"},
		{http.MethodPatch, "/api/admin/staff/staff-whatever"},
	} {
		resp := requestJSON(t, f.ts, &http.Client{}, c.method, c.path, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a cookie: %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}
}

// An installation token carrying admin:hub.staff is the machine equivalent
// of the operator at this boundary, and the wire admits it on every route:
// list, create and update all pass the empty-org gate with a 200.
func TestAdminStaffTakesInstallationTokenOverWire(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	secret, _, err := f.srv.store.CreateAdminToken("ci", f.ownerId, []authz.Capability{authz.AdminStaff})
	if err != nil {
		t.Fatal(err)
	}

	resp := f.withToken(t, secret, http.MethodGet, "/api/admin/staff")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/staff with an installation token: %d, want 200", resp.StatusCode)
	}

	resp = f.withTokenBody(t, secret, http.MethodPost, "/api/admin/staff", map[string]any{
		"slug": "staff-nova-00", "name": "Nova", "age": 30,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/admin/staff with an installation token: %d, want 200", resp.StatusCode)
	}
	var created Staff
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Slug != "staff-nova-00" || created.Name != "Nova" {
		t.Errorf("created = %+v, want the submitted row", created)
	}

	resp = f.withTokenBody(t, secret, http.MethodPatch, "/api/admin/staff/"+created.ID, map[string]any{
		"slug": "staff-nova-00", "name": "Nova Two",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/admin/staff/%s with an installation token: %d, want 200", created.ID, resp.StatusCode)
	}
	var updated Staff
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != "Nova Two" {
		t.Errorf("updated = %+v, want the same row renamed", updated)
	}
}

// A credential that reaches the handler without the capability is refused at
// the empty boundary: an installation token missing admin:hub.staff, and an
// org-scoped token that cannot stand at the installation even when it lists
// the verb.
func TestAdminStaffGateRefusesWrongCredentials(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	cases := []struct {
		name string
		cred authz.Credential
	}{
		{
			name: "installation token without the capability",
			cred: authz.Credential{
				Requester: authz.Requester{Account: "account-ops", Platform: true},
				Grant:     &authz.Grant{Installation: true, Scopes: []authz.Capability{authz.ReadOrg}},
			},
		},
		{
			name: "org token carrying the capability",
			cred: authz.Credential{
				Requester: authz.Requester{Account: "account-ops", Platform: true},
				Grant:     &authz.Grant{Org: "org-1", Scopes: []authz.Capability{authz.AdminStaff}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/staff", nil)
			rec := httptest.NewRecorder()
			f.srv.handleListStaff(rec, req, c.cred)
			if rec.Code != http.StatusForbidden {
				t.Errorf("list staff: %d, want 403", rec.Code)
			}
		})
	}
}
