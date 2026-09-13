package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/store"
)

// --- helpers ---

// operatorCred is the credential a test hub's own operator has: a session,
// so no grant, on an installation that has no organizations yet. That is the
// self-host shape, and it lets a handler be called directly without staging
// an account and a membership first.
var operatorCred = authz.Credential{Actor: authz.Actor{Platform: true, Unpartitioned: true}}

// seedOwner returns an account that owns an organization on this hub,
// creating both when the hub has none.
//
// A token now needs a subject and a boundary, so tests that used to call
// CreateApiToken("test") need somebody to be its author. This supplies one
// rather than each test growing the same four-line preamble.
func seedOwner(t *testing.T, s *Store) (account, org string) {
	t.Helper()
	orgs, err := s.ListOrgs()
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	if len(orgs) == 0 {
		created, err := s.CreateOrg("Test Org")
		if err != nil {
			t.Fatalf("CreateOrg: %v", err)
		}
		orgs = []Org{*created}
	}
	org = orgs[0].Id

	members, err := s.ListOrgMembers(org)
	if err != nil {
		t.Fatalf("ListOrgMembers: %v", err)
	}
	for _, m := range members {
		if m.Role == string(authz.RoleOwner) {
			return m.AccountId, org
		}
	}
	hash, err := auth.HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	created, err := s.CreateAccount("owner-"+org+"@example.com", hash)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := s.AddOrgMember(org, created.Id, authz.RoleOwner); err != nil {
		t.Fatalf("AddOrgMember: %v", err)
	}
	return created.Id, org
}

// attachToProject puts a machine inside a project so a scoped credential can
// reach it.
//
// An unattached machine is invisible to every token on purpose: it has no
// owner to check a boundary against. A test that wants to reach one therefore
// takes the same step an operator does in the cockpit.
func attachToProject(t *testing.T, s *Store, org, connectorId string) string {
	t.Helper()
	project, err := s.CreateProject(org, "Test Project", connectorId, "/srv/app", "", "", "", "")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := s.AttachProjectConnector(project.Id, connectorId); err != nil {
		t.Fatalf("AttachProjectConnector: %v", err)
	}
	return project.Id
}

// fleetToken mints a credential that can reach connectorId, with the verbs the
// caller names. No scopes means every grantable verb.
func fleetToken(t *testing.T, s *Store, connectorId string, scopes ...authz.Capability) string {
	t.Helper()
	account, org := seedOwner(t, s)
	if connectorId != "" {
		attachToProject(t, s, org, connectorId)
	}
	if len(scopes) == 0 {
		scopes = authz.GrantableScopes()
	}
	secret, _, err := s.CreateApiToken("test", account, authz.Grant{Org: org, Scopes: scopes})
	if err != nil {
		t.Fatalf("CreateApiToken: %v", err)
	}
	return secret
}

// mintToken issues a credential for this fixture's owner. Tests name the
// verbs they need, which is the point of the change: a test that wants a wide
// token has to say so.
func (f *adminFixture) mintToken(t *testing.T, g authz.Grant) string {
	t.Helper()
	if g.Org == "" {
		g.Org = f.orgId
	}
	secret, _, err := f.srv.store.CreateApiToken("ci", f.ownerId, g)
	if err != nil {
		t.Fatalf("CreateApiToken: %v", err)
	}
	return secret
}

// withToken performs a request carrying a bearer instead of a session.
func (f *adminFixture) withToken(t *testing.T, token, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, f.ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Error
}

// --- the three axes ---

// The store refuses a row missing an axis, so a handler bug cannot reintroduce
// the credential this whole change exists to retire.
func TestStoreRefusesAnUnscopedToken(t *testing.T) {
	s := testStore(t)
	account, org := seedOwner(t, s)

	cases := []struct {
		name    string
		account string
		grant   authz.Grant
	}{
		{"no subject", "", authz.Grant{Org: org, Scopes: []authz.Capability{authz.ReadConnector}}},
		{"no boundary", account, authz.Grant{Scopes: []authz.Capability{authz.ReadConnector}}},
		{"no verbs", account, authz.Grant{Org: org}},
		{"nothing at all", "", authz.Grant{}},
	}
	for _, c := range cases {
		if _, _, err := s.CreateApiToken("ci", c.account, c.grant); err != ErrTokenUnscoped {
			t.Errorf("%s: CreateApiToken error = %v; want ErrTokenUnscoped", c.name, err)
		}
	}
}

