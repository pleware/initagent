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
		{"one owner", map[string]Role{"acc-1": RoleOwner}, 1},
		{"two owners and a member", map[string]Role{
			"acc-1": RoleOwner, "acc-2": RoleOwner, "acc-3": RoleMember,
		}, 2},
		{"no owner at all", map[string]Role{"acc-1": RoleAdmin}, 0},
	}
	for _, c := range cases {
		if got := org(c.members).Owners(); got != c.want {
			t.Errorf("%s: Owners() = %d; want %d", c.name, got, c.want)
		}
	}
}

func TestAuthorizeRoleChange(t *testing.T) {
	soleOwner := map[string]Role{"acc-own": RoleOwner, "acc-adm": RoleAdmin, "acc-mem": RoleMember}
	twoOwners := map[string]Role{"acc-own": RoleOwner, "acc-own2": RoleOwner, "acc-mem": RoleMember}

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
			actor: actorIn("acc-adm", RoleAdmin), members: soleOwner,
			target: "acc-mem", newRole: RoleAdmin, want: nil,
		},
		{
			name:  "an owner promotes a member to owner",
			actor: actorIn("acc-own", RoleOwner), members: soleOwner,
			target: "acc-mem", newRole: RoleOwner, want: nil,
		},
		{
			name:  "setting the role somebody already holds is a no-op",
			actor: actorIn("acc-adm", RoleAdmin), members: soleOwner,
			target: "acc-mem", newRole: RoleMember, want: nil,
		},
		{
			name:  "an unknown role never reaches the store",
			actor: actorIn("acc-own", RoleOwner), members: soleOwner,
			target: "acc-mem", newRole: Role("superuser"), want: ErrRoleUnknown,
		},
		{
			name:  "a plain member cannot change roles",
			actor: actorIn("acc-mem", RoleMember), members: soleOwner,
			target: "acc-adm", newRole: RoleMember, want: ErrForbidden,
		},
		{
			name:  "a stranger to the org cannot change roles",
			actor: Actor{Account: "acc-x"}, members: soleOwner,
			target: "acc-mem", newRole: RoleAdmin, want: ErrForbidden,
		},
		{
			// Permission is checked before existence, so this stays
			// ErrForbidden rather than telling an outsider who is a member.
			name:  "a stranger asking about a non-member still gets forbidden",
			actor: Actor{Account: "acc-x"}, members: soleOwner,
			target: "acc-nobody", newRole: RoleAdmin, want: ErrForbidden,
		},
		{
			name:  "the target has to be a member already",
			actor: actorIn("acc-adm", RoleAdmin), members: soleOwner,
			target: "acc-nobody", newRole: RoleMember, want: ErrNotMember,
		},
		{
			name:  "an admin cannot promote anyone to owner",
			actor: actorIn("acc-adm", RoleAdmin), members: soleOwner,
			target: "acc-mem", newRole: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:  "an admin cannot promote themselves to owner",
			actor: actorIn("acc-adm", RoleAdmin), members: soleOwner,
			target: "acc-adm", newRole: RoleOwner, want: ErrOwnerOnly,
		},
		{
			name:  "an admin cannot demote an owner",
			actor: actorIn("acc-adm", RoleAdmin), members: twoOwners,
			target: "acc-own2", newRole: RoleAdmin, want: ErrOwnerOnly,
		},
		{
			name:  "the last owner cannot demote themselves",
			actor: actorIn("acc-own", RoleOwner), members: soleOwner,
			target: "acc-own", newRole: RoleAdmin, want: ErrLastOwner,
		},
		{
			name:  "an owner may step down once a second owner exists",
			actor: actorIn("acc-own", RoleOwner), members: twoOwners,
			target: "acc-own", newRole: RoleMember, want: nil,
		},
		{
			name:  "an owner may demote another owner",
			actor: actorIn("acc-own", RoleOwner), members: twoOwners,
			target: "acc-own2", newRole: RoleMember, want: nil,
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
	soleOwner := map[string]Role{"acc-own": RoleOwner, "acc-adm": RoleAdmin, "acc-mem": RoleMember}
	twoOwners := map[string]Role{"acc-own": RoleOwner, "acc-own2": RoleOwner, "acc-mem": RoleMember}

	cases := []struct {
		name    string
		actor   Actor
		members map[string]Role
		target  string
		want    error
	}{
		{
			name:  "an admin removes a member",
			actor: actorIn("acc-adm", RoleAdmin), members: soleOwner,
			target: "acc-mem", want: nil,
		},
		{
			// Leaving needs no administrative right: somebody who accepted
			// the wrong invite should not have to ask to be let out.
			name:  "a member leaves on their own",
			actor: actorIn("acc-mem", RoleMember), members: soleOwner,
			target: "acc-mem", want: nil,
		},
		{
			name:  "a member cannot remove somebody else",
			actor: actorIn("acc-mem", RoleMember), members: soleOwner,
			target: "acc-adm", want: ErrForbidden,
		},
		{
			name:  "a stranger cannot remove anyone",
			actor: Actor{Account: "acc-x"}, members: soleOwner,
			target: "acc-mem", want: ErrForbidden,
		},
		{
			name:  "the target has to be a member",
			actor: actorIn("acc-adm", RoleAdmin), members: soleOwner,
			target: "acc-nobody", want: ErrNotMember,
		},
		{
			name:  "an admin cannot remove an owner",
			actor: actorIn("acc-adm", RoleAdmin), members: twoOwners,
			target: "acc-own2", want: ErrOwnerOnly,
		},
		{
			name:  "an owner removes another owner",
			actor: actorIn("acc-own", RoleOwner), members: twoOwners,
			target: "acc-own2", want: nil,
		},
		{
			name:  "the last owner cannot leave",
			actor: actorIn("acc-own", RoleOwner), members: soleOwner,
			target: "acc-own", want: ErrLastOwner,
		},
		{
			name:  "an owner may leave once a second owner exists",
			actor: actorIn("acc-own", RoleOwner), members: twoOwners,
			target: "acc-own", want: nil,
		},
	}

	for _, c := range cases {
		err := AuthorizeRemoval(c.actor, org(c.members), c.target)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v; want %v", c.name, err, c.want)
		}
	}
}

// A legacy operator session carries no account id. An empty target must not
// match it into a "removing myself" shortcut that skips the permission check.
func TestEmptyAccountIsNotSelfRemoval(t *testing.T) {
	legacy := Actor{Platform: true}
	state := org(map[string]Role{"": RoleOwner, "acc-own": RoleOwner})
	if err := AuthorizeRemoval(legacy, state, ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("error = %v; want ErrForbidden", err)
	}
}
