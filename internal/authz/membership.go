package authz

// OrgState is an organization's roster at the moment a change is requested.
//
// The rules below need the whole roster, not just the actor and the target,
// because the interesting refusals are about what the org looks like
// afterwards: an org with no owner left is one nobody can administer, and no
// amount of per-row permission checking notices that.
type OrgState struct {
	ID string
	// Members maps account id to role. A missing key is not a member.
	Members map[string]Role
}

// Owners counts the accounts holding `owner` in this org.
func (o OrgState) Owners() int {
	n := 0
	for _, r := range o.Members {
		if r == RoleOwner {
			n++
		}
	}
	return n
}

// AuthorizeRoleChange decides whether actor may set target's role in org.
//
// Beyond "may this person administer this org", two rules exist that a
// capability check alone does not express:
//
//   - **Only an owner may create or unmake an owner.** Without this, an
//     `admin` promotes themselves and the distinction between the two roles
//     lasts exactly one request.
//   - **The last owner cannot be demoted.** An org with no owner keeps its
//     members and its billing and loses the only role that can delete it or
//     hand it over, which is a state no API call can undo.
//
// Setting the role somebody already holds is allowed and does nothing: a
// retried request is not an error.
func AuthorizeRoleChange(actor Actor, org OrgState, target string, newRole Role) error {
	if rank[newRole] == 0 {
		return ErrRoleUnknown
	}
	// Permission before existence: a caller who may not administer this org
	// does not get to learn who is in it from the error.
	if !actor.Can(AdminOrg, org.ID) {
		return ErrForbidden
	}
	current, ok := org.Members[target]
	if !ok {
		return ErrNotMember
	}
	if current == newRole {
		return nil
	}
	if (current == RoleOwner || newRole == RoleOwner) && actor.Role(org.ID) != RoleOwner {
		return ErrOwnerOnly
	}
	if current == RoleOwner && org.Owners() == 1 {
		return ErrLastOwner
	}
	return nil
}

// AuthorizeRemoval decides whether actor may remove target from org.
//
// Leaving on your own is allowed without administering anything — a member
// who joined the wrong org should not have to ask. The owner rules are the
// same as for a role change, and for the same reason: the last owner cannot
// walk out and leave an org nobody can administer, not even voluntarily.
func AuthorizeRemoval(actor Actor, org OrgState, target string) error {
	self := actor.Account != "" && actor.Account == target
	if !self && !actor.Can(AdminOrg, org.ID) {
		return ErrForbidden
	}
	current, ok := org.Members[target]
	if !ok {
		return ErrNotMember
	}
	if current == RoleOwner {
		if !self && actor.Role(org.ID) != RoleOwner {
			return ErrOwnerOnly
		}
		if org.Owners() == 1 {
			return ErrLastOwner
		}
	}
	return nil
}
