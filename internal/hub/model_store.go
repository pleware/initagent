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
// only, no weights. Org is the Hugging Face namespace the pin's source
// lives under (the part of source before the first "/"), so the catalog
// groups org → model → quant. Digest is the BLAKE3 of the pinned artifact,
// admin-provided after verification; empty means unverified. Quant is the
// canonical GGUF quantization (see ParseQuant) and empty for non-GGUF
// models (embedding, stt).
type Model struct {
	ID      string `json:"id"`
	Org     string `json:"org"`
	Source  string `json:"source"`
	Quant   string `json:"quant"`
	Digest  string `json:"digest"`
	Licence string `json:"licence"`
	Purpose string `json:"purpose"`
}

// modelPurposes names the five purposes a pinned model can serve: the
// persona's LLM, the coder's LLM, the embedder, the speech-to-text
// transcriber, and the voice-activity detector.
var modelPurposes = map[string]bool{
	"persona":   true,
	"worker":    true,
	"embedding": true,
	"stt":       true,
	"vad":       true,
}

// modelPurposeOrder lists the five purposes in canonical order. Map
// iteration order is not deterministic, so ResolvedModels walks this slice
// to build the roster in a stable order.
var modelPurposeOrder = []string{"persona", "worker", "embedding", "stt", "vad"}

// ParsePurpose accepts a model purpose from the wire, trimmed and
// case-insensitive. There is no default — a model's purpose is required —
// and an unknown name is refused rather than defaulted, the same convention
// ParseEdition follows: a typo that silently became `persona` would pin the
// wrong model to the wrong role.
func ParsePurpose(s string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(s))
	if !modelPurposes[p] {
		return "", fmt.Errorf("purpose %q: want persona, worker, embedding, stt or vad", s)
	}
	return p, nil
}

// canonicalQuants is the canonical GGUF quantization dictionary documented
// at huggingface.co/docs/hub/gguf: floats, legacy quants, K-quants and
// I-quants, in their exact uppercase spelling. The registry stores only
// these spellings, so a typo'd quant is refused at write time instead of
// failing at pull time.
var canonicalQuants = map[string]bool{
	"F32": true, "F16": true, "BF16": true,
	"Q4_0": true, "Q4_1": true, "Q5_0": true, "Q5_1": true, "Q8_0": true, "Q8_1": true,
	"Q2_K": true, "Q3_K_S": true, "Q3_K_M": true, "Q3_K_L": true,
	"Q4_K_S": true, "Q4_K_M": true, "Q4_K_L": true,
	"Q5_K_S": true, "Q5_K_M": true, "Q5_K_L": true, "Q6_K": true,
	"IQ1_S": true, "IQ1_M": true,
	"IQ2_XXS": true, "IQ2_XS": true, "IQ2_S": true, "IQ2_M": true,
	"IQ3_XXS": true, "IQ3_XS": true, "IQ3_S": true, "IQ3_M": true,
	"IQ4_XS": true, "IQ4_NL": true,
}

// ParseQuant validates a quantization name against the canonical GGUF set
// and answers its canonical uppercase spelling. Matching is
// case-insensitive, so "q4_k_m" becomes "Q4_K_M"; an empty string is
// allowed and passes through unchanged — non-GGUF models (embedding, stt)
// carry no quant. A name outside the dictionary is an error, never a
// default: silently storing a made-up quant would break the puller that
// later resolves the .gguf file by its filename quant.
func ParseQuant(s string) (string, error) {
	q := strings.ToUpper(strings.TrimSpace(s))
	if q == "" {
		return "", nil
	}
	if !canonicalQuants[q] {
		return "", fmt.Errorf("quant %q: not a canonical GGUF quantization (huggingface.co/docs/hub/gguf)", s)
	}
	return q, nil
}

// modelScanner is the shared shape of sql.Row and sql.Rows Scan methods,
// the same role skillScanner plays for the skills table.
type modelScanner interface {
	Scan(dest ...any) error
}

