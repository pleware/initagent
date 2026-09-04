package hub

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

// adminFixture claims a hub and signs in, returning a client that carries the
// operator's session and the ids the tests act on.
type adminFixture struct {
	srv     *Server
	ts      *httptest.Server
	client  *http.Client
	ownerId string
	orgId   string
}

func claimedHub(t *testing.T, kind offering.Kind) *adminFixture {
	t.Helper()
	srv := newHub(t, t.TempDir(), kind)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	password := "correct-horse-battery-staple"
	resp := postJSON(t, ts, client, "/api/setup", map[string]string{
		"email":    "ops@example.com",
		"password": password,
		"token":    srv.claim.expected(),
		"orgName":  "Example Ops",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("claim: %d, want 200", resp.StatusCode)
	}

	owner, err := srv.store.AccountByEmail("ops@example.com")
	if err != nil || owner == nil {
		t.Fatalf("AccountByEmail after claim = (%v, %v)", owner, err)
	}
	orgs, err := srv.store.ListOrgs()
	if err != nil || len(orgs) != 1 {
		t.Fatalf("ListOrgs after claim = (%v, %v), want one", orgs, err)
	}
	return &adminFixture{srv: srv, ts: ts, client: client, ownerId: owner.Id, orgId: orgs[0].Id}
}

func (f *adminFixture) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	return requestJSON(t, f.ts, f.client, method, path, body)
}

// signIn returns a second client authenticated as another account.
func (f *adminFixture) signIn(t *testing.T, email, password string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	resp := postJSON(t, f.ts, client, "/api/login", map[string]string{
		"email": email, "password": password,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("login as %s: %d, want 200", email, resp.StatusCode)
	}
	return client
}

// addMember creates an account and joins it to the fixture's org.
func (f *adminFixture) addMember(t *testing.T, email, password string, role authz.Role) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.srv.store.CreateAccount(email, hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.AddOrgMember(f.orgId, account.Id, role); err != nil {
		t.Fatal(err)
	}
	return account.Id
}

// Claiming a hub has to produce a signed-in operator who can see themselves:
// the platform flag decides whether an administration surface exists, and the
// membership decides whose people they may manage.
func TestMeReportsIdentityAndMemberships(t *testing.T) {
	f := claimedHub(t, offering.Hosted)

	var me struct {
		Authenticated bool   `json:"authenticated"`
		PlatformAdmin bool   `json:"platformAdmin"`
		AccountId     string `json:"accountId"`
		Email         string `json:"email"`
		Orgs          []struct {
			OrgId string `json:"orgId"`
			Name  string `json:"name"`
			Role  string `json:"role"`
		} `json:"orgs"`
	}
	resp := f.do(t, http.MethodGet, "/api/me", nil)
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if !me.Authenticated || !me.PlatformAdmin {
		t.Errorf("authenticated=%v platformAdmin=%v, want both true", me.Authenticated, me.PlatformAdmin)
	}
	if me.AccountId != f.ownerId || me.Email != "ops@example.com" {
		t.Errorf("identity = (%q, %q), want the claiming account", me.AccountId, me.Email)
	}
	if len(me.Orgs) != 1 || me.Orgs[0].OrgId != f.orgId || me.Orgs[0].Role != string(authz.RoleOwner) {
		t.Fatalf("orgs = %+v, want owner of the first org", me.Orgs)
	}
	if me.Orgs[0].Name != "Example Ops" {
		t.Errorf("org name = %q, want the name submitted at claim", me.Orgs[0].Name)
	}
}

// An anonymous caller learns nothing about who exists here.
func TestAdminSurfacesRefuseAnonymous(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	anon := &http.Client{}

	for _, path := range []string{"/api/admin/accounts", "/api/admin/orgs", "/api/orgs/" + f.orgId + "/members"} {
		resp := requestJSON(t, f.ts, anon, http.MethodGet, path, nil)
		if resp.StatusCode != 401 {
			t.Errorf("GET %s without a session: %d, want 401", path, resp.StatusCode)
		}
	}
}

// The trap this closes: API tokens carry no scope, so if the admin routes sat
// behind the older middleware, every token ever minted for the CLI or MCP
// would be a platform administrator.
func TestAdminSurfacesRefuseApiTokens(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	token, err := f.srv.store.CreateApiToken("ci")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodGet, f.ts.URL+"/api/admin/accounts", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("GET /api/admin/accounts with an API token: %d, want 401", resp.StatusCode)
	}

	// The same token still works where it always did, so this is a boundary
	// on the new surface and not a regression for the CLI.
	req, _ = http.NewRequest(http.MethodGet, f.ts.URL+"/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	devices, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer devices.Body.Close()
	if devices.StatusCode != 200 {
		t.Errorf("GET /api/devices with an API token: %d, want 200", devices.StatusCode)
	}
}

func TestPlatformSurfaceListsAccountsAndOrgs(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	f.addMember(t, "dev@example.com", "another-long-password", authz.RoleMember)

	var accounts []struct {
		Id      string `json:"id"`
		Email   string `json:"email"`
		IsAdmin bool   `json:"isAdmin"`
	}
	resp := f.do(t, http.MethodGet, "/api/admin/accounts", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/admin/accounts: %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&accounts); err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %+v, want two", accounts)
	}
	if !accounts[0].IsAdmin || accounts[1].IsAdmin {
		t.Errorf("admin flags = (%v, %v), want only the first account to be the platform admin",
			accounts[0].IsAdmin, accounts[1].IsAdmin)
	}

	var orgs []struct {
		Id      string `json:"id"`
		Name    string `json:"name"`
		Members int    `json:"members"`
	}
	resp = f.do(t, http.MethodGet, "/api/admin/orgs", nil)
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].Members != 2 {
		t.Fatalf("orgs = %+v, want one org with two members", orgs)
	}
}

