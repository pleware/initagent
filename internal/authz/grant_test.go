package authz

import (
	"errors"
	"slices"
	"testing"
)

func member(org string, r Role) Actor {
	return Actor{Account: "account-1", Orgs: map[string]Role{org: r}}
}

// A session carries no grant, so the role is the whole answer. This is the
// path the cockpit takes, and it must not change shape when tokens gain axes.
func TestSessionIsRoleOnly(t *testing.T) {
	session := Credential{Actor: member("org-1", RoleAdmin)}

	if session.Scoped() {
		t.Error("a session reported itself as scoped")
	}
	for _, c := range []Capability{ReadProject, CreateProject, ExecConnector, AttachTerminal} {
		if !session.Can(c, "org-1", "project-1") {
			t.Errorf("admin session refused %q", c)
		}
	}
	if session.Can(DeleteOrg, "org-1", "") {
		t.Error("admin session was allowed to delete the org")
	}
	if session.Can(ReadProject, "org-other", "") {
		t.Error("session reached an org it does not belong to")
	}
}

// The property the whole design rests on: a token is the intersection of what
// its author may do and what the row lists. Neither half alone is enough.
func TestTokenIsTheIntersection(t *testing.T) {
	const org = "org-1"

	cases := []struct {
		name  string
		role  Role
		scope []Capability
		cap   Capability
		want  bool
	}{
		{"role and scope agree", RoleAdmin, []Capability{CreateProject}, CreateProject, true},
		{"scope without the role", RoleMember, []Capability{CreateProject}, CreateProject, false},
		{"role without the scope", RoleAdmin, []Capability{ReadProject}, CreateProject, false},
		{"neither", RoleMember, []Capability{ReadProject}, DeleteOrg, false},
		{"empty scope set grants nothing", RoleOwner, nil, ReadProject, false},
		{"exec must be asked for by name", RoleOwner, []Capability{ReadConnector}, ExecConnector, false},
		{"exec when named", RoleOwner, []Capability{ExecConnector}, ExecConnector, true},
	}
	for _, c := range cases {
		cred := Credential{
			Actor: member(org, c.role),
			Grant: &Grant{Org: org, Scopes: c.scope},
		}
		if got := cred.Can(c.cap, org, "project-1"); got != c.want {
			t.Errorf("%s: Can(%q) = %v; want %v", c.name, c.cap, got, c.want)
		}
		if !cred.Scoped() {
			t.Errorf("%s: token did not report itself as scoped", c.name)
		}
	}
}

// Losing the membership must be enough. If a token outlived it, revoking a
// person's access would mean hunting down every secret they ever minted.
func TestTokenDiesWithTheMembership(t *testing.T) {
	stranger := Actor{Account: "account-1"} // removed from every org
	cred := Credential{
		Actor: stranger,
		Grant: &Grant{Org: "org-1", Scopes: []Capability{ReadProject, ExecConnector}},
	}
	for _, c := range []Capability{ReadProject, ExecConnector} {
		if cred.Can(c, "org-1", "project-1") {
			t.Errorf("%q survived the loss of membership", c)
		}
	}
}

func TestBoundary(t *testing.T) {
	const org = "org-1"
	all := []Capability{ReadProject, ReadTask}

	cases := []struct {
		name          string
		grantOrg      string
		grantProject  string
		targetOrg     string
		targetProject string
		want          bool
	}{
		{"tenant token inside its org", org, "", org, "project-1", true},
		{"tenant token, any project", org, "", org, "project-9", true},
		{"tenant token, org-wide target", org, "", org, "", true},
		{"another tenant", org, "", "org-2", "project-1", false},
		{"project token on its project", org, "project-1", org, "project-1", true},
		{"project token on another project", org, "project-1", org, "project-2", false},
		{"project token asked to act org-wide", org, "project-1", org, "", false},
		{"project token in another tenant", org, "project-1", "org-2", "project-1", false},
		// A token's boundary is a project or a tenant (09). Running the
		// installation is not something a machine secret can reach.
		{"tenant token at the installation", org, "", "", "", false},
		{"project token at the installation", org, "project-1", "", "", false},
		// A zero Grant must reach nothing rather than everything.
		{"zero grant", "", "", org, "project-1", false},
		{"zero grant, org-wide", "", "", org, "", false},
	}
	for _, c := range cases {
		cred := Credential{
			// Owner everywhere, so only the boundary can refuse.
			Actor: Actor{Account: "account-1", Platform: true, Orgs: map[string]Role{
				org: RoleOwner, "org-2": RoleOwner,
			}},
			Grant: &Grant{Org: c.grantOrg, Project: c.grantProject, Scopes: all},
		}
		if got := cred.Can(ReadProject, c.targetOrg, c.targetProject); got != c.want {
			t.Errorf("%s: Can(ReadProject, %q, %q) = %v; want %v",
				c.name, c.targetOrg, c.targetProject, got, c.want)
		}
	}
}

