package authz

import (
	"errors"
	"testing"
)

func org(members map[string]Role) OrgState {
	return OrgState{ID: "org-1", Members: members}
}

func actorIn(account string, role Role) Actor {
	return Actor{Account: account, Orgs: map[string]Role{"org-1": role}}
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
		name    string
		actor   Actor
		members map[string]Role
		target  string
		newRole Role
		want    error
	}{
		{
			name:  "an admin promotes a member to admin",
			actor: actorIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", newRole: RoleAdmin, want: nil,
		},
		{
			name:  "an owner promotes a member to owner",
			actor: actorIn("account-own", RoleOwner), members: soleOwner,
			target: "account-mem", newRole: RoleOwner, want: nil,
		},
		{
			name:  "setting the role somebody already holds is a no-op",
			actor: actorIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", newRole: RoleMember, want: nil,
		},
		{
			name:  "an unknown role never reaches the store",
			actor: actorIn("account-own", RoleOwner), members: soleOwner,
			target: "account-mem", newRole: Role("superuser"), want: ErrRoleUnknown,
		},
		{
			name:  "a plain member cannot change roles",
			actor: actorIn("account-mem", RoleMember), members: soleOwner,
			target: "account-adm", newRole: RoleMember, want: ErrForbidden,
		},
		{
			name:  "a stranger to the org cannot change roles",
			actor: Actor{Account: "account-x"}, members: soleOwner,
			target: "account-mem", newRole: RoleAdmin, want: ErrForbidden,
		},
		{
			// Permission is checked before existence, so this stays
			// ErrForbidden rather than telling an outsider who is a member.
			name:  "a stranger asking about a non-member still gets forbidden",
			actor: Actor{Account: "account-x"}, members: soleOwner,
			target: "account-nobody", newRole: RoleAdmin, want: ErrForbidden,
		},
		{
			name:  "the target has to be a member already",
			actor: actorIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-nobody", newRole: RoleMember, want: ErrNotMember,
		},
		{
			name:  "an admin cannot promote anyone to owner",
			actor: actorIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", newRole: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:  "an admin cannot promote themselves to owner",
			actor: actorIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-adm", newRole: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:  "an admin cannot demote an owner",
			actor: actorIn("account-adm", RoleAdmin), members: twoOwners,
			target: "account-own2", newRole: RoleAdmin, want: ErrOwnerOnly,
		},
		{
			name:  "the last owner cannot demote themselves",
			actor: actorIn("account-own", RoleOwner), members: soleOwner,
			target: "account-own", newRole: RoleAdmin, want: ErrLastOwner,
		},
		{
			name:  "an owner may step down once a second owner exists",
			actor: actorIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own", newRole: RoleMember, want: nil,
		},
		{
			name:  "an owner may demote another owner",
			actor: actorIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own2", newRole: RoleMember, want: nil,
		},
	}

	for _, c := range cases {
		err := AuthorizeRoleChange(c.actor, org(c.members), c.target, c.newRole)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
}

func TestAuthorizeRemoval(t *testing.T) {
	soleOwner := map[string]Role{"account-own": RoleOwner, "account-adm": RoleAdmin, "account-mem": RoleMember}
	twoOwners := map[string]Role{"account-own": RoleOwner, "account-own2": RoleOwner, "account-mem": RoleMember}

	cases := []struct {
		name    string
		actor   Actor
		members map[string]Role
		target  string
		want    error
	}{
		{
			name:  "an admin removes a member",
			actor: actorIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-mem", want: nil,
		},
		{
			// Leaving needs no administrative right: somebody who accepted
			// the wrong invite should not have to ask to be let out.
			name:  "a member leaves on their own",
			actor: actorIn("account-mem", RoleMember), members: soleOwner,
			target: "account-mem", want: nil,
		},
		{
			name:  "a member cannot remove somebody else",
			actor: actorIn("account-mem", RoleMember), members: soleOwner,
			target: "account-adm", want: ErrForbidden,
		},
		{
			name:  "a stranger cannot remove anyone",
			actor: Actor{Account: "account-x"}, members: soleOwner,
			target: "account-mem", want: ErrForbidden,
		},
		{
			name:  "the target has to be a member",
			actor: actorIn("account-adm", RoleAdmin), members: soleOwner,
			target: "account-nobody", want: ErrNotMember,
		},
		{
			name:  "an admin cannot remove an owner",
			actor: actorIn("account-adm", RoleAdmin), members: twoOwners,
			target: "account-own2", want: ErrOwnerOnly,
		},
		{
			name:  "an owner removes another owner",
			actor: actorIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own2", want: nil,
		},
		{
			name:  "the last owner cannot leave",
			actor: actorIn("account-own", RoleOwner), members: soleOwner,
			target: "account-own", want: ErrLastOwner,
		},
		{
			name:  "an owner may leave once a second owner exists",
			actor: actorIn("account-own", RoleOwner), members: twoOwners,
			target: "account-own", want: nil,
		},
	}

	for _, c := range cases {
		err := AuthorizeRemoval(c.actor, org(c.members), c.target)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
}

func TestAuthorizeInvite(t *testing.T) {
	soleOwner := map[string]Role{"account-own": RoleOwner, "account-adm": RoleAdmin, "account-mem": RoleMember}

	cases := []struct {
		name  string
		actor Actor
		role  Role
		want  error
	}{
		{
			name:  "an owner invites a member",
			actor: actorIn("account-own", RoleOwner), role: RoleMember, want: nil,
		},
		{
			name:  "an admin invites a member",
			actor: actorIn("account-adm", RoleAdmin), role: RoleMember, want: nil,
		},
		{
			name:  "an admin invites an admin",
			actor: actorIn("account-adm", RoleAdmin), role: RoleAdmin, want: nil,
		},
		{
			name:  "an owner invites an owner",
			actor: actorIn("account-own", RoleOwner), role: RoleOwner, want: nil,
		},
		{
			name:  "an admin cannot invite an owner",
			actor: actorIn("account-adm", RoleAdmin), role: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:  "a member cannot invite",
			actor: actorIn("account-mem", RoleMember), role: RoleMember, want: ErrForbidden,
		},
		{
			name:  "a stranger cannot invite",
			actor: Actor{Account: "account-x"}, role: RoleMember, want: ErrForbidden,
		},
		{
			name:  "an unknown role never reaches the store",
			actor: actorIn("account-own", RoleOwner), role: Role("superuser"), want: ErrRoleUnknown,
		},
	}
	for _, c := range cases {
		err := AuthorizeInvite(c.actor, org(soleOwner), c.role)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
}

// A legacy operator session carries no account id. An empty target must not
// match it into a "removing myself" shortcut that skips the permission check.
func TestEmptyAccountIsNotSelfRemoval(t *testing.T) {
	legacy := Actor{Platform: true}
	state := org(map[string]Role{"": RoleOwner, "account-own": RoleOwner})
	if err := AuthorizeRemoval(legacy, state, ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("error = %v; want ErrForbidden", err)
	}
}
