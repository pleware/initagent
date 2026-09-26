package hub

import (
	"database/sql"
	"errors"
	"fmt"
)

// --- box model overrides ---

// ErrUnknownBox reports an override write whose box_id is not in the boxes
// table: an override belongs to a box, so the box must exist first.
var ErrUnknownBox = errors.New("no such box")

// BoxModelOverride is one per-box pin: the model a purpose resolves to on
// this box, shadowing the factory assignment (the override ?? canonical
// pair). One row per box and purpose.
type BoxModelOverride struct {
	BoxID   string `json:"boxId"`
	Purpose string `json:"purpose"`
	ModelID string `json:"modelId"`
}

// overrideScanner is the shared shape of sql.Row and sql.Rows Scan methods,
// the same role modelScanner plays for the models table.
type overrideScanner interface {
	Scan(dest ...any) error
}

// scanOverride reads one box_model_overrides row selected in schema order:
// box_id, purpose, model_id. A missing row is (nil, nil).
func scanOverride(row overrideScanner) (*BoxModelOverride, error) {
	var o BoxModelOverride
	if err := row.Scan(&o.BoxID, &o.Purpose, &o.ModelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &o, nil
}

// SetBoxModelOverride pins one box's purpose to one model, upserting on the
// (box_id, purpose) primary key — one row per box and purpose. The pin must
// pass the full cross-validation before it is written: the purpose parses,
// the box exists, the model exists in the registry, the model serves that
// purpose, and the model's digest is filled (an unverified pin is not
// assignable). It answers with the override row written.
//
// The upsert and the box's bump commit together: only this box's roster
// changed, so only this box re-syncs.
func (s *Store) SetBoxModelOverride(boxID, purpose, modelID string) (*BoxModelOverride, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	box, err := s.GetBox(boxID)
	if err != nil {
		return nil, err
	}
	if box == nil {
		return nil, ErrUnknownBox
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
	o := &BoxModelOverride{BoxID: box.ID, Purpose: purpose, ModelID: m.ID}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO box_model_overrides (box_id, purpose, model_id)
		VALUES (?, ?, ?)
		ON CONFLICT(box_id, purpose) DO UPDATE SET model_id = excluded.model_id`,
		o.BoxID, o.Purpose, o.ModelID)
	if err != nil {
		return nil, err
	}
	if err := bumpBoxConfig(tx, box.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return o, nil
}

// ListBoxModelOverrides returns a box's overrides, ordered by purpose. A
// box with no overrides yields an empty slice, not nil.
func (s *Store) ListBoxModelOverrides(boxID string) ([]BoxModelOverride, error) {
	rows, err := s.db.Query(`SELECT box_id, purpose, model_id
		FROM box_model_overrides WHERE box_id = ? ORDER BY purpose`, boxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BoxModelOverride{}
	for rows.Next() {
		o, err := scanOverride(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

// ClearBoxModelOverride removes a box's per-box pin for one purpose and
// reports whether a row was removed. Clearing an override that does not
// exist is (false, nil).
//
// The delete and the box's bump commit together, and only when a row was
// actually removed: a no-op clear must not re-sync the box for nothing.
func (s *Store) ClearBoxModelOverride(boxID, purpose string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM box_model_overrides WHERE box_id = ? AND purpose = ?`, boxID, purpose)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		if err := bumpBoxConfig(tx, boxID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ResolvedModels answers the model roster a box resolves against: for each
// purpose in canonical order the box's override wins when present, the
// factory assignment answers when not, and a purpose with neither is
// omitted — a box treats a missing purpose as "no model", not as an error.
// The map is keyed by purpose; walking the canonical list keeps the build
// order stable for the callers that serialize the roster (wave 4).
//
// A row that references a model no longer in the registry is an error, not
// a silent skip: the store guards keep that state unreachable, so reaching
// it means the database was edited by hand and the roster must not paper
// over it.
func (s *Store) ResolvedModels(boxID string) (map[string]Model, error) {
	overrides, err := s.ListBoxModelOverrides(boxID)
	if err != nil {
		return nil, err
	}
	assignments, err := s.ListAssignments()
	if err != nil {
		return nil, err
	}
	overrideFor := map[string]string{}
	for _, o := range overrides {
		overrideFor[o.Purpose] = o.ModelID
	}
	assignmentFor := map[string]string{}
	for _, a := range assignments {
		assignmentFor[a.Purpose] = a.ModelID
	}

	out := map[string]Model{}
	for _, p := range modelPurposeOrder {
		modelID := overrideFor[p]
		if modelID == "" {
			modelID = assignmentFor[p]
		}
		if modelID == "" {
			continue
		}
		m, err := s.GetModel(modelID)
		if err != nil {
			return nil, err
		}
		if m == nil {
			return nil, fmt.Errorf("purpose %s references a model no longer in the registry: %q", p, modelID)
		}
		out[p] = *m
	}
	return out, nil
}
