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
type Staff struct {
	ID         string    `json:"id"`
	Slug       string    `json:"slug"`
	Name       string    `json:"name"`
	Locale     string    `json:"locale"`
	Model      string    `json:"model"`
	BigFive    Character `json:"bigFive"`
	Brief      string    `json:"brief"`
	Age        int       `json:"age"`
	WordBudget int       `json:"wordBudget"`
	CreatedAt  int64     `json:"createdAt"`
	UpdatedAt  int64     `json:"updatedAt"`
}

// OrgStaffOverride is one organization's tuning of a staff member. Every
// field except the key pair is optional: NULL means "inherit the staff row",
// so an override holding only a model keeps the base BigFive and brief.
type OrgStaffOverride struct {
	OrgID      string     `json:"orgId"`
	StaffID    string     `json:"staffId"`
	BigFive    *Character `json:"bigFive,omitempty"`
	Brief      *string    `json:"brief,omitempty"`
	WordBudget *int       `json:"wordBudget,omitempty"`
	Model      *string    `json:"model,omitempty"`
}
