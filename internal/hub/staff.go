package hub

// Character is the Big Five (OCEAN) personality profile of a staff member.
// Each trait is a 0..1 float; the zero Character is a valid blank profile,
// which is what the staff table's `{}` default round-trips to.
type Character struct {
	Openness          float64 `json:"openness"`
	Conscientiousness float64 `json:"conscientiousness"`
	Extraversion      float64 `json:"extraversion"`
	Agreeableness     float64 `json:"agreeableness"`
	Neuroticism       float64 `json:"neuroticism"`
}

// Staff is a named synthetic teammate of a hub (`initagent.hub.staff`). It is
// installation-scoped, like a skill: the org_staff_overrides table carries
// per-organization differences on top of the shared base row. BigFive is
// stored as JSON text in staff.big_five, the same round-trip shape skill.mcp
// uses for its optional config.
//
// Scope splits the table in two: org-scoped members are the roster every
// organization inherits and overrides, box-scoped members (Scope "box",
// BoxID set) are a box's narrator — the being whose clones are that box's
// running gateways (58). Box-scoped rows have no org override.
//
// SoulCore is always the canonical staff.soul_core value. An org roster
// (StaffForOrg) additionally carries the org's own text in SoulOverride —
// the two are separate fields, never merged; an org with no override has
// SoulOverride empty.
type Staff struct {
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	Name          string    `json:"name"`
	Locale        string    `json:"locale"`
	AvatarModel3D string    `json:"avatarModel3d"`
	BigFive       Character `json:"bigFive"`
	Brief         string    `json:"brief"`
	Age           int       `json:"age"`
	WordBudget    int       `json:"wordBudget"`
	SoulCore      string    `json:"soulCore"`
	SoulOverride  string    `json:"soulOverride,omitempty"`
	Voice         string    `json:"voice"`
	Scope         string    `json:"scope"`
	BoxID         string    `json:"boxId,omitempty"`
	CreatedAt     int64     `json:"createdAt"`
	UpdatedAt     int64     `json:"updatedAt"`
}

// OrgStaffOverride is one organization's tuning of a staff member. Every
// field except the key pair is optional: NULL means "inherit the staff row",
// so an override holding only an avatar model keeps the base BigFive and brief.
type OrgStaffOverride struct {
	OrgID         string     `json:"orgId"`
	StaffID       string     `json:"staffId"`
	Name          *string    `json:"name,omitempty"`
	Age           *int       `json:"age,omitempty"`
	SoulOverride  *string    `json:"soulOverride,omitempty"`
	Voice         *string    `json:"voice,omitempty"`
	BigFive       *Character `json:"bigFive,omitempty"`
	Brief         *string    `json:"brief,omitempty"`
	WordBudget    *int       `json:"wordBudget,omitempty"`
	AvatarModel3D *string    `json:"avatarModel3d,omitempty"`
}
