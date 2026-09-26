package hub

import (
	"database/sql"
	"errors"
)

// --- model generation limits ---

// The generation-limits layer (the limits wave of the model layer): a
// per-slot ceiling on the tokens a generative model may emit, and a wall-clock
// cap on one generation request. Limits are a property of the *slot*, not of
// the pin: they travel with the role, so reassigning a model to a slot keeps
// the slot's limits. The factory wave is model_limits (one row per purpose);
// the per-box shadow is box_model_limits — the same override ?? factory pair
// the model roster resolves against.

// ErrLimitsNegative reports a limit below zero. 0 means "no limit" and is
// valid; a negative number is a mistake, not a request.
var ErrLimitsNegative = errors.New("generation limits must not be negative")

// ErrMaxTokensTooLarge reports a maxTokens above the hard cap. The cap is a
// fixed constant, not a model's context length, because a limit is per-slot
// and the model serving that slot can change.
var ErrMaxTokensTooLarge = errors.New("maxTokens exceeds the hard cap of 32768")

// maxTokensCap is the largest maxTokens the hub accepts (32768, a 32K
// context). A larger -n on llama.cpp would be clamped by context anyway, so
// refusing it keeps the config honest.
const maxTokensCap = 32768

// validateLimits enforces the wire rules: 0/absent means "no limit" (valid),
// a negative value is refused, and maxTokens above the cap is refused.
func validateLimits(maxTokens, timeoutSeconds int) error {
	if maxTokens < 0 || timeoutSeconds < 0 {
		return ErrLimitsNegative
	}
	if maxTokens > maxTokensCap {
		return ErrMaxTokensTooLarge
	}
	return nil
}

// generativePurposes names the slots whose limits mean something: the three
// LLM roles the box serves through llama.cpp. embedding, stt, vad and tts are
// non-generative engines and carry no limits — the manifest omits the field
// for them.
var generativePurposes = map[string]bool{
	"persona":  true,
	"worker":   true,
	"narrator": true,
}

