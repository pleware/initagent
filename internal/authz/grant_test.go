package authz

import (
	"errors"
	"slices"
	"testing"
)

func member(org string, r Role) Requester {
	return Requester{Account: "account-1", Orgs: map[string]Role{org: r}}
}

// A session carries no grant, so the role is the whole answer. This is the
// path the cockpit takes, and it must not change shape when tokens gain axes.
func TestSessionIsRoleOnly(t *testing.T) {
	session := Credential{Requester: member("org-1", RoleAdmin)}

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
			Requester: member(org, c.role),
			Grant:     &Grant{Org: org, Scopes: c.scope},
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
	stranger := Requester{Account: "account-1"} // removed from every org
	cred := Credential{
		Requester: stranger,
		Grant:     &Grant{Org: "org-1", Scopes: []Capability{ReadProject, ExecConnector}},
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
			Requester: Requester{Account: "account-1", Platform: true, Orgs: map[string]Role{
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
	operator := Requester{Account: "account-1", Platform: true}

	session := Credential{Requester: operator}
	if !session.Can(AdminAccounts, "", "") {
		t.Error("operator session lost account administration")
	}
	if !session.Can(AdminUpdate, "", "") {
		t.Error("operator session lost update administration")
	}

	// Even a grant that names the capability cannot reach the installation,
	// because the boundary check refuses an empty org outright.
	token := Credential{
		Requester: operator,
		Grant:     &Grant{Org: "org-1", Scopes: []Capability{AdminAccounts, AdminUpdate, ReadOrg}},
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

// An installation grant is a second class of token whose boundary is the
// installation itself. Three invariants hold: its own scope list is still
// the first check; only the installation-grantable verbs exist for it, no
// matter what a hand-crafted row says; and an org grant keeps refusing the
// empty boundary outright.
func TestInstallationGrant(t *testing.T) {
	operator := Requester{Account: "account-1", Platform: true}

	cases := []struct {
		name         string
		grant        Grant
		cap          Capability
		org, project string
		want         bool
	}{
		{"named capability at the installation", Grant{Installation: true, Scopes: []Capability{AdminOrg}}, AdminOrg, "", "", true},
		{"named skill at the installation", Grant{Installation: true, Scopes: []Capability{AdminSkill}}, AdminSkill, "", "", true},
		{"enumerate orgs at the installation", Grant{Installation: true, Scopes: []Capability{ReadOrg}}, ReadOrg, "", "", true},
		{"accounts stay with the person", Grant{Installation: true, Scopes: []Capability{AdminOrg}}, AdminAccounts, "", "", false},
		// The regression that caught the blocker: the token honours its own
		// scopes, and installationGrantable is not a substitute for them.
		{"token honours its own scopes", Grant{Installation: true, Scopes: []Capability{ReadOrg}}, AdminOrg, "", "", false},
		// Defense in depth: even a hand-crafted row that names a
		// non-grantable installation verb is refused by the whitelist.
		{"a non-grantable scope cannot be smuggled in", Grant{Installation: true, Scopes: []Capability{AdminAccounts}}, AdminAccounts, "", "", false},
		{"an installation token cannot enter a tenant", Grant{Installation: true, Scopes: []Capability{AdminOrg}}, AdminOrg, "org-1", "", false},
		{"an installation token cannot enter a project", Grant{Installation: true, Scopes: []Capability{AdminOrg}}, AdminOrg, "org-1", "project-1", false},
		// The old class keeps its shape: an org grant still refuses the
		// installation, even when it names an installation verb.
		{"org token at the installation", Grant{Org: "org-1", Scopes: []Capability{ReadOrg}}, ReadOrg, "", "", false},
		{"org token with an installation scope", Grant{Org: "org-1", Scopes: []Capability{AdminOrg}}, AdminOrg, "", "", false},
	}
	for _, c := range cases {
		cred := Credential{Requester: operator, Grant: &c.grant}
		if got := cred.Can(c.cap, c.org, c.project); got != c.want {
			t.Errorf("%s: Can(%q, %q, %q) = %v; want %v", c.name, c.cap, c.org, c.project, got, c.want)
		}
	}
}

// Contains is the boundary axis. An installation grant covers exactly the
// installation — the empty org with no project — and never a tenant or a
// project inside one.
func TestInstallationGrantBoundary(t *testing.T) {
	g := &Grant{Installation: true, Scopes: []Capability{AdminOrg}}
	if !g.Contains("", "") {
		t.Error("an installation grant refused its own boundary")
	}
	for _, target := range [][2]string{{"org-1", ""}, {"", "project-1"}, {"org-1", "project-1"}} {
		if g.Contains(target[0], target[1]) {
			t.Errorf("an installation grant reached %q/%q", target[0], target[1])
		}
	}
}

// A token mints nothing beyond its author. A customer's token — Platform
// false — never passes an installation capability, whatever the grant says.
func TestInstallationGrantNeedsThePlatformOperator(t *testing.T) {
	customer := Requester{Account: "account-2", Orgs: map[string]Role{"org-1": RoleOwner}}
	cred := Credential{
		Requester: customer,
		Grant:     &Grant{Installation: true, Scopes: []Capability{AdminOrg, AdminSkill, ReadOrg}},
	}
	for _, c := range []Capability{AdminOrg, AdminSkill, ReadOrg} {
		if cred.Can(c, "", "") {
			t.Errorf("a customer token exercised %q at the installation", c)
		}
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
		"admin:hub.account",             // installation-only, never grantable
		"admin:hub.update",              // same
		"admin:hub.skill",               // installation-grantable, but not org-grantable
		"admin:hub.org admin:hub.skill", // mixed classes are refused whole
		"Read:project.task",             // case matters
		"read:project.tasks",            // typo
	} {
		if _, err := ParseScopes(bad); !errors.Is(err, ErrScopeUnknown) {
			t.Errorf("ParseScopes(%q) error = %v; want ErrScopeUnknown", bad, err)
		}
	}
}

// The installation parser reads only the installation grantable set. The org
// path is untouched: an installation scope is not org-grantable and an org
// scope is not installation-grantable, so neither parser accepts the other's
// vocabulary.
func TestParseInstallationScopes(t *testing.T) {
	got, err := ParseInstallationScopes("admin:hub.skill admin:hub.org")
	if err != nil {
		t.Fatalf("ParseInstallationScopes: %v", err)
	}
	want := []Capability{AdminOrg, AdminSkill} // sorted
	if !slices.Equal(got, want) {
		t.Errorf("ParseInstallationScopes = %v; want %v", got, want)
	}

	// Storage is normalised on the way in, same as the org parser.
	got, err = ParseInstallationScopes("  admin:hub.org   admin:hub.org ")
	if err != nil {
		t.Fatalf("ParseInstallationScopes with repeats: %v", err)
	}
	if !slices.Equal(got, []Capability{AdminOrg}) {
		t.Errorf("ParseInstallationScopes deduplicated = %v; want [admin:hub.org]", got)
	}

	if got, err := ParseInstallationScopes(""); err != nil || got != nil {
		t.Errorf("ParseInstallationScopes(\"\") = %v, %v; want nil, nil", got, err)
	}
	if got, err := ParseInstallationScopes("   "); err != nil || got != nil {
		t.Errorf("ParseInstallationScopes(blank) = %v, %v; want nil, nil", got, err)
	}

	// The org path stays closed to installation vocabulary, and the
	// installation path stays closed to everything not installation-grantable.
	if _, err := ParseScopes("admin:hub.skill"); !errors.Is(err, ErrScopeUnknown) {
		t.Errorf("ParseScopes(admin:hub.skill) error = %v; want ErrScopeUnknown", err)
	}
	for _, bad := range []string{
		"admin:hub.account", // stays session-only, never grantable
		"admin:hub.update",  // same
		"read:hub.update",   // installation-only, never grantable
		"read:project.task", // org-grantable, not installation-grantable
		"admin:hub.skil",    // typo
	} {
		if _, err := ParseInstallationScopes(bad); !errors.Is(err, ErrScopeUnknown) {
			t.Errorf("ParseInstallationScopes(%q) error = %v; want ErrScopeUnknown", bad, err)
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

// The installation grantable set is a subset of the installation set and
// deliberately excludes account and update administration: those stay with
// the person in front of a browser no matter what a token row says.
func TestInstallationGrantableIsANarrowInstallation(t *testing.T) {
	scopes := InstallationGrantableScopes()
	if !slices.IsSorted(scopes) {
		t.Error("InstallationGrantableScopes() is not sorted")
	}
	for _, c := range scopes {
		if !installation[c] {
			t.Errorf("%q is installation-grantable but not an installation capability", c)
		}
	}
	for _, c := range []Capability{AdminAccounts, AdminUpdate} {
		if installationGrantable[c] {
			t.Errorf("%q must stay session-only, never grantable to a token", c)
		}
	}
}

// A hub claimed before accounts existed has no org, so its operator has no
// membership to check a fleet capability against. Refusing would lock the
// installation's only administrator out of their own machines.
func TestUnpartitionedOperatorHoldsTheFleet(t *testing.T) {
	legacy := Credential{Requester: Requester{Platform: true, Unpartitioned: true}}
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
	partitioned := Credential{Requester: Requester{Platform: true}}
	if partitioned.Can(ReadConnector, "org-1", "project-1") {
		t.Error("platform admin reached an org's fleet without membership")
	}

	// It cannot be a back door for a non-operator either.
	impostor := Credential{Requester: Requester{Account: "account-9", Unpartitioned: true}}
	if impostor.Can(ReadConnector, "org-1", "project-1") {
		t.Error("a non-operator was let in by the unpartitioned flag")
	}

	// And a token is still bounded, because a legacy hub can still mint one
	// once it has an org — the grant does the refusing, not the role.
	token := Credential{
		Requester: Requester{Platform: true, Unpartitioned: true},
		Grant:     &Grant{Org: "org-1", Scopes: []Capability{ReadConnector}},
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

// Every installation-grantable scope carries a description for the mint
// form: a checkbox with no prose is a token nobody should hand out on faith.
// Non-grantable capabilities get none, so a cockpit bug cannot dress an
// ungrantable verb in a description.
func TestInstallationScopeDescriptionCoversGrantable(t *testing.T) {
	for _, c := range InstallationGrantableScopes() {
		if InstallationScopeDescription(c) == "" {
			t.Errorf("%q is installation-grantable but has no description", c)
		}
	}
	for _, c := range Capabilities() {
		if installationGrantable[c] {
			continue
		}
		if InstallationScopeDescription(c) != "" {
			t.Errorf("%q is not installation-grantable but has a description", c)
		}
	}
}
