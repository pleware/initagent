// Package orgplan is the hosted organization-plan catalogue (drafts 48, 25, 26).
//
// A plan is a closed entitlement id stored on the org, not a minted prefix
// and not hub offering (hosted vs selfhost). Theme family "enterprise" and
// plan id "enterprise" are separate words that happen to match (17).
package orgplan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pleware/initagent/internal/offering"
)

// ID is a stable catalogue key stored on the org row. It is not shown as a
// Stripe Price id and it is not a theme family.
type ID string

const (
	Free       ID = "free"
	Starter    ID = "starter"
	Team       ID = "team"
	Enterprise ID = "enterprise"
)

// PersonUSD is the advertised monthly USD per person on self-serve paid
// plans. It is not a Stripe Price id and it is not a price per machine.
const PersonUSD = 5

// ChargeKind is how this plan is sold. It is not an offering and not a theme.
type ChargeKind string

const (
	ChargeFree    ChargeKind = "free"
	ChargeUSD     ChargeKind = "usd"
	ChargeContact ChargeKind = "contact"
)

// Charge is the advertised sale motion. Stripe Product/Price ids stay in ops.
// PerPerson means USD is billed per org member (a person). Never per
// enrolled machine — workers stay the customer's hardware (01).
type Charge struct {
	Kind      ChargeKind `json:"kind"`
	USD       int        `json:"usd"`
	PerPerson bool       `json:"perPerson"`
}

// ThemeFamily is a cockpit skin id from draft 17. It is not a plan id.
type ThemeFamily string

const (
	ThemeDefault    ThemeFamily = "default"
	ThemeEnterprise ThemeFamily = "enterprise"
)

// Limits mix two scopes. Projects and People are per organization.
// WorkersPerProject is how many of the customer's machines one project may
// enroll. A field of 0 means no cap from this catalogue.
type Limits struct {
	Projects          int `json:"projects"`
	WorkersPerProject int `json:"workersPerProject"`
	People            int `json:"people"`
	IdleDays          int `json:"idleDays"`
	LogDays           int `json:"logDays"`
}

// Unlimited is what self-host (and any caller that ignores offering) sees.
var Unlimited Limits

// Plan is one shipped organization plan.
type Plan struct {
	ID          ID          `json:"id"`
	Label       string      `json:"label"`
	SelfServe   bool        `json:"selfServe"`
	Charge      Charge      `json:"charge"`
	ThemeFamily ThemeFamily `json:"themeFamily"`
	Limits      Limits      `json:"limits"`
}

var catalogue = []Plan{
	{
		ID:          Free,
		Label:       "Free",
		SelfServe:   true,
		Charge:      Charge{Kind: ChargeFree},
		ThemeFamily: ThemeDefault,
		Limits: Limits{
			Projects:          1,
			WorkersPerProject: 2,
			People:            1,
			IdleDays:          60,
			LogDays:           7,
		},
	},
	{
		ID:          Starter,
		Label:       "Starter",
		SelfServe:   true,
		Charge:      Charge{Kind: ChargeUSD, USD: PersonUSD, PerPerson: true},
		ThemeFamily: ThemeDefault,
		Limits: Limits{
			Projects:          2,
			WorkersPerProject: 3,
		},
	},
	{
		ID:          Team,
		Label:       "Team",
		SelfServe:   true,
		Charge:      Charge{Kind: ChargeUSD, USD: PersonUSD, PerPerson: true},
		ThemeFamily: ThemeDefault,
		Limits: Limits{
			Projects:          5,
			WorkersPerProject: 5,
		},
	},
	{
		ID:          Enterprise,
		Label:       "Enterprise",
		SelfServe:   false,
		Charge:      Charge{Kind: ChargeContact},
		ThemeFamily: ThemeEnterprise,
		// Limits stay 0: no cap. Machines are the customer's, running PWare.
	},
}

// Catalogue returns every shipped plan in signup order.
func Catalogue() []Plan {
	return slices.Clone(catalogue)
}

// Lookup finds a plan by its catalogue id.
func Lookup(id string) (Plan, bool) {
	i := slices.IndexFunc(catalogue, func(p Plan) bool {
		return string(p.ID) == id
	})
	if i < 0 {
		return Plan{}, false
	}
	return catalogue[i], true
}

// Default is the hosted signup plan.
func Default() Plan {
	p, _ := Lookup(string(Free))
	return p
}

// Parse accepts one catalogue id, trimmed, case-insensitive.
func Parse(s string) (ID, error) {
	got := ID(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := Lookup(string(got)); ok {
		return got, nil
	}
	if got == "" {
		return "", fmt.Errorf("orgplan: empty")
	}
	return "", fmt.Errorf("orgplan %q: unknown", s)
}

// SelfServe reports whether a customer can take this plan without talking
// to sales (signup for free, checkout for paid). Enterprise is contact-sales.
func SelfServe(id string) bool {
	p, ok := Lookup(id)
	return ok && p.SelfServe
}

// ContactSales reports whether this plan is sold only by talking to us.
func ContactSales(id string) bool {
	p, ok := Lookup(id)
	return ok && p.Charge.Kind == ChargeContact
}

// Caps returns the walls that apply for this installation and org plan.
// Self-host is unlimited. An unknown hosted id fails closed to free.
func Caps(kind offering.Kind, id ID) Limits {
	if kind != offering.Hosted {
		return Unlimited
	}
	p, ok := Lookup(string(id))
	if !ok {
		return Default().Limits
	}
	return p.Limits
}

// Allows reports whether count is within limit. A limit of 0 means no cap.
func Allows(count, limit int) bool {
	return limit == 0 || count <= limit
}

// AllowsAnother reports whether adding one more stays within limit.
func AllowsAnother(count, limit int) bool {
	return limit == 0 || count < limit
}