// ModelLimits is one factory generation limit: the ceiling a purpose resolves
// to when a box carries no limit override of its own.
type ModelLimits struct {
	Purpose        string `json:"purpose"`
	MaxTokens      int    `json:"maxTokens"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

// BoxModelLimits is one per-box generation limit, shadowing the factory limit
// for that box alone.
type BoxModelLimits struct {
	BoxID          string `json:"boxId"`
	Purpose        string `json:"purpose"`
	MaxTokens      int    `json:"maxTokens"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

// limitScanner is the shared shape of sql.Row and sql.Rows Scan methods, the
// same role assignmentScanner plays for the assignments table.
type limitScanner interface {
	Scan(dest ...any) error
}

// scanLimit reads one model_limits row selected in schema order: purpose,
// max_tokens, timeout_seconds. A missing row is (nil, nil).
func scanLimit(row limitScanner) (*ModelLimits, error) {
	var l ModelLimits
	if err := row.Scan(&l.Purpose, &l.MaxTokens, &l.TimeoutSeconds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}

// scanBoxLimit reads one box_model_limits row selected in schema order:
// box_id, purpose, max_tokens, timeout_seconds. A missing row is (nil, nil).
func scanBoxLimit(row limitScanner) (*BoxModelLimits, error) {
	var l BoxModelLimits
	if err := row.Scan(&l.BoxID, &l.Purpose, &l.MaxTokens, &l.TimeoutSeconds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}

// SetLimit pins one purpose's factory limit, upserting on the purpose primary
// key — one row per purpose. The upsert and the fleet bump commit together: a
// changed factory limit changes the resolved limit of every box without an
// override for that purpose, and over-bumping boxes that carry one is
// acceptable.
func (s *Store) SetLimit(purpose string, maxTokens, timeoutSeconds int) (*ModelLimits, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	if err := validateLimits(maxTokens, timeoutSeconds); err != nil {
		return nil, err
	}
	l := &ModelLimits{Purpose: purpose, MaxTokens: maxTokens, TimeoutSeconds: timeoutSeconds}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO model_limits (purpose, max_tokens, timeout_seconds)
		VALUES (?, ?, ?)
		ON CONFLICT(purpose) DO UPDATE SET max_tokens = excluded.max_tokens, timeout_seconds = excluded.timeout_seconds`,
		l.Purpose, l.MaxTokens, l.TimeoutSeconds)
	if err != nil {
		return nil, err
	}
	if err := bumpAllBoxes(tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return l, nil
}

// ListLimits returns every factory limit on this installation, ordered by
// purpose. A store with no limits yields an empty slice, not nil.
func (s *Store) ListLimits() ([]ModelLimits, error) {
	rows, err := s.db.Query(`SELECT purpose, max_tokens, timeout_seconds FROM model_limits ORDER BY purpose`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelLimits{}
	for rows.Next() {
		l, err := scanLimit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// LimitFor answers which factory limit a purpose carries. A purpose with no
// limit is (nil, nil), not an error.
func (s *Store) LimitFor(purpose string) (*ModelLimits, error) {
	return scanLimit(s.db.QueryRow(`SELECT purpose, max_tokens, timeout_seconds
		FROM model_limits WHERE purpose = ?`, purpose))
}

// ClearLimit removes a purpose's factory limit and reports whether a row was
// removed. Clearing a purpose that has no limit is (false, nil). The delete
// and the fleet bump commit together, and only when a row was actually
// removed: a no-op clear must not re-sync the fleet for nothing.
func (s *Store) ClearLimit(purpose string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM model_limits WHERE purpose = ?`, purpose)
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

// SetBoxLimit pins one box's purpose to a limit, upserting on the (box_id,
// purpose) primary key — one row per box and purpose. The box must exist
// first, the purpose must parse, and the limits must validate. A limit can be
// set for a slot before a model is assigned: the limit is per-slot, not
// per-pin. The upsert and the box's bump commit together: only this box's
// manifest changed.
func (s *Store) SetBoxLimit(boxID, purpose string, maxTokens, timeoutSeconds int) (*BoxModelLimits, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	if err := validateLimits(maxTokens, timeoutSeconds); err != nil {
		return nil, err
	}
	box, err := s.GetBox(boxID)
	if err != nil {
		return nil, err
	}
	if box == nil {
		return nil, ErrUnknownBox
	}
	l := &BoxModelLimits{BoxID: box.ID, Purpose: purpose, MaxTokens: maxTokens, TimeoutSeconds: timeoutSeconds}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO box_model_limits (box_id, purpose, max_tokens, timeout_seconds)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(box_id, purpose) DO UPDATE SET max_tokens = excluded.max_tokens, timeout_seconds = excluded.timeout_seconds`,
		l.BoxID, l.Purpose, l.MaxTokens, l.TimeoutSeconds)
	if err != nil {
		return nil, err
	}
	if err := bumpBoxConfig(tx, box.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return l, nil
}

// ListBoxLimits returns a box's limit overrides, ordered by purpose. A box
// with no overrides yields an empty slice, not nil.
func (s *Store) ListBoxLimits(boxID string) ([]BoxModelLimits, error) {
	rows, err := s.db.Query(`SELECT box_id, purpose, max_tokens, timeout_seconds
		FROM box_model_limits WHERE box_id = ? ORDER BY purpose`, boxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BoxModelLimits{}
	for rows.Next() {
		l, err := scanBoxLimit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// ClearBoxLimit removes a box's per-box limit for one purpose and reports
// whether a row was removed. Clearing a limit that does not exist is (false,
// nil). The delete and the box's bump commit together, and only when a row
// was actually removed.
func (s *Store) ClearBoxLimit(boxID, purpose string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM box_model_limits WHERE box_id = ? AND purpose = ?`, boxID, purpose)
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

// ResolvedLimits answers the limit a box resolves against for each purpose in
// canonical order: the box's override wins when present, the factory limit
// answers when not, and a purpose with neither is omitted. A purpose resolves
// its limit here even when it resolves no model — the manifest builder only
// emits the limit when the purpose also resolves a model.
func (s *Store) ResolvedLimits(boxID string) (map[string]ModelLimits, error) {
	boxLimits, err := s.ListBoxLimits(boxID)
	if err != nil {
		return nil, err
	}
	factoryLimits, err := s.ListLimits()
	if err != nil {
		return nil, err
	}
	boxFor := map[string]ModelLimits{}
	for _, l := range boxLimits {
		boxFor[l.Purpose] = ModelLimits{Purpose: l.Purpose, MaxTokens: l.MaxTokens, TimeoutSeconds: l.TimeoutSeconds}
	}
	factoryFor := map[string]ModelLimits{}
	for _, l := range factoryLimits {
		factoryFor[l.Purpose] = l
	}
	out := map[string]ModelLimits{}
	for _, p := range modelPurposeOrder {
		if l, ok := boxFor[p]; ok {
			out[p] = l
			continue
		}
		if l, ok := factoryFor[p]; ok {
			out[p] = l
		}
	}
	return out, nil
}