// The installation capabilities are reachable by a session and by no token,
// which is what keeps a leaked machine secret out of hub administration.
func TestInstallationStaysWithThePerson(t *testing.T) {
	operator := Actor{Account: "account-1", Platform: true}

	session := Credential{Actor: operator}
	if !session.Can(AdminAccounts, "", "") {
		t.Error("operator session lost account administration")
	}
	if !session.Can(AdminUpdate, "", "") {
		t.Error("operator session lost update administration")
	}

	// Even a grant that names the capability cannot reach the installation,
	// because the boundary check refuses an empty org outright.
	token := Credential{
		Actor: operator,
		Grant: &Grant{Org: "org-1", Scopes: []Capability{AdminAccounts, AdminUpdate, ReadOrg}},
	}
	if token.Can(AdminAccounts, "", "") {
		t.Error("a token administered hub accounts")
	}
	if token.Can(AdminUpdate, "", "") {
		t.Error("a token administered hub updates")
	}
	// ReadOrg is grantable, so this is the boundary refusing rather than the
	// verb: a token may read its own org, never enumerate the installation.
	if token.Can(ReadOrg, "", "") {
		t.Error("a token enumerated the installation's organizations")
	}
}

func TestParseScopes(t *testing.T) {
	got, err := ParseScopes("read:project.task create:project.task")
	if err != nil {
		t.Fatalf("ParseScopes: %v", err)
	}
	want := []Capability{CreateTask, ReadTask} // sorted
	if !slices.Equal(got, want) {
		t.Errorf("ParseScopes = %v; want %v", got, want)
	}

	// Storage is normalised on the way in, so a hand-edited row with repeats
	// or odd spacing still has one meaning.
	got, err = ParseScopes("  read:project.task   read:project.task ")
	if err != nil {
		t.Fatalf("ParseScopes with repeats: %v", err)
	}
	if !slices.Equal(got, []Capability{ReadTask}) {
		t.Errorf("ParseScopes deduplicated = %v; want [read:project.task]", got)
	}

	if got, err := ParseScopes(""); err != nil || got != nil {
		t.Errorf("ParseScopes(\"\") = %v, %v; want nil, nil", got, err)
	}
	if got, err := ParseScopes("   "); err != nil || got != nil {
		t.Errorf("ParseScopes(blank) = %v, %v; want nil, nil", got, err)
	}

	// Refused rather than dropped: a row must not authorise something other
	// than what it reads as.
	for _, bad := range []string{
		"read:project.task write:hub.invented",
		"admin:hub.account",  // installation-only, never grantable
		"admin:hub.update",   // same
		"Read:project.task",  // case matters
		"read:project.tasks", // typo
	} {
		if _, err := ParseScopes(bad); !errors.Is(err, ErrScopeUnknown) {
			t.Errorf("ParseScopes(%q) error = %v; want ErrScopeUnknown", bad, err)
		}
	}
}

func TestFormatScopes(t *testing.T) {
	if got := FormatScopes(nil); got != "" {
		t.Errorf("FormatScopes(nil) = %q; want empty", got)
	}
	got := FormatScopes([]Capability{ReadTask, CreateTask, ReadTask})
	if got != "create:project.task read:project.task" {
		t.Errorf("FormatScopes = %q; want sorted and deduplicated", got)
	}
}

