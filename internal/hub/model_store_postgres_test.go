package hub

import (
	"os"
	"testing"
)

// TestModelStorePostgresSeed is the models half of the Postgres smoke: the
// migration that adds a column and the seed pass that fills it have to run on
// Postgres too, not only on the temporary SQLite the default run uses.
//
// The gap this closes was real — `TestStorePostgresSmoke` never touched
// `models`, so a schema or SQL mistake in that table reached the live hub
// unseen while every local and CI test stayed green.
func TestModelStorePostgresSeed(t *testing.T) {
	dsn := os.Getenv("INITAGENT_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("INITAGENT_TEST_POSTGRES_URL not set; skipping Postgres integration test")
	}
	s, err := OpenStorePostgres(dsn)
	if err != nil {
		t.Fatalf("OpenStorePostgres: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// The seed pass runs on open. Every factory pin must be readable with
	// its metadata — a SELECT list that disagrees with scanModel's
	// destinations fails here and nowhere else.
	list, err := s.ListModels()
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(list) != 23 {
		t.Fatalf("ListModels = %d rows, want the twenty-three factory pins", len(list))
	}

	m, err := s.GetModel("ternary-bonsai-2-27b-pq2_0")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if m == nil {
		t.Fatal("GetModel(ternary pin) = nil after the seed pass")
	}
	if m.Size != 7206168928 {
		t.Errorf("size = %d, want 7206168928", m.Size)
	}
	if len(m.Files) != 2 {
		t.Fatalf("files = %+v, want the two artifacts", m.Files)
	}
	if m.Files[1].Size != 629246976 {
		t.Errorf("projector size = %d, want 629246976", m.Files[1].Size)
	}

	// A second open proves the migration is idempotent (the column exists
	// and the drift check is quiet), and a re-seed proves the size survives
	// a pass that does not rewrite the row.
	if err := s.EnsureSeedModels(); err != nil {
		t.Fatalf("EnsureSeedModels on a seeded Postgres store: %v", err)
	}
	again, err := s.GetModel("ternary-bonsai-2-27b-pq2_0")
	if err != nil || again == nil {
		t.Fatalf("GetModel after reseed = (%v, %v)", again, err)
	}
	if again.Size != 7206168928 {
		t.Errorf("size after reseed = %d, want it kept", again.Size)
	}

	// The repair path, which is the one the live hub needed: the first cut of
	// the column was int4 on every dialect, so a hub that took that migration
	// carries models.size as INTEGER and cannot store a 27B pin's count —
	// ensureColumn sees the column, adds nothing, and the hub crash-loops.
	// Put the column back into that shape and prove the next open widens it.
	if _, err := s.db.Exec(`UPDATE models SET size = 0`); err != nil {
		t.Fatalf("zeroing sizes: %v", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE models ALTER COLUMN size TYPE INTEGER`); err != nil {
		t.Fatalf("narrowing size to int4: %v", err)
	}
	_ = s.Close()

	repaired, err := OpenStorePostgres(dsn)
	if err != nil {
		t.Fatalf("open on a store carrying models.size as int4: %v", err)
	}
	t.Cleanup(func() { _ = repaired.Close() })
	var dataType string
	if err := repaired.db.QueryRow(`SELECT data_type FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'models' AND column_name = 'size'`).Scan(&dataType); err != nil {
		t.Fatalf("reading models.size's type: %v", err)
	}
	if dataType != "bigint" {
		t.Errorf("models.size type = %q, want bigint after the repair", dataType)
	}
	m, err = repaired.GetModel("ternary-bonsai-2-27b-pq2_0")
	if err != nil || m == nil {
		t.Fatalf("GetModel after repair = (%v, %v)", m, err)
	}
	if m.Size != 7206168928 {
		t.Errorf("size after repair = %d, want the seed's 7206168928", m.Size)
	}
}
