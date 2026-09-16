package hub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
