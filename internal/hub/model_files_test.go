package hub

import (
	"path/filepath"
	"testing"

	"github.com/pleware/initagent/internal/store"
)

// A store whose models table predates the files column gains it on reopen,
// and a pin that predates it keeps its single artifact: no list is invented
// for a model the factory has not yet described as a directory. This is the
// live-store regression the same wave fixed for org and file — the fresh
// schema carries files, but CREATE TABLE IF NOT EXISTS never adds a column
// to a table that already exists.
func TestOpenStoreMigratesModelFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-files-migration.db")

	db, err := store.OpenDB(store.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE models (
		id      TEXT PRIMARY KEY,
		org     TEXT NOT NULL,
		source  TEXT NOT NULL,
		quant   TEXT NOT NULL,
		file    TEXT NOT NULL,
		digest  TEXT NOT NULL,
		licence TEXT NOT NULL,
		purpose TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO models (id, org, source, quant, file, digest, licence, purpose)
		VALUES ('pre-files-pin', 'Systran', 'Systran/faster-whisper-medium@rev', '', 'model.bin', 'anchor-digest', 'MIT', 'stt')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen on a pre-files models table: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	ok, err := again.hasColumn("models", "files")
	if err != nil || !ok {
		t.Fatalf("models.files after reopen: ok=%v err=%v", ok, err)
	}
	m, err := again.GetModel("pre-files-pin")
	if err != nil || m == nil {
		t.Fatalf("GetModel after migration = (%v, %v), want the carried row", m, err)
	}
	if len(m.Files) != 0 {
		t.Errorf("migrated files = %+v, want no list", m.Files)
	}
	if m.File != "model.bin" || m.Digest != "anchor-digest" {
		t.Errorf("migrated pin = %+v, want the anchor and its digest untouched", m)
	}
}

// SetModelFiles writes the list and reads it back, refuses a list that does
// not contain the pin's anchor, and accepts the empty list as the
// single-artifact shape it already is.
func TestSetModelFilesAnchorAndRoundTrip(t *testing.T) {
	s := testStore(t)
	m, err := s.CreateModel("dir-pin", "Systran", "Systran/faster-whisper-medium@rev",
		"", "model.bin", "anchor-digest", "MIT", "stt")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SetModelFiles(m.ID, []ModelFile{{File: "config.json"}}); err == nil {
		t.Error("SetModelFiles accepted a list without the anchor: a file-consuming program would be pointed at an artifact nothing pulls")
	}
	if _, err := s.SetModelFiles(m.ID, []ModelFile{{File: "model.bin"}, {File: "model.bin"}}); err == nil {
		t.Error("SetModelFiles accepted a duplicated file name")
	}
	if _, err := s.SetModelFiles(m.ID, []ModelFile{{File: "model.bin"}, {File: " "}}); err == nil {
		t.Error("SetModelFiles accepted an unnamed file")
	}

	want := []ModelFile{{File: "model.bin", Digest: "anchor-digest"}, {File: "config.json"}, {File: "tokenizer.json"}}
	updated, err := s.SetModelFiles(m.ID, want)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Files) != 3 || updated.Files[2].File != "tokenizer.json" || updated.Files[1].Digest != "" {
		t.Errorf("SetModelFiles stored %+v, want %+v", updated.Files, want)
	}
	reread, err := s.GetModel(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reread.Files) != 3 {
		t.Errorf("GetModel returned %d files, want 3 — the list must survive the round trip", len(reread.Files))
	}

	cleared, err := s.SetModelFiles(m.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.Files) != 0 {
		t.Errorf("SetModelFiles(nil) left %+v, want the single-artifact shape", cleared.Files)
	}
}

// A seed pass says which files a model is made of; it must never wipe a
// digest an adoption filled in. The seed's own list carries names only, so
// writing it verbatim would erase every verified digest on every sync.
func TestSeedKeepsAdoptedFileDigests(t *testing.T) {
	s := testStore(t)
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	seeded, err := s.GetModel("faster-whisper-medium")
	if err != nil || seeded == nil {
		t.Fatalf("GetModel(faster-whisper-medium) = (%v, %v), want the factory pin", seeded, err)
	}
	if len(seeded.Files) == 0 {
		t.Fatal("the factory stt pin carries no file list: the ear loads a directory")
	}

	// An adoption fills in what the bytes are.
	adopted := []ModelFile{
		{File: "model.bin", Digest: "adopted-model"},
		{File: "config.json", Digest: "adopted-config"},
		{File: "tokenizer.json", Digest: "adopted-tokenizer"},
		{File: "vocabulary.txt", Digest: "adopted-vocabulary"},
	}
	if _, err := s.SetModelFiles(seeded.ID, adopted); err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetModel(seeded.ID)
	if err != nil || after == nil {
		t.Fatalf("GetModel after the seed pass = (%v, %v)", after, err)
	}
	if len(after.Files) != 4 {
		t.Fatalf("after the seed pass files = %+v, want the four seeded names", after.Files)
	}
	for _, f := range after.Files {
		if f.Digest == "" {
			t.Errorf("file %q lost its digest to a seed pass: %+v", f.File, after.Files)
		}
	}
}

// The factory's two stt pins describe the two different snapshots as they
// actually are on disk: medium ships vocabulary.txt where large-v3 ships
// vocabulary.json, and medium has no preprocessor_config.json. A pin that
// assumed one shape for both would leave the ear unable to load one of them.
func TestSTTSeedListsTheFilesEachModelHas(t *testing.T) {
	s := testStore(t)
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"faster-whisper-medium":   {"model.bin", "config.json", "tokenizer.json", "vocabulary.txt"},
		"faster-whisper-large-v3": {"model.bin", "config.json", "tokenizer.json", "vocabulary.json", "preprocessor_config.json"},
	}
	for id, want := range cases {
		m, err := s.GetModel(id)
		if err != nil || m == nil {
			t.Fatalf("GetModel(%s) = (%v, %v)", id, m, err)
		}
		got := map[string]bool{}
		for _, f := range m.Files {
			got[f.File] = true
		}
		if len(m.Files) != len(want) {
			t.Errorf("%s files = %+v, want %v", id, m.Files, want)
			continue
		}
		for _, name := range want {
			if !got[name] {
				t.Errorf("%s is missing %q (has %+v)", id, name, m.Files)
			}
		}
		if !got[m.File] {
			t.Errorf("%s anchor %q is not in its own file list (%+v)", id, m.File, m.Files)
		}
	}
}