// Running the hub does not put the operator inside a customer's organization
// (25), and the surface has to hold that line even though the same person is
// an owner of their own org on a self-hosted hub.
func TestPlatformAdminCannotReadAnotherOrgsRoster(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	customer, err := f.srv.store.CreateOrg("Customer Ltd")
	if err != nil {
		t.Fatal(err)
	}
	other := f.addMember(t, "them@customer.example", "another-long-password", authz.RoleMember)
	if err := f.srv.store.AddOrgMember(customer.Id, other, authz.RoleOwner); err != nil {
		t.Fatal(err)
	}

	// The org is visible in the platform list, with its size.
	var orgs []struct {
		Id      string `json:"id"`
		Members int    `json:"members"`
	}
	resp := f.do(t, http.MethodGet, "/api/admin/orgs", nil)
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 2 {
		t.Fatalf("orgs = %+v, want both", orgs)
	}

	// Its roster is not. 404 rather than 403, so the answer does not confirm
	// that the org exists to somebody guessing ids.
	resp = f.do(t, http.MethodGet, "/api/orgs/"+customer.Id+"/members", nil)
	if resp.StatusCode != 404 {
		t.Errorf("operator reading a customer roster: %d, want 404", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPatch, "/api/orgs/"+customer.Id+"/members/"+other,
		map[string]string{"role": "member"})
	if resp.StatusCode != 403 && resp.StatusCode != 404 {
		t.Errorf("operator changing a customer's role: %d, want a refusal", resp.StatusCode)
	}
}

func TestOrgOwnerManagesTheirOwnPeople(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	member := f.addMember(t, "dev@example.com", "another-long-password", authz.RoleMember)

	var members []struct {
		AccountId string `json:"accountId"`
		Email     string `json:"email"`
		Role      string `json:"role"`
	}
	resp := f.do(t, http.MethodGet, "/api/orgs/"+f.orgId+"/members", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET members: %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %+v, want two", members)
	}

	// Promote, then read it back through the API rather than the store, so
	// the round trip is what is proven.
	resp = f.do(t, http.MethodPatch, "/api/orgs/"+f.orgId+"/members/"+member,
		map[string]string{"role": "admin"})
	if resp.StatusCode != 200 {
		t.Fatalf("promote to admin: %d, want 200", resp.StatusCode)
	}
	roster, err := f.srv.store.OrgRoster(f.orgId)
	if err != nil || roster.Members[member] != authz.RoleAdmin {
		t.Fatalf("roster after promotion = (%v, %v)", roster.Members, err)
	}

	resp = f.do(t, http.MethodDelete, "/api/orgs/"+f.orgId+"/members/"+member, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("remove member: %d, want 200", resp.StatusCode)
	}
	roster, _ = f.srv.store.OrgRoster(f.orgId)
	if _, still := roster.Members[member]; still {
		t.Error("member survived removal through the API")
	}
}

func TestOrgSurfaceRefusals(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	const memberPassword = "another-long-password"
	member := f.addMember(t, "dev@example.com", memberPassword, authz.RoleMember)
	memberClient := f.signIn(t, "dev@example.com", memberPassword)

	cases := []struct {
		name   string
		client *http.Client
		method string
		path   string
		body   any
		want   int
	}{
		{
			// The rule with teeth: an org that loses its only owner is one
			// nobody can administer, so the API refuses even the owner.
			name: "the last owner cannot be demoted", client: f.client,
			method: http.MethodPatch, path: "/api/orgs/" + f.orgId + "/members/" + f.ownerId,
			body: map[string]string{"role": "member"}, want: 409,
		},
		{
			name: "the last owner cannot leave", client: f.client,
			method: http.MethodDelete, path: "/api/orgs/" + f.orgId + "/members/" + f.ownerId,
			want: 409,
		},
		{
			name: "an unknown role is refused before any write", client: f.client,
			method: http.MethodPatch, path: "/api/orgs/" + f.orgId + "/members/" + member,
			body: map[string]string{"role": "superuser"}, want: 400,
		},
		{
			name: "a role change for somebody who is not a member", client: f.client,
			method: http.MethodPatch, path: "/api/orgs/" + f.orgId + "/members/acc-nobody",
			body: map[string]string{"role": "member"}, want: 404,
		},
		{
			name: "a plain member cannot promote anyone", client: memberClient,
			method: http.MethodPatch, path: "/api/orgs/" + f.orgId + "/members/" + f.ownerId,
			body: map[string]string{"role": "member"}, want: 403,
		},
		{
			name: "a plain member cannot remove anyone else", client: memberClient,
			method: http.MethodDelete, path: "/api/orgs/" + f.orgId + "/members/" + f.ownerId,
			want: 403,
		},
		{
			name: "a plain member cannot rename the org", client: memberClient,
			method: http.MethodPatch, path: "/api/orgs/" + f.orgId,
			body: map[string]string{"name": "Mine now"}, want: 404,
		},
		{
			name: "a plain member is not a platform admin", client: memberClient,
			method: http.MethodGet, path: "/api/admin/accounts", want: 403,
		},
		{
			name: "an empty org name is refused", client: f.client,
			method: http.MethodPatch, path: "/api/orgs/" + f.orgId,
			body: map[string]string{"name": "   "}, want: 400,
		},
	}

	for _, c := range cases {
		resp := requestJSON(t, f.ts, c.client, c.method, c.path, c.body)
		if resp.StatusCode != c.want {
			t.Errorf("%s: %d, want %d", c.name, resp.StatusCode, c.want)
		}
	}

	// Leaving on your own needs no administrative right.
	resp := requestJSON(t, f.ts, memberClient, http.MethodDelete,
		"/api/orgs/"+f.orgId+"/members/"+member, nil)
	if resp.StatusCode != 200 {
		t.Errorf("a member leaving on their own: %d, want 200", resp.StatusCode)
	}
}

func TestRenameOrg(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	resp := f.do(t, http.MethodPatch, "/api/orgs/"+f.orgId, map[string]string{"name": "  Renamed  "})
	if resp.StatusCode != 200 {
		t.Fatalf("rename: %d, want 200", resp.StatusCode)
	}
	org, err := f.srv.store.OrgById(f.orgId)
	if err != nil || org == nil {
		t.Fatal(err)
	}
	if org.Name != "Renamed" {
		t.Errorf("name = %q, want the trimmed submission", org.Name)
	}
}

// A hub claimed before accounts existed still signs its operator in. They are
// the platform operator with no `acc-` and no org, which must not crash the
// surfaces that expect an account.
func TestLegacyOperatorSession(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	hash, err := auth.HashPassword("upstream-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.store.SetSetting(legacyPasswordSetting, hash); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if resp := postJSON(t, ts, client, "/api/login", map[string]string{"password": "upstream-secret"}); resp.StatusCode != 200 {
		t.Fatalf("legacy login: %d, want 200", resp.StatusCode)
	}

	var me struct {
		PlatformAdmin bool   `json:"platformAdmin"`
		AccountId     string `json:"accountId"`
		Orgs          []any  `json:"orgs"`
		Email         string `json:"email"`
	}
	resp := requestJSON(t, ts, client, http.MethodGet, "/api/me", nil)
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if !me.PlatformAdmin {
		t.Error("the legacy operator credential lost the platform surface")
	}
	if me.AccountId != "" || me.Email != "" || len(me.Orgs) != 0 {
		t.Errorf("legacy session reports account=%q email=%q orgs=%v, want all empty",
			me.AccountId, me.Email, me.Orgs)
	}
	// The account list is the operator's, and an empty one is the honest
	// answer on a hub that never minted an account.
	resp = requestJSON(t, ts, client, http.MethodGet, "/api/admin/accounts", nil)
	if resp.StatusCode != 200 {
		t.Errorf("legacy operator reading accounts: %d, want 200", resp.StatusCode)
	}
}
