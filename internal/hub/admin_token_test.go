package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

// mintAdminToken posts a mint request as the fixture's session and returns
// the secret alongside the row, failing the test when the response is not a
// 201.
func mintAdminToken(t *testing.T, f *adminFixture, body map[string]any) (string, ApiToken) {
	t.Helper()
	resp := f.do(t, http.MethodPost, "/api/admin/tokens", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/admin/tokens: %d, want 201", resp.StatusCode)
	}
	var out struct {
		Token string   `json:"token"`
		Row   ApiToken `json:"row"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Token, out.Row
}

// The platform operator mints through the wire, and the secret that comes
// back resolves as an installation-scoped credential naming the operator.
func TestAdminTokenMintAndAuthenticate(t *testing.T) {
	f := claimedHub(t, offering.Hosted)

	secret, row := mintAdminToken(t, f, map[string]any{
		"name":   "ci",
		"scopes": []string{string(authz.AdminSkill), string(authz.ReadOrg)},
	})
	if secret == "" {
		t.Fatal("mint returned an empty secret")
	}
	if row.AccountId != f.ownerId {
		t.Errorf("row account = %q, want %q", row.AccountId, f.ownerId)
	}
	if row.OrgId != "" || row.ProjectId != "" {
		t.Errorf("row boundary = (%q, %q), want empty", row.OrgId, row.ProjectId)
	}
	if len(row.Scopes) != 2 {
		t.Errorf("row scopes = %v, want the two minted", row.Scopes)
	}

	got, ok, err := f.srv.store.ApiTokenAuth(secret)
	if err != nil || !ok {
		t.Fatalf("ApiTokenAuth = (%v, %v)", ok, err)
	}
	if !got.Grant.Installation {
		t.Error("the minted token is not installation-scoped")
	}
	if got.AccountId != f.ownerId {
		t.Errorf("resolved account = %q, want %q", got.AccountId, f.ownerId)
	}
	if got.Grant.Org != "" || got.Grant.Project != "" {
		t.Errorf("resolved boundary = (%q, %q), want empty", got.Grant.Org, got.Grant.Project)
	}
	if len(got.Grant.Scopes) != 2 {
		t.Errorf("resolved scopes = %v, want both minted", got.Grant.Scopes)
	}
}

// A token never reaches these routes: requireSession admits a browser session
// and refuses every bearer, so a token cannot mint or revoke a token. A
// customer session is authenticated but not the platform operator, so the
// gate refuses it with 403.
func TestAdminTokenSurfaceRefusals(t *testing.T) {
	f := hostedCustomer(t)

	wide := f.mintToken(t, authz.Grant{Scopes: authz.GrantableScopes()})
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/tokens"},
		{http.MethodPost, "/api/admin/tokens"},
		{http.MethodDelete, "/api/admin/tokens/token-whatever"},
	} {
		resp := f.withToken(t, wide, c.method, c.path)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s with a token: %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/tokens"},
		{http.MethodPost, "/api/admin/tokens"},
		{http.MethodDelete, "/api/admin/tokens/token-whatever"},
	} {
		resp := f.do(t, c.method, c.path, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as a customer: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}
}

// The scope list is parsed against the installation-grantable set, so an
// empty list and a verb no installation token may carry are both refused
// before a row is written.
func TestAdminTokenMintRejectsBadInput(t *testing.T) {
	f := claimedHub(t, offering.Hosted)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"no name", map[string]any{"scopes": []string{string(authz.AdminSkill)}}},
		{"no scopes", map[string]any{"name": "ci"}},
		{"empty scopes", map[string]any{"name": "ci", "scopes": []string{}}},
		{"non-grantable scope", map[string]any{"name": "ci", "scopes": []string{string(authz.AdminAccounts)}}},
		{"unknown scope", map[string]any{"name": "ci", "scopes": []string{"read:hub.invented"}}},
	}
	for _, c := range cases {
		resp := f.do(t, http.MethodPost, "/api/admin/tokens", c.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", c.name, resp.StatusCode)
		}
	}
}

// The operator list carries installation tokens only, and revocation through
// the wire is a hard stop for the secret it names.
func TestAdminTokenListAndRevoke(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	secret, row := mintAdminToken(t, f, map[string]any{
		"name":   "ci",
		"scopes": []string{string(authz.ReadOrg)},
	})

	// An org-scoped token belongs to the person's own list, not the
	// operator's.
	if _, _, err := f.srv.store.CreateApiToken("org-ci", f.ownerId, authz.Grant{
		Org: f.orgId, Scopes: []authz.Capability{authz.ReadConnector},
	}); err != nil {
		t.Fatal(err)
	}

	var listed []ApiToken
	resp := f.do(t, http.MethodGet, "/api/admin/tokens", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/tokens: %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Id != row.Id {
		t.Fatalf("listed = %+v, want only the installation token", listed)
	}

	resp = f.do(t, http.MethodDelete, "/api/admin/tokens/"+row.Id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/admin/tokens/%s: %d, want 200", row.Id, resp.StatusCode)
	}
	if _, ok, _ := f.srv.store.ApiTokenAuth(secret); ok {
		t.Error("a revoked admin token still resolves")
	}
	// A second delete answers 404 rather than claiming a second success.
	resp = f.do(t, http.MethodDelete, "/api/admin/tokens/"+row.Id, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting a revoked token: %d, want 404", resp.StatusCode)
	}
}

// A hub claimed before accounts existed has nobody to attribute a token to,
// and the refusal says what to do about it rather than failing obscurely.
func TestLegacyHubCannotMintAnAdminToken(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	cred := authz.Credential{Requester: authz.Requester{Platform: true, Unpartitioned: true}}

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/admin/tokens",
		strings.NewReader(`{"name":"ci","scopes":["admin:hub.skill"]}`))
	if err != nil {
		t.Fatal(err)
	}
	srv.handleCreateAdminToken(w, req, cred)
	if w.Code != http.StatusConflict {
		t.Fatalf("legacy mint: %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "claim this hub") {
		t.Errorf("refusal = %q; want it to say how to fix this", w.Body.String())
	}
}