// scanModel reads one models row selected in schema order:
// id, org, source, quant, digest, licence, purpose. A missing row is
// (nil, nil).
func scanModel(row modelScanner) (*Model, error) {
	var m Model
	if err := row.Scan(&m.ID, &m.Org, &m.Source, &m.Quant, &m.Digest, &m.Licence, &m.Purpose); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// CreateModel registers a model pin. The id is the pin slug the admin
// chooses (e.g. "qwen3.5-4b-q4_k_m"): it names one model+quant, so it is
// not minted. Org is the Hugging Face namespace the source lives under.
// The purpose runs through ParsePurpose and the quant through ParseQuant
// (empty allowed for non-GGUF pins); digest may be empty — an unverified
// pin — and an admin fills it after checking the artifact. A collision on
// id returns ErrModelIDTaken.
//
// The insert and the fleet bump commit together: a new pin changes the
// catalogue every box's resolved roster draws from, so every box's
// config_version advances — boxes carrying an override included, which is
// an acceptable over-bump.
func (s *Store) CreateModel(id, org, source, quant, digest, licence, purpose string) (*Model, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	quant, err = ParseQuant(quant)
	if err != nil {
		return nil, err
	}
	m := &Model{ID: id, Org: org, Source: source, Quant: quant, Digest: digest, Licence: licence, Purpose: purpose}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO models (id, org, source, quant, digest, licence, purpose)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, m.ID, m.Org, m.Source, m.Quant, m.Digest, m.Licence, m.Purpose)
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
	return scanModel(s.db.QueryRow(`SELECT id, org, source, quant, digest, licence, purpose
		FROM models WHERE id = ?`, id))
}

// ListModels returns every pinned model on this installation, ordered by id.
func (s *Store) ListModels() ([]Model, error) {
	rows, err := s.db.Query(`SELECT id, org, source, quant, digest, licence, purpose
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
func (s *Store) UpdateModel(id, org, source, quant, digest, licence, purpose string) (*Model, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	quant, err = ParseQuant(quant)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE models SET org = ?, source = ?, quant = ?, digest = ?, licence = ?, purpose = ?
		WHERE id = ?`, org, source, quant, digest, licence, purpose, id)
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
	org     string
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
			org:     "bartowski",
			source:  "bartowski/Qwen_Qwen3.5-4B-GGUF@4168f45a16a1290d65a4ec0fa312ae917a4c15d6",
			quant:   "Q4_K_M",
			licence: "Apache-2.0",
			purpose: "persona",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
		{
			id:      "qwen2.5-coder-7b-q4_k_m",
			org:     "Qwen",
			source:  "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF@13fb94bfda8c8cf22497dc57b78f391a9acb426a",
			quant:   "Q4_K_M",
			licence: "Apache-2.0",
			purpose: "worker",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
		{
			id:      "bge-m3",
			org:     "gpustack",
			source:  "gpustack/bge-m3-GGUF@2d48f1737679ad900d5c26c5aad5410e9c70fdca",
			quant:   "Q4_K_M",
			licence: "MIT",
			purpose: "embedding",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
		{
			id:      "faster-whisper-medium",
			org:     "Systran",
			source:  "Systran/faster-whisper-medium@08e178d48790749d25932bbc082711ddcfdfbc4f",
			quant:   "",
			licence: "MIT",
			purpose: "stt",
			// TODO(developer): compute BLAKE3 from the pinned HF artifact and fill
		},
		{
			id:      "silero-vad",
			org:     "istupakov",
			source:  "istupakov/silero-vad-onnx@b3e3ee3cce4c11ceb63b1a0b229d916069c1ddf6",
			quant:   "",
			licence: "MIT",
			purpose: "vad",
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
		if _, err := tx.Exec(`INSERT INTO models (id, org, source, quant, digest, licence, purpose)
			VALUES (?, ?, ?, ?, '', ?, ?)`, sm.id, sm.org, sm.source, sm.quant, sm.licence, sm.purpose); err != nil {
			return err
		}
	}
	if err := bumpAllBoxes(tx); err != nil {
		return err
	}
	return tx.Commit()
}
