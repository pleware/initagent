package hub

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
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

	_, org, err := s.ClaimHub("ops@example.com", "hash", "default")
	if err != nil {
		t.Fatalf("ClaimHub: %v", err)
	}
	proj, err := s.CreateProject(org.Id, "smoke-proj", deviceID, "/srv/smoke", "http://gateway")
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

	// Accounts are the reason to care about this test rather than trusting
	// the SQLite run: the hosted hub is the Postgres one, so the partial
	// unique index that keeps a hub to one owner has to hold *here*. A
	// syntax difference would not fail a test, it would fail a deploy.
	if _, err := s.db.Exec(`DELETE FROM org_members`); err != nil {
		t.Fatalf("clearing org members: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM orgs`); err != nil {
		t.Fatalf("clearing orgs: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM accounts`); err != nil {
		t.Fatalf("clearing accounts: %v", err)
	}
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	first := "ops-" + strconv.FormatInt(uniq, 10) + "@example.com"
	account, org, err := s.ClaimHub(first, hash, "Postgres Ops")
	if err != nil {
		t.Fatalf("ClaimHub: %v", err)
	}
	if _, _, err := s.ClaimHub("second-"+first, hash, "Second"); err == nil {
		t.Fatal("Postgres accepted a second admin account; the hub would have two owners")
	}
	// The refused claim ran three inserts inside a transaction and must have
	// rolled all of them back. On Postgres a failed statement poisons the
	// whole transaction, so a leftover org here would mean the rollback is
	// not happening at all.
	orgs, err := s.ListOrgs()
	if err != nil || len(orgs) != 1 || orgs[0].Id != org.Id {
		t.Fatalf("ListOrgs after a refused claim = (%v, %v), want only the first org", orgs, err)
	}
	if orgs[0].Members != 1 {
		t.Errorf("org member count = %d, want 1 (the aggregate join differs per dialect)", orgs[0].Members)
	}
	roster, err := s.OrgRoster(org.Id)
	if err != nil || roster.Members[account.Id] != authz.RoleOwner {
		t.Errorf("OrgRoster = (%v, %v), want the claiming account as owner", roster.Members, err)
	}

	found, err := s.AccountByEmail(first)
	if err != nil || found == nil {
		t.Fatalf("AccountByEmail = (%v, %v), want the account", found, err)
	}
	if !found.VerifyPassword("correct-horse-battery") {
		t.Error("password did not verify against the row read back from Postgres")
	}
	if n, err := s.CountAccounts(); err != nil || n != 1 {
		t.Fatalf("CountAccounts = (%d, %v), want (1, nil)", n, err)
	}

	// The live hosted hub was claimed on `v0.2.0`, which shipped the claim
	// before organizations, so its first start on a newer image runs
	// BackfillOperatorOrg against Postgres — an admin account, no orgs. That
	// is the one code path production takes and the SQLite test cannot prove:
	// dropping the orgs while keeping the account reproduces it exactly.
	if _, err := s.db.Exec(`DELETE FROM org_members`); err != nil {
		t.Fatalf("clearing org members: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM orgs`); err != nil {
		t.Fatalf("clearing orgs: %v", err)
	}
	filled, err := s.BackfillOperatorOrg()
	if err != nil || filled == nil {
		t.Fatalf("BackfillOperatorOrg on Postgres = (%v, %v), want an org", filled, err)
	}
	if filled.Name != auth.DefaultOrgName {
		t.Errorf("backfilled org name = %q, want %q", filled.Name, auth.DefaultOrgName)
	}
	backRoster, err := s.OrgRoster(filled.Id)
	if err != nil || backRoster.Members[account.Id] != authz.RoleOwner {
		t.Fatalf("backfilled roster = (%v, %v), want the admin as owner", backRoster.Members, err)
	}
	if again, err := s.BackfillOperatorOrg(); err != nil || again != nil {
		t.Fatalf("second BackfillOperatorOrg = (%v, %v), want (nil, nil) — every restart calls it", again, err)
	}
}

func TestOpenStorePostgresMigratesLegacyProjectsTable(t *testing.T) {
	dsn := os.Getenv("INITAGENT_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("INITAGENT_TEST_POSTGRES_URL not set; skipping Postgres integration test")
	}

	s, err := OpenStorePostgres(dsn)
	if err != nil {
		t.Fatalf("OpenStorePostgres: %v", err)
	}
	// Recreate the live v0.3.1 shape: projects without org_id / gateway_url.
	// v0.3.2 put CREATE INDEX ON org_id in the CREATE TABLE batch, so open
	// died before the ALTER and the hosted hub crash-looped.
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS projects_org_id`); err != nil {
		t.Fatalf("dropping projects_org_id: %v", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE projects DROP COLUMN IF EXISTS org_id`); err != nil {
		t.Fatalf("dropping org_id: %v", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE projects DROP COLUMN IF EXISTS gateway_url`); err != nil {
		t.Fatalf("dropping gateway_url: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenStorePostgres(dsn)
	if err != nil {
		t.Fatalf("OpenStorePostgres on a pre-org projects table: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, col := range []string{"org_id", "gateway_url"} {
		ok, err := s.hasColumn("projects", col)
		if err != nil || !ok {
			t.Fatalf("column %s after reopen: ok=%v err=%v", col, ok, err)
		}
	}
}
