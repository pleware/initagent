package hub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/store"
)

func TestParsePurpose(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{"persona", "persona", "persona", false},
		{"worker", "worker", "worker", false},
		{"embedding", "embedding", "embedding", false},
		{"stt", "stt", "stt", false},
		{"trimmed and folded", "  Worker  ", "worker", false},
		{"unknown", "chat", "", true},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePurpose(tt.in)
			if tt.err {
				if err == nil {
					t.Fatalf("ParsePurpose(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePurpose(%q) error = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParsePurpose(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseQuant(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		// canonical passthrough: floats, legacy, K-quants, I-quants.
		{"float f32", "F32", "F32", false},
		{"float f16", "F16", "F16", false},
		{"float bf16", "BF16", "BF16", false},
		{"legacy q8_0", "Q8_0", "Q8_0", false},
		{"legacy q8_1", "Q8_1", "Q8_1", false},
		{"k-quant q6_k", "Q6_K", "Q6_K", false},
		{"k-quant q3_k_l", "Q3_K_L", "Q3_K_L", false},
		{"i-quant iq2_xxs", "IQ2_XXS", "IQ2_XXS", false},
		{"i-quant iq4_nl", "IQ4_NL", "IQ4_NL", false},
		// case-insensitive normalization to uppercase canonical.
		{"normalize lowercase", "q4_k_m", "Q4_K_M", false},
		{"normalize mixed", "Iq3_S", "IQ3_S", false},
		{"normalize with whitespace", "  q5_0  ", "Q5_0", false},
		// empty is allowed: non-GGUF pins (embedding, stt) carry no quant.
		{"empty", "", "", false},
		{"whitespace only", "   ", "", false},
		// outside the dictionary: refused, never defaulted.
		{"unknown", "bogus", "", true},
		{"near-miss k-quant", "Q4_K", "", true},
		{"near-miss i-quant", "IQ2_X", "", true},
		{"lowercase unknown", "q4_k_m_x", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseQuant(tt.in)
			if tt.err {
				if err == nil {
					t.Fatalf("ParseQuant(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseQuant(%q) error = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseQuant(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCreateModelRejectsUnknownQuant(t *testing.T) {
	s := testStore(t)
	before, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateModel("bad-quant", "SomeOrg", "s", "bogus", "", "", "MIT", "persona"); err == nil {
		t.Fatal("CreateModel accepted a non-canonical quant")
	}
	after, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("ListModels after refused create has %d rows, want %d", len(after), len(before))
	}
}

func TestUpdateModelRejectsUnknownQuant(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateModel("quant-guard", "SomeOrg", "s", "Q4_0", "", "", "MIT", "persona")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateModel(created.ID, "SomeOrg", "s2", "bogus", "", "", "MIT", "persona"); err == nil {
		t.Fatal("UpdateModel accepted a non-canonical quant")
	}
	got, err := s.GetModel(created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetModel after refused update = (%v, %v), want the untouched pin", got, err)
	}
	if got.Quant != "Q4_0" || got.Source != "s" {
		t.Errorf("refused update left %+v, want the original row", got)
	}
}

func TestModelCRUDRoundTrip(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateModel("test-model-7b-q4_0", "SomeOrg", "SomeOrg/test-model-7B-GGUF@rev", "q4_0", "", "", "Apache-2.0", "persona")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "test-model-7b-q4_0" || created.Org != "SomeOrg" || created.Quant != "Q4_0" ||
		created.Purpose != "persona" || created.Digest != "" {
		t.Errorf("created = %+v, want id, org, canonical q4_0 quant, persona purpose and empty digest", created)
	}

	got, err := s.GetModel(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("GetModel returned nil for a created pin")
	}
	if got.Org != "SomeOrg" || got.Source != "SomeOrg/test-model-7B-GGUF@rev" || got.Quant != "Q4_0" || got.Licence != "Apache-2.0" {
		t.Errorf("read-back = %+v, want the created fields", got)
	}

	updated, err := s.UpdateModel(created.ID, "RenamedOrg", "other-source", "q5_k_m", "", "blake3-of-the-artifact", "MIT", "Worker")
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil {
		t.Fatal("UpdateModel returned nil for an existing pin")
	}
	if updated.ID != created.ID || updated.Org != "RenamedOrg" || updated.Source != "other-source" || updated.Quant != "Q5_K_M" ||
		updated.Digest != "blake3-of-the-artifact" || updated.Licence != "MIT" || updated.Purpose != "worker" {
		t.Errorf("updated = %+v, want the replaced fields with the canonical worker purpose", updated)
	}

	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list {
		if m.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("ListModels = %+v, want the updated pin", list)
	}
}

func TestGetModelMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.GetModel("no-such-model")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("GetModel = %+v, want nil", got)
	}
}

func TestUpdateModelMissing(t *testing.T) {
	s := testStore(t)
	got, err := s.UpdateModel("no-such-model", "o", "s", "", "", "", "MIT", "persona")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("UpdateModel = %+v, want nil", got)
	}
}

func TestCreateModelDuplicateID(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateModel("dup-model", "o", "s", "", "", "", "MIT", "persona"); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateModel("dup-model", "o2", "second", "", "", "", "MIT", "worker")
	if !errors.Is(err, ErrModelIDTaken) {
		t.Fatalf("err = %v, want ErrModelIDTaken", err)
	}
}

func TestCreateModelRejectsUnknownPurpose(t *testing.T) {
	s := testStore(t)
	before, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateModel("bad-purpose", "o", "s", "", "", "", "MIT", "chat"); err == nil {
		t.Fatal("CreateModel accepted an unknown purpose")
	}
	after, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("ListModels after refused create has %d rows, want %d", len(after), len(before))
	}
}

func TestDeleteModel(t *testing.T) {
	s := testStore(t)
	created, err := s.CreateModel("doomed-model", "o", "s", "", "", "", "MIT", "stt")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteModel(created.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteModel = (%v, %v), want (true, nil)", deleted, err)
	}
	got, err := s.GetModel(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("GetModel after delete = %+v, want nil", got)
	}
	// Deleting a missing pin is not an error.
	deleted, err = s.DeleteModel(created.ID)
	if err != nil || deleted {
		t.Fatalf("second DeleteModel = (%v, %v), want (false, nil)", deleted, err)
	}
}

func TestDeleteModelInUse(t *testing.T) {
	s := testStore(t)
	// SetAssignment and SetBoxModelOverride both refuse an empty digest, so
	// the pin carries one.
	created, err := s.CreateModel("pinned-model", "o", "s", "", "", "pinned-digest", "MIT", "persona")
	if err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("pin-box", "Pin", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Referenced by an assignment: refused.
	if _, err := s.SetAssignment("persona", created.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteModel(created.ID)
	if !errors.Is(err, ErrModelInUse) || deleted {
		t.Fatalf("DeleteModel on an assigned pin = (%v, %v), want (ErrModelInUse, false)", deleted, err)
	}

	// Referenced by an override instead: refused the same way.
	if _, err := s.ClearAssignment("persona"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBoxModelOverride(box.ID, "persona", created.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err = s.DeleteModel(created.ID)
	if !errors.Is(err, ErrModelInUse) || deleted {
		t.Fatalf("DeleteModel on an overridden pin = (%v, %v), want (ErrModelInUse, false)", deleted, err)
	}

	// Unreferenced: deletes.
	if _, err := s.ClearBoxModelOverride(box.ID, "persona"); err != nil {
		t.Fatal(err)
	}
	deleted, err = s.DeleteModel(created.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteModel on a free pin = (%v, %v), want (true, nil)", deleted, err)
	}
}

func TestEnsureSeedModelsSeedsFactoryPins(t *testing.T) {
	s := testStore(t) // openStore already ran the seed
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 11 {
		t.Fatalf("ListModels = %+v, want the eleven factory pins", list)
	}
	byID := map[string]Model{}
	for _, m := range list {
		byID[m.ID] = m
	}
	tests := []struct {
		id           string
		org          string
		sourcePrefix string
		quant        string
		digest       string
		licence      string
		purpose      string
	}{
		{"qwen3.5-4b-q4_k_m", "bartowski", "bartowski/Qwen_Qwen3.5-4B-GGUF@", "Q4_K_M", "fe7ad96fac5c979c790dc2a8ae06cf85ddf1ffd5a4d4f83d1fdaddc350d17980", "Apache-2.0", "persona"},
		{"qwen2.5-coder-7b-q4_k_m", "Qwen", "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF@", "Q4_K_M", "e0abfc1f71fa8f1454f3bc443f7608a63263ed2d41664f2820c3d92c45f3bd52", "Apache-2.0", "worker"},
		{"bge-m3", "gpustack", "gpustack/bge-m3-GGUF@", "Q4_K_M", "f455475d60569f7ba086863c6ff4b79bb19201664259c5128b9f4f131408dd32", "MIT", "embedding"},
		{"faster-whisper-medium", "Systran", "Systran/faster-whisper-medium@", "", "7b1053dea7640cc96b5b65b7168487db81010bfce317115d17ca358db970673d", "MIT", "stt"},
		{"faster-whisper-large-v3", "Systran", "Systran/faster-whisper-large-v3@", "", "64b4dc2dfe6589860e4e39e0ba4f50ea0f6026e509447d8873368e1a73a3bd0a", "MIT", "stt"},
		{"silero-vad", "istupakov", "istupakov/silero-vad-onnx@", "", "bd861b19a51c83ee067b54d7d8b7f40bc11bafcc526506edc00b163e1c53bb8e", "MIT", "vad"},
		{"pl_PL-bass-high", "rhasspy", "rhasspy/piper-voices@", "", "d122a10b565681d97ae302b0a4cc617ca59e0cd87b5196ec667ff9c125357b9f", "MIT", "tts"},
		{"pl_PL-darkman-medium", "rhasspy", "rhasspy/piper-voices@", "", "7554030dd8b3cd40529098054600dc7194f4d146e29fc24f89b89a94c7a43df4", "MIT", "tts"},
		{"pl_PL-gosia-medium", "rhasspy", "rhasspy/piper-voices@", "", "cec3f38aa9c14d2dfbe43465e818253ee0ed05854288cde7bfda7131acc4fa1b", "MIT", "tts"},
		{"pl_PL-mc_speech-medium", "rhasspy", "rhasspy/piper-voices@", "", "9ee4676f29dc7125a591f7eb1bdd7a26808040183b3629a7cef56e158fc9132d", "MIT", "tts"},
		{"pl_PL-mls_6892-low", "rhasspy", "rhasspy/piper-voices@", "", "e9e2971ac7132984c6f6958c21501dec46638332b9e1bcc113a417aead270cde", "MIT", "tts"},
	}
	for _, tt := range tests {
		m, ok := byID[tt.id]
		if !ok {
			t.Errorf("seed missing pin %s", tt.id)
			continue
		}
		if m.Org != tt.org || m.Quant != tt.quant || m.Licence != tt.licence || m.Purpose != tt.purpose {
			t.Errorf("%s = org %q quant %q licence %q purpose %q, want %q/%q/%q/%q",
				tt.id, m.Org, m.Quant, m.Licence, m.Purpose, tt.org, tt.quant, tt.licence, tt.purpose)
		}
		if !strings.HasPrefix(m.Source, tt.sourcePrefix) || !strings.Contains(m.Source, "@") {
			t.Errorf("%s source = %q, want a pinned repo@rev", tt.id, m.Source)
		}
		if m.Digest != tt.digest {
			t.Errorf("%s digest = %q, want %q", tt.id, m.Digest, tt.digest)
		}
	}
}

func TestEnsureSeedModelsRestoresFactoryPins(t *testing.T) {
	s := testStore(t)
	// The admin edits a factory pin; the seed restores it to the factory
	// definition — factory pins are factory-owned, an admin customizes through
	// assignments/overrides, not by editing the pin itself.
	if _, err := s.UpdateModel("qwen3.5-4b-q4_k_m", "custom-org", "custom-source", "q4_k_m", "", "admin-computed-digest", "Custom", "persona"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetModel("qwen3.5-4b-q4_k_m")
	if err != nil || m == nil {
		t.Fatalf("GetModel after reseed = (%v, %v), want the factory row", m, err)
	}
	if m.Org != "bartowski" || m.Source != "bartowski/Qwen_Qwen3.5-4B-GGUF@4168f45a16a1290d65a4ec0fa312ae917a4c15d6" || m.Digest != "fe7ad96fac5c979c790dc2a8ae06cf85ddf1ffd5a4d4f83d1fdaddc350d17980" {
		t.Errorf("reseed did not restore the factory definition: %+v", m)
	}
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 11 {
		t.Errorf("ListModels after reseed has %d rows, want 11", len(list))
	}
}

func TestEnsureSeedModelsBumpsBoxesOnce(t *testing.T) {
	s := testStore(t)
	// Simulate a live store that predates the seed: the factory rows are
	// gone and boxes exist.
	if _, err := s.db.Exec(`DELETE FROM models`); err != nil {
		t.Fatal(err)
	}
	box, err := s.CreateBox("seed-box", "Seed", "", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox = (%v, %v), want the box", got, err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version = %d, want 2 (one seed bump)", got.ConfigVersion)
	}

	// A second run inserts nothing and bumps nothing.
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox after second run = (%v, %v)", got, err)
	}
	if got.ConfigVersion != 2 {
		t.Errorf("config_version after second run = %d, want 2 (no re-bump)", got.ConfigVersion)
	}
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 11 {
		t.Errorf("ListModels after reseed has %d rows, want 11", len(list))
	}
}

func TestOpenStoreSeedsModelsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed-models.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 11 {
		t.Errorf("fresh open has %d pins, want the eleven seeds", len(list))
	}
	box, err := s.CreateBox("reopen-box", "Reopen", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	// The pins already existed, so the reopen inserted nothing and bumped
	// nothing: the box keeps config_version 1.
	got, err := again.GetBox(box.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBox after reopen = (%v, %v)", got, err)
	}
	if got.ConfigVersion != 1 {
		t.Errorf("config_version after reopen = %d, want 1 (seed must not re-bump)", got.ConfigVersion)
	}
	list, err = again.ListModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 11 {
		t.Errorf("reopen has %d pins, want 11", len(list))
	}
}

// A store whose models table predates the org column gains it on reopen
// and the rows it carries get org backfilled from source. This is the
// live-store regression the hf-browser wave introduced: the fresh schema
// carries org, but CREATE TABLE IF NOT EXISTS never adds a column to a
// table that already exists, so the admin models page used to fail with
// "column org does not exist". The backfill only touches rows still on the
// empty default, so a second open is a no-op and an admin-set org survives.
func TestOpenStoreMigratesModelOrg(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-org-migration.db")

	// Build the pre-hf-browser shape by hand: a models table without org
	// and one seed-like row whose source carries the namespace.
	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE models (
		id      TEXT PRIMARY KEY,
		source  TEXT NOT NULL,
		quant   TEXT NOT NULL,
		digest  TEXT NOT NULL,
		licence TEXT NOT NULL,
		purpose TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO models (id, source, quant, digest, licence, purpose)
		VALUES ('pre-org-pin', 'unsloth/Qwen3.5-4B-GGUF@rev', 'Q4_K_M', '', 'Apache-2.0', 'persona')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen on a pre-org models table: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	ok, err := again.hasColumn("models", "org")
	if err != nil || !ok {
		t.Fatalf("models.org after reopen: ok=%v err=%v", ok, err)
	}
	m, err := again.GetModel("pre-org-pin")
	if err != nil || m == nil {
		t.Fatalf("GetModel after migration = (%v, %v), want the carried row", m, err)
	}
	if m.Org != "unsloth" {
		t.Errorf("migrated org = %q, want unsloth backfilled from source", m.Org)
	}

	// An admin-set org survives a reopen: the backfill never rewrites a
	// row that no longer carries the empty default.
	if _, err := again.db.Exec(`UPDATE models SET org = 'custom' WHERE id = 'pre-org-pin'`); err != nil {
		t.Fatal(err)
	}
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { third.Close() })
	m, err = third.GetModel("pre-org-pin")
	if err != nil || m == nil {
		t.Fatalf("GetModel after third open = (%v, %v), want the carried row", m, err)
	}
	if m.Org != "custom" {
		t.Errorf("org after third open = %q, want custom untouched", m.Org)
	}
}

// A store whose models table predates the canonical-quant normalization
// gains it on reopen: q4_k_m becomes Q4_K_M. The write only touches rows
// whose quant differs from its uppercase form, so a second open is a no-op
// and an empty quant (a non-GGUF pin: embedding, stt) stays empty.
func TestOpenStoreMigratesModelQuant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-quant-migration.db")

	// Build the pre-normalization shape by hand: org present (that
	// migration already ran), quants still lowercase.
	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE models (
		id      TEXT PRIMARY KEY,
		org     TEXT NOT NULL,
		source  TEXT NOT NULL,
		quant   TEXT NOT NULL,
		digest  TEXT NOT NULL,
		licence TEXT NOT NULL,
		purpose TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	rows := []string{
		`INSERT INTO models (id, org, source, quant, digest, licence, purpose)
			VALUES ('pre-quant-pin', 'unsloth', 'unsloth/Qwen3.5-4B-GGUF@rev', 'q4_k_m', '', 'Apache-2.0', 'persona')`,
		`INSERT INTO models (id, org, source, quant, digest, licence, purpose)
			VALUES ('pre-quant-embed', 'BAAI', 'BAAI/bge-m3@rev', '', '', 'MIT', 'embedding')`,
	}
	for _, insert := range rows {
		if _, err := db.Exec(insert); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen on a lowercase-quant models table: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	m, err := again.GetModel("pre-quant-pin")
	if err != nil || m == nil {
		t.Fatalf("GetModel after migration = (%v, %v), want the carried row", m, err)
	}
	if m.Quant != "Q4_K_M" {
		t.Errorf("migrated quant = %q, want Q4_K_M", m.Quant)
	}
	embed, err := again.GetModel("pre-quant-embed")
	if err != nil || embed == nil {
		t.Fatalf("GetModel of the embedding pin = (%v, %v)", embed, err)
	}
	if embed.Quant != "" {
		t.Errorf("embedding quant after migration = %q, want the empty default untouched", embed.Quant)
	}

	// A second open finds canonical quants and writes nothing.
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { third.Close() })
	m, err = third.GetModel("pre-quant-pin")
	if err != nil || m == nil {
		t.Fatalf("GetModel after third open = (%v, %v)", m, err)
	}
	if m.Quant != "Q4_K_M" {
		t.Errorf("quant after third open = %q, want Q4_K_M unchanged", m.Quant)
	}
}

func TestListModelsPublic(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/models", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/models: %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"Id"`, `"Org"`, `"Source"`, `"Quant"`, `"Licence"`} {
		if strings.Contains(string(body), banned) {
			t.Errorf("public payload leaks a Go field name: %s", banned)
		}
	}
	var got []Model
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 {
		t.Fatalf("public catalog has %d pins, want the eleven seeds", len(got))
	}
	for _, m := range got {
		if m.ID == "" || m.Org == "" || m.Source == "" || m.Licence == "" || m.Purpose == "" {
			t.Errorf("public pin with empty fields: %+v", m)
		}
	}
}

func TestAdminModelEndpoints(t *testing.T) {
	f := claimedHub(t, offering.Hosted)

	// Create.
	resp := f.do(t, http.MethodPost, "/api/admin/models", map[string]string{
		"id": "admin-model", "org": "AdminOrg", "source": "Org/repo@rev", "quant": "q4_0",
		"digest": "", "licence": "MIT", "purpose": "worker",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d, want 201", resp.StatusCode)
	}
	var created Model
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "admin-model" || created.Org != "AdminOrg" || created.Quant != "Q4_0" || created.Purpose != "worker" {
		t.Errorf("created = %+v, want the submitted pin with canonical q4_0 quant", created)
	}

	// Duplicate id.
	resp = f.do(t, http.MethodPost, "/api/admin/models", map[string]string{"id": "admin-model", "purpose": "worker"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate create: %d, want 409", resp.StatusCode)
	}

	// Bad purpose.
	resp = f.do(t, http.MethodPost, "/api/admin/models", map[string]string{"id": "bad-purpose", "purpose": "chat"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad purpose: %d, want 400", resp.StatusCode)
	}

	// Bad quant: outside the canonical GGUF dictionary.
	resp = f.do(t, http.MethodPost, "/api/admin/models", map[string]string{"id": "bad-quant", "quant": "bogus", "purpose": "worker"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad quant: %d, want 400", resp.StatusCode)
	}

	// Update.
	resp = f.do(t, http.MethodPatch, "/api/admin/models/admin-model", map[string]string{
		"org": "RenamedOrg", "source": "new-source", "quant": "", "digest": "verified-digest", "licence": "MIT", "purpose": "stt",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d, want 200", resp.StatusCode)
	}
	var updated Model
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Org != "RenamedOrg" || updated.Source != "new-source" || updated.Digest != "verified-digest" || updated.Purpose != "stt" {
		t.Errorf("updated = %+v, want the replaced fields", updated)
	}

	// Update missing.
	resp = f.do(t, http.MethodPatch, "/api/admin/models/no-such-model", map[string]string{"purpose": "worker"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("update missing: %d, want 404", resp.StatusCode)
	}

	// List includes the new pin (four seeds + one).
	resp = f.do(t, http.MethodGet, "/api/admin/models", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d, want 200", resp.StatusCode)
	}
	var list []Model
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 12 {
		t.Errorf("list has %d pins, want 12 (seeds + admin-model)", len(list))
	}

	// Delete.
	resp = f.do(t, http.MethodDelete, "/api/admin/models/admin-model", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d, want 200", resp.StatusCode)
	}

	// Delete missing.
	resp = f.do(t, http.MethodDelete, "/api/admin/models/admin-model", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete missing: %d, want 404", resp.StatusCode)
	}
}

func TestAdminModelRoutesRefuseNonAdmin(t *testing.T) {
	f := hostedCustomer(t)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/admin/models", nil},
		{http.MethodPost, "/api/admin/models", map[string]string{"id": "x", "purpose": "worker"}},
		{http.MethodPatch, "/api/admin/models/qwen3.5-4b-q4_k_m", map[string]string{"purpose": "worker"}},
		{http.MethodDelete, "/api/admin/models/qwen3.5-4b-q4_k_m", nil},
	}
	for _, c := range cases {
		resp := f.do(t, c.method, c.path, c.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}
}

func TestAdminModelRoutesRequireAuth(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/admin/models", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/admin/models bare: %d, want 401", resp.StatusCode)
	}
}
