package hub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

// seedSkill writes one skill straight into the store.
func seedSkill(t *testing.T, s *Store, name string, enabled bool) *Skill {
	t.Helper()
	sk, err := s.CreateSkill(name, name+" description", "echo "+name, nil, enabled, "acc-seed")
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func TestListSkillsPublic(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)
	seedSkill(t, srv.store, "zebra", true)
	seedSkill(t, srv.store, "apple", true)
	seedSkill(t, srv.store, "hidden", false)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/skills", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/skills: %d, want 200", resp.StatusCode)
	}
	var got []skillSummary
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("summaries = %+v, want the two enabled skills", got)
	}
	if got[0].Name != "apple" || got[1].Name != "zebra" {
		t.Errorf("order = (%q, %q), want alphabetical", got[0].Name, got[1].Name)
	}
	if got[0].ID == "" || got[0].Description == "" {
		t.Errorf("summary fields incomplete: %+v", got[0])
	}
}

func TestListSkillsPublicEmpty(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/skills", nil)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[]\n" {
		t.Errorf("empty catalog body = %q, want []", body)
	}
}

func TestGetSkillPublic(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	enabled, err := srv.store.CreateSkill("shipper", "Ships things", "# ship\n\necho hi", &MCPConfig{
		Command: "npx", Args: []string{"-y", "shipper"}, URL: "https://x.example", Env: map[string]string{"TOKEN": "secret"},
	}, true, "acc-seed")
	if err != nil {
		t.Fatal(err)
	}
	disabled := seedSkill(t, srv.store, "dark", false)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/skills/"+enabled.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enabled get: %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"enabled"`, `"createdBy"`, `"createdAt"`, `"updatedAt"`, `"env"`} {
		if strings.Contains(string(body), banned) {
			t.Errorf("public payload contains %s: %s", banned, body)
		}
	}
	var dl skillDownload
	if err := json.Unmarshal(body, &dl); err != nil {
		t.Fatal(err)
	}
	if dl.ID != enabled.ID || dl.Name != "shipper" || dl.Body != "# ship\n\necho hi" {
		t.Errorf("download = %+v, want the seeded skill", dl)
	}
	if dl.MCP == nil || dl.MCP.Command != "npx" || len(dl.MCP.Args) != 2 || dl.MCP.URL != "https://x.example" || len(dl.MCP.Env) != 0 {
		t.Errorf("mcp = %+v, want command/args/url with no env", dl.MCP)
	}

	// Disabled and missing are the same answer, so the flag leaks nothing.
	for _, id := range []string{disabled.ID, "skill-nope"} {
		resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/skills/"+id, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("get %s: %d, want 404", id, resp.StatusCode)
		}
	}
}

func TestAdminListSkills(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	seedSkill(t, f.srv.store, "on", true)
	seedSkill(t, f.srv.store, "off", false)

	resp := f.do(t, http.MethodGet, "/api/admin/skills", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/skills: %d, want 200", resp.StatusCode)
	}
	var got []Skill
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("skills = %+v, want two including the disabled one", got)
	}
	if got[0].Name != "off" || got[1].Name != "on" {
		t.Errorf("order = (%q, %q), want alphabetical", got[0].Name, got[1].Name)
	}
}

