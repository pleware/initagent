package hub

import (
	"database/sql"
	"errors"
)

// --- model assignments ---

// ErrUnknownModel reports a SetAssignment whose model_id is not in the
// models registry: an assignment may only point at a pinned model.
var ErrUnknownModel = errors.New("no such model in the registry")

// ErrModelPurposeMismatch reports a SetAssignment whose model serves a
// different purpose than the one being assigned: the persona slot cannot
// ride a worker model.
var ErrModelPurposeMismatch = errors.New("model purpose does not match the assignment")

// ErrModelUnverified reports a SetAssignment whose model pin still carries
// an empty digest. The hub cannot vouch for an unverified artifact, so an
// admin has to compute the BLAKE3 and fill the pin before the factory can
// point a box at it.
var ErrModelUnverified = errors.New("model digest is empty: verify the pinned artifact and fill it first")

// ModelAssignment is one factory pin: the model a purpose resolves to when
// a box has no override of its own (the override ?? canonical pair, wave 3).
type ModelAssignment struct {
	Purpose string `json:"purpose"`
	ModelID string `json:"modelId"`
}

// assignmentScanner is the shared shape of sql.Row and sql.Rows Scan
// methods, the same role modelScanner plays for the models table.
type assignmentScanner interface {
	Scan(dest ...any) error
}

// scanAssignment reads one model_assignments row selected in schema order:
// purpose, model_id. A missing row is (nil, nil).
func scanAssignment(row assignmentScanner) (*ModelAssignment, error) {
	var a ModelAssignment
	if err := row.Scan(&a.Purpose, &a.ModelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// SetAssignment pins one purpose to one model, upserting on the purpose
// primary key — one row per purpose. The pin must pass the full
// cross-validation before it is written: the purpose parses, the model
// exists in the registry, the model serves that purpose, and the model's
// digest is filled (an unverified pin is not assignable). It answers with
// the assignment row written.
//
// The upsert and the fleet bump commit together: a changed factory pin
// changes the resolved roster of every box without an override for that
// purpose, and over-bumping boxes that do carry one is acceptable.
func (s *Store) SetAssignment(purpose, modelID string) (*ModelAssignment, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	m, err := s.GetModel(modelID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrUnknownModel
	}
	if !m.ServesPurpose(purpose) {
		return nil, ErrModelPurposeMismatch
	}
	if m.Digest == "" {
		return nil, ErrModelUnverified
	}
	a := &ModelAssignment{Purpose: purpose, ModelID: m.ID}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO model_assignments (purpose, model_id)
		VALUES (?, ?)
		ON CONFLICT(purpose) DO UPDATE SET model_id = excluded.model_id`, a.Purpose, a.ModelID)
	if err != nil {
		return nil, err
	}
	if err := bumpAllBoxes(tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a, nil
}

// ListAssignments returns every factory pin on this installation, ordered
// by purpose. A store with no assignments yields an empty slice, not nil.
func (s *Store) ListAssignments() ([]ModelAssignment, error) {
	rows, err := s.db.Query(`SELECT purpose, model_id FROM model_assignments ORDER BY purpose`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelAssignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// AssignmentFor answers which model a purpose is pinned to. A purpose with
// no assignment is (nil, nil), not an error: wave 3's resolution skips a
// purpose without a model rather than failing the roster.
func (s *Store) AssignmentFor(purpose string) (*ModelAssignment, error) {
	return scanAssignment(s.db.QueryRow(`SELECT purpose, model_id
		FROM model_assignments WHERE purpose = ?`, purpose))
}

// ClearAssignment removes a purpose's factory pin and reports whether a
// row was removed. Clearing a purpose that has no assignment is (false,
// nil).
//
// The delete and the fleet bump commit together, and only when a row was
// actually removed: a no-op clear must not re-sync the fleet for nothing.
func (s *Store) ClearAssignment(purpose string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM model_assignments WHERE purpose = ?`, purpose)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		if err := bumpAllBoxes(tx); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}