func TestOpenStoreRewritesInheritedDeviceScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scopes.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	account, org := seedOwner(t, s)
	secret, row, err := s.CreateApiToken("ci", account, authz.Grant{
		Org: org, Scopes: []authz.Capability{authz.ReadConnector},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE api_tokens SET scopes = ?
		WHERE id = ?`, "read:fleet.device admin:fleet.device", row.Id); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen after rewriting scopes to the inherited spellings: %v", err)
	}
	t.Cleanup(func() { again.Close() })

	got, ok, err := again.ApiTokenAuth(secret)
	if err != nil || !ok {
		t.Fatalf("a token whose scopes were rewritten must still resolve: ok=%v err=%v", ok, err)
	}
	if !slices.Contains(got.Grant.Scopes, authz.ReadConnector) ||
		!slices.Contains(got.Grant.Scopes, authz.AdminConnector) {
		t.Fatalf("resolved scopes = %v; want read and admin on fleet.connector", got.Grant.Scopes)
	}

	// A second open must not touch rows that already say connector.
	if err := again.ensureFleetConnectorScopes(); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := again.db.QueryRow(`SELECT scopes FROM api_tokens WHERE id = ?`, row.Id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "fleet.device") {
		t.Fatalf("scopes after a second pass = %q; want no fleet.device left", stored)
	}
}

func TestApiTokenLifecycle(t *testing.T) {
	s := testStore(t)
	account, org := seedOwner(t, s)
	grant := authz.Grant{Org: org, Scopes: []authz.Capability{authz.ReadConnector, authz.CreateTask}}

	secret, row, err := s.CreateApiToken("ci", account, grant)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(row.Id, "token-") {
		t.Errorf("token id = %q; want a token- identifier (06)", row.Id)
	}

	got, ok, err := s.ApiTokenAuth(secret)
	if err != nil || !ok {
		t.Fatalf("ApiTokenAuth = (%v, %v)", ok, err)
	}
	if got.AccountId != account || got.Grant.Org != org || got.Grant.Project != "" {
		t.Errorf("resolved grant = %+v; want the one minted", got)
	}
	if len(got.Grant.Scopes) != 2 {
		t.Errorf("resolved scopes = %v; want both", got.Grant.Scopes)
	}

	listed, err := s.ListApiTokens(account)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListApiTokens = (%d rows, %v), want one", len(listed), err)
	}

	// Another account's list must not include it, or one person could read
	// how far another person's credentials reach.
	other, err := s.CreateAccount("stranger@example.com", "x")
	if err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.ListApiTokens(other.Id); len(rows) != 0 {
		t.Errorf("a stranger listed %d tokens; want none", len(rows))
	}
	// Nor may they revoke it.
	if revoked, _ := s.RevokeApiToken(row.Id, other.Id); revoked {
		t.Error("a stranger revoked somebody else's token")
	}

	revoked, err := s.RevokeApiToken(row.Id, account)
	if err != nil || !revoked {
		t.Fatalf("RevokeApiToken = (%v, %v), want true", revoked, err)
	}
	// A revoked row is not found at all, rather than found and flagged.
	if _, ok, _ := s.ApiTokenAuth(secret); ok {
		t.Error("a revoked token still resolved")
	}
	if rows, _ := s.ListApiTokens(account); len(rows) != 0 {
		t.Errorf("a revoked token is still listed (%d rows)", len(rows))
	}
	// Revoking twice reports honestly rather than claiming success.
	if again, _ := s.RevokeApiToken(row.Id, account); again {
		t.Error("revoking twice reported a second success")
	}
}

// --- admission versus refusal on the wire ---

// Replaces the old assertion that a token gets 401 on the account surfaces.
// It no longer does, because a token now names an `account-`; what refuses it is
// the scope list and the boundary.
func TestAdminSurfacesTakeScopedTokens(t *testing.T) {
	f := hostedCustomer(t)

	scoped := f.mintToken(t, authz.Grant{Scopes: []authz.Capability{authz.ReadOrg}})
	resp := f.withToken(t, scoped, http.MethodGet, "/api/orgs/"+f.orgId+"/members")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("read:hub.org token on its own members: %d, want 200", resp.StatusCode)
	}

	// The installation is not a tenant, so no token reaches it however it is
	// scoped — this is the boundary rule, not a missing verb.
	for _, path := range []string{"/api/admin/accounts", "/api/admin/orgs", "/api/updates"} {
		resp := f.withToken(t, scoped, http.MethodGet, path)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s with a token: %d, want 403", path, resp.StatusCode)
		}
	}

	// A token without the verb is refused on a surface it could otherwise
	// reach, and the refusal says which verb is missing.
	narrow := f.mintToken(t, authz.Grant{Scopes: []authz.Capability{authz.ReadConnector}})
	resp = f.withToken(t, narrow, http.MethodGet, "/api/orgs/"+f.orgId+"/members")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("token without read:hub.org: %d, want 403", resp.StatusCode)
	}

	// An anonymous caller is still unauthenticated rather than unauthorized.
	anon := f.withToken(t, "iagt_not-a-real-token", http.MethodGet, "/api/projects")
	if anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("garbage bearer: %d, want 401", anon.StatusCode)
	}
}

func TestProjectsTakeScopedTokens(t *testing.T) {
	f := hostedCustomer(t)
	device := f.addConnector(t)
	project := attachToProject(t, f.srv.store, f.orgId, device)

	scoped := f.mintToken(t, authz.Grant{Scopes: []authz.Capability{authz.ReadProject}})
	resp := f.withToken(t, scoped, http.MethodGet, "/api/projects")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read:hub.project token listing: %d, want 200", resp.StatusCode)
	}
	var listed []Project
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Id != project {
		t.Fatalf("listed = %+v; want just the org's project", listed)
	}

	// Reading is not deleting, even for a token minted by the org's owner.
	// The project is inside this token's boundary, so the refusal names the
	// verb it lacks rather than pretending the row is not there.
	resp = f.withToken(t, scoped, http.MethodDelete, "/api/projects/"+project)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("delete with a read-only token: %d, want 403", resp.StatusCode)
	}
	if msg := bodyOf(t, resp); !strings.Contains(msg, string(authz.DeleteProject)) {
		t.Errorf("refusal = %q; want it to name %q", msg, authz.DeleteProject)
	}
}

// The refusal has to name the axis that failed. After invalidation every
// operator is re-minting, and a bare 403 tells them nothing about what to
// put in the new token.
func TestRefusalNamesTheMissingScope(t *testing.T) {
	f := hostedCustomer(t)
	device := f.addConnector(t)
	attachToProject(t, f.srv.store, f.orgId, device)

	narrow := f.mintToken(t, authz.Grant{Scopes: []authz.Capability{authz.ReadConnector}})
	resp := f.withToken(t, narrow, http.MethodPost, "/api/connectors/"+device+"/exec")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("exec without the scope: %d, want 403", resp.StatusCode)
	}
	if msg := bodyOf(t, resp); !strings.Contains(msg, string(authz.ExecConnector)) {
		t.Errorf("refusal = %q; want it to name %q", msg, authz.ExecConnector)
	}
}

// --- the boundary axis ---

func TestTokenCannotCrossIntoAnotherProject(t *testing.T) {
	f := hostedCustomer(t)
	mine := f.addConnector(t)
	minePrj := attachToProject(t, f.srv.store, f.orgId, mine)

	theirs, _, err := f.srv.store.CreateConnector("other", "other.local", "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	theirPrj := attachToProject(t, f.srv.store, f.orgId, theirs)

	scoped := f.mintToken(t, authz.Grant{
		Project: minePrj,
		Scopes:  []authz.Capability{authz.ReadConnector, authz.ExecConnector, authz.ReadProject},
	})

	if resp := f.withToken(t, scoped, http.MethodGet, "/api/connectors/"+mine+"/setup"); resp.StatusCode == http.StatusForbidden {
		t.Error("a project-scoped token was refused its own machine")
	}
	resp := f.withToken(t, scoped, http.MethodGet, "/api/connectors/"+theirs+"/setup")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("neighbouring machine: %d, want 403", resp.StatusCode)
	}
	if msg := bodyOf(t, resp); !strings.Contains(msg, "another project") {
		t.Errorf("refusal = %q; want it to name the boundary", msg)
	}

	// Its project list narrows to the one it holds rather than the tenant.
	resp = f.withToken(t, scoped, http.MethodGet, "/api/projects")
	var listed []Project
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Id != minePrj {
		t.Fatalf("listed = %+v; want only %s", listed, minePrj)
	}
	if listed[0].Id == theirPrj {
		t.Error("a project-scoped token listed its neighbour")
	}
}

// A machine attached to nothing has no owner to check against, so nothing
// scoped reaches it. Fail closed rather than treat it as everyone's.
func TestOrphanDeviceIsUnreachableByToken(t *testing.T) {
	f := hostedCustomer(t)
	orphan := f.addConnector(t)

	wide := f.mintToken(t, authz.Grant{Scopes: authz.GrantableScopes()})
	if resp := f.withToken(t, wide, http.MethodGet, "/api/connectors/"+orphan+"/setup"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("orphan machine reached by a token: %d, want 403", resp.StatusCode)
	}

	resp := f.withToken(t, wide, http.MethodGet, "/api/connectors")
	var views []connectorView
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Id == orphan {
			t.Error("an orphan machine was listed to a scoped token")
		}
	}
}

func TestTokenCannotReachAnotherTenant(t *testing.T) {
	f := hostedCustomer(t)
	other, err := f.srv.store.CreateOrg("Other")
	if err != nil {
		t.Fatal(err)
	}
	device, _, err := f.srv.store.CreateConnector("theirs", "theirs.local", "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	hidden := attachToProject(t, f.srv.store, other.Id, device)

	wide := f.mintToken(t, authz.Grant{Scopes: authz.GrantableScopes()})
	if resp := f.withToken(t, wide, http.MethodGet, "/api/connectors/"+device+"/setup"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("another tenant's machine: %d, want 403", resp.StatusCode)
	}
	// Naming the project directly must not widen the reach either.
	if resp := f.withToken(t, wide, http.MethodGet, "/api/tasks/task-1?project="+hidden); resp.StatusCode != http.StatusForbidden {
		t.Errorf("another tenant's gateway: %d, want 403", resp.StatusCode)
	}
}

// --- hardening ---

// A token that can mint a token launders a narrow grant into a wide one, and
// then every scope check above it is decoration.
func TestTokenCannotMintOrRevokeTokens(t *testing.T) {
	f := hostedCustomer(t)
	wide := f.mintToken(t, authz.Grant{Scopes: authz.GrantableScopes()})

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/tokens"},
		{http.MethodPost, "/api/tokens"},
		{http.MethodDelete, "/api/tokens/token-whatever"},
	} {
		resp := f.withToken(t, wide, c.method, c.path)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s with a token: %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}

	// The account behind a credential is off limits for the same reason:
	// a read-only token must not be tradeable for a full session.
	if resp := f.withToken(t, wide, http.MethodPatch, "/api/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("PATCH /api/me with a token: %d, want 401", resp.StatusCode)
	}
}

// Nobody hands out more than they hold, so a token cannot be a way to escape
// your own role.
func TestMintingCannotExceedTheMinter(t *testing.T) {
	f := hostedCustomer(t)
	f.addMember(t, "dev@example.com", "correct-horse-battery-staple", authz.RoleMember)
	dev := f.signIn(t, "dev@example.com", "correct-horse-battery-staple")

	resp := requestJSON(t, f.ts, dev, http.MethodPost, "/api/tokens", map[string]any{
		"name":   "escalate",
		"orgId":  f.orgId,
		"scopes": []string{string(authz.AdminOrg)},
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member minting an admin verb: %d, want 403", resp.StatusCode)
	}
	if msg := bodyOf(t, resp); !strings.Contains(msg, string(authz.AdminOrg)) {
		t.Errorf("refusal = %q; want it to name the verb", msg)
	}

	// What they do hold, they may hand out.
	resp = requestJSON(t, f.ts, dev, http.MethodPost, "/api/tokens", map[string]any{
		"name":   "ci",
		"orgId":  f.orgId,
		"scopes": []string{string(authz.ReadConnector)},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("member minting a read verb: %d, want 201", resp.StatusCode)
	}
}

func TestMintingRejectsBadInput(t *testing.T) {
	f := hostedCustomer(t)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"no name", map[string]any{"scopes": []string{string(authz.ReadConnector)}}, http.StatusBadRequest},
		{"no scopes", map[string]any{"name": "ci"}, http.StatusBadRequest},
		{"unknown scope", map[string]any{"name": "ci", "scopes": []string{"write:hub.invented"}}, http.StatusBadRequest},
		// Installation powers are not grantable at all, so they read as
		// unknown rather than as forbidden.
		{"installation power", map[string]any{"name": "ci", "scopes": []string{string(authz.AdminUpdate)}}, http.StatusBadRequest},
		{"project in another org", map[string]any{
			"name": "ci", "projectId": "project-nope", "scopes": []string{string(authz.ReadConnector)},
		}, http.StatusNotFound},
	}
	for _, c := range cases {
		resp := f.do(t, http.MethodPost, "/api/tokens", c.body)
		if resp.StatusCode != c.want {
			t.Errorf("%s: %d, want %d", c.name, resp.StatusCode, c.want)
		}
	}
}

// A hub claimed before accounts existed has nobody to attribute a token to,
// and the refusal says what to do about it rather than failing obscurely.
func TestLegacyHubCannotMintWithoutAnAccount(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	cred := authz.Credential{Actor: authz.Actor{Platform: true, Unpartitioned: true}}

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"name":"ci"}`))
	if err != nil {
		t.Fatal(err)
	}
	srv.handleCreateApiToken(w, req, cred)
	if w.Code != http.StatusConflict {
		t.Fatalf("legacy mint: %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "claim this hub") {
		t.Errorf("refusal = %q; want it to say how to fix this", w.Body.String())
	}
}