func TestAdminCreateSkill(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	resp := f.do(t, http.MethodPost, "/api/admin/skills", map[string]any{
		"name":        "  greeter  ",
		"description": "Greets",
		"body":        "echo hello",
		"mcp":         map[string]any{"command": "npx", "args": []string{"-y", "greeter"}},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d, want 201", resp.StatusCode)
	}
	var sk Skill
	if err := json.NewDecoder(resp.Body).Decode(&sk); err != nil {
		t.Fatal(err)
	}
	if sk.Name != "greeter" || sk.Body != "echo hello" || !sk.Enabled || sk.CreatedBy != f.ownerId {
		t.Errorf("created = %+v, want trimmed name, body, enabled, createdBy %q", sk, f.ownerId)
	}
	if sk.MCP == nil || sk.MCP.Command != "npx" || len(sk.MCP.Args) != 2 {
		t.Errorf("mcp = %+v, want the submitted config", sk.MCP)
	}
	if sk.CreatedAt == 0 || sk.UpdatedAt == 0 {
		t.Errorf("timestamps missing: %+v", sk)
	}
}

func TestAdminCreateSkillRefusals(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	seedSkill(t, f.srv.store, "taken", true)
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"empty name", map[string]any{"name": "  ", "body": "x"}, http.StatusBadRequest},
		{"empty body", map[string]any{"name": "ok", "body": ""}, http.StatusBadRequest},
		{"whitespace-only body", map[string]any{"name": "ok", "body": "   "}, http.StatusBadRequest},
		{"mcp with neither command nor url", map[string]any{"name": "ok", "body": "x", "mcp": map[string]any{}}, http.StatusBadRequest},
		{"whitespace-only mcp command", map[string]any{"name": "ok", "body": "x", "mcp": map[string]any{"command": "  "}}, http.StatusBadRequest},
		{"duplicate name", map[string]any{"name": "taken", "body": "x"}, http.StatusConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := f.do(t, http.MethodPost, "/api/admin/skills", c.body)
			if resp.StatusCode != c.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
}

func TestAdminUpdateSkill(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	created := seedSkill(t, f.srv.store, "before", true)

	resp := f.do(t, http.MethodPatch, "/api/admin/skills/"+created.ID, map[string]any{
		"name": "after", "body": "echo two", "enabled": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d, want 200", resp.StatusCode)
	}
	var sk Skill
	if err := json.NewDecoder(resp.Body).Decode(&sk); err != nil {
		t.Fatal(err)
	}
	if sk.ID != created.ID || sk.Name != "after" || sk.Body != "echo two" || sk.Enabled {
		t.Errorf("updated = %+v, want renamed, rewritten, disabled", sk)
	}

	resp = f.do(t, http.MethodPatch, "/api/admin/skills/skill-nope", map[string]string{"name": "x", "body": "y"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("update missing id: %d, want 404", resp.StatusCode)
	}
}

func TestAdminDeleteSkill(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	created := seedSkill(t, f.srv.store, "doomed", true)

	resp := f.do(t, http.MethodDelete, "/api/admin/skills/"+created.ID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d, want 200", resp.StatusCode)
	}
	var ok map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&ok); err != nil {
		t.Fatal(err)
	}
	if !ok["ok"] {
		t.Errorf("payload = %v, want ok:true", ok)
	}
	if remaining, err := f.srv.store.ListSkills(); err != nil || len(remaining) != 0 {
		t.Errorf("store after delete = (%v, %v), want empty", remaining, err)
	}
}

func TestAdminSkillRoutesRefuseNonAdmin(t *testing.T) {
	f := hostedCustomer(t)
	created := seedSkill(t, f.srv.store, "target", true)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/admin/skills", nil},
		{http.MethodPost, "/api/admin/skills", map[string]string{"name": "x", "body": "y"}},
		{http.MethodPatch, "/api/admin/skills/" + created.ID, map[string]string{"name": "x", "body": "y"}},
		{http.MethodDelete, "/api/admin/skills/" + created.ID, nil},
	}
	for _, c := range cases {
		resp := f.do(t, c.method, c.path, c.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as customer: %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}
}

// The gate is credential.Can, not "is there a session": a token whose scope
// list names admin:hub.account but whose grant has no boundary cannot pass,
// because no token reaches the installation (Grant.Contains("", "") is
// false). Constructed directly because the store will not mint that scope.
func TestAdminSkillHandlersRefuseInstallationToken(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	cred := authz.Credential{
		Requester: authz.Requester{Account: "account-ops", Platform: true},
		Grant:     &authz.Grant{Org: "", Scopes: []authz.Capability{authz.AdminAccounts}},
	}
	call := func(handler credHandler, method, target string) int {
		t.Helper()
		req := httptest.NewRequest(method, target, nil)
		rec := httptest.NewRecorder()
		handler(rec, req, cred)
		return rec.Code
	}
	if got := call(srv.handleAdminListSkills, http.MethodGet, "/api/admin/skills"); got != http.StatusForbidden {
		t.Errorf("list with an installation-scoped token: %d, want 403", got)
	}
	if got := call(srv.handleDeleteSkill, http.MethodDelete, "/api/admin/skills/skill-any"); got != http.StatusForbidden {
		t.Errorf("delete with an installation-scoped token: %d, want 403", got)
	}
}
