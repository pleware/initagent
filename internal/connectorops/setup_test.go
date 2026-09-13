package connectorops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/protocol"
)

func TestSetupCatalogueHasCrossPlatformCommands(t *testing.T) {
	want := map[string]bool{"node": true, "codex": true, "claude": true, "gemini": true, "tailscale": true}
	for _, spec := range setupSpecs {
		if !want[spec.id] {
			t.Fatalf("unexpected setup tool %q", spec.id)
		}
		delete(want, spec.id)
		if spec.name == "" || spec.binary == "" || spec.installUnix == "" || spec.installWindows == "" || spec.docsURL == "" {
			t.Errorf("setup tool %q is missing required cross-platform metadata", spec.id)
		}
	}
	for id := range want {
		t.Errorf("setup tool %q is missing", id)
	}
}

func TestAgentLoginCommandsPreferRemoteFriendlyFlows(t *testing.T) {
	for _, spec := range setupSpecs {
		switch spec.id {
		case "codex":
			if spec.authUnix != "codex login --device-auth" || spec.authWindows != "codex login --device-auth" {
				t.Error("Codex should use device-code login on remote machines")
			}
		case "claude", "gemini", "tailscale":
			if spec.authUnix == "" || spec.authWindows == "" {
				t.Errorf("%s should have a login/connect command on both platforms", spec.id)
			}
		}
	}
}

// installedConn answers every exec probe with an installed tool (version
// 1.2.3) and a successful auth status, so the whole board reads connected.
func installedConn(t *testing.T) *fakeConn {
	t.Helper()
	fc := newFakeConn()
	fc.callFn = func(_ context.Context, typ string, payload, out any) error {
		if typ != protocol.TypeExec {
			t.Errorf("typ = %q", typ)
		}
		*(out.(*protocol.ExecResult)) = protocol.ExecResult{ExitCode: 0, Stdout: "installed\n1.2.3\n"}
		return nil
	}
	return fc
}

func TestSetupStatusProbesTools(t *testing.T) {
	fc := installedConn(t)
	overview := SetupStatus(context.Background(), fc, "linux", "amd64")
	if overview.OS != "linux" || overview.Arch != "amd64" {
		t.Fatalf("overview = %+v", overview)
	}
	if len(overview.Tools) != len(setupSpecs) {
		t.Fatalf("tools = %d", len(overview.Tools))
	}
	byID := map[string]SetupTool{}
	for _, tool := range overview.Tools {
		byID[tool.ID] = tool
	}
	if !byID["node"].Installed || byID["node"].Version != "1.2.3" {
		t.Fatalf("node = %+v", byID["node"])
	}
	if byID["codex"].Auth != "connected" {
		t.Fatalf("codex auth = %q", byID["codex"].Auth)
	}
	if byID["gemini"].Auth != "ready" {
		t.Fatalf("gemini auth = %q", byID["gemini"].Auth)
	}
	if !strings.Contains(overview.BundleCommand, "codex") || !strings.Contains(overview.BundleCommand, " && ") {
		t.Fatalf("bundle = %q", overview.BundleCommand)
	}
}

func TestSetupStatusProbeError(t *testing.T) {
	fc := newFakeConn()
	fc.callFn = func(context.Context, string, any, any) error { return errors.New("offline") }
	overview := SetupStatus(context.Background(), fc, "linux", "amd64")
	for _, tool := range overview.Tools {
		if tool.Auth != "unknown" || tool.Installed {
			t.Fatalf("tool = %+v", tool)
		}
	}
}

func TestSetupStatusWindowsBundleSeparator(t *testing.T) {
	fc := installedConn(t)
	overview := SetupStatus(context.Background(), fc, "windows", "amd64")
	if !strings.Contains(overview.BundleCommand, "; ") {
		t.Fatalf("bundle = %q", overview.BundleCommand)
	}
}
