// Package brand holds every product-identifying string in one place.
//
// Upstream has no equivalent package, and that is the point. The branding
// strings live in the handful of files upstream edits most often, so renaming
// them in place would put a conflict in every merge. Referencing a constant
// from here keeps our rename to one file that upstream never touches.
//
// Constants are the rename inventory. Call sites import them instead of
// carrying literals, so a later identity tweak is one file upstream never
// touches.
//
// Visual tokens start in colors.css. That file is a starter palette, not yet
// applied to the cockpit or the site.
package brand

const (
	// Name is the product name shown to people.
	Name = "initagent"

	// Binary is the executable name, and the stem of generated helper
	// scripts and service units.
	Binary = "initagent"

	// ConfigDir is the per-user data directory, relative to $HOME. Holds
	// DBFile on a hub and the connector config on a worker.
	ConfigDir = ".initagent"

	// WindowsAppDir is the folder under %LOCALAPPDATA% for binaries and
	// setup-center tools on Windows.
	WindowsAppDir = "Initagent"

	// DBFile is the hub's SQLite database inside ConfigDir.
	DBFile = "initagent.db"

	// GatewayDBFile is the gateway's SQLite database inside ConfigDir.
	// A second process, a second file — the hub database stays theirs.
	GatewayDBFile = "gateway.db"

	// ConnectorConfigFile is the worker connector's config inside ConfigDir.
	ConnectorConfigFile = "connector.json"

	// FleetConfigFile is the fleet CLI config inside ConfigDir.
	FleetConfigFile = "fleet.json"

	// TokenPrefix marks an API token so it is recognisable in a log line
	// without being resolvable. The token value itself stays CSPRNG.
	TokenPrefix = "iagt_"

	// SessionCookie names the hub's browser session cookie. Renaming it logs
	// every open browser out once, which is why it moves with the rest.
	SessionCookie = "initagent_auth"
)

// Environment variables. Every one we read is derived from EnvPrefix, so a
// rename cannot leave half the tree reading the old name.
const (
	EnvPrefix = "INITAGENT_"

	// EnvManaged is set by an installed service to mark the process as
	// supervised, which changes how self-update behaves.
	EnvManaged = EnvPrefix + "MANAGED"

	// EnvWindowsTask names the Scheduled Task to restart after an update.
	EnvWindowsTask = EnvPrefix + "WINDOWS_TASK"

	// EnvHub and EnvToken configure the CLI without a config file.
	EnvHub   = EnvPrefix + "HUB"
	EnvToken = EnvPrefix + "TOKEN"

	EnvRepo    = EnvPrefix + "REPO"
	EnvVersion = EnvPrefix + "VERSION"
	EnvPurge   = EnvPrefix + "PURGE"

	// EnvDatabaseURL switches the hub store from SQLite to Postgres.
	EnvDatabaseURL = EnvPrefix + "DATABASE_URL"
)

// Service identities. Renaming these breaks upgrades of an already-installed
// connector, so they change in one commit rather than gradually.
const (
	ConnectorUnit   = Binary + "-connector"
	LaunchdLabel    = "dev.initagent.connector"
	WindowsTaskName = "InitagentConnector"

	// HubUnit is the systemd unit for a hub install.
	HubUnit = Binary + "-hub"

	// HubLaunchdLabel is the macOS LaunchAgent label for the hub.
	HubLaunchdLabel = "dev.initagent.hub"

	// HubWindowsTask is the Scheduled Task name for a hub install.
	HubWindowsTask = "InitagentHub"
)

// TmuxKindOpt is the tmux user option carrying coder.kind on a session, read
// back when listing sessions. Must stay a valid tmux option name.
const TmuxKindOpt = "@initagent_coder_kind"

// CommandDir is the module-relative repository directory of the single-binary
// entry point. Developer-facing messages (how to build from source) reference
// it through this constant, so a layout move stays one edit in this file.
const CommandDir = "cmd/initagent"

// ReleaseSource is where the hub fetches connector binaries for platforms it
// is not running on. A var, not a const, because it is overridable at build
// time — continuing what upstream already anticipated for forks:
//
//	-ldflags "-X github.com/pleware/initagent/internal/brand.ReleaseSource=owner/repo"
var ReleaseSource = "pleware/initagent"

// ReleaseAsset is the GitHub release filename for a platform build.
func ReleaseAsset(goos, goarch string) string {
	return Binary + "_" + goos + "_" + goarch
}
