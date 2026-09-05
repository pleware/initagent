package hub

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

func TestListPlansIsPublic(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	resp, err := http.Get(f.ts.URL + "/api/plans")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/plans: %d", resp.StatusCode)
	}
	var got []orgplan.Plan
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := orgplan.Catalogue()
	if len(got) != len(want) {
		t.Fatalf("plans = %d, want %d", len(got), len(want))
	}
	if got[0].ID != orgplan.Free || got[len(got)-1].ID != orgplan.Enterprise {
		t.Fatalf("catalogue order = %v", got)
	}
}

func TestHostedFreeRefusesASecondProject(t *testing.T) {
	f := hostedCustomer(t)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": "One"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first project: %d, want 201", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": "Two"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second project: %d, want 409", resp.StatusCode)
	}
	var body struct {
		Code  string `json:"code"`
		Wall  string `json:"wall"`
		Limit int    `json:"limit"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "plan_limit" || body.Wall != "projects" || body.Limit != 1 {
		t.Fatalf("wall = %+v", body)
	}
	if body.Error == "" {
		t.Fatal("empty error message")
	}
}

func TestSelfHostIgnoresProjectCap(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	for _, name := range []string{"One", "Two"} {
		resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": name})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("%s: %d, want 201", name, resp.StatusCode)
		}
	}
}

func TestStarterAllowsTwoProjects(t *testing.T) {
	f := hostedCustomer(t)
	if err := f.srv.store.SetOrgPlan(f.orgId, orgplan.Starter); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"One", "Two"} {
		resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": name})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("%s: %d, want 201", name, resp.StatusCode)
		}
	}
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": "Three"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("third project: %d, want 409", resp.StatusCode)
	}
}

func TestHostedFreeRefusesASecondPerson(t *testing.T) {
	f := hostedCustomer(t)
	hash, err := auth.HashPassword("another-long-password")
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.srv.store.CreateAccount("dev@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	err = f.srv.store.AddOrgMember(f.orgId, account.Id, authz.RoleMember)
	if !errors.Is(err, ErrPlanLimit) {
		t.Fatalf("AddOrgMember = %v, want ErrPlanLimit", err)
	}
}

func TestEnterpriseAllowsASecondPerson(t *testing.T) {
	f := hostedCustomer(t)
	if err := f.srv.store.SetOrgPlan(f.orgId, orgplan.Enterprise); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("another-long-password")
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.srv.store.CreateAccount("dev@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.AddOrgMember(f.orgId, account.Id, authz.RoleMember); err != nil {
		t.Fatalf("AddOrgMember on enterprise: %v", err)
	}
}

func TestHostedFreeAllowsTwoMachinesAndRefusesAThird(t *testing.T) {
	f := hostedCustomer(t)
	first := f.addDevice(t)
	second := f.addDevice(t)
	third := f.addDevice(t)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Storefront", "deviceId": first, "path": "/srv/store",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d, want 201", resp.StatusCode)
	}
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	if len(project.DeviceIds) != 1 || project.DeviceIds[0] != first {
		t.Fatalf("create enrolled = %v", project.DeviceIds)
	}
	resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": second})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second machine: %d, want 201", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	if len(project.DeviceIds) != 2 {
		t.Fatalf("two machines = %v", project.DeviceIds)
	}
	resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": third})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("third machine: %d, want 409", resp.StatusCode)
	}
	var body struct {
		Code  string `json:"code"`
		Wall  string `json:"wall"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "plan_limit" || body.Wall != "machines" || body.Limit != 2 {
		t.Fatalf("wall = %+v", body)
	}
}

func TestSwitchingTheSelectedMachineIsNotAnotherMachine(t *testing.T) {
	f := hostedCustomer(t)
	first := f.addDevice(t)
	second := f.addDevice(t)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Storefront", "deviceId": first, "path": "/srv/store",
	})
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": second})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second machine: %d", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPatch, "/api/projects/"+project.Id, map[string]string{"deviceId": first})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("switch selected: %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	if project.DeviceId != first || len(project.DeviceIds) != 2 {
		t.Fatalf("after switch = %+v", project)
	}
}

func TestSelfHostIgnoresMachineCap(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": "One"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		id := f.addDevice(t)
		resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": id})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("machine %d: %d, want 201", i+1, resp.StatusCode)
		}
	}
}

func TestStarterAllowsThreeMachines(t *testing.T) {
	f := hostedCustomer(t)
	if err := f.srv.store.SetOrgPlan(f.orgId, orgplan.Starter); err != nil {
		t.Fatal(err)
	}
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{"name": "One"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		id := f.addDevice(t)
		resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": id})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("machine %d: %d, want 201", i+1, resp.StatusCode)
		}
	}
	resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": f.addDevice(t)})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("fourth machine: %d, want 409", resp.StatusCode)
	}
}

func TestDetachFreesAMachineSlot(t *testing.T) {
	f := hostedCustomer(t)
	first := f.addDevice(t)
	second := f.addDevice(t)
	third := f.addDevice(t)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Storefront", "deviceId": first, "path": "/srv/store",
	})
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": second})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second: %d", resp.StatusCode)
	}
	resp = f.do(t, http.MethodDelete, "/api/projects/"+project.Id+"/devices/"+first, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detach: %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	if project.DeviceId != second || len(project.DeviceIds) != 1 {
		t.Fatalf("after detach = %+v", project)
	}
	resp = f.do(t, http.MethodPost, "/api/projects/"+project.Id+"/devices", map[string]string{"deviceId": third})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("reuse slot: %d, want 201", resp.StatusCode)
	}
}
