// Package authz owns who may do what on the hub's own surface.
//
// Three role layers exist and they are deliberately not one flat list
// (drafts 08, 25): the platform `admin` runs the installation, org roles run
// a customer's organization, and project roles run the work inside a project.
// This package covers the first two. Project roles belong with the project
// once it has enforced membership rows.
//
// Nothing outside this package checks a role directly. Roles resolve to
// capabilities in the `verb:entity` grammar (05) and callers ask for a
// capability inside a boundary, which is what stops one screen from growing
// two permission paths: a platform admin's list of people and an org owner's
// list of people are the same question asked with a different boundary —
// 09's subject / boundary / verbs axes.
package authz

import "errors"

// Role is an organization role (25). A role is a named bundle of
// capabilities, never a thing to compare against at a call site.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// rank orders the org roles so a rule can say "admin or better" without
// listing every role above it — the way that list stops being updated the
// day a fourth role lands. Zero means unknown, which is never sufficient.
var rank = map[Role]int{
	RoleMember: 1,
	RoleAdmin:  2,
	RoleOwner:  3,
}

// ParseRole accepts a role name from the wire. An unknown name is refused
// rather than defaulted: a typo that silently became `member` would be a
// permission decision made by a spelling mistake.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if rank[r] == 0 {
		return "", ErrRoleUnknown
	}
	return r, nil
}

// Roles lists the assignable org roles, weakest first. The cockpit renders
// its dropdown from this so the API and the form cannot disagree.
func Roles() []Role { return []Role{RoleMember, RoleAdmin, RoleOwner} }

func (r Role) atLeast(min Role) bool { return rank[r] != 0 && rank[r] >= rank[min] }

// Capability is a `verb:entity` pair from the naming ontology (05). The set
// is small on purpose; it grows when a surface needs a distinction, not in
// anticipation of one.
type Capability string

const (
	// AdminAccounts is installation-wide account administration. Draft 09
	// lists this scope by name. It belongs to the operator of the hub, not
	// to any organization on it.
	AdminAccounts Capability = "admin:hub.account"

	// ReadOrg means different things at different boundaries, which is the
	// whole point of carrying a boundary: at the installation it is
	// "enumerate the organizations on this hub", inside one it is "see this
	// organization and who is in it".
	ReadOrg Capability = "read:hub.org"

	// AdminOrg is membership administration inside one org: change a role,
	// remove a person (25 gives this to `admin` and above).
	AdminOrg Capability = "admin:hub.org"

	// DeleteOrg is the owner's alone (25).
	DeleteOrg Capability = "delete:hub.org"
)

// installation lists the capabilities that exist at the hub boundary. A
// capability absent here can never be exercised with an empty boundary, so a
// new org capability does not accidentally become a platform power.
var installation = map[Capability]bool{
	AdminAccounts: true,
	ReadOrg:       true,
}

// orgMinimum is the weakest role that carries each capability inside an org.
var orgMinimum = map[Capability]Role{
	ReadOrg:   RoleMember,
	AdminOrg:  RoleAdmin,
	DeleteOrg: RoleOwner,
}

// Actor is the resolved identity behind a request. The hub builds it at the
// edge from the session and the store; every decision below reads only this.
type Actor struct {
	// Account is the `acc-` this request acts as. Empty means a hub that was
	// claimed before accounts existed, whose anonymous operator password is
	// still the only credential (26's legacy path).
	Account string

	// Platform marks the installation's operator: the hub-level `admin` role
	// from 08, or the legacy operator credential.
	Platform bool

	// Orgs is this account's membership, keyed by org id. An account may
	// belong to many (25), so there is no "current org" here — the boundary
	// arrives with the request.
	Orgs map[string]Role
}

// Can reports whether the actor may exercise c inside org.
//
// An empty org means the installation itself, and only the platform operator
// has anything there.
//
// A platform admin gets **no** org-boundary capability from that flag alone.
// That is not an oversight: 25 says the platform admin is not an org member,
// and 09 still has "does a hub admin have any technical path into a customer
// project" open. Granting it here would answer that question by accident, in
// the direction that is hardest to walk back. The operator of a self-hosted
// hub is unaffected, because claiming mints them a real owner membership in
// the hub's first org — they hold both, and they hold the second one visibly.
func (a Actor) Can(c Capability, org string) bool {
	if org == "" {
		return a.Platform && installation[c]
	}
	min, ok := orgMinimum[c]
	if !ok {
		return false
	}
	return a.Role(org).atLeast(min)
}

// Role returns this actor's role in org, or "" when they are not a member.
func (a Actor) Role(org string) Role { return a.Orgs[org] }

// Errors the HTTP edge maps onto status codes. Sentinels rather than strings,
// so a handler cannot drift from the rule it is reporting.
var (
	ErrForbidden   = errors.New("not allowed")
	ErrRoleUnknown = errors.New("unknown role")
	ErrNotMember   = errors.New("not a member of this organization")
	ErrOwnerOnly   = errors.New("only an owner may change an owner")
	ErrLastOwner   = errors.New("an organization must keep at least one owner")
)
