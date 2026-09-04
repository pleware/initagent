package brand_test

import (
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/brand"
)

// Const packages still need a lock so renames cannot silently break installers,
// env prefixes, or token prefixes (CONSTRAINTS.md).
func TestExportedIdentity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Name", brand.Name, "initagent"},
		{"Binary", brand.Binary, "initagent"},
		{"ConfigDir", brand.ConfigDir, ".initagent"},
		{"WindowsAppDir", brand.WindowsAppDir, "Initagent"},
		{"DBFile", brand.DBFile, "initagent.db"},
		{"GatewayDBFile", brand.GatewayDBFile, "gateway.db"},
		{"ConnectorConfigFile", brand.ConnectorConfigFile, "connector.json"},
		{"FleetConfigFile", brand.FleetConfigFile, "fleet.json"},
		{"OfferingFile", brand.OfferingFile, "offering"},
		{"EnvOffering", brand.EnvOffering, "INITAGENT_OFFERING"},
		{"TokenPrefix", brand.TokenPrefix, "iagt_"},
		{"SessionCookie", brand.SessionCookie, "initagent_auth"},
		{"EnvPrefix", brand.EnvPrefix, "INITAGENT_"},
		{"ConnectorUnit", brand.ConnectorUnit, "initagent-connector"},
		{"LaunchdLabel", brand.LaunchdLabel, "dev.initagent.connector"},
		{"WindowsTaskName", brand.WindowsTaskName, "InitagentConnector"},
		{"HubUnit", brand.HubUnit, "initagent-hub"},
		{"HubLaunchdLabel", brand.HubLaunchdLabel, "dev.initagent.hub"},
		{"HubWindowsTask", brand.HubWindowsTask, "InitagentHub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
			if strings.TrimSpace(tc.got) == "" {
				t.Fatalf("%s is blank", tc.name)
			}
		})
	}
}

func TestReleaseAsset(t *testing.T) {
	t.Parallel()
	got := brand.ReleaseAsset("linux", "amd64")
	want := brand.Binary + "_linux_amd64"
	if got != want {
		t.Fatalf("ReleaseAsset = %q, want %q", got, want)
	}
}
