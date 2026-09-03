package brand_test

import (
	"strings"
	"testing"

	"github.com/ErzenXz/overseer/internal/brand"
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
		{"DBFile", brand.DBFile, "initagent.db"},
		{"GatewayDBFile", brand.GatewayDBFile, "gateway.db"},
		{"TokenPrefix", brand.TokenPrefix, "iagt_"},
		{"SessionCookie", brand.SessionCookie, "initagent_auth"},
		{"EnvPrefix", brand.EnvPrefix, "INITAGENT_"},
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
