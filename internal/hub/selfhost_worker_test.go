package hub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pleware/initagent/internal/agent"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/gateway"
	"github.com/pleware/initagent/internal/offering"
)

func TestShouldBindSelfhostWorker(t *testing.T) {
	first := &Project{Id: "prj-1"}
	withDevice := &Project{Id: "prj-1", DeviceId: "dev-1"}
	cases := []struct {
		name    string
		kind    offering.Kind
		gateway string
		project *Project
		count   int
		want    bool
	}{
		{name: "selfhost first project", kind: offering.Selfhost, gateway: "http://gw", project: first, count: 1, want: true},
		{name: "hosted first project", kind: offering.Hosted, gateway: "http://gw", project: first, count: 1, want: false},
		{name: "zero offering is not selfhost", kind: "", gateway: "http://gw", project: first, count: 1, want: false},
		{name: "no gateway", kind: offering.Selfhost, gateway: "", project: first, count: 1, want: false},
		{name: "already has a device", kind: offering.Selfhost, gateway: "http://gw", project: withDevice, count: 1, want: false},
		{name: "second project", kind: offering.Selfhost, gateway: "http://gw", project: first, count: 2, want: false},
		{name: "nil project", kind: offering.Selfhost, gateway: "http://gw", project: nil, count: 1, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldBindSelfhostWorker(tc.kind, tc.gateway, tc.project, tc.count); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelfhostFirstProjectEnrollsThisBox(t *testing.T) {
	gw, err := gateway.Open(gateway.Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	gwURL := httptest.NewServer(gw.Handler())
	t.Cleanup(gwURL.Close)

	f := claimedHub(t, offering.Selfhost)
	f.srv.opts.GatewayURL = gwURL.URL

	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "olx", "templateId": "software",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	if project.DeviceId == "" {
		t.Fatal("first self-host project should have this box attached")
	}

	cfg, err := agent.LoadConfigFrom(filepath.Join(f.srv.opts.DataDir, brand.ConnectorConfigFile))
	if err != nil {
		t.Fatalf("connector in hub data dir: %v", err)
	}
	if cfg.DeviceId != project.DeviceId {
		t.Fatalf("connector device %s, project device %s", cfg.DeviceId, project.DeviceId)
	}
	if cfg.HubURL != gwURL.URL {
		t.Fatalf("connector hubUrl = %q, want the gateway", cfg.HubURL)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, gwURL.URL+"/api/devices", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(brand.ProjectHeader, project.Id)
	list, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	if list.StatusCode != http.StatusOK {
		t.Fatalf("gateway devices: %d", list.StatusCode)
	}
	var devices []gateway.DeviceView
	if err := json.NewDecoder(list.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != project.DeviceId {
		t.Fatalf("gateway devices = %+v, want %s", devices, project.DeviceId)
	}

	second := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "other", "templateId": "software",
	})
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("second create: %d", second.StatusCode)
	}
	var other Project
	if err := json.NewDecoder(second.Body).Decode(&other); err != nil {
		t.Fatal(err)
	}
	if other.DeviceId != "" {
		t.Fatalf("second project should not auto-bind, got %s", other.DeviceId)
	}
}

func TestHostedFirstProjectDoesNotEnrollTheHubBox(t *testing.T) {
	gw, err := gateway.Open(gateway.Options{DataDir: t.TempDir(), Addr: "127.0.0.1:4201"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	gwURL := httptest.NewServer(gw.Handler())
	t.Cleanup(gwURL.Close)

	f := hostedCustomer(t)
	f.srv.opts.GatewayURL = gwURL.URL
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "Storefront", "templateId": "software",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	if project.DeviceId != "" {
		t.Fatalf("hosted must not enroll the hub box, got %s", project.DeviceId)
	}
	if _, err := os.Stat(filepath.Join(f.srv.opts.DataDir, brand.ConnectorConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("hosted wrote a connector config: %v", err)
	}
}

func TestRecoverSelfhostWorkerBindsExistingFirstProject(t *testing.T) {
	gw, err := gateway.Open(gateway.Options{DataDir: t.TempDir(), Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	gwURL := httptest.NewServer(gw.Handler())
	t.Cleanup(gwURL.Close)

	f := claimedHub(t, offering.Selfhost)
	resp := f.do(t, http.MethodPost, "/api/projects", map[string]string{
		"name": "test", "templateId": "software",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, readBody(t, resp))
	}
	var created Project
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.DeviceId != "" {
		t.Fatal("project created with no gateway should not auto-bind")
	}

	f.srv.opts.GatewayURL = gwURL.URL
	f.srv.recoverSelfhostWorker(t.Context())

	row, err := f.srv.store.ProjectById(created.Id)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row.DeviceId == "" {
		t.Fatal("recover should attach this box once a gateway exists")
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err.Error()
	}
	return string(b)
}
