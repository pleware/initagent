package hub

import (
	"database/sql"
	"encoding/json"
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
// groups org → model → quant. File is the artifact name within the source
// repo (the exact .gguf/.bin/.onnx the pin resolves to). Digest is the
// BLAKE3 of the pinned artifact, admin-provided after verification; empty
// means unverified. Quant is the canonical GGUF quantization (see ParseQuant)
// and empty for non-GGUF models (embedding, stt).
type Model struct {
	ID            string `json:"id"`
	Org           string `json:"org"`
	Source        string `json:"source"`
	Quant         string `json:"quant"`
	File          string `json:"file"`
	Digest        string `json:"digest"`
	Licence       string `json:"licence"`
	Purpose       string `json:"purpose"`
	PipelineTag   string `json:"pipelineTag"`
	LibraryName   string `json:"libraryName"`
	BaseModel     string `json:"baseModel"`
	Architecture  string `json:"architecture"`
	ContextLength int64  `json:"contextLength"`
	Downloads     int64  `json:"downloads"`
	Gated         bool   `json:"gated"`

	// Files is every artifact the pin is made of, when that is more than the
	// one File names. Empty is the single-artifact shape (File + Digest). A
	// pin whose consumer loads a *directory* — faster-whisper reads
	// model.bin, config.json, tokenizer.json, vocabulary.json and
	// preprocessor_config.json — lists every member here, each with the
	// BLAKE3 the puller verifies when it has one. File stays the anchor: the
	// member a file-consuming program is pointed at, and one of these.
	Files []ModelFile `json:"files,omitempty"`
}

// ModelFile is one artifact inside a pinned model: the exact file name
// within the source repo, and its BLAKE3 — empty until the artifact is
// verified, the same rule the pin's own Digest follows.
type ModelFile struct {
	File   string `json:"file"`
	Digest string `json:"digest"`
}

// marshalFiles renders a file list for storage: the empty string for an
// empty list, so a single-artifact pin keeps the column default and the
// scan can tell "no list" from "a list that failed to parse".
func marshalFiles(files []ModelFile) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// sameFileNames reports whether two file lists name the same artifacts, in
// any order. Digests are deliberately not compared: the factory seed says
// which files a model is made of, and a verified digest is filled in later
// by an adoption — a seed pass must not fight that.
func sameFileNames(a, b []ModelFile) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, f := range a {
		seen[f.File] = true
	}
	for _, f := range b {
		if !seen[f.File] {
			return false
		}
	}
	return true
}

// mergeSeedFiles keeps the names the seed declares and the digests the
// store already holds — the factory owns *which* artifacts a model is made
// of, an admin (or a zest adoption) owns *what their bytes are*. A file the
// seed has dropped is dropped along with its digest.
func mergeSeedFiles(stored, seeded []ModelFile) []ModelFile {
	if len(seeded) == 0 {
		return nil
	}
	held := make(map[string]string, len(stored))
	for _, f := range stored {
		held[f.File] = f.Digest
	}
	merged := make([]ModelFile, 0, len(seeded))
	for _, f := range seeded {
		merged = append(merged, ModelFile{File: f.File, Digest: held[f.File]})
	}
	return merged
}

// ValidateModelFiles checks a file list against a pin's anchor: names must
// be present and unique, and when the pin names an anchor File it must be
// one of the listed artifacts — otherwise a file-consuming program would be
// pointed at something the puller never fetches. An empty list is valid:
// that is the single-artifact shape.
func ValidateModelFiles(anchor string, files []ModelFile) error {
	if len(files) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		name := strings.TrimSpace(f.File)
		if name == "" {
			return errors.New("a listed file has no name")
		}
		if seen[name] {
			return fmt.Errorf("file %q is listed twice", name)
		}
		seen[name] = true
	}
	if anchor != "" && !seen[anchor] {
		return fmt.Errorf("anchor file %q is not among the model's files", anchor)
	}
	return nil
}

