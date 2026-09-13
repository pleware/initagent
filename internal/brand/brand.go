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

import (
	"cmp"
	"strings"
)

const (
	// Name is the machine name: binary stem, GitHub, hostnames, env.
	Name = "initagent"

	// DisplayName is the people-facing wordmark in headers, titles, and
	// cockpit chrome. Commands stay Name.
	DisplayName = "initAgent"

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

	// GdeskConfigFile is the glass-desk YAML inside ConfigDir. Optional:
	// missing means environment only. A present file is configuration, and
	// the process environment overrides every field.
	GdeskConfigFile = "gdesk.yaml"

	// LegacyDeskConfigFile is the name that file had before the desk became
	// the `gdesk` context (workspace draft 05). Read when GdeskConfigFile is
	// absent: a developer's home directory is not a place to break, and the
	// file holds a token that cannot be regenerated from anywhere else.
	LegacyDeskConfigFile = "desk.yaml"

	// OfferingFile names the hub offering token inside ConfigDir.
	// Missing means selfhost. Values: hosted | selfhost. No secrets.
	OfferingFile = "offering"

	// ClaimTokenFile holds the one-time bootstrap token an unclaimed hub
	// mints at start, so an operator who cannot read the log can read the
	// file instead. Unlike OfferingFile this *is* a secret: mode 0600, and
	// it is removed the moment the hub is claimed.
	ClaimTokenFile = "bootstrap-token"

	// TokenPrefix marks an API token so it is recognisable in a log line
	// without being resolvable. The token value itself stays CSPRNG.
	TokenPrefix = "iagt_"

	// SessionCookie names the hub's browser session cookie. Renaming it logs
	// every open browser out once, which is why it moves with the rest.
	SessionCookie = "initagent_auth"

	// ProjectHeader carries the hub's placement decision to a gateway: which
	// project- this request acts on. One gateway process serves many projects
	// (18), so the process cannot be the answer.
	ProjectHeader = "X-Initagent-Project"
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

	// EnvOffering is the one-shot override for the hub offering file
	// (<data-dir>/offering). Same tokens: hosted or selfhost.
	EnvOffering = EnvPrefix + "OFFERING"

	// EnvResendAPIKey is the hosted transactional-mail key. Empty on
	// self-host (silent) and on hosted until ops fills it. Never a flag.
	EnvResendAPIKey = EnvPrefix + "RESEND_API_KEY"

	// EnvMailFrom is the From address Resend will send as, e.g.
	// `initAgent <noreply@initagent.dev>`. Required when the key is set.
	EnvMailFrom = EnvPrefix + "MAIL_FROM"

	// EnvStripeSecretKey is the hosted Stripe secret. Empty on self-host
	// and on hosted until ops fills it. Never a flag.
	EnvStripeSecretKey = EnvPrefix + "STRIPE_SECRET_KEY"

	// EnvStripeWebhookSecret verifies POST /api/billing/webhook. Empty
	// refuses every Stripe event.
	EnvStripeWebhookSecret = EnvPrefix + "STRIPE_WEBHOOK_SECRET"

	// EnvStripePriceStarter / EnvStripePriceTeam override the catalogue
	// Price ids when the YAML slug is still empty (first deploy). They
	// are not secrets.
	EnvStripePriceStarter = EnvPrefix + "STRIPE_PRICE_STARTER"
	EnvStripePriceTeam    = EnvPrefix + "STRIPE_PRICE_TEAM"

	// EnvFakturowniaToken is the Fakturownia.pl API token. Fiscal
	// invoices (and KSeF) are issued after Stripe marks a payment paid.
	EnvFakturowniaToken = EnvPrefix + "FAKTUROWNIA_API_TOKEN"

	// EnvFakturowniaDomain is the account host, e.g.
	// `acme.fakturownia.pl`. Empty skips invoice create.
	EnvFakturowniaDomain = EnvPrefix + "FAKTUROWNIA_DOMAIN"

	// EnvGatewaySecret is the shared secret the hub presents to a gateway on
	// the control routes. Empty leaves them open, which is the single-box
	// self-host default; ops sets it wherever the gateway is reachable. It
	// carries no project scope — scoped tokens are 09.
	EnvGatewaySecret = EnvPrefix + "GATEWAY_SECRET"

	// EnvTrustedProxies is a comma-separated list of CIDRs whose
	// X-Forwarded-For header the hub may use as the client address. Empty
	// means the connection address, which is the self-host default (`26`).
	EnvTrustedProxies = EnvPrefix + "TRUSTED_PROXIES"

	// EnvGdeskChat, EnvGdeskSTT and EnvGdeskTTS bind one glass-desk role to a
	// provider and a model, written `provider/model`. An unset role leaves
	// the desk without it — the connector still serves connectors and MCP, so a
	// missing voice must not stop it starting (`53`).
	EnvGdeskChat = EnvPrefix + "GDESK_CHAT"
	EnvGdeskSTT  = EnvPrefix + "GDESK_STT"
	EnvGdeskTTS  = EnvPrefix + "GDESK_TTS"

	// EnvGdeskProviderPrefix begins a provider entry. After it comes the
	// provider id — uppercased, dashes as underscores — then one field:
	// SHAPE, BASE_URL, API_PATH or SECRET_KIND. The value of the key itself
	// is never here; SECRET_KIND names it (`41`, `24`).
	EnvGdeskProviderPrefix = EnvPrefix + "GDESK_PROVIDER_"

	// EnvGdeskConfig is an explicit path to the glass-desk YAML. Unset means
	// ~/ConfigDir/GdeskConfigFile when that file exists. Set and missing
	// is a configuration error, not a silent fallback to env-only.
	EnvGdeskConfig = EnvPrefix + "GDESK_CONFIG"

	// EnvGdeskSeamAddr is where the desk's local seam listens. Loopback only:
	// a remote device reaches the desk through the hub as a relay, so this
	// number never faces the network (workspace docs/GDESK-SCOPES.md).
	EnvGdeskSeamAddr = EnvPrefix + "GDESK_SEAM_ADDR"

	// EnvGdeskSeamToken is what a caller presents to join the desk on that
	// address. Unset closes the seam rather than opening it to anything on
	// the box. Never a flag, for the same reason as a provider key.
	EnvGdeskSeamToken = EnvPrefix + "GDESK_SEAM_TOKEN"

	// EnvGdeskVisionCamera names the camera index for the walk-up sensor.
	// Set — including to 0 — starts pware-vision as a child of the desk.
	// Unset leaves local sensing declared rather than running.
	EnvGdeskVisionCamera = EnvPrefix + "GDESK_VISION_CAMERA"

	// EnvGdeskVisionSensor stamps facts from that process. Empty means
	// camera-<index>, which is stable on one box.
	EnvGdeskVisionSensor = EnvPrefix + "GDESK_VISION_SENSOR"

	// EnvGdeskVisionCommand overrides the executable. Tests use it; production
	// leaves it empty and launches pware-vision from PATH.
	EnvGdeskVisionCommand = EnvPrefix + "GDESK_VISION_COMMAND"

	// GdeskEnvPrefix is what every glass-desk variable above begins with.
	GdeskEnvPrefix = EnvPrefix + "GDESK_"

	// LegacyGdeskEnvPrefix is what every variable above began with before the
	// desk became the `gdesk` context. A caller still exporting one is
	// honoured — see gdesk.AliasLegacyEnv — because the alternative is a
	// working box going silent on an upgrade with nothing to point at.
	LegacyGdeskEnvPrefix = EnvPrefix + "DESK_"

	// EnvAPIKeySuffix ends the variable holding one provider key. Never a
	// flag: a flag lands in ps output and shell history.
	EnvAPIKeySuffix = "_API_KEY"
)

// EnvAPIKey names the environment variable holding the value for a named
// secret kind, e.g. "openai" becomes INITAGENT_OPENAI_API_KEY. The kind is
// configuration and travels freely; only this variable holds the secret, and
// a `sec-` row replaces it later without renaming anything else (`24`, `41`).
func EnvAPIKey(secretKind string) string {
	return EnvPrefix + EnvAPIKeyAlias(secretKind)
}

// EnvAPIKeyAlias is the unprefixed name operators already have on the
// machine — OPENAI_API_KEY for kind openai. Lookup tries EnvAPIKey first,
// then this, so a filled INITAGENT_* still wins and a laptop that only
// exported the vendor name does not need a second copy.
func EnvAPIKeyAlias(secretKind string) string {
	return strings.ToUpper(strings.ReplaceAll(secretKind, "-", "_")) + EnvAPIKeySuffix
}

// LookupAPIKey reads the value for a secret kind from an env snapshot.
// Empty means the key has not arrived; a Silence, not a refusal.
func LookupAPIKey(env map[string]string, secretKind string) string {
	return cmp.Or(
		strings.TrimSpace(env[EnvAPIKey(secretKind)]),
		strings.TrimSpace(env[EnvAPIKeyAlias(secretKind)]),
	)
}

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
