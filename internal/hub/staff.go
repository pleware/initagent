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
// Staff is org-scoped only. A box's narrator is not a Staff row — it is the
// box's own 1:1 being, held by the box_narrator table (see Narrator) — so
// staff members never carry a box identity.
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
	// BiologicalGender is the staff member's sex — "male" or "female", the
	// two values a gender-grammatical language (Polish declensions) needs to
	// self-inflect. Always set: validateBiologicalGender refuses anything else,
	// including the empty string (a migrated row may read "" until its next
	// edit, but no new write may leave it unset).
	BiologicalGender string `json:"biologicalGender"`
	CreatedAt     int64     `json:"createdAt"`
	UpdatedAt     int64     `json:"updatedAt"`
}

// narratorSlug is the fixed slug of every box's narrator — the one being that
// is the box itself (58). Unlike staff slugs it is never stored as a unique
// key: a narrator is 1:1 with its box, so the box's id is its identity and
// the slug is a constant marker the manifest emits.
const narratorSlug = "st_b_pi"

// Narrator is a box's own voice — the box's 1:1 being, its property rather
// than an installation-scoped staff member (58). It has no id of its own:
// box_narrator.box_id is the primary key, so the box's identity is the
// narrator's identity. It always carries the fixed slug st_b_pi and never
// appears in an org roster.
type Narrator struct {
	Slug             string    `json:"slug"`
	Name             string    `json:"name"`
	Locale           string    `json:"locale"`
	AvatarModel3D    string    `json:"avatarModel3d"`
	BigFive          Character `json:"bigFive"`
	Brief            string    `json:"brief"`
	Age              int       `json:"age"`
	WordBudget       int       `json:"wordBudget"`
	SoulCore         string    `json:"soulCore"`
	Voice            string    `json:"voice"`
	BiologicalGender string    `json:"biologicalGender"`
	CreatedAt        int64     `json:"createdAt"`
	UpdatedAt        int64     `json:"updatedAt"`
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
