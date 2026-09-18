package hub

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/id"
	"github.com/pleware/initagent/internal/store"
)

// --- staff ---

// staffScanner is the shared shape of sql.Row and sql.Rows Scan methods,
// the same role skillScanner plays for the skills table.
type staffScanner interface {
	Scan(dest ...any) error
}

// scanStaff reads one staff row selected in schema order:
// id, slug, name, locale, age, big_five, brief, word_budget, model,
// soul_core, voice, scope, box_id, created_at, updated_at. A missing row is
// (nil, nil). big_five travels as JSON text, the same round-trip shape
// skill.mcp uses for its optional config; box_id is NULL on org-scoped rows.
func scanStaff(row staffScanner) (*Staff, error) {
	var st Staff
	var bigFive string
	var boxID sql.NullString
	if err := row.Scan(&st.ID, &st.Slug, &st.Name, &st.Locale, &st.Age, &bigFive,
		&st.Brief, &st.WordBudget, &st.Model, &st.SoulCore, &st.Voice, &st.Scope, &boxID, &st.CreatedAt, &st.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	st.BoxID = boxID.String
	if bigFive != "" {
		if err := json.Unmarshal([]byte(bigFive), &st.BigFive); err != nil {
			return nil, fmt.Errorf("staff: decode big_five: %w", err)
		}
	}
	return &st, nil
}

// encodeBigFive turns a personality profile into its column form. The column
// is NOT NULL, so the zero profile still writes "{}" rather than "".
func encodeBigFive(c Character) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ListStaff returns every staff member on this installation, ordered by slug.
func (s *Store) ListStaff() ([]Staff, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, locale, age, big_five, brief, word_budget, model, soul_core, voice, scope, box_id, created_at, updated_at
		FROM staff ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Staff{}
	for rows.Next() {
		st, err := scanStaff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

// StaffById looks up one staff member. A missing one is (nil, nil).
func (s *Store) StaffById(id string) (*Staff, error) {
	return scanStaff(s.db.QueryRow(`SELECT id, slug, name, locale, age, big_five, brief, word_budget, model, soul_core, voice, scope, box_id, created_at, updated_at
		FROM staff WHERE id = ?`, id))
}

// ErrStaffScopeMismatch reports an UpsertStaff whose slug and scope disagree:
// a st_b_* slug names a box-scoped narrator, every other slug an org-scoped
// member, and the two cannot be mixed (58).
var ErrStaffScopeMismatch = errors.New("staff scope does not match its slug")

// validateStaffScope enforces the slug↔scope agreement the store insists on,
// not only the HTTP layer: the scope is part of the slug, so a st_b_* row
// must be box-scoped and carry the box, while staff-*, sto_* and every other
// slug must be org-scoped with no box.
func validateStaffScope(slug, scope, boxID string) error {
	if strings.HasPrefix(slug, "st_b_") {
		if scope != "box" || boxID == "" {
			return fmt.Errorf("%w: slug %q is box-scoped; want scope %q and a non-empty box_id, got %q and %q",
				ErrStaffScopeMismatch, slug, "box", scope, boxID)
		}
		return nil
	}
	if scope != "org" || boxID != "" {
		return fmt.Errorf("%w: slug %q is org-scoped; want scope %q and an empty box_id, got %q and %q",
			ErrStaffScopeMismatch, slug, "org", scope, boxID)
	}
	return nil
}

// UpsertStaff writes a staff member keyed by scope-aware slug: an org-scoped
// slug updates the row carrying that slug, a box-scoped slug updates the row
// carrying that slug inside that box (58). An existing key updates the row
// and refreshes updated_at, a new key mints a `staff-` identifier. scope and
// boxID are validated against the slug convention: a st_b_* slug requires
// scope "box" and the box id, any other slug scope "org" and no box.
//
// The write runs in a transaction with the config_version bump it causes:
// an org-scoped write changes every box's manifest (bumpAllBoxes), while a
// box-scoped write — the narrator — changes only that box and does not bump,
// because a fresh box already starts at version 1 (CreateBox seeds it).
func (s *Store) UpsertStaff(slug, name, locale, model, brief, soulCore, voice, scope, boxID string, age, wordBudget int, bigFive Character) (*Staff, error) {
	if err := validateStaffScope(slug, scope, boxID); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	st, existingID, err := upsertStaffTx(tx, slug, name, locale, model, brief, soulCore, voice, scope, boxID, age, wordBudget, bigFive)
	if err != nil {
		return nil, err
	}
	if scope == "org" {
		if err := bumpAllBoxes(tx); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if st != nil {
		return st, nil
	}
	return s.StaffById(existingID)
}

// upsertStaffTx is the SELECT/UPDATE/INSERT core of a staff upsert, shared
// by UpsertStaff and UpdateBoxNarrator. It runs inside the caller's
// transaction: an existing key is updated and the row's id comes back with
// a nil staff — the caller re-reads it after commit, so it sees the
// committed row — while a new key mints a `staff-` identifier and returns
// the freshly built row whose fields are exactly the submitted values. The
// config_version bump the write causes is the caller's business, not the
// core's.
func upsertStaffTx(tx *store.Tx, slug, name, locale, model, brief, soulCore, voice, scope, boxID string, age, wordBudget int, bigFive Character) (*Staff, string, error) {
	bigFiveJSON, err := encodeBigFive(bigFive)
	if err != nil {
		return nil, "", err
	}
	var existing string
	err = tx.QueryRow(`SELECT id FROM staff WHERE slug = ? AND COALESCE(box_id, '') = COALESCE(?, '')`, slug, boxID).Scan(&existing)
	if err == nil {
		if _, err = tx.Exec(`UPDATE staff SET name = ?, locale = ?, model = ?, brief = ?, age = ?, word_budget = ?, soul_core = ?, voice = ?, big_five = ?, scope = ?, box_id = ?, updated_at = ?
			WHERE id = ?`, name, locale, model, brief, age, wordBudget, soulCore, voice, bigFiveJSON, scope, boxID, time.Now().Unix(), existing); err != nil {
			return nil, "", err
		}
		return nil, existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}

	staffId, err := id.New(id.Staff)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().Unix()
	st := &Staff{
		ID:         staffId,
		Slug:       slug,
		Name:       name,
		Locale:     locale,
		Model:      model,
		Brief:      brief,
		Age:        age,
		WordBudget: wordBudget,
		SoulCore:   soulCore,
		Voice:      voice,
		Scope:      scope,
		BoxID:      boxID,
		BigFive:    bigFive,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if _, err = tx.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, model, soul_core, voice, scope, box_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.Slug, st.Name, st.Locale, st.Age, bigFiveJSON, st.Brief, st.WordBudget, st.Model, st.SoulCore, st.Voice, st.Scope, st.BoxID, st.CreatedAt, st.UpdatedAt); err != nil {
		if uniqueConstraint(err) {
			return nil, "", fmt.Errorf("staff slug %q already exists: %w", slug, err)
		}
		return nil, "", err
	}
	return st, "", nil
}

// UpdateBoxNarrator writes the narrator staff row of one box — the
// box-scoped "Data" (st_b_dt) — and bumps the box's config_version in the
// same transaction, so a connector's next sync picks the edited narrator
// up. CreateBox seeds the row, so the normal path is an update; the upsert
// core also creates the row when it is missing. The bump is single-box
// (bumpBoxConfig), unlike UpsertStaff's org-scoped bumpAllBoxes: an edit
// changes only this box's manifest.
func (s *Store) UpdateBoxNarrator(boxID, name, locale, model, brief, soulCore, voice string, age, wordBudget int, bigFive Character) (*Staff, error) {
	if err := validateStaffScope("st_b_dt", "box", boxID); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	st, existingID, err := upsertStaffTx(tx, "st_b_dt", name, locale, model, brief, soulCore, voice, "box", boxID, age, wordBudget, bigFive)
	if err != nil {
		return nil, err
	}
	if err := bumpBoxConfig(tx, boxID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if st != nil {
		return st, nil
	}
	return s.StaffById(existingID)
}

// StaffForOrg returns the staff roster as one organization sees it: each
// overridable field is the org override when the override column is not NULL,
// the shared base row otherwise (override ?? base). Box-scoped narrators are
// never part of an org roster.
func (s *Store) StaffForOrg(orgID string) ([]Staff, error) {
	rows, err := s.db.Query(`SELECT st.id, st.slug, st.name, st.locale, st.age, st.big_five, st.brief, st.word_budget, st.model, st.soul_core, st.voice, st.scope, st.created_at, st.updated_at,
		ov.name, ov.age, ov.soul_override, ov.voice, ov.big_five, ov.brief, ov.word_budget, ov.model
		FROM staff st
		LEFT JOIN org_staff_overrides ov ON ov.org_id = ? AND ov.staff_id = st.id
		WHERE st.scope = 'org'
		ORDER BY st.slug`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Staff{}
	for rows.Next() {
		st, err := scanStaffForOrg(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

// StaffForBox returns the box-scoped staff rows attached to one box — the
// narrator roster a box syncs down (58). Box-scoped staff carries no org
// override, so this is a plain select with no join; org-scoped members are
// never listed for a box.
func (s *Store) StaffForBox(boxID string) ([]Staff, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, locale, age, big_five, brief, word_budget, model, soul_core, voice, scope, box_id, created_at, updated_at
		FROM staff WHERE scope = 'box' AND box_id = ? ORDER BY slug`, boxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Staff{}
	for rows.Next() {
		st, err := scanStaff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

// scanStaffForOrg reads one joined staff+override row. A NULL override column
// means "inherit the base row"; a set column replaces it even when it holds a
// zero value, because NULL — not emptiness — is the inherit marker. The soul
// is the one exception: SoulCore stays the canonical staff.soul_core value
// and the override column travels separately as SoulOverride.
func scanStaffForOrg(row staffScanner) (*Staff, error) {
	var st Staff
	var baseBigFive string
	var ovName, ovSoul, ovVoice, ovBigFive, ovBrief, ovModel sql.NullString
	var ovAge, ovWordBudget sql.NullInt64
	if err := row.Scan(&st.ID, &st.Slug, &st.Name, &st.Locale, &st.Age, &baseBigFive,
		&st.Brief, &st.WordBudget, &st.Model, &st.SoulCore, &st.Voice, &st.Scope, &st.CreatedAt, &st.UpdatedAt,
		&ovName, &ovAge, &ovSoul, &ovVoice, &ovBigFive, &ovBrief, &ovWordBudget, &ovModel); err != nil {
		return nil, err
	}
	bigFiveJSON := baseBigFive
	if ovBigFive.Valid {
		bigFiveJSON = ovBigFive.String
	}
	if bigFiveJSON != "" {
		if err := json.Unmarshal([]byte(bigFiveJSON), &st.BigFive); err != nil {
			return nil, fmt.Errorf("staff: decode big_five: %w", err)
		}
	}
	if ovName.Valid {
		st.Name = ovName.String
	}
	if ovAge.Valid {
		st.Age = int(ovAge.Int64)
	}
	if ovSoul.Valid {
		st.SoulOverride = ovSoul.String
	}
	if ovVoice.Valid {
		st.Voice = ovVoice.String
	}
	if ovBrief.Valid {
		st.Brief = ovBrief.String
	}
	if ovModel.Valid {
		st.Model = ovModel.String
	}
	if ovWordBudget.Valid {
		st.WordBudget = int(ovWordBudget.Int64)
	}
	return &st, nil
}

// SetOrgStaffOverride writes one organization's tuning of a staff member:
// name, age, soul override, voice, BigFive, brief, model and word budget. A
// nil pointer leaves the column NULL ("inherit the base row"); the upsert
// replaces a previous override in place. An override reaches the manifest
// of every box bound to the org, so the write and the config_version bump
// on those boxes commit together.
func (s *Store) SetOrgStaffOverride(orgID, staffID string, name *string, age *int, soulOverride, voice *string, bigFive *Character, brief, model *string, wordBudget *int) error {
	var bigFiveJSON any
	if bigFive != nil {
		b, err := json.Marshal(bigFive)
		if err != nil {
			return err
		}
		bigFiveJSON = string(b)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO org_staff_overrides (org_id, staff_id, name, age, soul_override, voice, big_five, brief, word_budget, model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(org_id, staff_id) DO UPDATE SET
			name = excluded.name,
			age = excluded.age,
			soul_override = excluded.soul_override,
			voice = excluded.voice,
			big_five = excluded.big_five,
			brief = excluded.brief,
			word_budget = excluded.word_budget,
			model = excluded.model`,
		orgID, staffID, nullableString(name), nullableInt(age), nullableString(soulOverride), nullableString(voice), bigFiveJSON, nullableString(brief), nullableInt(wordBudget), nullableString(model)); err != nil {
		return err
	}
	if err := bumpConfigForOrg(tx, orgID); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearOrgStaffOverride drops one organization's tuning of a staff member.
// Clearing a missing override is not an error. Like the write, the clear
// bumps the config_version of the boxes bound to the org in the same
// transaction.
func (s *Store) ClearOrgStaffOverride(orgID, staffID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM org_staff_overrides WHERE org_id = ? AND staff_id = ?`, orgID, staffID); err != nil {
		return err
	}
	if err := bumpConfigForOrg(tx, orgID); err != nil {
		return err
	}
	return tx.Commit()
}

func nullableString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// seedStaff is one member of the baseline staff catalogue. Values are
// placeholders: the real age and model are product content that may replace
// them later; the seed only guarantees the two baseline members exist.
type seedStaff struct {
	slug   string
	name   string
	locale string
	age    int
	model  string
	voice  string
}

// neutralCharacter is the baseline Big Five profile a seeded staff member
// starts with: mid-scale on every trait until product content replaces it.
func neutralCharacter() Character {
	return Character{
		Openness:          0.5,
		Conscientiousness: 0.5,
		Extraversion:      0.5,
		Agreeableness:     0.5,
		Neuroticism:       0.5,
	}
}

// EnsureSeedStaff creates the two baseline staff members when they are
// missing and leaves them alone otherwise, so product content written over
// the seeds survives a restart. Idempotent.
func (s *Store) EnsureSeedStaff() error {
	for _, sd := range []seedStaff{
		{slug: "staff-male-00", name: "Adam", locale: "pl", age: 35, voice: "pl_PL-mc_speech-medium"},
		{slug: "staff-female-00", name: "Ewa", locale: "pl", age: 32, model: "arianna.glb", voice: "pl_PL-gosia-medium"},
	} {
		var existing string
		err := s.db.QueryRow(`SELECT id FROM staff WHERE slug = ?`, sd.slug).Scan(&existing)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.UpsertStaff(sd.slug, sd.name, sd.locale, sd.model, "", "", sd.voice, "org", "", sd.age, 0, neutralCharacter()); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSeedBoxNarrator creates the narrator staff row of a box — the
// box-scoped "Data" (st_b_dt) — when it is missing and leaves it alone
// otherwise, so content written over the seed survives a restart.
// Idempotent.
//
// The existence check keys on the box, not on the slug: slug uniqueness is
// per scope (staff(slug) for org, staff(box_id, slug) for box), so every box
// carries its own st_b_dt and UpsertStaff updates the one this box owns (58).
func (s *Store) EnsureSeedBoxNarrator(boxID string) error {
	var existing string
	err := s.db.QueryRow(`SELECT id FROM staff WHERE scope = 'box' AND box_id = ?`, boxID).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.UpsertStaff("st_b_dt", "Data", "pl", "", "", "", "pl_PL-mc_speech-medium", "box", boxID, 0, 0, neutralCharacter())
	return err
}
