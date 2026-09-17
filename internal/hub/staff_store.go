package hub

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pleware/initagent/internal/id"
)

// --- staff ---

// staffScanner is the shared shape of sql.Row and sql.Rows Scan methods,
// the same role skillScanner plays for the skills table.
type staffScanner interface {
	Scan(dest ...any) error
}

// scanStaff reads one staff row selected in schema order:
// id, slug, name, locale, age, big_five, brief, word_budget, model,
// created_at, updated_at. A missing row is (nil, nil). big_five travels as
// JSON text, the same round-trip shape skill.mcp uses for its optional
// config.
func scanStaff(row staffScanner) (*Staff, error) {
	var st Staff
	var bigFive string
	if err := row.Scan(&st.ID, &st.Slug, &st.Name, &st.Locale, &st.Age, &bigFive,
		&st.Brief, &st.WordBudget, &st.Model, &st.CreatedAt, &st.UpdatedAt); err != nil {
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

// ListStaff returns every staff member on this installation, ordered by slug.
func (s *Store) ListStaff() ([]Staff, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, locale, age, big_five, brief, word_budget, model, created_at, updated_at
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
	return scanStaff(s.db.QueryRow(`SELECT id, slug, name, locale, age, big_five, brief, word_budget, model, created_at, updated_at
		FROM staff WHERE id = ?`, id))
}

// UpsertStaff writes a staff member keyed by slug: an existing slug updates
// the row and refreshes updated_at, a new slug mints a `staff-` identifier.
func (s *Store) UpsertStaff(slug, name, locale, model, brief string, age, wordBudget int, bigFive Character) (*Staff, error) {
	bigFiveJSON, err := encodeBigFive(bigFive)
	if err != nil {
		return nil, err
	}
	var existing string
	err = s.db.QueryRow(`SELECT id FROM staff WHERE slug = ?`, slug).Scan(&existing)
	if err == nil {
		_, err = s.db.Exec(`UPDATE staff SET name = ?, locale = ?, model = ?, brief = ?, age = ?, word_budget = ?, big_five = ?, updated_at = ?
			WHERE id = ?`, name, locale, model, brief, age, wordBudget, bigFiveJSON, time.Now().Unix(), existing)
		if err != nil {
			return nil, err
		}
		return s.StaffById(existing)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	staffId, err := id.New(id.Staff)
	if err != nil {
		return nil, err
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
		BigFive:    bigFive,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err = s.db.Exec(`INSERT INTO staff (id, slug, name, locale, age, big_five, brief, word_budget, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.Slug, st.Name, st.Locale, st.Age, bigFiveJSON, st.Brief, st.WordBudget, st.Model, st.CreatedAt, st.UpdatedAt)
	if uniqueConstraint(err) {
		return nil, fmt.Errorf("staff slug %q already exists: %w", slug, err)
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// StaffForOrg returns the staff roster as one organization sees it: each
// overridable field is the org override when the override column is not NULL,
// the shared base row otherwise (override ?? base).
func (s *Store) StaffForOrg(orgID string) ([]Staff, error) {
	rows, err := s.db.Query(`SELECT st.id, st.slug, st.name, st.locale, st.age, st.big_five, st.brief, st.word_budget, st.model, st.created_at, st.updated_at,
		ov.big_five, ov.brief, ov.word_budget, ov.model
		FROM staff st
		LEFT JOIN org_staff_overrides ov ON ov.org_id = ? AND ov.staff_id = st.id
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
// zero value, because NULL — not emptiness — is the inherit marker.
func scanStaffForOrg(row staffScanner) (*Staff, error) {
	var st Staff
	var baseBigFive string
	var ovBigFive, ovBrief, ovModel sql.NullString
	var ovWordBudget sql.NullInt64
	if err := row.Scan(&st.ID, &st.Slug, &st.Name, &st.Locale, &st.Age, &baseBigFive,
		&st.Brief, &st.WordBudget, &st.Model, &st.CreatedAt, &st.UpdatedAt,
		&ovBigFive, &ovBrief, &ovWordBudget, &ovModel); err != nil {
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

// SetOrgStaffOverride writes one organization's tuning of a staff member. A
// nil pointer leaves the column NULL ("inherit the base row"); the upsert
// replaces a previous override in place.
func (s *Store) SetOrgStaffOverride(orgID, staffID string, bigFive *Character, brief, model *string, wordBudget *int) error {
	var bigFiveJSON any
	if bigFive != nil {
		b, err := json.Marshal(bigFive)
		if err != nil {
			return err
		}
		bigFiveJSON = string(b)
	}
	_, err := s.db.Exec(`INSERT INTO org_staff_overrides (org_id, staff_id, big_five, brief, word_budget, model)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(org_id, staff_id) DO UPDATE SET
			big_five = excluded.big_five,
			brief = excluded.brief,
			word_budget = excluded.word_budget,
			model = excluded.model`,
		orgID, staffID, bigFiveJSON, nullableString(brief), nullableInt(wordBudget), nullableString(model))
	return err
}

// ClearOrgStaffOverride drops one organization's tuning of a staff member.
// Clearing a missing override is not an error.
func (s *Store) ClearOrgStaffOverride(orgID, staffID string) error {
	_, err := s.db.Exec(`DELETE FROM org_staff_overrides WHERE org_id = ? AND staff_id = ?`, orgID, staffID)
	return err
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
}

// EnsureSeedStaff creates the two baseline staff members when they are
// missing and leaves them alone otherwise, so product content written over
// the seeds survives a restart. Idempotent.
func (s *Store) EnsureSeedStaff() error {
	neutral := Character{
		Openness:          0.5,
		Conscientiousness: 0.5,
		Extraversion:      0.5,
		Agreeableness:     0.5,
		Neuroticism:       0.5,
	}
	for _, sd := range []seedStaff{
		{slug: "staff-male-00", name: "Adam", locale: "pl", age: 35},
		{slug: "staff-female-00", name: "Ewa", locale: "pl", age: 32, model: "arianna.glb"},
	} {
		var existing string
		err := s.db.QueryRow(`SELECT id FROM staff WHERE slug = ?`, sd.slug).Scan(&existing)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.UpsertStaff(sd.slug, sd.name, sd.locale, sd.model, "", sd.age, 0, neutral); err != nil {
			return err
		}
	}
	return nil
}
