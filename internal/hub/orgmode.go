package hub

import (
	"fmt"
	"strings"
)

// OrgMode is how an organization behaves on a hosted hub. It decides plan
// limits and which Stripe environment (live or test) its checkout runs on.
// Self-host never reads it: caps are unlimited and billing is absent there.
//
// The empty string is the standard mode — a normal, billed customer. It is
// the zero value so a fresh org, an org created before the column existed,
// and an org an operator has not touched all behave the same way.
type OrgMode string

const (
	// OrgModeStandard is a normal customer: plan limits apply, checkout runs
	// on Stripe live, and a paid invoice mints a KSeF fiscal invoice.
	OrgModeStandard OrgMode = ""
	// OrgModeTest lifts plan limits so an operator can run an unbilled trial
	// against the same catalogue without touching plan rows.
	OrgModeTest OrgMode = "test"
	// OrgModeDevelop keeps plan limits but pays on Stripe test and mints no
	// KSeF fiscal invoice — the full wall + checkout flow with fake money.
	OrgModeDevelop OrgMode = "develop"
)

// ParseOrgMode accepts the three named modes and the empty string, trimmed
// and case-insensitive. Anything else is refused so a typo cannot silently
// drop an org back into standard mode.
func ParseOrgMode(s string) (OrgMode, error) {
	switch m := OrgMode(strings.ToLower(strings.TrimSpace(s))); m {
	case OrgModeStandard, "standard":
		return OrgModeStandard, nil
	case OrgModeTest:
		return OrgModeTest, nil
	case OrgModeDevelop:
		return OrgModeDevelop, nil
	default:
		return OrgModeStandard, fmt.Errorf("org mode %q: want standard, test or develop", s)
	}
}

// isTest reports whether this org is exempt from plan limits.
func (m OrgMode) isTest() bool { return m == OrgModeTest }

// isDevelop reports whether this org pays on Stripe test and mints no KSeF
// invoice.
func (m OrgMode) isDevelop() bool { return m == OrgModeDevelop }

// OrgStatus is whether an organization may use the hub. The empty string is
// the active status — the zero value, so a fresh org, an org created before
// the column existed, and an org an operator has not touched all behave the
// same way. The store carries the flag; the gate that blocks a suspended org
// reads it.
type OrgStatus string

const (
	// OrgStatusActive is a normal organization that may use the hub.
	OrgStatusActive OrgStatus = ""
	// OrgStatusSuspended blocks an organization from using the hub until an
	// operator reactivates it.
	OrgStatusSuspended OrgStatus = "suspended"
)

// ParseOrgStatus accepts the empty string, "active", and "suspended",
// trimmed and case-insensitive. Anything else is refused so a typo cannot
// silently drop an org back into active.
func ParseOrgStatus(s string) (OrgStatus, error) {
	switch st := OrgStatus(strings.ToLower(strings.TrimSpace(s))); st {
	case OrgStatusActive, "active":
		return OrgStatusActive, nil
	case OrgStatusSuspended:
		return OrgStatusSuspended, nil
	default:
		return OrgStatusActive, fmt.Errorf("org status %q: want active or suspended", s)
	}
}
