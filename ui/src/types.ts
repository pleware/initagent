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
  // The display name register writes when nobody names the company.
  // Boarding compares the current org to this rather than repeating it.
  defaultOrgName?: string
  authenticated: boolean
  version: string
  platformAdmin?: boolean
  accountId?: string
  email?: string
  locale?: string
  orgs?: Membership[]
  // The assignable org roles, weakest first. It arrives from the hub rather
  // than being a second list here, because a role name that exists in only
  // one of the two places is a permission bug waiting for a typo.
  orgRoles?: string[]
  // The verbs a token may carry, for the same reason as orgRoles.
  tokenScopes?: TokenScope[]
}

// Account is a person who can sign in. Only one account per installation
// carries isAdmin: the operator who claimed the hub.
export interface Account {
  id: string
  email: string
  isAdmin: boolean
  createdAt: number
}

// KPISnapshot is GET /api/admin/kpis — the hosted funnel, not telemetry.
export interface KPISnapshot {
  acquisition: {
    ctaOpenApp: number
    ctaSelfHost: number
    signups: number
    inviteRedeems: number
    signupRate?: number
  }
  activation: {
    orgsWithProject: number
    orgsWithWorker: number
    orgsWithTask: number
    projectRate?: number
    workerRate?: number
    taskRate?: number
    timeToValueHours?: number
  }
  hygiene: {
    planLimitHits: number
    idleWarned: number
    idleDeleted: number
  }
  retention: {
    d7Eligible: number
    d7Returned: number
    d7Rate?: number
    d30Eligible: number
    d30Returned: number
    d30Rate?: number
  }
  conversion: {
    paidOrgs: number
    convertedRate?: number
    timeToPayAvailable: boolean
    mrrAvailable: boolean
  }
  cost: {
    onlineWorkers: number
  }
}

// Org is a customer organization. `members` is the roster size, which is as
// far as the platform surface sees into an org it is not a member of.
export interface Org {
  id: string
  name: string
  plan: string
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

// OrgInvite is a pending invitation as the People screen lists it.
// The one-time secret is never here; only create returns the link.
export interface OrgInvite {
  id: string
  orgId: string
  email: string
  role: string
  expiresAt: number
  createdAt: number
}

// Membership is the same relation from the signed-in person's side: which
// organizations are mine, and what am I in them.
export type { PlanSlug } from './lib/org-plans.gen'

export interface Membership {
  orgId: string
  name: string
  plan: string
  role: string
}

export interface Connector {
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
  connectorId: string
  connectorIds?: string[]
  path: string
  templateId?: string
  repoRemote?: string
  repoHost?: string
  createdAt: number
  updatedAt: number
}

export interface ProjectTemplate {
  id: string
  label: string
  live: boolean
  contract: string
  needsRepo: boolean
  requiredTasks: string[]
}

export interface ExecResult {
  exitCode: number
  stdout: string
  stderr: string
  truncated?: boolean
}

export type TaskLaunch = 'exec' | 'process' | 'send_keys'

export interface TaskView {
  id: string
  projectId: string
  state: string
  command: string
  launch?: string
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
  connectorId: string
  connectorName: string
}

export interface Preset {
  id: number
  name: string
  command: string
  kind: string
}

// ApiTokenInfo is one issued credential as the cockpit lists it. Every field
// after `name` is one of draft 09's three axes — who it acts as, how far it
// reaches, what it may do — because a token the operator cannot read those
// off is a token they cannot decide to revoke.
export interface ApiTokenInfo {
  id: string
  name: string
  accountId: string
  orgId: string
  projectId: string
  scopes: string[]
  createdAt: number
  lastUsedAt: number
}

// TokenScope is one verb the hub will accept in a mint. `dangerous` is what
// keeps arbitrary command execution off the same footing as reading a device
// list: the mint form separates it and leaves it unchecked.
export interface TokenScope {
  scope: string
  dangerous: boolean
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
  type: 'connector.online' | 'connector.offline' | 'connector.stats' | 'sessions.changed'
  connectorId?: string
  stats?: Stats
}
