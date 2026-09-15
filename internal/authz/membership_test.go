package authz

import (
	"errors"
	"testing"
)

func org(members map[string]Role) OrgState {
	return OrgState{ID: "org-1", Members: members}
}

func requesterIn(account string, role Role) Requester {
	return Requester{Account: account, Orgs: map[string]Role{"org-1": role}}
}

func TestOwners(t *testing.T) {
	cases := []struct {
		name    string
		members map[string]Role
		want    int
	}{
		{"empty", nil, 0},
		{"one owner", map[string]Role{"account-1": RoleOwner}, 1},
		{"two owners and a member", map[string]Role{
			"account-1": RoleOwner, "account-2": RoleOwner, "account-3": RoleMember,
		}, 2},
		{"no owner at all", map[string]Role{"account-1": RoleAdmin}, 0},
	}
	for _, c := range cases {
		if got := org(c.members).Owners(); got != c.want {
			t.Errorf("%s: Owners() = %d; want %d", c.name, got, c.want)
		}
	}
}

func TestAuthorizeRoleChange(t *testing.T) {
	soleOwner := map[string]Role{"account-own": RoleOwner, "account-adm": RoleAdmin, "account-mem": RoleMember}
	twoOwners := map[string]Role{"account-own": RoleOwner, "account-own2": RoleOwner, "account-mem": RoleMember}

	cases := []struct {
		name      string
		requester Requester
		members   map[string]Role
		target    string
		newRole   Role
		want      error
	}{
		{
			name:      "an admin promotes a member to admin",
			requester: requesterIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", newRole: RoleAdmin, want: nil,
		},
		{
			name:      "an owner promotes a member to owner",
			requester: requesterIn("account-own", RoleOwner), members: soleOwner,
			target: "account-mem", newRole: RoleOwner, want: nil,
		},
		{
			name:      "setting the role somebody already holds is a no-op",
			requester: requesterIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", newRole: RoleMember, want: nil,
		},
		{
			name:      "an unknown role never reaches the store",
			requester: requesterIn("account-own", RoleOwner), members: soleOwner,
			target: "account-mem", newRole: Role("superuser"), want: ErrRoleUnknown,
		},
		{
			name:      "a plain member cannot change roles",
			requester: requesterIn("account-mem", RoleMember), members: soleOwner,
			target: "account-adm", newRole: RoleMember, want: ErrForbidden,
		},
		{
			name:      "a stranger to the org cannot change roles",
			requester: Requester{Account: "account-x"}, members: soleOwner,
			target: "account-mem", newRole: RoleAdmin, want: ErrForbidden,
		},
		{
			// Permission is checked before existence, so this stays
			// ErrForbidden rather than telling an outsider who is a member.
			name:      "a stranger asking about a non-member still gets forbidden",
			requester: Requester{Account: "account-x"}, members: soleOwner,
			target: "account-nobody", newRole: RoleAdmin, want: ErrForbidden,
		},
		{
			name:      "the target has to be a member already",
			requester: requesterIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-nobody", newRole: RoleMember, want: ErrNotMember,
		},
		{
			name:      "an admin cannot promote anyone to owner",
			requester: requesterIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", newRole: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:      "an admin cannot promote themselves to owner",
			requester: requesterIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-adm", newRole: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:      "an admin cannot demote an owner",
			requester: requesterIn("account-adm", RoleAdmin), members: twoOwners,
			target: "account-own2", newRole: RoleAdmin, want: ErrOwnerOnly,
		},
		{
			name:      "the last owner cannot demote themselves",
			requester: requesterIn("account-own", RoleOwner), members: soleOwner,
			target: "account-own", newRole: RoleAdmin, want: ErrLastOwner,
		},
		{
			name:      "an owner may step down once a second owner exists",
			requester: requesterIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own", newRole: RoleMember, want: nil,
		},
		{
			name:      "an owner may demote another owner",
			requester: requesterIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own2", newRole: RoleMember, want: nil,
		},
	}

	for _, c := range cases {
		err := AuthorizeRoleChange(c.requester, org(c.members), c.target, c.newRole)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
}

func TestAuthorizeRemoval(t *testing.T) {
	soleOwner := map[string]Role{"account-own": RoleOwner, "account-adm": RoleAdmin, "account-mem": RoleMember}
	twoOwners := map[string]Role{"account-own": RoleOwner, "account-own2": RoleOwner, "account-mem": RoleMember}

	cases := []struct {
		name      string
		requester Requester
		members   map[string]Role
		target    string
		want      error
	}{
		{
			name:      "an admin removes a member",
			requester: requesterIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", want: nil,
		},
		{
			// Leaving needs no administrative right: somebody who accepted
			// the wrong invite should not have to ask to be let out.
			name:      "a member leaves on their own",
			requester: requesterIn("account-mem", RoleMember), members: soleOwner,
			target: "account-mem", want: nil,
		},
		{
			name:      "a member cannot remove somebody else",
			requester: requesterIn("account-mem", RoleMember), members: soleOwner,
			target: "account-adm", want: ErrForbidden,
		},
		{
			name:      "a stranger cannot remove anyone",
			requester: Requester{Account: "account-x"}, members: soleOwner,
			target: "account-mem", want: ErrForbidden,
		},
		{
			name:      "the target has to be a member",
			requester: requesterIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-nobody", want: ErrNotMember,
		},
		{
			name:      "an admin cannot remove an owner",
			requester: requesterIn("account-adm", RoleAdmin), members: twoOwners,
			target: "account-own2", want: ErrOwnerOnly,
		},
		{
			name:      "an owner removes another owner",
			requester: requesterIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own2", want: nil,
		},
		{
			name:      "the last owner cannot leave",
			requester: requesterIn("account-own", RoleOwner), members: soleOwner,
			target: "account-own", want: ErrLastOwner,
		},
		{
			name:      "an owner may leave once a second owner exists",
			requester: requesterIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own", want: nil,
		},
	}

	for _, c := range cases {
		err := AuthorizeRemoval(c.requester, org(c.members), c.target)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
}

func TestAuthorizeInvite(t *testing.T) {
	soleOwner := map[string]Role{"account-own": RoleOwner, "account-adm": RoleAdmin, "account-mem": RoleMember}

	cases := []struct {
		name      string
		requester Requester
		role      Role
		want      error
	}{
		{
			name:      "an owner invites a member",
			requester: requesterIn("account-own", RoleOwner), role: RoleMember, want: nil,
		},
		{
			name:      "an admin invites a member",
			requester: requesterIn("account-adm", RoleAdmin), role: RoleMember, want: nil,
		},
		{
			name:      "an admin invites an admin",
			requester: requesterIn("account-adm", RoleAdmin), role: RoleAdmin, want: nil,
		},
		{
			name:      "an owner invites an owner",
			requester: requesterIn("account-own", RoleOwner), role: RoleOwner, want: nil,
		},
		{
			name:      "an admin cannot invite an owner",
			requester: requesterIn("account-adm", RoleAdmin), role: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:      "a member cannot invite",
			requester: requesterIn("account-mem", RoleMember), role: RoleMember, want: ErrForbidden,
		},
		{
			name:      "a stranger cannot invite",
			requester: Requester{Account: "account-x"}, role: RoleMember, want: ErrForbidden,
		},
		{
			name:      "an unknown role never reaches the store",
			requester: requesterIn("account-own", RoleOwner), role: Role("superuser"), want: ErrRoleUnknown,
		},
	}
	for _, c := range cases {
		err := AuthorizeInvite(c.requester, org(soleOwner), c.role)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
}

// A legacy operator session carries no account id. An empty target must not
// match it into a "removing myself" shortcut that skips the permission check.
func TestEmptyAccountIsNotSelfRemoval(t *testing.T) {
	legacy := Requester{Platform: true}
	state := org(map[string]Role{"": RoleOwner, "account-own": RoleOwner})
	if err := AuthorizeRemoval(legacy, state, ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("error = %v; want ErrForbidden", err)
	}
}
