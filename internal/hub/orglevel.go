package hub

import (
	"fmt"
	"strings"
)

// OrgLevel is how much of the box's ceiling an org's members may use: the
// box's edition is the ceiling, the org's level subtracts from it (08). A
// firm's org and a family member's org carry the same field — every org is
// the same bundle (57).
//
// The empty string is the full level — the zero value, so a fresh org, an
// org created before the column existed, and an org nobody has restricted
// all behave the same way.
type OrgLevel string

const (
	// OrgLevelFull is an unrestricted org: its members get the whole box
	// ceiling the edition allows.
	OrgLevelFull OrgLevel = ""
	// OrgLevelChild subtracts from the ceiling: a young person's org — no
	// purchases, no third-party connectors, staff-led gates. The exact
	// subtraction matrix per edition is 08's open item; the tier itself is
	// the box's contract.
	OrgLevelChild OrgLevel = "child"
)

// ParseOrgLevel accepts "full" and "child", plus the empty string, trimmed
// and case-insensitive. Anything else is refused so a typo cannot silently
// drop an org back into full.
func ParseOrgLevel(s string) (OrgLevel, error) {
	switch l := OrgLevel(strings.ToLower(strings.TrimSpace(s))); l {
	case OrgLevelFull, "full":
		return OrgLevelFull, nil
	case OrgLevelChild:
		return OrgLevelChild, nil
	default:
		return OrgLevelFull, fmt.Errorf("org level %q: want full or child", s)
	}
}
