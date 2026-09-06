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

import (
	"errors"
	"slices"
)

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

	// ReadProject is seeing the projects that belong to an organization.
	// Members have it: they can see the catalogue even if they cannot
	// create one (25).
	ReadProject Capability = "read:hub.project"

	// CreateProject is minting a project under an organization. 25 gives
	// this to admin and above. The project uses the hub's existing gateway
	// rather than provisioning a new one — that question is still open
	// for a second project on a second machine (02, 18).
	CreateProject Capability = "create:hub.project"

	// DeleteProject is removing a project from an organization. Same floor
	// as create: an admin who can add one can take it away.
	DeleteProject Capability = "delete:hub.project"

	// AdminProject is editing a project that already exists: its name, its
	// repository, and which machines may run its work.
	AdminProject Capability = "admin:hub.project"

	// ReadDevice is seeing the machines enrolled into a project.
	ReadDevice Capability = "read:fleet.device"

	// EnrollDevice mints the token that joins a machine, so it hands out
	// capability rather than reading it.
	EnrollDevice Capability = "create:fleet.device"

	// AdminDevice renames or removes an enrolled machine.
	AdminDevice Capability = "admin:fleet.device"

	// ExecDevice runs an arbitrary command on someone's machine. Draft 09
	// keeps this out of every credential unless it was asked for by name:
	// the attributable path is a task, which is queued, bounded and logged.
	// A role still carries it, because the cockpit's own terminal runs on a
	// session where a person is present; what a *token* may do is the axis
	// this constant exists to make explicit.
	ExecDevice Capability = "exec:fleet.device"

	// ReadTerminal lists terminal sessions and reads their output.
	ReadTerminal Capability = "read:fleet.terminal"

	// AttachTerminal opens, steers or kills a terminal session. Draft 09
	// names `attach:fleet.terminal` and deliberately avoids saying `tmux`:
	// a backend name in a permission breaks on the first Windows worker.
	AttachTerminal Capability = "attach:fleet.terminal"

	// ReadFile lists and downloads files from a machine.
	ReadFile Capability = "read:fleet.file"

	// WriteFile uploads onto a machine.
	WriteFile Capability = "write:fleet.file"

	// ReadTask is reading a queued or finished task.
	ReadTask Capability = "read:project.task"

	// CreateTask submits work. This is the normal path for running something
	// (09), which is why its floor is lower than ExecDevice's blast radius
	// would suggest: a task is attributable and bounded.
	CreateTask Capability = "create:project.task"

	// ReadTemplate is reading the project templates a new project starts from.
	ReadTemplate Capability = "read:hub.template"

	// ReadPreset is reading the saved command presets.
	ReadPreset Capability = "read:hub.preset"

	// AdminPreset creates or removes a preset.
	AdminPreset Capability = "admin:hub.preset"

	// ReadEvent subscribes to the hub's event stream.
	ReadEvent Capability = "read:hub.event"

	// ReadUpdate is the installation's own release status.
	ReadUpdate Capability = "read:hub.update"

	// AdminUpdate installs or rolls back the installation's binary. This is
	// operating the hub, not using it, which is why it lives at the
	// installation boundary where no token can reach it.
	AdminUpdate Capability = "admin:hub.update"
)

// installation lists the capabilities that exist at the hub boundary. A
// capability absent here can never be exercised with an empty boundary, so a
// new org capability does not accidentally become a platform power.
var installation = map[Capability]bool{
	AdminAccounts: true,
	ReadOrg:       true,
	ReadUpdate:    true,
	AdminUpdate:   true,
}

// orgMinimum is the weakest role that carries each capability inside an org.
//
// Reads sit at member and mutations at admin, following the project rows that
// were here first. The interactive fleet verbs — exec, attach, upload — are
// the exception at member, because that is what an authenticated caller can
// already do today and tightening the cockpit's own terminal is a separate
// change with its own UI consequences (09 keeps the question open).
var orgMinimum = map[Capability]Role{
	ReadOrg:        RoleMember,
	AdminOrg:       RoleAdmin,
	DeleteOrg:      RoleOwner,
	ReadProject:    RoleMember,
	CreateProject:  RoleAdmin,
	DeleteProject:  RoleAdmin,
	AdminProject:   RoleAdmin,
	ReadDevice:     RoleMember,
	EnrollDevice:   RoleAdmin,
	AdminDevice:    RoleAdmin,
	ExecDevice:     RoleMember,
	ReadTerminal:   RoleMember,
	AttachTerminal: RoleMember,
	ReadFile:       RoleMember,
	WriteFile:      RoleMember,
	ReadTask:       RoleMember,
	CreateTask:     RoleMember,
	ReadTemplate:   RoleMember,
	ReadPreset:     RoleMember,
	AdminPreset:    RoleAdmin,
	ReadEvent:      RoleMember,
}

// Capabilities lists every capability this hub understands, sorted.
//
// It is derived from the two maps above rather than kept beside them: a
// hand-maintained third list is how a capability ends up grantable but never
// enforced, or enforced but impossible to grant.
func Capabilities() []Capability {
	all := make([]Capability, 0, len(installation)+len(orgMinimum))
	for c := range installation {
		all = append(all, c)
	}
	for c := range orgMinimum {
		if !installation[c] {
			all = append(all, c)
		}
	}
	slices.Sort(all)
	return all
}

// GrantableScopes lists what a token may carry, sorted. Installation
// administration is absent by construction: 09 gives an API token a project
// or a tenant as its boundary, so running the hub itself is not on offer to
// a secret that was handed to a machine.
func GrantableScopes() []Capability {
	all := make([]Capability, 0, len(orgMinimum))
	for c := range orgMinimum {
		all = append(all, c)
	}
	slices.Sort(all)
	return all
}

// Dangerous marks a scope whose worst case is arbitrary code execution on
// someone else's machine. The cockpit separates these in its picker and
// leaves them unchecked; keeping the judgement here means the form cannot
// disagree with the rule it is rendering.
func Dangerous(c Capability) bool { return c == ExecDevice }

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

	// Unpartitioned marks the operator of an installation that has no
	// organizations at all: a hub claimed before accounts existed, whose
	// projects still carry an empty org_id (26's legacy path).
	//
	// Such an actor holds everything, because there is no second tenant to
	// be isolated from and refusing would lock the only administrator out of
	// their own fleet. The flag is a fact about the installation, not a
	// power: it is false the moment an organization exists, and it can only
	// be set for the platform operator.
	Unpartitioned bool
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
	// A hub with no organizations has no boundary to enforce, so its
	// operator is not refused by one. This keeps a pre-accounts self-host
	// installation working after tokens gained axes, and it evaporates as
	// soon as the hub has a tenant worth separating.
	if a.Unpartitioned && a.Platform {
		return true
	}
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

// SoleOrg is the only organization this actor belongs to, or empty when they
// belong to none or to more than one. A request that omitted org_id can
// use this on a hub that still has one organization — the self-host case
// and the first hosted claim — without inventing a "current org" on the
// actor.
func (a Actor) SoleOrg() string {
	if len(a.Orgs) != 1 {
		return ""
	}
	var id string
	for id = range a.Orgs {
	}
	return id
}

// Errors the HTTP edge maps onto status codes. Sentinels rather than strings,
// so a handler cannot drift from the rule it is reporting.
var (
	ErrForbidden   = errors.New("not allowed")
	ErrRoleUnknown = errors.New("unknown role")
	ErrNotMember   = errors.New("not a member of this organization")
	ErrOwnerOnly   = errors.New("only an owner may change an owner")
	ErrLastOwner   = errors.New("an organization must keep at least one owner")
)