// Storage round-trips, or a token means something different after a restart.
func TestScopesRoundTrip(t *testing.T) {
	in := GrantableScopes()
	back, err := ParseScopes(FormatScopes(in))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if !slices.Equal(back, in) {
		t.Errorf("round trip = %v; want %v", back, in)
	}
}

// The picker, the parser and the enforcement all read the same lists, so a
// capability cannot be grantable but unenforced, or enforced but unpickable.
func TestRegistriesAgreeWithEnforcement(t *testing.T) {
	all := Capabilities()
	if len(all) == 0 {
		t.Fatal("Capabilities() is empty")
	}
	if !slices.IsSorted(all) {
		t.Error("Capabilities() is not sorted")
	}

	grantable := GrantableScopes()
	if !slices.IsSorted(grantable) {
		t.Error("GrantableScopes() is not sorted")
	}

	for _, c := range grantable {
		if !slices.Contains(all, c) {
			t.Errorf("%q is grantable but not a known capability", c)
		}
		// Grantable means "means something inside an org", which is exactly
		// having a role floor. A capability that is installation-only has
		// none, so it cannot reach the picker.
		if _, ok := orgMinimum[c]; !ok {
			t.Errorf("%q is grantable with no role floor", c)
		}
	}

	// ReadOrg is deliberately in both maps: at the installation it enumerates
	// organizations, inside one it reads that organization. Being grantable
	// is therefore correct, and the boundary check is what stops a token
	// using it hub-wide (see TestInstallationStaysWithThePerson).
	if !slices.Contains(grantable, ReadOrg) || !installation[ReadOrg] {
		t.Error("ReadOrg should be both grantable and an installation power")
	}

	// Every capability the hub knows is either an installation power or has a
	// role floor. One that is neither can never be exercised at all, which
	// would be a constant nothing enforces.
	for _, c := range all {
		_, org := orgMinimum[c]
		if !installation[c] && !org {
			t.Errorf("%q is neither an installation power nor an org capability", c)
		}
	}
}

// A hub claimed before accounts existed has no org, so its operator has no
// membership to check a fleet capability against. Refusing would lock the
// installation's only administrator out of their own machines.
func TestUnpartitionedOperatorHoldsTheFleet(t *testing.T) {
	legacy := Credential{Actor: Actor{Platform: true, Unpartitioned: true}}
	for _, c := range Capabilities() {
		if !legacy.Can(c, "", "") {
			t.Errorf("legacy operator refused %q on a hub with no orgs", c)
		}
	}
	// Projects on such a hub carry an empty org_id, which is the boundary
	// these requests arrive with.
	if !legacy.Can(ReadConnector, "", "project-1") {
		t.Error("legacy operator refused a project with no org")
	}

	// The flag is a fact about the installation, not a power. Once an org
	// exists the ordinary rules apply, and a platform admin is still not a
	// member of a customer's org.
	partitioned := Credential{Actor: Actor{Platform: true}}
	if partitioned.Can(ReadConnector, "org-1", "project-1") {
		t.Error("platform admin reached an org's fleet without membership")
	}

	// It cannot be a back door for a non-operator either.
	impostor := Credential{Actor: Actor{Account: "account-9", Unpartitioned: true}}
	if impostor.Can(ReadConnector, "org-1", "project-1") {
		t.Error("a non-operator was let in by the unpartitioned flag")
	}

	// And a token is still bounded, because a legacy hub can still mint one
	// once it has an org — the grant does the refusing, not the role.
	token := Credential{
		Actor: Actor{Platform: true, Unpartitioned: true},
		Grant: &Grant{Org: "org-1", Scopes: []Capability{ReadConnector}},
	}
	if token.Can(ReadConnector, "org-2", "project-1") {
		t.Error("a token on a legacy hub crossed into another org")
	}
	if token.Can(ExecConnector, "org-1", "project-1") {
		t.Error("a token on a legacy hub exceeded its scope list")
	}
}

func TestDangerousIsExecOnly(t *testing.T) {
	if !Dangerous(ExecConnector) {
		t.Error("exec:fleet.connector is not marked dangerous")
	}
	for _, c := range GrantableScopes() {
		if c == ExecConnector {
			continue
		}
		if Dangerous(c) {
			t.Errorf("%q is marked dangerous; only exec should be", c)
		}
	}
}
