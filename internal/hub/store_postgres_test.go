package hub

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// TestStorePostgresSmoke exercises the hub store against a real Postgres so
// the dialect-specific paths — schemaPostgres (BIGSERIAL / BIGINT), RETURNING
// id, placeholder rebinding, and the ON CONFLICT upsert — are proven, not
// assumed. It is gated on INITAGENT_TEST_POSTGRES_URL so the default run and
// CI never require a live database (try-testable: temporary SQLite is the
// normal path; Postgres is opt-in).
func TestStorePostgresSmoke(t *testing.T) {
	dsn := os.Getenv("INITAGENT_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("INITAGENT_TEST_POSTGRES_URL not set; skipping Postgres integration test")
	}

	s, err := OpenStorePostgres(dsn)
	if err != nil {
		t.Fatalf("OpenStorePostgres: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	uniq := time.Now().UnixNano()

	// Presets are seeded by open; assert they exist, then add one more so the
	// RETURNING id path is exercised against BIGSERIAL.
	seeded, err := s.ListPresets()
	if err != nil {
		t.Fatalf("ListPresets: %v", err)
	}
	if len(seeded) < 3 {
		t.Fatalf("expected seeded presets, got %d", len(seeded))
	}
	presetID, err := s.CreatePreset("smoke-"+strconv.FormatInt(uniq, 10), "echo hi", "shell")
	if err != nil {
		t.Fatalf("CreatePreset (RETURNING id): %v", err)
	}
	if presetID == 0 {
		t.Fatal("CreatePreset returned a zero id; RETURNING did not fire")
	}

	// Settings exercise the ON CONFLICT upsert against Postgres.
	if err := s.SetSetting("smoke_key", "one"); err != nil {
		t.Fatalf("SetSetting insert: %v", err)
	}
	if err := s.SetSetting("smoke_key", "two"); err != nil {
		t.Fatalf("SetSetting upsert: %v", err)
	}
	if v, _ := s.Setting("smoke_key"); v != "two" {
		t.Fatalf("Setting after upsert = %q, want %q", v, "two")
	}

	// Device + project + token cover the remaining rebinding paths.
	deviceID, token, err := s.CreateDevice("smoke", "smoke.local", "linux", "amd64", false)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if d, _ := s.DeviceByToken(token); d == nil || d.Id != deviceID {
		t.Fatalf("DeviceByToken: got %+v", d)
	}

	proj, err := s.CreateProject("smoke-proj", deviceID, "/srv/smoke")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if got, _ := s.ProjectById(proj.Id); got == nil {
		t.Fatal("ProjectById returned nil for a fresh project")
	}

	enroll, err := s.CreateEnrollToken(time.Minute)
	if err != nil {
		t.Fatalf("CreateEnrollToken: %v", err)
	}
	if ok, _ := s.ConsumeEnrollToken(enroll); !ok {
		t.Fatal("enroll token did not consume on first use")
	}

	apiTok, err := s.CreateApiToken("smoke-ci")
	if err != nil {
		t.Fatalf("CreateApiToken: %v", err)
	}
	if ok, _ := s.ValidApiToken(apiTok); !ok {
		t.Fatal("fresh api token should validate")
	}
}
