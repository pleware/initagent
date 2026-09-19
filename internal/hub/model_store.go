package hub

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// --- models ---

// ErrModelIDTaken reports a CreateModel whose id collides with an existing
// pin: the id is the primary key of the models table.
var ErrModelIDTaken = errors.New("a model with this id already exists")

// ErrModelInUse reports a DeleteModel whose pin is still referenced by a
// model_assignments or box_model_overrides row. The hub refuses to orphan
// those references rather than delete the pin from under them.
var ErrModelInUse = errors.New("this model is still in use by an assignment or override")

// Model is one pin in the hub's model registry: identity and provenance
// only, no weights. Digest is the BLAKE3 of the pinned artifact,
// admin-provided after verification; empty means unverified. Quant is empty
// for non-GGUF models (embedding, stt).
type Model struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Quant   string `json:"quant"`
	Digest  string `json:"digest"`
	Licence string `json:"licence"`
	Purpose string `json:"purpose"`
}

// modelPurposes names the four purposes a pinned model can serve: the
// persona's LLM, the coder's LLM, the embedder, and the speech-to-text
// transcriber.
var modelPurposes = map[string]bool{
	"persona":   true,
	"worker":    true,
	"embedding": true,
	"stt":       true,
}

// modelPurposeOrder lists the four purposes in canonical order. Map
// iteration order is not deterministic, so ResolvedModels walks this slice
// to build the roster in a stable order.
var modelPurposeOrder = []string{"persona", "worker", "embedding", "stt"}

// ParsePurpose accepts a model purpose from the wire, trimmed and
// case-insensitive. There is no default — a model's purpose is required —
// and an unknown name is refused rather than defaulted, the same convention
// ParseEdition follows: a typo that silently became `persona` would pin the
// wrong model to the wrong role.
func ParsePurpose(s string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(s))
	if !modelPurposes[p] {
		return "", fmt.Errorf("purpose %q: want persona, worker, embedding or stt", s)
	}
	return p, nil
}

// modelScanner is the shared shape of sql.Row and sql.Rows Scan methods,
// the same role skillScanner plays for the skills table.
type modelScanner interface {
	Scan(dest ...any) error
}

