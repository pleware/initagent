package brand_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
		{"DisplayName", brand.DisplayName, "initAgent"},
		{"Binary", brand.Binary, "initagent"},
		{"ConfigDir", brand.ConfigDir, ".initagent"},
		{"WindowsAppDir", brand.WindowsAppDir, "Initagent"},
		{"DBFile", brand.DBFile, "initagent.db"},
		{"GatewayDBFile", brand.GatewayDBFile, "gateway.db"},
		{"ConnectorConfigFile", brand.ConnectorConfigFile, "connector.json"},
		{"FleetConfigFile", brand.FleetConfigFile, "fleet.json"},
		{"DeskConfigFile", brand.DeskConfigFile, "desk.yaml"},
		{"OfferingFile", brand.OfferingFile, "offering"},
		{"EnvDeskConfig", brand.EnvDeskConfig, "INITAGENT_DESK_CONFIG"},
		{"ClaimTokenFile", brand.ClaimTokenFile, "bootstrap-token"},
		{"EnvOffering", brand.EnvOffering, "INITAGENT_OFFERING"},
		{"EnvResendAPIKey", brand.EnvResendAPIKey, "INITAGENT_RESEND_API_KEY"},
		{"EnvMailFrom", brand.EnvMailFrom, "INITAGENT_MAIL_FROM"},
		{"EnvTrustedProxies", brand.EnvTrustedProxies, "INITAGENT_TRUSTED_PROXIES"},
		{"EnvDeskSeamAddr", brand.EnvDeskSeamAddr, "INITAGENT_DESK_SEAM_ADDR"},
		{"EnvDeskSeamToken", brand.EnvDeskSeamToken, "INITAGENT_DESK_SEAM_TOKEN"},
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

func TestDisplayNameDiffersFromName(t *testing.T) {
	t.Parallel()
	if brand.DisplayName == brand.Name {
		t.Fatal("DisplayName must be the wordmark, not the machine name")
	}
}

func TestDisplayNameMatchesChromeFile(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "web", "brand.ts")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("export const displayName = '" + brand.DisplayName + "'")
	if !bytes.Contains(got, want) {
		t.Fatalf("%s missing %q", path, want)
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

// TestEnvAPIKey locks the derivation an operator has to type. A secret kind
// is configuration and travels in bug reports; only the variable this names
// holds the value, so getting the name wrong looks like a missing key.
func TestEnvAPIKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind string
		want string
	}{
		{"openai", brand.EnvPrefix + "OPENAI_API_KEY"},
		{"azure-openai", brand.EnvPrefix + "AZURE_OPENAI_API_KEY"},
		{"elevenlabs", brand.EnvPrefix + "ELEVENLABS_API_KEY"},
	}
	for _, tc := range cases {
		if got := brand.EnvAPIKey(tc.kind); got != tc.want {
			t.Errorf("EnvAPIKey(%q) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestEnvAPIKeyAliasIsTheVendorName(t *testing.T) {
	t.Parallel()
	if got := brand.EnvAPIKeyAlias("openai"); got != "OPENAI_API_KEY" {
		t.Errorf("EnvAPIKeyAlias(openai) = %q, want OPENAI_API_KEY", got)
	}
	if got := brand.EnvAPIKeyAlias("azure-openai"); got != "AZURE_OPENAI_API_KEY" {
		t.Errorf("EnvAPIKeyAlias(azure-openai) = %q, want AZURE_OPENAI_API_KEY", got)
	}
}

func TestLookupAPIKeyPrefersThePrefixedName(t *testing.T) {
	t.Parallel()
	got := brand.LookupAPIKey(map[string]string{
		brand.EnvAPIKey("openai"):      "from-prefixed",
		brand.EnvAPIKeyAlias("openai"): "from-alias",
	}, "openai")
	if got != "from-prefixed" {
		t.Errorf("LookupAPIKey = %q, want from-prefixed", got)
	}
}

func TestLookupAPIKeyFallsBackToTheVendorName(t *testing.T) {
	t.Parallel()
	got := brand.LookupAPIKey(map[string]string{
		brand.EnvAPIKeyAlias("openai"): "from-alias",
	}, "openai")
	if got != "from-alias" {
		t.Errorf("LookupAPIKey = %q, want from-alias", got)
	}
	if got := brand.LookupAPIKey(map[string]string{}, "openai"); got != "" {
		t.Errorf("LookupAPIKey empty env = %q, want empty", got)
	}
}
