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
// id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d,
// soul_core, voice, biological_gender, created_at, updated_at.
// A missing row is (nil, nil). big_five travels as JSON text, the same
// round-trip shape skill.mcp uses for its optional config.
func scanStaff(row staffScanner) (*Staff, error) {
	var st Staff
	var bigFive string
	if err := row.Scan(&st.ID, &st.Slug, &st.Name, &st.Locale, &st.Age, &bigFive,
		&st.Brief, &st.WordBudget, &st.AvatarModel3D, &st.SoulCore, &st.Voice, &st.BiologicalGender, &st.CreatedAt, &st.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
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

// ListStaff returns the canonical staff catalogue, ordered by slug. Staff
// are org-scoped only; a box's narrator is never part of it.
func (s *Store) ListStaff() ([]Staff, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d, soul_core, voice, biological_gender, created_at, updated_at
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
	return scanStaff(s.db.QueryRow(`SELECT id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d, soul_core, voice, biological_gender, created_at, updated_at
		FROM staff WHERE id = ?`, id))
}

// ErrBoxScopedSlugRejected reports a staff slug in the box-scoped namespace
// st_b_*: staff are org-scoped only, and the one box-scoped being — a box's
// narrator — is not a staff row (58).
var ErrBoxScopedSlugRejected = errors.New("st_b_* is the narrator namespace, not a staff slug")

// validateStaffSlug enforces that a staff slug stays out of the box-scoped
// namespace. Staff are org-scoped only; a st_b_* slug names a narrator, which
// lives in box_narrator, not in the staff table.
func validateStaffSlug(slug string) error {
	if strings.HasPrefix(slug, "st_b_") {
		return fmt.Errorf("%w: slug %q is box-scoped; a box's narrator is not a staff row", ErrBoxScopedSlugRejected, slug)
	}
	return nil
}

// validateBiologicalGender admits only the two values a gender-grammatical
// language (Polish declensions) can self-inflect on: "male" or "female". The
// empty string is refused — a staff member's sex is always set (the seeds
// assign one, and the handlers require one).
func validateBiologicalGender(s string) error {
	if s == "male" || s == "female" {
		return nil
	}
	return fmt.Errorf("biological_gender must be \"male\" or \"female\", got %q", s)
}

// UpsertStaff writes a staff member keyed by slug. Staff are org-scoped only;
// an existing slug updates the row and refreshes updated_at, a new slug mints
// a `staff-` identifier. A st_b_* slug is refused — that namespace belongs to
// a box's narrator, which is not a staff row (58).
//
// The write runs in a transaction with the config_version bump it causes: a
// staff edit changes every box's manifest (bumpAllBoxes), because StaffForOrg
// serves the roster regardless of which org a box carries.
func (s *Store) UpsertStaff(slug, name, locale, avatarModel3D, brief, soulCore, voice, biologicalGender string, age, wordBudget int, bigFive Character) (*Staff, error) {
	if err := validateStaffSlug(slug); err != nil {
		return nil, err
	}
	if err := validateBiologicalGender(biologicalGender); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	st, existingID, err := upsertStaffTx(tx, slug, name, locale, avatarModel3D, brief, soulCore, voice, biologicalGender, age, wordBudget, bigFive)
	if err != nil {
		return nil, err
	}
	if err := bumpAllBoxes(tx); err != nil {
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

// upsertStaffTx is the SELECT/UPDATE/INSERT core of a staff upsert. It runs
// inside the caller's transaction: an existing key is updated and the row's
// id comes back with a nil staff — the caller re-reads it after commit, so it
// sees the committed row — while a new key mints a `staff-` identifier and
// returns the freshly built row whose fields are exactly the submitted
// values. The config_version bump the write causes is the caller's business,
// not the core's.
func upsertStaffTx(tx *store.Tx, slug, name, locale, avatarModel3D, brief, soulCore, voice, biologicalGender string, age, wordBudget int, bigFive Character) (*Staff, string, error) {
	bigFiveJSON, err := encodeBigFive(bigFive)
	if err != nil {
		return nil, "", err
	}
	var existing string
	err = tx.QueryRow(`SELECT id FROM staff WHERE slug = ?`, slug).Scan(&existing)
	if err == nil {
		if _, err = tx.Exec(`UPDATE staff SET name = ?, locale = ?, avatar_model_3d = ?, brief = ?, age = ?, word_budget = ?, soul_core = ?, voice = ?, biological_gender = ?, big_five = ?, updated_at = ?
			WHERE id = ?`, name, locale, avatarModel3D, brief, age, wordBudget, soulCore, voice, biologicalGender, bigFiveJSON, time.Now().Unix(), existing); err != nil {
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
		ID:            staffId,
		Slug:          slug,
		Name:          name,
		Locale:        locale,
		AvatarModel3D: avatarModel3D,
		Brief:         brief,
		Age:           age,
		WordBudget:    wordBudget,
		SoulCore:      soulCore,
		Voice:         voice,
		BiologicalGender: biologicalGender,
		BigFive:       bigFive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err = tx.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, avatar_model_3d, soul_core, voice, biological_gender, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.Slug, st.Name, st.Locale, st.Age, bigFiveJSON, st.Brief, st.WordBudget, st.AvatarModel3D, st.SoulCore, st.Voice, st.BiologicalGender, st.CreatedAt, st.UpdatedAt); err != nil {
		if uniqueConstraint(err) {
			return nil, "", fmt.Errorf("staff slug %q already exists: %w", slug, err)
		}
		return nil, "", err
	}
	return st, "", nil
}

// StaffForOrg returns the staff roster as one organization sees it: each
// overridable field is the org override when the override column is not NULL,
// the shared base row otherwise (override ?? base). Box-scoped narrators are
// never part of an org roster.
func (s *Store) StaffForOrg(orgID string) ([]Staff, error) {
	rows, err := s.db.Query(`SELECT st.id, st.slug, st.name, st.locale, st.age, st.big_five, st.brief, st.word_budget, st.avatar_model_3d, st.soul_core, st.voice, st.biological_gender, st.created_at, st.updated_at,
		ov.name, ov.age, ov.soul_override, ov.voice, ov.big_five, ov.brief, ov.word_budget, ov.avatar_model_3d
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

// scanStaffForOrg reads one joined staff+override row. A NULL override column
// means "inherit the base row"; a set column replaces it even when it holds a
// zero value, because NULL — not emptiness — is the inherit marker. The soul
// is the one exception: SoulCore stays the canonical staff.soul_core value
// and the override column travels separately as SoulOverride.
func scanStaffForOrg(row staffScanner) (*Staff, error) {
	var st Staff
	var baseBigFive string
	var ovName, ovSoul, ovVoice, ovBigFive, ovBrief, ovAvatarModel3D sql.NullString
	var ovAge, ovWordBudget sql.NullInt64
	if err := row.Scan(&st.ID, &st.Slug, &st.Name, &st.Locale, &st.Age, &baseBigFive,
		&st.Brief, &st.WordBudget, &st.AvatarModel3D, &st.SoulCore, &st.Voice, &st.BiologicalGender, &st.CreatedAt, &st.UpdatedAt,
		&ovName, &ovAge, &ovSoul, &ovVoice, &ovBigFive, &ovBrief, &ovWordBudget, &ovAvatarModel3D); err != nil {
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
	if ovAvatarModel3D.Valid {
		st.AvatarModel3D = ovAvatarModel3D.String
	}
	if ovWordBudget.Valid {
		st.WordBudget = int(ovWordBudget.Int64)
	}
	return &st, nil
}

// SetOrgStaffOverride writes one organization's tuning of a staff member:
// name, age, soul override, voice, BigFive, brief, avatar model and word
// budget. A nil pointer leaves the column NULL ("inherit the base row"); the
// upsert replaces a previous override in place. An override reaches the
// manifest of every box bound to the org, so the write and the config_version
// bump on those boxes commit together.
func (s *Store) SetOrgStaffOverride(orgID, staffID string, name *string, age *int, soulOverride, voice *string, bigFive *Character, brief, avatarModel3D *string, wordBudget *int) error {
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
	if _, err := tx.Exec(`INSERT INTO org_staff_overrides (org_id, staff_id, name, age, soul_override, voice, big_five, brief, word_budget, avatar_model_3d)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(org_id, staff_id) DO UPDATE SET
			name = excluded.name,
			age = excluded.age,
			soul_override = excluded.soul_override,
			voice = excluded.voice,
			big_five = excluded.big_five,
			brief = excluded.brief,
			word_budget = excluded.word_budget,
			avatar_model_3d = excluded.avatar_model_3d`,
		orgID, staffID, nullableString(name), nullableInt(age), nullableString(soulOverride), nullableString(voice), bigFiveJSON, nullableString(brief), nullableInt(wordBudget), nullableString(avatarModel3D)); err != nil {
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
// placeholders: the real age and avatar model are product content that may
// replace them later; the seed only guarantees the two baseline members
// exist.
type seedStaff struct {
	slug             string
	name             string
	locale           string
	age              int
	avatarModel3D    string
	voice            string
	biologicalGender string
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
		{slug: "staff-male-00", name: "Adam", locale: "pl", age: 35, voice: "pl_PL-mc_speech-medium", biologicalGender: "male"},
		{slug: "staff-female-00", name: "Ewa", locale: "pl", age: 32, avatarModel3D: "arianna.glb", voice: "pl_PL-gosia-medium", biologicalGender: "female"},
	} {
		var existing string
		err := s.db.QueryRow(`SELECT id FROM staff WHERE slug = ?`, sd.slug).Scan(&existing)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.UpsertStaff(sd.slug, sd.name, sd.locale, sd.avatarModel3D, "", "", sd.voice, sd.biologicalGender, sd.age, 0, neutralCharacter()); err != nil {
			return err
		}
	}
	return nil
}