// ModelMeta is the derived metadata the hub learns about a pin from the
// Hugging Face catalog and the GGUF header — the capability and context
// signals that are not part of the pin's identity. It is learned, not
// admin-authored: empty/zero means unknown, never fabricated.
type ModelMeta struct {
	PipelineTag   string `json:"pipelineTag"`
	LibraryName   string `json:"libraryName"`
	BaseModel     string `json:"baseModel"`
	Architecture  string `json:"architecture"`
	ContextLength int64  `json:"contextLength"`
	Downloads     int64  `json:"downloads"`
	Gated         bool   `json:"gated"`
}

// modelPurposes names the six purposes a pinned model can serve: the
// persona's LLM, the coder's LLM, the embedder, the speech-to-text
// transcriber, the voice-activity detector, and the text-to-speech voice.
var modelPurposes = map[string]bool{
	"persona":   true,
	"worker":    true,
	"embedding": true,
	"stt":       true,
	"vad":       true,
	"tts":       true,
}

// modelPurposeOrder lists the six purposes in canonical order. Map
// iteration order is not deterministic, so ResolvedModels walks this slice
// to build the roster in a stable order.
var modelPurposeOrder = []string{"persona", "worker", "embedding", "stt", "vad", "tts"}

// ParsePurpose accepts a model purpose from the wire, trimmed and
// case-insensitive. There is no default — a model's purpose is required —
// and an unknown name is refused rather than defaulted, the same convention
// ParseEdition follows: a typo that silently became `persona` would pin the
// wrong model to the wrong role.
func ParsePurpose(s string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(s))
	if !modelPurposes[p] {
		return "", fmt.Errorf("purpose %q: want persona, worker, embedding, stt, vad or tts", s)
	}
	return p, nil
}

