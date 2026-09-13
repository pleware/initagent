package hub

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pleware/initagent/internal/authz"
)

func (f *adminFixture) addConnector(t *testing.T) string {
	t.Helper()
	id, _, err := f.srv.store.CreateConnector("studio", "studio.local", "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCreateProjectLandsInTheSoleOrg(t *testing.T) {
	f := hostedCustomer(t)
	f.srv.opts.GatewayURL = "http://gateway.test"
	device := f.addConnector(t)

	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Storefront", "connectorId": device, "path": "/srv/store",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d, want 201", resp.StatusCode)
	}
	var p Project
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.OrgId != f.orgId {
		t.Errorf("orgId = %q, want the customer's only org", p.OrgId)
	}
	if p.GatewayURL != "http://gateway.test" {
		t.Errorf("gatewayUrl = %q, want the hub's existing gateway", p.GatewayURL)
	}

	resp = f.do(t, http.MethodGet, "/api/projects", nil)
	var listed []Project
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Id != p.Id {
		t.Fatalf("list = %+v, want the project just created", listed)
	}
}

func TestMemberCannotCreateAProject(t *testing.T) {
	f := hostedCustomer(t)
	f.addMember(t, "dev@example.com", "correct-horse-battery-staple", authz.RoleMember)
	dev := f.signIn(t, "dev@example.com", "correct-horse-battery-staple")
	device := f.addConnector(t)

	resp := requestJSON(t, f.ts, dev, http.MethodPost, "/api/projects", map[string]string{
		"name": "Nope", "connectorId": device, "path": "/tmp",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member create: %d, want 403", resp.StatusCode)
	}
}

func TestProjectInAnotherOrgIsNotFound(t *testing.T) {
	f := hostedCustomer(t)
	other, err := f.srv.store.CreateOrg("Other")
	if err != nil {
		t.Fatal(err)
	}
	device := f.addConnector(t)
	hidden, err := f.srv.store.CreateProject(other.Id, "Secret", device, "/secret", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	resp := f.do(t, http.MethodDelete, "/api/projects/"+hidden.Id, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete other org's project: %d, want 404", resp.StatusCode)
	}

	resp = f.do(t, http.MethodGet, "/api/projects?org="+other.Id, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("list other org: %d, want 403", resp.StatusCode)
	}
}

// The project surfaces used to refuse every token outright. A scoped token is
// now admitted to what it names — see TestProjectsTakeScopedTokens and
// TestTokenCannotCrossIntoAnotherProject in tokens_test.go.

func TestListTemplates(t *testing.T) {
	f := hostedCustomer(t)
	resp := f.do(t, http.MethodGet, "/api/templates", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("templates: %d, want 200", resp.StatusCode)
	}
	var list []struct {
		ID   string `json:"id"`
		Live bool   `json:"live"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	var live []string
	for _, tmpl := range list {
		if tmpl.Live {
			live = append(live, tmpl.ID)
		}
	}
	if len(live) != 2 || live[0] != "software" || live[1] != "later" {
		t.Fatalf("templates = %+v, want software and later live", list)
	}
}

func TestCreateProjectWithoutDevice(t *testing.T) {
	f := hostedCustomer(t)
	f.srv.opts.GatewayURL = "http://gateway.test"

	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Storefront", "templateId": "software",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d, want 201", resp.StatusCode)
	}
	var p Project
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Storefront" || p.TemplateId != "software" || p.ConnectorId != "" {
		t.Fatalf("project = %+v", p)
	}

	resp = f.do(t, http.MethodPatch, "/api/projects/"+p.Id, map[string]string{
		"repoRemote": "https://github.com/acme/app.git",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bind repo: %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.RepoHost != "github" || p.RepoRemote != "https://github.com/acme/app.git" {
		t.Fatalf("repo = %+v", p)
	}
}

func TestCreateProjectLaterTemplate(t *testing.T) {
	f := hostedCustomer(t)
	f.srv.opts.GatewayURL = "http://gateway.test"

	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Scratch", "templateId": "later",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create later: %d, want 201", resp.StatusCode)
	}
	var p Project
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Scratch" || p.TemplateId != "later" || p.RepoRemote != "" {
		t.Fatalf("later project = %+v", p)
	}
}

func TestCreateProjectRejectsComingSoonTemplate(t *testing.T) {
	f := hostedCustomer(t)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Clip", "templateId": "video",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("coming soon: %d, want 400", resp.StatusCode)
	}
}

func TestCreateProjectRequiresAName(t *testing.T) {
	f := hostedCustomer(t)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"templateId": "software",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: %d, want 400", resp.StatusCode)
	}
}