// scanModel reads one models row selected in schema order:
// id, source, quant, digest, licence, purpose. A missing row is (nil, nil).
func scanModel(row modelScanner) (*Model, error) {
	var m Model
	if err := row.Scan(&m.ID, &m.Source, &m.Quant, &m.Digest, &m.Licence, &m.Purpose); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// CreateModel registers a model pin. The id is the pin slug the admin
// chooses (e.g. "qwen3.5-4b-q4_k_m"): it names one model+quant, so it is
// not minted. The purpose runs through ParsePurpose; digest may be empty —
// an unverified pin — and an admin fills it after checking the artifact. A
// collision on id returns ErrModelIDTaken.
//
// The insert and the fleet bump commit together: a new pin changes the
// catalogue every box's resolved roster draws from, so every box's
// config_version advances — boxes carrying an override included, which is
// an acceptable over-bump.
func (s *Store) CreateModel(id, source, quant, digest, licence, purpose string) (*Model, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	m := &Model{ID: id, Source: source, Quant: quant, Digest: digest, Licence: licence, Purpose: purpose}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO models (id, source, quant, digest, licence, purpose)
		VALUES (?, ?, ?, ?, ?, ?)`, m.ID, m.Source, m.Quant, m.Digest, m.Licence, m.Purpose)
	if uniqueConstraint(err) {
		return nil, ErrModelIDTaken
	}
	if err != nil {
		return nil, err
	}
	if err := bumpAllBoxes(tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m, nil
}

// GetModel returns one model pin by id. A missing pin is (nil, nil).
func (s *Store) GetModel(id string) (*Model, error) {
	return scanModel(s.db.QueryRow(`SELECT id, source, quant, digest, licence, purpose
		FROM models WHERE id = ?`, id))
}

// ListModels returns every pinned model on this installation, ordered by id.
func (s *Store) ListModels() ([]Model, error) {
	rows, err := s.db.Query(`SELECT id, source, quant, digest, licence, purpose
		FROM models ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Model{}
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// UpdateModel replaces the editable fields of a model pin and answers with
// the updated row. A missing pin is (nil, nil).
//
// The write and the fleet bump commit together: a changed pin changes the
// catalogue every box's resolved roster draws from. An update that matched
// no row (a missing pin) bumps nothing — a no-op must not re-sync the
// fleet.
func (s *Store) UpdateModel(id, source, quant, digest, licence, purpose string) (*Model, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE models SET source = ?, quant = ?, digest = ?, licence = ?, purpose = ?
		WHERE id = ?`, source, quant, digest, licence, purpose, id)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n > 0 {
		if err := bumpAllBoxes(tx); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetModel(id)
}

// DeleteModel removes a model pin and reports whether a row was removed. A
// missing pin is (false, nil). A pin that model_assignments or
// box_model_overrides still references is refused with ErrModelInUse rather
// than orphaned, the same hand-written cascade convention DeleteBox follows
// (the schema carries no foreign keys).
//
// The delete and the fleet bump commit together, and only when a row was
// actually removed: a refused or no-op delete bumps nothing. The in-use
// read stays outside the write transaction — it guards, it does not join
// the write it guards.
func (s *Store) DeleteModel(id string) (bool, error) {
	inUse, err := s.modelInUse(id)
	if err != nil {
		return false, err
	}
	if inUse {
		return false, ErrModelInUse
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM models WHERE id = ?`, id)
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

// modelInUse reports whether a model pin is referenced by either table that
// points at it: model_assignments (the factory purpose→model pin) or
// box_model_overrides (a per-box pin). Each table is hasTable-guarded so
// the check is correct before those tables land — they arrive in later
// waves of the model layer — and against a store that predates them.
func (s *Store) modelInUse(id string) (bool, error) {
	for _, table := range []string{"model_assignments", "box_model_overrides"} {
		ok, err := s.hasTable(table)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE model_id = ?`, id).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}

// seedModel is one factory model pin. Digest stays empty on purpose: the
// hub hosts no weights, so it cannot compute the BLAKE3 itself. The pin
// names the artifact and its provenance; an admin fills the digest after
// verifying the pinned file, and until then a later wave refuses to assign
// the model.
type seedModel struct {
	id      string
	source  string
	quant   string
	licence string
	purpose string
}

// EnsureSeedModels plants the four factory model pins when they are missing
// and leaves them alone otherwise, so an admin's later edit survives a
// restart. Idempotent.
//
// When the seed actually inserts rows on a store that already carries
// boxes, every box's config_version bumps in the same transaction: a new
// pin changes the catalogue a box manifest resolves against, so the fleet
// has to re-sync. A second run inserts nothing and bumps nothing, the
// ensureNarratorSlugRename "read then write then bump" shape.
func (s *Store) EnsureSeedModels() error {
	seeds := []seedModel{
		{
			id:      "qwen3.5-4b-q4_k_m",
			source:  "unsloth/Qwen3.5-4B-GGUF@e87f176479d0855a907a41277aca2f8ee7a09523",
			quant:   "q4_k_m",
			licence: "Apache-2.0",
			purpose: "persona",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
		{
			id:      "qwen2.5-coder-7b-q4_k_m",
			source:  "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF@13fb94bfda8c8cf22497dc57b78f391a9acb426a",
			quant:   "q4_k_m",
			licence: "Apache-2.0",
			purpose: "worker",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
		{
			id:      "bge-m3",
			source:  "BAAI/bge-m3@5617a9f61b028005a4858fdac845db406aefb181",
			quant:   "",
			licence: "MIT",
			purpose: "embedding",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
		{
			id:      "whisper-large-v3",
			source:  "openai/whisper-large-v3@06f233fe06e710322aca913c1bc4249a0d71fce1",
			quant:   "",
			licence: "MIT",
			purpose: "stt",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
	}

	missing := []seedModel{}
	for _, sm := range seeds {
		var existing string
		err := s.db.QueryRow(`SELECT id FROM models WHERE id = ?`, sm.id).Scan(&existing)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		missing = append(missing, sm)
	}
	if len(missing) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, sm := range missing {
		if _, err := tx.Exec(`INSERT INTO models (id, source, quant, digest, licence, purpose)
			VALUES (?, ?, ?, '', ?, ?)`, sm.id, sm.source, sm.quant, sm.licence, sm.purpose); err != nil {
			return err
		}
	}
	if err := bumpAllBoxes(tx); err != nil {
		return err
	}
	return tx.Commit()
}
