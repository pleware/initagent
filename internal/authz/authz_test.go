package authz

import (
	"errors"
	"slices"
	"testing"
)

func TestParseRole(t *testing.T) {
	for _, want := range Roles() {
		got, err := ParseRole(string(want))
		if err != nil || got != want {
			t.Errorf("ParseRole(%q) = %q, %v; want %q, nil", want, got, err, want)
		}
	}
	for _, bad := range []string{"", "Owner", "admins", "viewer", "platform", "root"} {
		if _, err := ParseRole(bad); !errors.Is(err, ErrRoleUnknown) {
			t.Errorf("ParseRole(%q) error = %v; want ErrRoleUnknown", bad, err)
		}
	}
}

// The cockpit renders its role dropdown from Roles(), so the order is part of
// the contract: weakest first, because a form that defaults to the top of the
// list should not default to owner.
func TestRolesIsWeakestFirst(t *testing.T) {
	got := Roles()
	want := []Role{RoleMember, RoleAdmin, RoleOwner}
	if !slices.Equal(got, want) {
		t.Fatalf("Roles() = %v; want %v", got, want)
	}
}

func TestCanAtInstallationBoundary(t *testing.T) {
	operator := Actor{Account: "acc-1", Platform: true}
	customer := Actor{Account: "acc-2", Orgs: map[string]Role{"org-1": RoleOwner}}

	cases := []struct {
		name  string
		actor Actor
		cap   Capability
		want  bool
	}{
		{"operator administers accounts", operator, AdminAccounts, true},
		{"operator enumerates orgs", operator, ReadOrg, true},
		// An org capability must not become a platform power just because the
		// boundary is empty: the installation is not "every org at once".
		{"operator cannot administer an org hub-wide", operator, AdminOrg, false},
		{"operator cannot delete an org hub-wide", operator, DeleteOrg, false},
		{"operator cannot create a project hub-wide", operator, CreateProject, false},
		{"an org owner is not a platform admin", customer, AdminAccounts, false},
		{"an org owner cannot enumerate the hub", customer, ReadOrg, false},
	}
	for _, c := range cases {
		if got := c.actor.Can(c.cap, ""); got != c.want {
			t.Errorf("%s: Can(%q, \"\") = %v; want %v", c.name, c.cap, got, c.want)
		}
	}
}

func TestCanInsideOrg(t *testing.T) {
	const org = "org-1"
	actor := func(r Role) Actor {
		return Actor{Account: "acc-1", Orgs: map[string]Role{org: r}}
	}

	cases := []struct {
		role                             Role
		read, admin, del                 bool
		readProj, createProj, deleteProj bool
	}{
		{RoleMember, true, false, false, true, false, false},
		{RoleAdmin, true, true, false, true, true, true},
		{RoleOwner, true, true, true, true, true, true},
	}
	for _, c := range cases {
		a := actor(c.role)
		if got := a.Can(ReadOrg, org); got != c.read {
			t.Errorf("%s Can(ReadOrg) = %v; want %v", c.role, got, c.read)
		}
		if got := a.Can(AdminOrg, org); got != c.admin {
			t.Errorf("%s Can(AdminOrg) = %v; want %v", c.role, got, c.admin)
		}
		if got := a.Can(DeleteOrg, org); got != c.del {
			t.Errorf("%s Can(DeleteOrg) = %v; want %v", c.role, got, c.del)
		}
		if got := a.Can(ReadProject, org); got != c.readProj {
			t.Errorf("%s Can(ReadProject) = %v; want %v", c.role, got, c.readProj)
		}
		if got := a.Can(CreateProject, org); got != c.createProj {
			t.Errorf("%s Can(CreateProject) = %v; want %v", c.role, got, c.createProj)
		}
		if got := a.Can(DeleteProject, org); got != c.deleteProj {
			t.Errorf("%s Can(DeleteProject) = %v; want %v", c.role, got, c.deleteProj)
		}
	}

	stranger := Actor{Account: "acc-9"}
	if stranger.Can(ReadOrg, org) {
		t.Error("a non-member can read an org")
	}
	if stranger.Role(org) != "" {
		t.Errorf("Role for a non-member = %q; want empty", stranger.Role(org))
	}
	// AdminAccounts has no meaning inside an org, and an unregistered
	// capability must fail closed rather than fall through to a default.
	if actor(RoleOwner).Can(AdminAccounts, org) {
		t.Error("an org owner administers hub accounts")
	}
	if actor(RoleOwner).Can(Capability("write:hub.invented"), org) {
		t.Error("an unknown capability was granted")
	}
}

// The rule with the most consequence in the package: running the hub does not
// put you inside a customer's organization (25), and 09 has not decided that
// it should.
func TestPlatformAdminIsNotAnOrgMember(t *testing.T) {
	operator := Actor{Account: "acc-1", Platform: true}
	for _, c := range []Capability{ReadOrg, AdminOrg, DeleteOrg, ReadProject, CreateProject, DeleteProject} {
		if operator.Can(c, "org-customer") {
			t.Errorf("platform admin was granted %q inside a customer org", c)
		}
	}

	// A self-hosted operator holds both, because claiming mints them a real
	// owner membership rather than relying on the platform flag.
	both := Actor{Account: "acc-1", Platform: true, Orgs: map[string]Role{"org-1": RoleOwner}}
	if !both.Can(DeleteOrg, "org-1") || !both.Can(AdminAccounts, "") {
		t.Error("an operator who owns the first org should hold both surfaces")
	}
}

// A hub claimed before accounts existed has no `acc-` behind its session. It
// is still the operator, and it is still nobody's org member.
func TestSoleOrg(t *testing.T) {
	if got := (Actor{}).SoleOrg(); got != "" {
		t.Errorf("empty actor SoleOrg = %q; want empty", got)
	}
	one := Actor{Orgs: map[string]Role{"org-1": RoleOwner}}
	if got := one.SoleOrg(); got != "org-1" {
		t.Errorf("one org SoleOrg = %q; want org-1", got)
	}
	two := Actor{Orgs: map[string]Role{"org-1": RoleOwner, "org-2": RoleMember}}
	if got := two.SoleOrg(); got != "" {
		t.Errorf("two orgs SoleOrg = %q; want empty — that is not a current-org", got)
	}
}

func TestLegacyOperatorActor(t *testing.T) {
	legacy := Actor{Platform: true}
	if !legacy.Can(AdminAccounts, "") {
		t.Error("legacy operator lost the platform surface")
	}
	if legacy.Can(ReadOrg, "org-1") {
		t.Error("legacy operator was let into an org")
	}
}
