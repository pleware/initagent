package authz

import (
	"errors"
	"slices"
	"strings"
)

// Grant is the reach of one issued secret, which is deliberately narrower
// than the person who issued it.
//
// Draft 09 requires three axes on every credential: a subject, a boundary and
// a verb set. The subject lives on the Actor — a token names the `acc-` it
// acts as — and the other two live here. A credential scoped to verbs but not
// to a boundary is still fleet-wide execution with extra steps, which is the
// mistake this type exists to make unrepresentable.
type Grant struct {
	// Org is the tenant this secret may act in. A zero Grant reaches nothing,
	// because an empty boundary matches no target below; omitting the field
	// cannot accidentally mean "everywhere".
	Org string

	// Project narrows the boundary to a single `prj-`. Empty means every
	// project in Org, which is what a tenant-wide token looks like.
	Project string

	// Scopes are the verbs, in the `verb:entity` grammar of 05. There is no
	// wildcard: a wildcard is how a scope set stops being read before it is
	// honoured.
	Scopes []Capability
}

// Credential is who is asking together with how much of what they can do this
// particular secret carries.
//
// The hub resolves one at the edge and asks it every question, so no handler
// re-reads a cookie or a bearer and no handler has to remember which of the
// two it is holding.
type Credential struct {
	Actor Actor

	// Grant is nil for a browser session. That is not a missing boundary: a
	// person is present, their role is the whole answer, and inventing a
	// boundary for them would be a second permission path for the same
	// screen. A token always carries one.
	Grant *Grant
}

// Can reports whether this credential may exercise c against a target
// organization and project.
//
// The three axes are an **intersection, never a union**. The account must
// hold the capability through its role, the token must list it, and the
// token's boundary must contain the target. Two properties follow, and both
// are the reason to write it this way:
//
//   - a token can never do more than the person who minted it, so reviewing
//     a token means reviewing one row rather than reasoning about a chain;
//   - removing someone from an organization collapses the reach of every
//     token they ever issued, with no sweep and no revocation list to forget.
func (c Credential) Can(cap Capability, org, project string) bool {
	if !c.Actor.Can(cap, org) {
		return false
	}
	if c.Grant == nil {
		return true
	}
	return c.Grant.allows(cap, org, project)
}

// Scoped reports whether this credential is a token rather than a session.
// The hub uses it for the surfaces that stay session-only on purpose —
// minting a token, and changing the password of the account behind it —
// where admitting a token would let a narrow secret widen itself.
func (c Credential) Scoped() bool { return c.Grant != nil }

// allows is the token half of the decision: the verb, then the boundary.
func (g *Grant) allows(c Capability, org, project string) bool {
	if !slices.Contains(g.Scopes, c) {
		return false
	}
	return g.Contains(org, project)
}

// Contains reports whether this grant's boundary covers a target.
//
// Exported so a refusal can say which axis failed. "This token is missing
// the exec:fleet.device scope" and "this token belongs to another project"
// send an operator to different places, and a single 403 sends them nowhere.
func (g *Grant) Contains(org, project string) bool {
	// The installation is not a tenant. 09 gives an API token a project or a
	// tenant, so hub-wide administration stays with the person in front of a
	// browser and a leaked token cannot reach it.
	if org == "" {
		return false
	}
	if g.Org != org {
		return false
	}
	if g.Project == "" {
		return true
	}
	// A project-scoped token asked to act org-wide is refused rather than
	// widened. Callers that legitimately list across a boundary narrow the
	// query to g.Project and ask again naming it, so a forgotten filter
	// fails closed instead of leaking the rest of the tenant.
	return g.Project == project
}

// ErrScopeUnknown is returned for a scope name this hub does not enforce.
var ErrScopeUnknown = errors.New("unknown scope")

// ParseScopes reads a stored scope list.
//
// An unrecognised name is an error rather than a silent drop. A row written
// by a newer hub, or edited by hand, must not end up authorising something
// different from what it says — in either direction.
func ParseScopes(s string) ([]Capability, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, nil
	}
	grantable := GrantableScopes()
	out := make([]Capability, 0, len(fields))
	for _, f := range fields {
		c := Capability(f)
		if !slices.Contains(grantable, c) {
			return nil, ErrScopeUnknown
		}
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out, nil
}

// FormatScopes renders a scope set for storage: sorted, deduplicated, space
// separated. One grant has one representation, so two rows that permit the
// same thing compare equal and a diff of a token list is readable.
func FormatScopes(cs []Capability) string {
	out := make([]Capability, 0, len(cs))
	for _, c := range cs {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	parts := make([]string, len(out))
	for i, c := range out {
		parts[i] = string(c)
	}
	return strings.Join(parts, " ")
}
