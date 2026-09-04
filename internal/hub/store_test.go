package hub

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/id"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDeviceLifecycle(t *testing.T) {
	s := testStore(t)
	id, token, err := s.CreateDevice("laptop", "laptop.local", "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.DeviceByToken(token)
	if err != nil || d == nil {
		t.Fatalf("DeviceByToken: %v, %v", d, err)
	}
	if d.Id != id || d.Name != "laptop" {
		t.Errorf("got %+v", d)
	}
	if d2, _ := s.DeviceByToken("wrong-token"); d2 != nil {
		t.Error("wrong token should not authenticate")
	}
	if err := s.DeleteDevice(id); err != nil {
		t.Fatal(err)
	}
	if d3, _ := s.DeviceByToken(token); d3 != nil {
		t.Error("deleted device should not authenticate")
	}
}

func TestEnrollTokenSingleUse(t *testing.T) {
	s := testStore(t)
	token, err := s.CreateEnrollToken(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.ConsumeEnrollToken(token); !ok {
		t.Fatal("first use should succeed")
	}
	if ok, _ := s.ConsumeEnrollToken(token); ok {
		t.Fatal("second use must fail")
	}
	if ok, _ := s.ConsumeEnrollToken("bogus"); ok {
		t.Fatal("bogus token must fail")
	}
}

func TestEnrollTokenExpiry(t *testing.T) {
	s := testStore(t)
	token, err := s.CreateEnrollToken(-time.Second) // already expired
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.ConsumeEnrollToken(token); ok {
		t.Fatal("expired token must fail")
	}
}

func TestAdminAccount(t *testing.T) {
	s := testStore(t)
	n, err := s.CountAccounts()
	if err != nil || n != 0 {
		t.Fatalf("CountAccounts on a fresh store = (%d, %v), want (0, nil)", n, err)
	}

	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	created, org, err := s.ClaimHub("ops@example.com", hash, "Example Ops")
	if err != nil {
		t.Fatalf("ClaimHub: %v", err)
	}
	if !strings.HasPrefix(created.Id, string(id.Account)+id.Separator) {
		t.Errorf("account id %q does not carry the acc- prefix", created.Id)
	}
	if !created.IsAdmin {
		t.Error("the first account is the platform admin")
	}
	if !strings.HasPrefix(org.Id, string(id.Org)+id.Separator) {
		t.Errorf("org id %q does not carry the org- prefix", org.Id)
	}

	found, err := s.AccountByEmail("ops@example.com")
	if err != nil || found == nil {
		t.Fatalf("AccountByEmail = (%v, %v), want the account", found, err)
	}
	if found.Id != created.Id || !found.IsAdmin {
		t.Errorf("read back %+v, want %+v", found, created)
	}
	if !found.VerifyPassword("correct-horse-battery") {
		t.Error("stored hash does not verify the password it was built from")
	}
	if found.VerifyPassword("something-else") {
		t.Error("a wrong password verified")
	}

	missing, err := s.AccountByEmail("nobody@example.com")
	if err != nil || missing != nil {
		t.Errorf("AccountByEmail for an unknown address = (%v, %v), want (nil, nil)", missing, err)
	}
}

// The database, not the handler, is what keeps a hub to one owner. Two
// concurrent claims both pass the in-memory checks; only the partial unique
// index stops the second from creating a second admin.
func TestOnlyOneAdminAccount(t *testing.T) {
	s := testStore(t)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimHub("first@example.com", hash, "First"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimHub("second@example.com", hash, "Second"); err == nil {
		t.Fatal("a second admin account was created; the hub now has two owners")
	}
	n, err := s.CountAccounts()
	if err != nil || n != 1 {
		t.Fatalf("CountAccounts = (%d, %v), want (1, nil)", n, err)
	}
	// The refused claim must not leave its organization behind: the whole
	// point of doing the three inserts in one transaction.
	orgs, err := s.ListOrgs()
	if err != nil || len(orgs) != 1 {
		t.Fatalf("ListOrgs after a refused claim = (%v, %v), want exactly one", orgs, err)
	}
	if orgs[0].Name != "First" {
		t.Errorf("surviving org is %q; want the one from the successful claim", orgs[0].Name)
	}
}

func TestAccountEmailIsUnique(t *testing.T) {
	s := testStore(t)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimHub("ops@example.com", hash, "Ops"); err != nil {
		t.Fatal(err)
	}
	// Same address, and this time not as an admin, so only the email
	// constraint can refuse it.
	if _, err := s.db.Exec(`INSERT INTO accounts (id, email, password_hash, is_admin, created_at)
		VALUES (?, ?, ?, 0, 0)`, "acc-duplicate", "ops@example.com", hash); err == nil {
		t.Fatal("two accounts share one email address")
	}
}

// Claiming a hub has to leave a hub whose own rule holds: an account, an
// organization, and the membership that makes the operator its owner. Any two
// of the three is a state no screen can repair.
func TestClaimHubMintsTheFirstOrgAndOwner(t *testing.T) {
	s := testStore(t)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	account, org, err := s.ClaimHub("ops@example.com", hash, "Example Ops")
	if err != nil {
		t.Fatal(err)
	}

	orgs, err := s.ListOrgs()
	if err != nil || len(orgs) != 1 {
		t.Fatalf("ListOrgs = (%v, %v), want one org", orgs, err)
	}
	if orgs[0].Id != org.Id || orgs[0].Name != "Example Ops" || orgs[0].Members != 1 {
		t.Errorf("ListOrgs[0] = %+v, want %q with one member", orgs[0], "Example Ops")
	}

	members, err := s.ListOrgMembers(org.Id)
	if err != nil || len(members) != 1 {
		t.Fatalf("ListOrgMembers = (%v, %v), want one member", members, err)
	}
	if members[0].AccountId != account.Id || members[0].Role != string(authz.RoleOwner) {
		t.Errorf("member = %+v, want the claiming account as owner", members[0])
	}
	if members[0].Email != "ops@example.com" {
		t.Errorf("member email = %q, want the account's address", members[0].Email)
	}

	roles, err := s.AccountOrgRoles(account.Id)
	if err != nil || roles[org.Id] != authz.RoleOwner {
		t.Errorf("AccountOrgRoles = (%v, %v), want owner of the first org", roles, err)
	}
	mine, err := s.ListAccountOrgs(account.Id)
	if err != nil || len(mine) != 1 || mine[0].Name != "Example Ops" {
		t.Errorf("ListAccountOrgs = (%v, %v), want the first org by name", mine, err)
	}
}

func TestOrgMembership(t *testing.T) {
	s := testStore(t)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	owner, org, err := s.ClaimHub("ops@example.com", hash, "Example Ops")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateAccount("dev@example.com", hash)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if second.IsAdmin {
		t.Error("an account created after the claim must not be the platform admin")
	}

	if err := s.AddOrgMember(org.Id, second.Id, authz.RoleMember); err != nil {
		t.Fatalf("AddOrgMember: %v", err)
	}
	// The composite primary key is what makes "already a member" a refusal
	// rather than a duplicate row with two different roles.
	if err := s.AddOrgMember(org.Id, second.Id, authz.RoleAdmin); err == nil {
		t.Error("the same account joined one org twice")
	}

	roster, err := s.OrgRoster(org.Id)
	if err != nil {
		t.Fatal(err)
	}
	if roster.Members[owner.Id] != authz.RoleOwner || roster.Members[second.Id] != authz.RoleMember {
		t.Errorf("roster = %+v, want owner and member", roster.Members)
	}
	if roster.Owners() != 1 {
		t.Errorf("Owners() = %d, want 1", roster.Owners())
	}

	if err := s.SetOrgMemberRole(org.Id, second.Id, authz.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	roster, _ = s.OrgRoster(org.Id)
	if roster.Members[second.Id] != authz.RoleAdmin {
		t.Errorf("role after promotion = %q, want admin", roster.Members[second.Id])
	}

	if err := s.RemoveOrgMember(org.Id, second.Id); err != nil {
		t.Fatal(err)
	}
	roster, _ = s.OrgRoster(org.Id)
	if _, still := roster.Members[second.Id]; still {
		t.Error("member survived removal")
	}
	// Leaving an org is not leaving the hub: the account is still there, and
	// on a hosted hub it may still belong to somebody else's org.
	if a, err := s.AccountById(second.Id); err != nil || a == nil {
		t.Errorf("AccountById after removal = (%v, %v), want the account to survive", a, err)
	}
}

func TestListAccountsAndLookups(t *testing.T) {
	s := testStore(t)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := s.ClaimHub("ops@example.com", hash, "Ops")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAccount("dev@example.com", hash); err != nil {
		t.Fatal(err)
	}

	accounts, err := s.ListAccounts()
	if err != nil || len(accounts) != 2 {
		t.Fatalf("ListAccounts = (%v, %v), want two", accounts, err)
	}
	// The list is written straight to JSON, so it must not be able to carry a
	// credential even if somebody adds a field later.
	for _, a := range accounts {
		if a.passwordHash != "" {
			t.Errorf("account %s carries a password hash into the list view", a.Id)
		}
	}

	found, err := s.AccountById(admin.Id)
	if err != nil || found == nil || found.Email != "ops@example.com" {
		t.Errorf("AccountById = (%v, %v), want the admin", found, err)
	}
	missing, err := s.AccountById("acc-nobody")
	if err != nil || missing != nil {
		t.Errorf("AccountById for an unknown id = (%v, %v), want (nil, nil)", missing, err)
	}

	if err := s.RenameOrg(mustOrgId(t, s), "Renamed"); err != nil {
		t.Fatal(err)
	}
	orgs, _ := s.ListOrgs()
	if orgs[0].Name != "Renamed" {
		t.Errorf("org name after rename = %q, want Renamed", orgs[0].Name)
	}
	one, err := s.OrgById(orgs[0].Id)
	if err != nil || one == nil || one.Members != 1 {
		t.Errorf("OrgById = (%v, %v), want the org with one member", one, err)
	}
	if gone, err := s.OrgById("org-nobody"); err != nil || gone != nil {
		t.Errorf("OrgById for an unknown id = (%v, %v), want (nil, nil)", gone, err)
	}
}

// `v0.2.0` shipped claiming before organizations, so a hub claimed on that
// image has an owner and nothing to own. First-run never runs twice, which
// makes start the only place left to repair it.
func TestBackfillOperatorOrg(t *testing.T) {
	s := testStore(t)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}

	// A hub with no accounts at all — the legacy operator password — gets
	// nothing, because an organization needs an owner.
	if org, err := s.BackfillOperatorOrg(); err != nil || org != nil {
		t.Fatalf("BackfillOperatorOrg on an accountless hub = (%v, %v), want (nil, nil)", org, err)
	}

	// Reproduce the old shape: an admin account inserted without an org.
	admin, err := s.CreateAccount("ops@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE accounts SET is_admin = 1 WHERE id = ?`, admin.Id); err != nil {
		t.Fatal(err)
	}

	org, err := s.BackfillOperatorOrg()
	if err != nil || org == nil {
		t.Fatalf("BackfillOperatorOrg = (%v, %v), want an org", org, err)
	}
	if org.Name != auth.DefaultOrgName {
		t.Errorf("org name = %q, want %q", org.Name, auth.DefaultOrgName)
	}
	roster, err := s.OrgRoster(org.Id)
	if err != nil || roster.Members[admin.Id] != authz.RoleOwner {
		t.Fatalf("roster = (%v, %v), want the admin as owner", roster.Members, err)
	}

	// Idempotent: the next start must not mint a second org.
	if again, err := s.BackfillOperatorOrg(); err != nil || again != nil {
		t.Fatalf("second BackfillOperatorOrg = (%v, %v), want (nil, nil)", again, err)
	}
	if orgs, _ := s.ListOrgs(); len(orgs) != 1 {
		t.Errorf("orgs after two runs = %d, want 1", len(orgs))
	}
}

func mustOrgId(t *testing.T, s *Store) string {
	t.Helper()
	orgs, err := s.ListOrgs()
	if err != nil || len(orgs) == 0 {
		t.Fatalf("ListOrgs = (%v, %v), want at least one", orgs, err)
	}
	return orgs[0].Id
}

func TestApiTokens(t *testing.T) {
	s := testStore(t)
	token, err := s.CreateApiToken("ci")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.ValidApiToken(token); !ok {
		t.Error("fresh token should validate")
	}
	tokens, _ := s.ListApiTokens()
	if len(tokens) != 1 {
		t.Fatalf("got %d tokens", len(tokens))
	}
	s.DeleteApiToken(tokens[0].Id)
	if ok, _ := s.ValidApiToken(token); ok {
		t.Error("revoked token should fail")
	}
}

func TestPresetsSeeded(t *testing.T) {
	s := testStore(t)
	presets, err := s.ListPresets()
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) < 3 {
		t.Errorf("expected seeded presets, got %d", len(presets))
	}
}

func TestProjectLifecycle(t *testing.T) {
	s := testStore(t)
	deviceId, _, err := s.CreateDevice("studio", "studio.local", "darwin", "arm64", false)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject("Storefront", deviceId, "/Users/dev/storefront")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Storefront" || p.DeviceId != deviceId {
		t.Fatalf("unexpected project: %+v", p)
	}
	projects, err := s.ListProjects()
	if err != nil || len(projects) != 1 {
		t.Fatalf("ListProjects: %+v, %v", projects, err)
	}
	updated, err := s.UpdateProject(p.Id, "Web store", deviceId, "/Users/dev/web-store")
	if err != nil || updated == nil || updated.Path != "/Users/dev/web-store" {
		t.Fatalf("UpdateProject: %+v, %v", updated, err)
	}
	if err := s.DeleteProject(p.Id); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ProjectById(p.Id); got != nil {
		t.Fatal("deleted project still exists")
	}
}

func TestDeletingDeviceDeletesItsProjects(t *testing.T) {
	s := testStore(t)
	deviceId, _, _ := s.CreateDevice("runner", "runner", "linux", "amd64", false)
	if _, err := s.CreateProject("API", deviceId, "/srv/api"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDevice(deviceId); err != nil {
		t.Fatal(err)
	}
	projects, err := s.ListProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("projects left after deleting device: %+v, %v", projects, err)
	}
}
