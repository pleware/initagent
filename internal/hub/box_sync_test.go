package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

// boxWithToken creates a box and mints its sync credential.
func boxWithToken(t *testing.T, s *Store, slug string) (*Box, string) {
	t.Helper()
	box, err := s.CreateBox(slug, slug, "", "")
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := s.CreateBoxToken(box.ID, "sync")
	if err != nil {
		t.Fatal(err)
	}
	return box, secret
}

// boxChangesGET polls the sync endpoint on a plain client — no session jar —
// with an optional bearer secret, proving the route needs neither.
func boxChangesGET(t *testing.T, f *adminFixture, boxID, secret, query string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.ts.URL+"/api/boxes/"+boxID+"/changes"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// A fresh box starts at version 1, so the first poll at since=0 serves the
// manifest; a poll at or past the current version is a 304; a missing since
// means the first poll; a negative or non-integer since is refused.
func TestBoxChangesPollSemantics(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, secret := boxWithToken(t, f.srv.store, "box-poll")

	resp := boxChangesGET(t, f, box.ID, secret, "?since=0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("poll at since=0: %d, want 200", resp.StatusCode)
	}
	var manifest map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["version"] != float64(1) {
		t.Errorf("manifest version = %v, want 1 on a fresh box", manifest["version"])
	}
	for _, key := range []string{"box", "orgs", "staff", "narrator", "models"} {
		if _, ok := manifest[key]; !ok {
			t.Errorf("manifest has no %q key: %v", key, manifest)
		}
	}

	for _, since := range []string{"1", "2"} {
		resp := boxChangesGET(t, f, box.ID, secret, "?since="+since)
		if resp.StatusCode != http.StatusNotModified {
			t.Errorf("poll at since=%s: %d, want 304", since, resp.StatusCode)
		}
	}

	resp = boxChangesGET(t, f, box.ID, secret, "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("poll without since: %d, want 200", resp.StatusCode)
	}

	for _, query := range []string{"?since=-1", "?since=abc"} {
		resp := boxChangesGET(t, f, box.ID, secret, query)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("poll %s: %d, want 400", query, resp.StatusCode)
		}
	}
}

// The sync door opens only to the box's own token: no bearer, a session
// cookie, an api token, an unknown secret and a revoked token are all 401.
func TestBoxChangesRefusesOtherCredentials(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, secret := boxWithToken(t, f.srv.store, "box-cred")

	resp := boxChangesGET(t, f, box.ID, "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("poll without a bearer: %d, want 401", resp.StatusCode)
	}
	resp = f.do(t, http.MethodGet, "/api/boxes/"+box.ID+"/changes", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("poll with the operator's session: %d, want 401", resp.StatusCode)
	}

	apiSecret, _, err := f.srv.store.CreateAdminToken("ci", f.ownerId, []authz.Capability{authz.ReadBox, authz.AdminBox})
	if err != nil {
		t.Fatal(err)
	}
	resp = boxChangesGET(t, f, box.ID, apiSecret, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("poll with an api token: %d, want 401", resp.StatusCode)
	}
	resp = boxChangesGET(t, f, box.ID, "not-a-box-secret", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("poll with an unknown secret: %d, want 401", resp.StatusCode)
	}

	tokens, err := f.srv.store.ListBoxTokens(box.ID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListBoxTokens = (%v, %v), want the one token", tokens, err)
	}
	if revoked, err := f.srv.store.RevokeBoxToken(tokens[0].Id, box.ID); err != nil || !revoked {
		t.Fatalf("RevokeBoxToken = (%v, %v), want (true, nil)", revoked, err)
	}
	resp = boxChangesGET(t, f, box.ID, secret, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("poll with a revoked token: %d, want 401", resp.StatusCode)
	}
}

// A box token is bound to its box: box A's token must not read box B's
// manifest, and each box reads its own.
func TestBoxChangesPathCrossCheck(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	boxA, secretA := boxWithToken(t, f.srv.store, "box-a")
	boxB, _ := boxWithToken(t, f.srv.store, "box-b")

	resp := boxChangesGET(t, f, boxB.ID, secretA, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("box A token on box B: %d, want 403", resp.StatusCode)
	}
	resp = boxChangesGET(t, f, boxA.ID, secretA, "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("box A token on box A: %d, want 200", resp.StatusCode)
	}
}

