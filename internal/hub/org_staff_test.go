package hub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

// A plain member reads the effective roster: the canonical rows with this
// org's overrides applied, and the override an org admin writes shows up in
// the member's view. A stranger's session learns nothing — the org is hidden.
func TestOrgMemberReadsEffectiveStaff(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	f.addMember(t, "dev@example.com", "another-long-password", authz.RoleMember)
	memberClient := f.signIn(t, "dev@example.com", "another-long-password")

	resp := requestJSON(t, f.ts, memberClient, http.MethodGet, "/api/orgs/"+f.orgId+"/staff", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member GET staff: %d, want 200", resp.StatusCode)
	}
	var roster []Staff
	if err := json.NewDecoder(resp.Body).Decode(&roster); err != nil {
		t.Fatal(err)
	}
	if len(roster) != 2 {
		t.Fatalf("effective roster = %d, want the two canonical members", len(roster))
	}
	target := roster[0]
	if target.Brief != "" {
		t.Errorf("brief before any override = %q, want the empty canonical value", target.Brief)
	}

	resp = f.do(t, http.MethodPatch, "/api/orgs/"+f.orgId+"/staff/"+target.ID,
		map[string]any{"brief": "shorter hello"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("override brief: %d, want 200", resp.StatusCode)
	}

	resp = requestJSON(t, f.ts, memberClient, http.MethodGet, "/api/orgs/"+f.orgId+"/staff", nil)
	if err := json.NewDecoder(resp.Body).Decode(&roster); err != nil {
		t.Fatal(err)
	}
	if got := staffBySlug(t, roster, target.Slug); got.Brief != "shorter hello" {
		t.Errorf("member's view carries brief %q after the override, want the org's value", got.Brief)
	}

	hash, err := auth.HashPassword("stranger-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.store.CreateAccount("stranger@example.com", hash); err != nil {
		t.Fatal(err)
	}
	strangerClient := f.signIn(t, "stranger@example.com", "stranger-password")
	resp = requestJSON(t, f.ts, strangerClient, http.MethodGet, "/api/orgs/"+f.orgId+"/staff", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("stranger GET staff: %d, want 404", resp.StatusCode)
	}
}

// An org admin sets every overridable field, the effective roster carries
// them, another org keeps the pure canonical row, and clearing returns the
// member to the canon. The override never leaks across orgs.
func TestOrgAdminSetsAndClearsStaffOverride(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	f.addMember(t, "admin@example.com", "another-long-password", authz.RoleAdmin)
	adminClient := f.signIn(t, "admin@example.com", "another-long-password")

	canonical, err := f.srv.store.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	target := canonical[0]
	other, err := f.srv.store.CreateOrg("Other Org")
	if err != nil {
		t.Fatal(err)
	}

	resp := requestJSON(t, f.ts, adminClient, http.MethodPatch, "/api/orgs/"+f.orgId+"/staff/"+target.ID, map[string]any{
		"name": "Renamed", "age": 55, "soulOverride": "terse and warm", "voice": "voice-7",
		"bigFive": map[string]float64{
			"openness": 0.9, "conscientiousness": 0.7, "extraversion": 0.4,
			"agreeableness": 0.6, "neuroticism": 0.2,
		},
		"brief": "short hello", "avatarModel3d": "custom.glb", "wordBudget": 700,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set override: %d, want 200", resp.StatusCode)
	}

	effective, err := f.srv.store.StaffForOrg(f.orgId)
	if err != nil {
		t.Fatal(err)
	}
	got := staffBySlug(t, effective, target.Slug)
	if got.Brief != "short hello" || got.AvatarModel3D != "custom.glb" || got.WordBudget != 700 ||
		got.BigFive.Openness != 0.9 || got.BigFive.Neuroticism != 0.2 {
		t.Errorf("effective row = %+v, want every overridden field", got)
	}
	if got.Name != "Renamed" || got.Age != 55 || got.SoulOverride != "terse and warm" || got.Voice != "voice-7" {
		t.Errorf("effective row = %+v, want the overridden name, age, soul and voice", got)
	}
	if got.SoulCore != target.SoulCore {
		t.Errorf("SoulCore = %q, want the canonical %q kept separate from the override", got.SoulCore, target.SoulCore)
	}

	otherEffective, err := f.srv.store.StaffForOrg(other.Id)
	if err != nil {
		t.Fatal(err)
	}
	if otherGot := staffBySlug(t, otherEffective, target.Slug); otherGot.Brief != "" || otherGot.AvatarModel3D != target.AvatarModel3D {
		t.Errorf("other org's row = %+v, want the untouched canonical fields", otherGot)
	}

	resp = requestJSON(t, f.ts, adminClient, http.MethodDelete,
		"/api/orgs/"+f.orgId+"/staff/"+target.ID+"/override", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear override: %d, want 200", resp.StatusCode)
	}
	effective, err = f.srv.store.StaffForOrg(f.orgId)
	if err != nil {
		t.Fatal(err)
	}
	if cleared := staffBySlug(t, effective, target.Slug); cleared.Brief != "" || cleared.WordBudget != 0 ||
		cleared.AvatarModel3D != target.AvatarModel3D || cleared.BigFive.Openness != target.BigFive.Openness ||
		cleared.Name != target.Name || cleared.Age != target.Age ||
		cleared.SoulCore != target.SoulCore || cleared.SoulOverride != "" || cleared.Voice != target.Voice {
		t.Errorf("row after clear = %+v, want the canonical fields back", cleared)
	}
}

// A name override has to be a real name: one character or whitespace-only
// rejects 400, a trimmable name lands trimmed, and a negative age rejects
// 400. The same admin session writes and reads through the wire.
func TestOrgStaffOverrideValidatesFields(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	f.addMember(t, "admin@example.com", "another-long-password", authz.RoleAdmin)
	adminClient := f.signIn(t, "admin@example.com", "another-long-password")

	canonical, err := f.srv.store.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	target := canonical[0]

	for _, c := range []struct {
		name string
		body map[string]any
	}{
		{"a one-character name", map[string]any{"name": "X"}},
		{"a whitespace-only name", map[string]any{"name": "   "}},
		{"a negative age", map[string]any{"age": -1}},
	} {
		resp := requestJSON(t, f.ts, adminClient, http.MethodPatch, "/api/orgs/"+f.orgId+"/staff/"+target.ID, c.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", c.name, resp.StatusCode)
		}
	}

	resp := requestJSON(t, f.ts, adminClient, http.MethodPatch,
		"/api/orgs/"+f.orgId+"/staff/"+target.ID, map[string]any{"name": "  Ren  "})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set a trimmable name: %d, want 200", resp.StatusCode)
	}
	effective, err := f.srv.store.StaffForOrg(f.orgId)
	if err != nil {
		t.Fatal(err)
	}
	if got := staffBySlug(t, effective, target.Slug); got.Name != "Ren" {
		t.Errorf("effective name = %q, want the trimmed value", got.Name)
	}
}

// Setting or clearing an override is organization administration. The hide
// half of hideOrRefuse answers a session that cannot see the org with 404;
// the refuse half answers a token already scoped into the org by name with
// 403 — a member's token whose account lacks the role, or a read-only
// token missing the verb.
func TestOrgStaffOverrideRefusals(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	member := f.addMember(t, "dev@example.com", "another-long-password", authz.RoleMember)
	memberClient := f.signIn(t, "dev@example.com", "another-long-password")

	canonical, err := f.srv.store.ListStaff()
	if err != nil {
		t.Fatal(err)
	}
	target := canonical[0].ID

	for _, c := range []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"a plain member session cannot set an override", http.MethodPatch, "/api/orgs/" + f.orgId + "/staff/" + target, http.StatusNotFound},
		{"a plain member session cannot clear an override", http.MethodDelete, "/api/orgs/" + f.orgId + "/staff/" + target + "/override", http.StatusNotFound},
	} {
		resp := requestJSON(t, f.ts, memberClient, c.method, c.path, nil)
		if resp.StatusCode != c.want {
			t.Errorf("%s: %d, want %d", c.name, resp.StatusCode, c.want)
		}
	}

	// A member's token carries every verb but the account behind it is a
	// member, so the intersection refuses it 403 rather than hiding it.
	memberToken, _, err := f.srv.store.CreateApiToken("member-ci", member, authz.Grant{
		Org: f.orgId, Scopes: authz.GrantableScopes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodPatch, "/api/orgs/" + f.orgId + "/staff/" + target},
		{http.MethodDelete, "/api/orgs/" + f.orgId + "/staff/" + target + "/override"},
	} {
		resp := f.withToken(t, memberToken, c.method, c.path)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s with a member's token: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}

	// An owner's token with only read:hub.org reaches the boundary but not
	// the verb, and the refusal says which scope to re-mint with.
	readOnly := f.mintToken(t, authz.Grant{Scopes: []authz.Capability{authz.ReadOrg}})
	resp := f.withToken(t, readOnly, http.MethodPatch, "/api/orgs/"+f.orgId+"/staff/"+target)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("read-only token setting an override: %d, want 403", resp.StatusCode)
	}
	resp = f.withToken(t, readOnly, http.MethodDelete, "/api/orgs/"+f.orgId+"/staff/"+target+"/override")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("read-only token clearing an override: %d, want 403", resp.StatusCode)
	}
	if !strings.Contains(bodyOf(t, resp), "admin:hub.staff") {
		t.Errorf("refusal = %q, want it to name the missing scope", bodyOf(t, resp))
	}
	// The same token still reads the roster, because the read gate is
	// ReadOrg, not AdminStaff.
	resp = f.withToken(t, readOnly, http.MethodGet, "/api/orgs/"+f.orgId+"/staff")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("read-only token reading the roster: %d, want 200", resp.StatusCode)
	}

	// A token scoped to another org cannot reach this one's rows even with
	// the verb, and the answer hides the target rather than naming it.
	other, err := f.srv.store.CreateOrg("Other Org")
	if err != nil {
		t.Fatal(err)
	}
	otherToken := f.mintToken(t, authz.Grant{Org: other.Id, Scopes: []authz.Capability{authz.AdminStaff}})
	resp = f.withToken(t, otherToken, http.MethodPatch, "/api/orgs/"+f.orgId+"/staff/"+target)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("other org's token setting an override: %d, want 404", resp.StatusCode)
	}
}
