export interface Stats {
  cpuPercent: number
  cpuCores: number
  load1: number
  load5: number
  load15: number
  memUsed: number
  memTotal: number
  swapUsed: number
  swapTotal: number
  diskUsed: number
  diskTotal: number
  netRxBytes: number
  netTxBytes: number
  processCount: number
  uptimeSec: number
}

// --- accounts and organizations ---

// Me is what /api/me answers: the hub's state before sign-in, and the
// identity behind the session after it.
//
// `platformAdmin` and `orgs` are what decide which surfaces the cockpit
// offers. Hiding a section is convenience only — every endpoint behind it
// checks the same capability on the hub.
export interface Me {
  claimed: boolean
  offering: string
  passwordMinLength: number
  // Hosted claimed hubs offer customer register next to login. Self-host
  // never does; the flag travels from the hub so the form cannot invent it.
  signup?: boolean
  authenticated: boolean
  version: string
  platformAdmin?: boolean
  accountId?: string
  email?: string
  orgs?: Membership[]
  // The assignable org roles, weakest first. It arrives from the hub rather
  // than being a second list here, because a role name that exists in only
  // one of the two places is a permission bug waiting for a typo.
  orgRoles?: string[]
}

// Account is a person who can sign in. Only one account per installation
// carries isAdmin: the operator who claimed the hub.
export interface Account {
  id: string
  email: string
  isAdmin: boolean
  createdAt: number
}

// Org is a customer organization. `members` is the roster size, which is as
// far as the platform surface sees into an org it is not a member of.
export interface Org {
  id: string
  name: string
  createdAt: number
  members: number
}

// OrgMember is one person's place in an organization, as its own people see
// it.
export interface OrgMember {
  accountId: string
  email: string
  role: string
  createdAt: number
}

// Membership is the same relation from the signed-in person's side: which
// organizations are mine, and what am I in them.
export interface Membership {
  orgId: string
  name: string
  role: string
}

export interface Device {
  id: string
  name: string
  hostname: string
  os: string
  arch: string
  isHub: boolean
  createdAt: number
  lastSeen: number
  online: boolean
  tmux: boolean
  agentVersion?: string
  platform?: string
  platformVersion?: string
  kernelVersion?: string
  stats?: Stats
}

export interface Project {
  id: string
  name: string
  orgId: string
  gatewayUrl: string
  deviceId: string
  path: string
  createdAt: number
  updatedAt: number
}

export interface ExecResult {
  exitCode: number
  stdout: string
  stderr: string
  truncated?: boolean
}

export interface TaskView {
  id: string
  projectId: string
  state: string
  command: string
  assignedWorkerId?: string
  exitCode: number
  reason?: string
  stdout?: string
  stderr?: string
}

export interface SetupTool {
  id: 'node' | 'codex' | 'claude' | 'gemini' | 'tailscale'
  name: string
  description: string
  installed: boolean
  version?: string
  auth: 'missing' | 'ready' | 'connected' | 'not-required' | 'unknown'
  installCommand: string
  authCommand?: string
  note?: string
  docsUrl: string
}

export interface SetupOverview {
  os: string
  arch: string
  tools: SetupTool[]
  bundleCommand: string
}

export interface Session {
  name: string
  kind: string
  status: 'working' | 'idle' | 'exited'
  createdAt: number
  lastActivity: number
  attached: boolean
  ephemeral: boolean
}

export interface FleetSession extends Session {
  deviceId: string
  deviceName: string
}

export interface Preset {
  id: number
  name: string
  command: string
  kind: string
}

export interface ApiTokenInfo {
  id: number
  name: string
  createdAt: number
}

export interface UpdateStatus {
  currentVersion: string
  latestVersion?: string
  rollbackVersion?: string
  updateAvailable: boolean
  autoUpdate: boolean
  managed: boolean
  checking: boolean
  applying: boolean
  lastChecked?: number
  error?: string
  fleetTotal: number
  fleetOutdated: number
}

export interface FsEntry {
  name: string
  dir: boolean
  size: number
  mode: string
  modTime: number
}

export interface FsListing {
  path: string
  entries: FsEntry[]
}

export interface HubEvent {
  type: 'device.online' | 'device.offline' | 'device.stats' | 'sessions.changed'
  deviceId?: string
  stats?: Stats
}