// A valid token whose box row is gone answers 404, not 401: the credential
// resolved, the box did not.
func TestBoxChangesMissingBox(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	box, secret := boxWithToken(t, f.srv.store, "box-vanished")
	if _, err := f.srv.store.db.Exec(`DELETE FROM boxes WHERE id = ?`, box.ID); err != nil {
		t.Fatal(err)
	}
	resp := boxChangesGET(t, f, box.ID, secret, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("poll on a vanished box: %d, want 404", resp.StatusCode)
	}
}

// An org-scoped staff write bumps every box — the canonical roster rides in
// every manifest — while a box-scoped narrator seed does not, and a fresh
// box lands at exactly version 1.
func TestOrgScopedStaffBumpsAllBoxes(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	s := f.srv.store
	boxA, err := s.CreateBox("box-bump-a", "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	boxB, err := s.CreateBox("box-bump-b", "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := s.CreateBoxToken(boxA.ID, "a")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpsertStaff("staff-custom-00", "Custom", "pl", "", "", "", "voice-x", "female", 30, 0, neutralBigFive()); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.GetBox(boxA.ID); a.ConfigVersion != 2 {
		t.Errorf("box A after an org-scoped upsert = %d, want 2", a.ConfigVersion)
	}
	if b, _ := s.GetBox(boxB.ID); b.ConfigVersion != 2 {
		t.Errorf("box B after an org-scoped upsert = %d, want 2", b.ConfigVersion)
	}

	// The staff seed is idempotent: a second run skips and bumps nothing.
	if err := s.EnsureSeedStaff(); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.GetBox(boxA.ID); a.ConfigVersion != 2 {
		t.Errorf("box A after a second seed = %d, want 2 (no re-bump)", a.ConfigVersion)
	}

	// Creating another box seeds its narrator as the box's own being: no
	// bump anywhere, and the new box is exactly version 1.
	boxC, err := s.CreateBox("box-bump-c", "C", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := s.GetBox(boxC.ID); c.ConfigVersion != 1 {
		t.Errorf("fresh box config_version = %d, want 1", c.ConfigVersion)
	}
	if a, _ := s.GetBox(boxA.ID); a.ConfigVersion != 2 {
		t.Errorf("box A after creating box C = %d, want 2 (narrator seed must not bump)", a.ConfigVersion)
	}
	// A narrator edit bumps only that box.
	if _, err := s.UpdateBoxNarrator(boxC.ID, "Lore", "en", "", "", "", "voice-y", "female", 0, 0, neutralBigFive()); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.GetBox(boxC.ID); c.ConfigVersion != 2 {
		t.Errorf("box C after a narrator tune = %d, want 2 (a narrator edit bumps the box)", c.ConfigVersion)
	}

	// A poll with the pre-bump version serves the new manifest.
	resp := boxChangesGET(t, f, boxA.ID, secret, "?since=1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("poll after the org-scoped upsert: %d, want 200", resp.StatusCode)
	}
	var manifest map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["version"] != float64(2) {
		t.Errorf("manifest version = %v, want 2", manifest["version"])
	}
}

// Renaming an organization reaches the manifest of its bound boxes only,
// and a poll at the pre-rename version serves the new name.
func TestRenameOrgBumpsBoundBoxes(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	s := f.srv.store
	org, err := s.CreateOrg("Before name")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := s.CreateBox("box-renamed", "Bound", "", "")
	if err != nil {
		t.Fatal(err)
	}
	unbound, err := s.CreateBox("box-renamed-free", "Free", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(bound.ID, []string{org.Id}); err != nil {
		t.Fatal(err)
	}
	secret, _, err := s.CreateBoxToken(bound.ID, "sync")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.GetBox(bound.ID)
	if err != nil || before == nil {
		t.Fatalf("GetBox = (%v, %v)", before, err)
	}

	if err := s.RenameOrg(org.Id, "After name"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetBox(bound.ID); got.ConfigVersion != before.ConfigVersion+1 {
		t.Errorf("bound box after rename = %d, want %d", got.ConfigVersion, before.ConfigVersion+1)
	}
	if got, _ := s.GetBox(unbound.ID); got.ConfigVersion != 1 {
		t.Errorf("unbound box after rename = %d, want 1 (rename must not touch it)", got.ConfigVersion)
	}

	resp := boxChangesGET(t, f, bound.ID, secret, fmt.Sprintf("?since=%d", before.ConfigVersion))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("poll at the pre-rename version: %d, want 200", resp.StatusCode)
	}
	var manifest map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	orgs := manifest["orgs"].([]any)
	if len(orgs) != 1 || orgs[0].(map[string]any)["name"] != "After name" {
		t.Errorf("manifest orgs after rename = %v, want the new name", orgs)
	}
}

// An org level reaches the bound boxes only: setting it bumps exactly the
// boxes carrying the org, and the level rides in their manifest (08).
func TestSetOrgLevelBumpsBoundBoxes(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	s := f.srv.store
	org, err := s.CreateOrg("Family org")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := s.CreateBox("box-level", "Bound", "", "")
	if err != nil {
		t.Fatal(err)
	}
	unbound, err := s.CreateBox("box-level-free", "Free", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(bound.ID, []string{org.Id}); err != nil {
		t.Fatal(err)
	}
	secret, _, err := s.CreateBoxToken(bound.ID, "sync")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.GetBox(bound.ID)
	if err != nil || before == nil {
		t.Fatalf("GetBox = (%v, %v)", before, err)
	}

	if err := s.SetOrgLevel(org.Id, OrgLevelChild); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetBox(bound.ID); got.ConfigVersion != before.ConfigVersion+1 {
		t.Errorf("bound box after set level = %d, want %d", got.ConfigVersion, before.ConfigVersion+1)
	}
	if got, _ := s.GetBox(unbound.ID); got.ConfigVersion != 1 {
		t.Errorf("unbound box after set level = %d, want 1 (level must not touch it)", got.ConfigVersion)
	}

	resp := boxChangesGET(t, f, bound.ID, secret, fmt.Sprintf("?since=%d", before.ConfigVersion))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("poll at the pre-level version: %d, want 200", resp.StatusCode)
	}
	var manifest map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	orgs := manifest["orgs"].([]any)
	if len(orgs) != 1 || orgs[0].(map[string]any)["level"] != "child" {
		t.Errorf("manifest orgs after set level = %v, want level child", orgs)
	}
}

// An org staff override reaches the bound boxes only, in both directions:
// setting and clearing it each bump exactly the boxes carrying the org.
func TestOrgStaffOverrideBumpsBoundBoxes(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	s := f.srv.store
	org, err := s.CreateOrg("Override org")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := s.CreateBox("box-overridden", "Bound", "", "")
	if err != nil {
		t.Fatal(err)
	}
	unbound, err := s.CreateBox("box-overridden-free", "Free", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBoxOrgs(bound.ID, []string{org.Id}); err != nil {
		t.Fatal(err)
	}
	roster, err := s.StaffForOrg(org.Id)
	if err != nil || len(roster) == 0 {
		t.Fatalf("StaffForOrg = (%v, %v), want the seeded roster", roster, err)
	}

	newVoice := "pl_PL-test-voice"
	if err := s.SetOrgStaffOverride(org.Id, roster[0].ID, nil, nil, nil, &newVoice, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetBox(bound.ID); got.ConfigVersion != 3 {
		t.Errorf("bound box after SetOrgStaffOverride = %d, want 3", got.ConfigVersion)
	}
	if got, _ := s.GetBox(unbound.ID); got.ConfigVersion != 1 {
		t.Errorf("unbound box after SetOrgStaffOverride = %d, want 1", got.ConfigVersion)
	}

	if err := s.ClearOrgStaffOverride(org.Id, roster[0].ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetBox(bound.ID); got.ConfigVersion != 4 {
		t.Errorf("bound box after ClearOrgStaffOverride = %d, want 4", got.ConfigVersion)
	}
	if got, _ := s.GetBox(unbound.ID); got.ConfigVersion != 1 {
		t.Errorf("unbound box after ClearOrgStaffOverride = %d, want 1", got.ConfigVersion)
	}
}