// canonicalQuants is the GGUF quantization dictionary: the canonical set
// documented at huggingface.co/docs/hub/gguf (floats, legacy quants,
// K-quants, I-quants) plus the Unsloth Dynamic tier — the Q*_K_XL K-quants
// and their UD- prefixed variants — that the QAT GGUF repos ship instead of
// the plain K/I quants. All in their exact uppercase spelling, so a typo'd
// quant is refused at write time instead of failing at pull time.
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
	// Unsloth Dynamic (UD-*): XL K-quants and dynamic I-quants.
	"Q2_K_XL": true, "Q3_K_XL": true, "Q4_K_XL": true, "Q5_K_XL": true, "Q6_K_XL": true,
	"UD-Q2_K_XL": true, "UD-Q3_K_XL": true, "UD-Q4_K_XL": true, "UD-Q5_K_XL": true, "UD-Q6_K_XL": true,
	"UD-Q4_0": true, "UD-Q8_0": true,
	"UD-IQ1_S": true, "UD-IQ1_M": true,
	"UD-IQ2_XXS": true, "UD-IQ2_XS": true, "UD-IQ2_S": true, "UD-IQ2_M": true,
	"UD-IQ3_XXS": true, "UD-IQ3_XS": true, "UD-IQ3_S": true, "UD-IQ3_M": true,
	"UD-IQ4_XS": true, "UD-IQ4_NL": true,
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
// id, org, source, quant, file, digest, licence, purpose, pipeline_tag,
// library_name, base_model, architecture, context_length, downloads, gated,
// files. A missing row is (nil, nil).
func scanModel(row modelScanner) (*Model, error) {
	var m Model
	var gated int
	var files string
	if err := row.Scan(&m.ID, &m.Org, &m.Source, &m.Quant, &m.File, &m.Digest, &m.Licence, &m.Purpose,
		&m.PipelineTag, &m.LibraryName, &m.BaseModel, &m.Architecture, &m.ContextLength, &m.Downloads, &gated,
		&files); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	m.Gated = gated != 0
	if files != "" {
		if err := json.Unmarshal([]byte(files), &m.Files); err != nil {
			return nil, fmt.Errorf("model %s: files column is not a file list: %w", m.ID, err)
		}
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
func (s *Store) CreateModel(id, org, source, quant, file, digest, licence, purpose string) (*Model, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	quant, err = ParseQuant(quant)
	if err != nil {
		return nil, err
	}
	m := &Model{ID: id, Org: org, Source: source, Quant: quant, File: file, Digest: digest, Licence: licence, Purpose: purpose}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO models (id, org, source, quant, file, digest, licence, purpose)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, m.ID, m.Org, m.Source, m.Quant, m.File, m.Digest, m.Licence, m.Purpose)
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
	return scanModel(s.db.QueryRow(`SELECT id, org, source, quant, file, digest, licence, purpose,
		pipeline_tag, library_name, base_model, architecture, context_length, downloads, gated, files
		FROM models WHERE id = ?`, id))
}

// ListModels returns every pinned model on this installation, ordered by id.
func (s *Store) ListModels() ([]Model, error) {
	rows, err := s.db.Query(`SELECT id, org, source, quant, file, digest, licence, purpose,
		pipeline_tag, library_name, base_model, architecture, context_length, downloads, gated, files
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
func (s *Store) UpdateModel(id, org, source, quant, file, digest, licence, purpose string) (*Model, error) {
	purpose, err := ParsePurpose(purpose)
	if err != nil {
		return nil, err
	}
	quant, err = ParseQuant(quant)
	if err != nil {
		return nil, err
	}
	// The anchor is editable, the file list is not touched here — so a
	// changed anchor must still be one of the artifacts the pin pulls.
	existing, err := s.GetModel(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	if err := ValidateModelFiles(file, existing.Files); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE models SET org = ?, source = ?, quant = ?, file = ?, digest = ?, licence = ?, purpose = ?
		WHERE id = ?`, org, source, quant, file, digest, licence, purpose, id)
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

// SetModelMeta writes the derived metadata the hub learned about a pin from
// the Hugging Face catalog and the GGUF header. It is a separate write from
// CreateModel/UpdateModel because the metadata is learned, not
// admin-authored: the identity/provenance fields are the pin the admin owns,
// and this is the enrichment the inspect endpoint fills. A missing pin is
// (nil, nil).
func (s *Store) SetModelMeta(id string, meta ModelMeta) (*Model, error) {
	var gated int
	if meta.Gated {
		gated = 1
	}
	res, err := s.db.Exec(`UPDATE models SET pipeline_tag = ?, library_name = ?, base_model = ?,
		architecture = ?, context_length = ?, downloads = ?, gated = ?
		WHERE id = ?`,
		meta.PipelineTag, meta.LibraryName, meta.BaseModel, meta.Architecture,
		meta.ContextLength, meta.Downloads, gated, id)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return s.GetModel(id)
}

// SetModelFiles replaces the file list of a directory-shaped pin and answers
// with the updated row. A missing pin is (nil, nil).
//
// It is a separate write from CreateModel/UpdateModel for the same reason
// SetModelMeta is: the list is a property of the artifact, not of the pin's
// identity, and an admin fills it from what the repo actually contains
// rather than from the pin's own fields. The list is validated against the
// pin's anchor (ValidateModelFiles) so a file-consuming program can never be
// pointed at an artifact the puller does not fetch. An empty list is the
// single-artifact shape and is allowed.
//
// The write and the fleet bump commit together: the list decides what every
// box pulls, so a box that has not seen the change cannot serve the model.
func (s *Store) SetModelFiles(id string, files []ModelFile) (*Model, error) {
	m, err := s.GetModel(id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	if err := ValidateModelFiles(m.File, files); err != nil {
		return nil, err
	}
	raw, err := marshalFiles(files)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE models SET files = ? WHERE id = ?`, raw, id)
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

// seedModel is one factory model pin. Digest is the BLAKE3 of the pinned
// artifact, computed once at pin time and hardcoded here — these are our own
// predefined models, so the factory can vouch for them without hosting the
// weights.
type seedModel struct {
	id            string
	org           string
	source        string
	quant         string
	file          string
	digest        string
	licence       string
	purpose       string
	pipelineTag   string
	libraryName   string
	baseModel     string
	architecture  string
	contextLength int64
	downloads     int64
	gated         bool

	// files names every artifact a directory-shaped pin is made of (see
	// Model.Files). Names only — the seed declares which files the model
	// consists of, an adoption or an admin fills in their digests, and a
	// seed pass merges rather than overwrites.
	files []ModelFile
}

// EnsureSeedModels keeps the factory model pins at their factory definition.
// A missing pin is inserted; a pin whose org/source/quant/file/licence/
// purpose/metadata drifted from the factory definition is updated back to it;
// a pin already at the definition is left alone. The factory pins are
// factory-owned — an admin customizes through the assignment and override
// layers, not by editing the factory pin itself — with one exception: the
// digest is admin-provided (computed once from the pinned artifact), so the
// seed writes it on a fresh store but never reverts a later change.
//
// The writes and the fleet bump commit together, and only when something
// actually changed: a second run over an in-sync store inserts nothing,
// updates nothing and bumps nothing (the ensureNarratorSlugRename "read then
// write then bump" shape).
func (s *Store) EnsureSeedModels() error {
	seeds := []seedModel{
		{
			id:            "qwen3.5-4b-q4_k_m",
			org:           "bartowski",
			source:        "bartowski/Qwen_Qwen3.5-4B-GGUF@4168f45a16a1290d65a4ec0fa312ae917a4c15d6",
			quant:         "Q4_K_M",
			file:          "Qwen_Qwen3.5-4B-Q4_K_M.gguf",
			digest:        "fe7ad96fac5c979c790dc2a8ae06cf85ddf1ffd5a4d4f83d1fdaddc350d17980",
			licence:       "Apache-2.0",
			purpose:       "persona",
			pipelineTag:   "image-text-to-text",
			baseModel:     "Qwen/Qwen3.5-4B",
			architecture:  "qwen35",
			contextLength: 262144,
			downloads:     335270,
		},
		{
			id:            "qwen2.5-coder-7b-q4_k_m",
			org:           "Qwen",
			source:        "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF@13fb94bfda8c8cf22497dc57b78f391a9acb426a",
			quant:         "Q4_K_M",
			file:          "qwen2.5-coder-7b-instruct-q4_k_m.gguf",
			digest:        "e0abfc1f71fa8f1454f3bc443f7608a63263ed2d41664f2820c3d92c45f3bd52",
			licence:       "Apache-2.0",
			purpose:       "worker",
			pipelineTag:   "text-generation",
			libraryName:   "transformers",
			baseModel:     "Qwen/Qwen2.5-Coder-7B-Instruct",
			architecture:  "qwen2",
			contextLength: 131072,
			downloads:     271625,
		},
		{
			id:            "bge-m3",
			org:           "gpustack",
			source:        "gpustack/bge-m3-GGUF@2d48f1737679ad900d5c26c5aad5410e9c70fdca",
			quant:         "Q4_K_M",
			file:          "bge-m3-Q4_K_M.gguf",
			digest:        "f455475d60569f7ba086863c6ff4b79bb19201664259c5128b9f4f131408dd32",
			licence:       "MIT",
			purpose:       "embedding",
			pipelineTag:   "sentence-similarity",
			libraryName:   "sentence-transformers",
			architecture:  "bert",
			contextLength: 8192,
			downloads:     54756,
		},
		{
			id:            "gemma-4-12b-qat",
			org:           "unsloth",
			source:        "unsloth/gemma-4-12B-it-qat-GGUF@980b060c40a8539ac159e0501a3e0f66a6365af3",
			quant:         "UD-Q4_K_XL",
			file:          "gemma-4-12B-it-qat-UD-Q4_K_XL.gguf",
			digest:        "",
			licence:       "Apache-2.0",
			purpose:       "persona",
			pipelineTag:   "any-to-any",
			libraryName:   "transformers",
			baseModel:     "google/gemma-4-12B-it-qat-q4_0-unquantized",
			architecture:  "gemma4",
			contextLength: 262144,
			downloads:     956012,
		},
		{
			id:            "gemma-4-e4b-qat",
			org:           "unsloth",
			source:        "unsloth/gemma-4-E4B-it-qat-GGUF@8c5a9e4fd5482e2be20fe0bf013b4c262a8f4265",
			quant:         "UD-Q4_K_XL",
			file:          "gemma-4-E4B-it-qat-UD-Q4_K_XL.gguf",
			digest:        "",
			licence:       "Apache-2.0",
			purpose:       "persona",
			pipelineTag:   "any-to-any",
			libraryName:   "transformers",
			baseModel:     "google/gemma-4-E4B-it-qat-q4_0-unquantized",
			architecture:  "gemma4",
			contextLength: 131072,
			downloads:     631021,
		},
		{
			id:            "qwen3.5-9b-q4_k_m",
			org:           "unsloth",
			source:        "unsloth/Qwen3.5-9B-GGUF@3885219b6810b007914f3a7950a8d1b469d598a5",
			quant:         "Q4_K_M",
			file:          "Qwen3.5-9B-Q4_K_M.gguf",
			digest:        "",
			licence:       "Apache-2.0",
			purpose:       "persona",
			pipelineTag:   "image-text-to-text",
			libraryName:   "transformers",
			baseModel:     "Qwen/Qwen3.5-9B",
			architecture:  "qwen35",
			contextLength: 262144,
			downloads:     1516287,
		},
		{
			id:            "gemma-4-26b-a4b-qat",
			org:           "unsloth",
			source:        "unsloth/gemma-4-26B-A4B-it-qat-GGUF@7b92b5b28818151e8669af2e45e88d6086f490dd",
			quant:         "UD-Q4_K_XL",
			file:          "gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf",
			digest:        "",
			licence:       "Apache-2.0",
			purpose:       "worker",
			pipelineTag:   "image-text-to-text",
			libraryName:   "transformers",
			baseModel:     "google/gemma-4-26B-A4B-it-qat-q4_0-unquantized",
			architecture:  "gemma4",
			contextLength: 262144,
			downloads:     632331,
		},
		{
			id:            "muse-glimmer-30b",
			org:           "unsloth",
			source:        "unsloth/Muse-Glimmer-30B-GGUF@faa5b025c584459c13febfa5c59883516710ae39",
			quant:         "UD-IQ3_M",
			file:          "Muse-Glimmer-30B-UD-IQ3_M.gguf",
			digest:        "",
			licence:       "Apache-2.0",
			purpose:       "worker",
			pipelineTag:   "image-text-to-text",
			libraryName:   "transformers",
			baseModel:     "meta-models/Muse-Glimmer-30B",
			architecture:  "muse-glimmer",
			contextLength: 131072,
			downloads:     317157,
		},
		{
			id:            "qwen3.6-35b-a3b-q4_k_m",
			org:           "ggml-org",
			source:        "ggml-org/Qwen3.6-35B-A3B-GGUF@baec3ebee244827cda0f4557eafa8b28f7545fa6",
			quant:         "Q4_K_M",
			file:          "Qwen3.6-35B-A3B-Q4_K_M.gguf",
			digest:        "b8a1dddb19cdffdc8105f7cfac93e1f987b99128edba855ed800306251dfd477",
			licence:       "Apache-2.0",
			purpose:       "persona",
			pipelineTag:   "image-text-to-text",
			baseModel:     "Qwen/Qwen3.6-35B-A3B",
			architecture:  "qwen35moe",
			contextLength: 262144,
			downloads:     173368,
		},
		{
			id:            "kat-coder-v2.5-dev-q4_k_m",
			org:           "bartowski",
			source:        "bartowski/Kwaipilot_KAT-Coder-V2.5-Dev-GGUF@d8f684f08d2950ea9d2db6a35ef7dada0707858b",
			quant:         "Q4_K_M",
			file:          "Kwaipilot_KAT-Coder-V2.5-Dev-Q4_K_M.gguf",
			digest:        "e8bd64e3e79f4d618422eefe4fa06c2903dab6f7cf4109bde9f087eae75da4ae",
			licence:       "Apache-2.0",
			purpose:       "worker",
			pipelineTag:   "text-generation",
			baseModel:     "Kwaipilot/KAT-Coder-V2.5-Dev",
			architecture:  "qwen35moe",
			contextLength: 262144,
			downloads:     382659,
		},
		{
			id:      "faster-whisper-medium",
			org:     "Systran",
			source:  "Systran/faster-whisper-medium@08e178d48790749d25932bbc082711ddcfdfbc4f",
			quant:   "",
			file:    "model.bin",
			digest:  "7b1053dea7640cc96b5b65b7168487db81010bfce317115d17ca358db970673d",
			licence: "MIT",
			purpose: "stt",
			// The ear loads a directory, not a file. This list is the
			// snapshot as it exists on the box (2026-09-23): medium ships
			// vocabulary.txt where large-v3 ships vocabulary.json, and has
			// no preprocessor_config.json at all — which is exactly why the
			// pin has to name them rather than assume a shape.
			files: []ModelFile{
				{File: "model.bin"},
				{File: "config.json"},
				{File: "tokenizer.json"},
				{File: "vocabulary.txt"},
			},
		},
		{
			id:      "faster-whisper-large-v3",
			org:     "Systran",
			source:  "Systran/faster-whisper-large-v3@edaa852ec7e145841d8ffdb056a99866b5f0a478",
			quant:   "",
			file:    "model.bin",
			digest:  "64b4dc2dfe6589860e4e39e0ba4f50ea0f6026e509447d8873368e1a73a3bd0a",
			licence: "MIT",
			purpose: "stt",
			files: []ModelFile{
				{File: "model.bin"},
				{File: "config.json"},
				{File: "tokenizer.json"},
				{File: "vocabulary.json"},
				{File: "preprocessor_config.json"},
			},
		},
		{
			id:      "silero-vad",
			org:     "istupakov",
			source:  "istupakov/silero-vad-onnx@b3e3ee3cce4c11ceb63b1a0b229d916069c1ddf6",
			quant:   "",
			file:    "silero_vad.onnx",
			digest:  "bd861b19a51c83ee067b54d7d8b7f40bc11bafcc526506edc00b163e1c53bb8e",
			licence: "MIT",
			purpose: "vad",
		},
		{
			id:      "pl_PL-bass-high",
			org:     "rhasspy",
			source:  "rhasspy/piper-voices@c10ece1aade47bb51c153c893d14e5bf8e5b7117",
			quant:   "",
			file:    "pl/pl_PL/bass/high/pl_PL-bass-high.onnx",
			digest:  "d122a10b565681d97ae302b0a4cc617ca59e0cd87b5196ec667ff9c125357b9f",
			licence: "MIT",
			purpose: "tts",
		},
		{
			id:      "pl_PL-darkman-medium",
			org:     "rhasspy",
			source:  "rhasspy/piper-voices@c10ece1aade47bb51c153c893d14e5bf8e5b7117",
			quant:   "",
			file:    "pl/pl_PL/darkman/medium/pl_PL-darkman-medium.onnx",
			digest:  "7554030dd8b3cd40529098054600dc7194f4d146e29fc24f89b89a94c7a43df4",
			licence: "MIT",
			purpose: "tts",
		},
		{
			id:      "pl_PL-gosia-medium",
			org:     "rhasspy",
			source:  "rhasspy/piper-voices@c10ece1aade47bb51c153c893d14e5bf8e5b7117",
			quant:   "",
			file:    "pl/pl_PL/gosia/medium/pl_PL-gosia-medium.onnx",
			digest:  "cec3f38aa9c14d2dfbe43465e818253ee0ed05854288cde7bfda7131acc4fa1b",
			licence: "MIT",
			purpose: "tts",
		},
		{
			id:      "pl_PL-mc_speech-medium",
			org:     "rhasspy",
			source:  "rhasspy/piper-voices@c10ece1aade47bb51c153c893d14e5bf8e5b7117",
			quant:   "",
			file:    "pl/pl_PL/mc_speech/medium/pl_PL-mc_speech-medium.onnx",
			digest:  "9ee4676f29dc7125a591f7eb1bdd7a26808040183b3629a7cef56e158fc9132d",
			licence: "MIT",
			purpose: "tts",
		},
		{
			id:      "pl_PL-mls_6892-low",
			org:     "rhasspy",
			source:  "rhasspy/piper-voices@c10ece1aade47bb51c153c893d14e5bf8e5b7117",
			quant:   "",
			file:    "pl/pl_PL/mls_6892/low/pl_PL-mls_6892-low.onnx",
			digest:  "e9e2971ac7132984c6f6958c21501dec46638332b9e1bcc113a417aead270cde",
			licence: "MIT",
			purpose: "tts",
		},
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	changed := false
	for _, sm := range seeds {
		m, err := scanModel(tx.QueryRow(`SELECT id, org, source, quant, file, digest, licence, purpose,
			pipeline_tag, library_name, base_model, architecture, context_length, downloads, gated, files
			FROM models WHERE id = ?`, sm.id))
		if err != nil {
			return err
		}
		var gated int
		if sm.gated {
			gated = 1
		}
		switch {
		case m == nil:
			filesJSON, err := marshalFiles(sm.files)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO models (id, org, source, quant, file, digest, licence, purpose,
				pipeline_tag, library_name, base_model, architecture, context_length, downloads, gated, files)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				sm.id, sm.org, sm.source, sm.quant, sm.file, sm.digest, sm.licence, sm.purpose,
				sm.pipelineTag, sm.libraryName, sm.baseModel, sm.architecture, sm.contextLength, sm.downloads, gated,
				filesJSON); err != nil {
				return err
			}
			changed = true
		case m.Org != sm.org || m.Source != sm.source || m.Quant != sm.quant || m.File != sm.file ||
			m.Licence != sm.licence || m.Purpose != sm.purpose ||
			m.PipelineTag != sm.pipelineTag || m.LibraryName != sm.libraryName || m.BaseModel != sm.baseModel ||
			m.Architecture != sm.architecture || m.ContextLength != sm.contextLength || m.Downloads != sm.downloads ||
			m.Gated != sm.gated || !sameFileNames(m.Files, sm.files):
			// The digest column is deliberately absent from this SET: the
			// factory seed never overwrites a verified digest. The file
			// list is written *merged* — the seed's names, the digests the
			// store already holds — so a seed pass cannot wipe an adoption
			// either, and it can still drop a file the seed has dropped.
			filesJSON, err := marshalFiles(mergeSeedFiles(m.Files, sm.files))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE models SET org = ?, source = ?, quant = ?, file = ?, licence = ?, purpose = ?,
				pipeline_tag = ?, library_name = ?, base_model = ?, architecture = ?, context_length = ?, downloads = ?, gated = ?, files = ?
				WHERE id = ?`,
				sm.org, sm.source, sm.quant, sm.file, sm.licence, sm.purpose,
				sm.pipelineTag, sm.libraryName, sm.baseModel, sm.architecture, sm.contextLength, sm.downloads, gated,
				filesJSON, sm.id); err != nil {
				return err
			}
			changed = true
		}
	}
	if changed {
		if err := bumpAllBoxes(tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
