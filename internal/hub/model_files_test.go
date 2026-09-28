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
		"", "model.bin", "anchor-digest", "MIT", []string{"stt"})
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
	seeded, err := s.GetModel("laya-multilingual")
	if err != nil || seeded == nil {
		t.Fatalf("GetModel(laya-multilingual) = (%v, %v), want the factory pin", seeded, err)
	}
	if len(seeded.Files) == 0 {
		t.Fatal("the factory encoder pin carries no file list: the encoder loads a directory")
	}

	// An adoption fills in what the bytes are.
	adopted := make([]ModelFile, 0, len(seeded.Files))
	for _, f := range seeded.Files {
		adopted = append(adopted, ModelFile{File: f.File, Digest: "adopted-" + f.File})
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
	if len(after.Files) != len(seeded.Files) {
		t.Fatalf("after the seed pass files = %+v, want the %d seeded names", after.Files, len(seeded.Files))
	}
	for _, f := range after.Files {
		if f.Digest == "" {
			t.Errorf("file %q lost its digest to a seed pass: %+v", f.File, after.Files)
		}
	}
}

// The other half of the merge: a store whose files carry no digests yet takes
// the seed's. That is the live hub's own state — the pin was created before the
// sums were known — and it needs the CHANGE DETECTION to notice it, not only the
// merge to be right: a name-only comparison writes nothing and the row stays
// bare for ever, which is exactly what the first attempt at this shipped.
func TestSeedFillsDigestsTheStoreDoesNotHave(t *testing.T) {
	s := testStore(t)
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	seeded, err := s.GetModel("laya-multilingual")
	if err != nil || seeded == nil {
		t.Fatalf("GetModel(laya-multilingual) = (%v, %v)", seeded, err)
	}
	if len(seeded.Files) == 0 {
		t.Fatal("the factory encoder pin carries no file list")
	}

	// A store whose list predates the sums: the seeded names, no digests.
	namesOnly := make([]ModelFile, 0, len(seeded.Files))
	for _, f := range seeded.Files {
		namesOnly = append(namesOnly, ModelFile{File: f.File})
	}
	if _, err := s.SetModelFiles(seeded.ID, namesOnly); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetModel(seeded.ID); err != nil || got.Files[0].Digest != "" {
		t.Fatalf("the store was given no digests and holds %+v (%v)", got.Files, err)
	}

	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetModel(seeded.ID)
	if err != nil || after == nil {
		t.Fatalf("GetModel after the seed pass = (%v, %v)", after, err)
	}
	for _, f := range after.Files {
		if f.Digest == "" {
			t.Errorf("%q still carries no digest after a seed pass — the change went undetected", f.File)
		}
	}
}

// The factory's stt pin is ONE artifact, and a different kind from the two
// that came before it: whisper.cpp opens the .bin and nothing beside it, so
// the pin carries no file list. The directory shape belonged to
// faster-whisper's snapshot, which the box no longer serves — and a pin that
// grew a file list would be a box pulling files nothing opens.
func TestTheSTTPinIsOneArtifact(t *testing.T) {
	s := testStore(t)
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetModel("ggml-large-v3-turbo")
	if err != nil || m == nil {
		t.Fatalf("GetModel(ggml-large-v3-turbo) = (%v, %v), want the factory pin", m, err)
	}
	if len(m.Files) != 0 {
		t.Errorf("the stt pin lists %+v, want the single-artifact shape", m.Files)
	}
	if m.File != "ggml-large-v3-turbo.bin" || m.Quant != "" {
		t.Errorf("the stt pin names %q (quant %q), want a ggml .bin and no quantization", m.File, m.Quant)
	}
	if m.Engine != "whisper.cpp" {
		t.Errorf("engine = %q, want whisper.cpp — the engine is what tells the box which program opens these bytes", m.Engine)
	}
}

// A pin whose files carry digests is the shape a box can actually pull: zest is
// handed only one digest per file, so an empty one is not "pulled unverified" —
// zest downloads the whole artifact and then fails it as a DigestMismatch, which
// is how three files of medium's snapshot were thrown away before their sums were
// known (99). The typed-decision encoder is the factory's directory-shaped pin
// today, so its list is the one held to that rule.
func TestVerifiedEncoderFilesCarryTheirDigest(t *testing.T) {
	s := testStore(t)
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetModel("laya-multilingual")
	if err != nil || m == nil {
		t.Fatalf("GetModel(laya-multilingual) = (%v, %v)", m, err)
	}
	if len(m.Files) == 0 {
		t.Fatal("the pin names no files")
	}
	for _, f := range m.Files {
		if f.Digest == "" {
			t.Errorf("%q carries no digest — a pull of it downloads the file and then fails it", f.File)
		}
	}
	if m.Files[0].File != m.File {
		t.Errorf("anchor is %q, want the pin's own %q listed first", m.Files[0].File, m.File)
	}
	for _, f := range m.Files {
		if f.File == m.File && f.Digest != m.Digest {
			t.Errorf("the anchor entry's digest %q is not the pin's own %q", f.Digest, m.Digest)
		}
	}
}

// The pin's own digest, one level up from the file list, and the same trap: a
// row seeded before the artifact was hashed has to take the factory's sum on
// the next pass, and a row that already carries one has to keep it. Leaving
// the column out of the UPDATE outright (the first attempt at this) meant the
// pin stayed bare for ever on every box that had already seeded it, while a
// fresh box took the sum at INSERT — so a deploy could not fix it either.
func TestSeedFillsThePinDigestTheStoreDoesNotHave(t *testing.T) {
	s := testStore(t)
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	seeded, err := s.GetModel("laya-multilingual")
	if err != nil || seeded == nil {
		t.Fatalf("GetModel(laya-multilingual) = (%v, %v), want the factory pin", seeded, err)
	}
	if seeded.Digest == "" {
		t.Fatal("the factory pin landed with no digest, and the seed carries one")
	}

	// A box that seeded the pin before the artifact was hashed.
	if _, err := s.UpdateModel(seeded.ID, seeded.Org, seeded.Source, seeded.Quant,
		seeded.File, "", seeded.Licence, seeded.Purposes); err != nil {
		t.Fatal(err)
	}
	emptied, err := s.GetModel(seeded.ID)
	if err != nil || emptied == nil {
		t.Fatalf("GetModel after emptying = (%v, %v)", emptied, err)
	}
	if emptied.Digest != "" {
		t.Fatalf("digest = %q, want the row emptied for the test", emptied.Digest)
	}

	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	filled, err := s.GetModel(seeded.ID)
	if err != nil || filled == nil {
		t.Fatalf("GetModel after the seed pass = (%v, %v)", filled, err)
	}
	if filled.Digest != seeded.Digest {
		t.Errorf("digest after a seed pass = %q, want the factory sum %q — the change went undetected",
			filled.Digest, seeded.Digest)
	}

	// The other half: a sum an adoption or an admin wrote is never overwritten.
	if _, err := s.UpdateModel(seeded.ID, seeded.Org, seeded.Source, seeded.Quant,
		seeded.File, "verified-elsewhere", seeded.Licence, seeded.Purposes); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatal(err)
	}
	kept, err := s.GetModel(seeded.ID)
	if err != nil || kept == nil {
		t.Fatalf("GetModel after the second seed pass = (%v, %v)", kept, err)
	}
	if kept.Digest != "verified-elsewhere" {
		t.Errorf("digest = %q, want the verified sum kept against the factory's", kept.Digest)
	}
}
