package hub

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pleware/initagent/internal/authz"
)

func (f *adminFixture) addDevice(t *testing.T) string {
	t.Helper()
	id, _, err := f.srv.store.CreateDevice("studio", "studio.local", "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCreateProjectLandsInTheSoleOrg(t *testing.T) {
	f := hostedCustomer(t)
	f.srv.opts.GatewayURL = "http://gateway.test"
	device := f.addDevice(t)

	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Storefront", "deviceId": device, "path": "/srv/store",
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
	device := f.addDevice(t)

	resp := requestJSON(t, f.ts, dev, http.MethodPost, "/api/projects", map[string]string{
		"name": "Nope", "deviceId": device, "path": "/tmp",
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
	device := f.addDevice(t)
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

func TestProjectsRefuseApiTokens(t *testing.T) {
	f := hostedCustomer(t)
	token, err := f.srv.store.CreateApiToken("ci")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, f.ts.URL+"/api/projects", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token list projects: %d, want 401", resp.StatusCode)
	}
}

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
	var live int
	var sawSoftware bool
	for _, tmpl := range list {
		if tmpl.Live {
			live++
		}
		if tmpl.ID == "software" {
			sawSoftware = tmpl.Live
		}
	}
	if !sawSoftware || live != 1 {
		t.Fatalf("templates = %+v, want software live and only that", list)
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
	if p.Name != "Storefront" || p.TemplateId != "software" || p.DeviceId != "" {
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
