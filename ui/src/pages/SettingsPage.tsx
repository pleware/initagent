import { FormEvent, useCallback, useMemo, useState } from 'react'
import { api, timeAgo } from '../api'
import { usePoll } from '../hooks'
import type { ApiTokenInfo, Me, Preset, Project, UpdateStatus } from '../types'

export default function SettingsPage({ me }: { me: Me }) {
  return (
    <div className="page-shell max-w-4xl">
      <p className="eyebrow mb-3">Hub controls</p>
      <h1 className="mb-8 text-3xl font-semibold tracking-[-0.04em] text-zinc-100">Settings</h1>
      <SoftwareUpdates />
      <ApiTokens me={me} />
      <Presets />
      <McpHelp />
    </div>
  )
}

const cardClass =
  'surface mb-5 rounded-2xl p-6'
const inputClass =
  'rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-lime-500'

function SoftwareUpdates() {
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')

  const load = useCallback(async () => {
    try {
      setStatus(await api.get<UpdateStatus>('/api/updates'))
    } catch {
      /* a restart briefly makes the hub unavailable */
    }
  }, [])
  usePoll(load, 10000)

  const run = async (action: 'check' | 'install' | 'rollback') => {
    setBusy(action)
    setMessage('')
    try {
      if (action === 'rollback' && !confirm(`Restore ${status?.rollbackVersion}? The hub will restart.`)) return
      const next = await api.post<UpdateStatus | { ok: boolean }>(`/api/updates/${action}`)
      if ('currentVersion' in next) setStatus(next)
      if (action !== 'check') {
        setMessage(action === 'install' ? 'Verified update is installing. The hub will restart automatically.' : 'Previous version is being restored. The hub will restart automatically.')
        setTimeout(() => location.reload(), 7000)
      }
      await load()
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Update action failed')
    } finally {
      setBusy('')
    }
  }

  const setAuto = async (enabled: boolean) => {
    if (!status) return
    setStatus({ ...status, autoUpdate: enabled })
    try {
      await api.patch('/api/updates', { autoUpdate: enabled })
    } catch (error) {
      setStatus({ ...status, autoUpdate: !enabled })
      setMessage(error instanceof Error ? error.message : 'Could not save update preference')
    }
  }

  return (
    <section className={cardClass}>
      <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <h2 className="text-lg font-medium text-zinc-100">Software updates</h2>
            {status?.updateAvailable && <span className="rounded-full bg-lime-400/10 px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-lime-300">Update ready</span>}
          </div>
          <p className="max-w-xl text-sm leading-6 text-zinc-400">
            Stable releases are checksum-verified, tested before replacement, and keep one previous version ready for rollback.
          </p>
        </div>
        <label className="flex shrink-0 items-center gap-3 text-sm text-zinc-300">
          <span>Auto-update</span>
          <input
            type="checkbox"
            checked={status?.autoUpdate ?? true}
            disabled={!status?.managed}
            onChange={(event) => setAuto(event.target.checked)}
            className="h-4 w-4 accent-lime-400"
          />
        </label>
      </div>

      <div className="mt-5 grid gap-px overflow-hidden rounded-xl border border-zinc-800 bg-zinc-800 sm:grid-cols-3">
        <div className="bg-zinc-950/70 p-4"><p className="eyebrow">Installed</p><p className="mt-2 font-mono text-sm text-zinc-200">{status?.currentVersion || 'Loading…'}</p></div>
        <div className="bg-zinc-950/70 p-4"><p className="eyebrow">Stable release</p><p className="mt-2 font-mono text-sm text-zinc-200">{status?.latestVersion || 'Not checked'}</p></div>
        <div className="bg-zinc-950/70 p-4"><p className="eyebrow">Device fleet</p><p className="mt-2 text-sm text-zinc-200">{status ? `${status.fleetTotal - status.fleetOutdated}/${status.fleetTotal} current` : 'Loading…'}</p></div>
      </div>

      {!status?.managed && status && (
        <p className="mt-4 rounded-lg border border-amber-500/20 bg-amber-500/5 p-3 text-xs leading-5 text-amber-200/80">
          This is a standalone/debug run. It can check releases, but automatic replacement is enabled after installing initagent as a background service. You can also run <code className="font-mono">initagent update</code> manually.
        </p>
      )}
      {(message || status?.error) && <p className="mt-4 text-xs leading-5 text-zinc-400">{message || status?.error}</p>}

      <div className="mt-5 flex flex-wrap gap-2">
        <button className="btn-secondary" disabled={!!busy} onClick={() => run('check')}>{busy === 'check' ? 'Checking…' : 'Check now'}</button>
        {status?.updateAvailable && status.managed && <button className="btn-primary" disabled={!!busy} onClick={() => run('install')}>{busy === 'install' ? 'Installing…' : `Install ${status.latestVersion}`}</button>}
        {status?.rollbackVersion && status.managed && <button className="btn-secondary" disabled={!!busy} onClick={() => run('rollback')}>Restore {status.rollbackVersion}</button>}
      </div>
      {status?.lastChecked ? <p className="mt-3 text-[11px] text-zinc-600">Last checked {timeAgo(status.lastChecked)} · managed agents retry automatically and update to the hub release.</p> : null}
    </section>
  )
}

// scopeGroups buckets `verb:entity` by its entity so the mint form reads as
// "what may this touch" rather than as twenty unsorted checkboxes.
function scopeGroups(scopes: string[]) {
  const groups = new Map<string, string[]>()
  for (const scope of scopes) {
    const entity = scope.slice(scope.indexOf(':') + 1)
    const bucket = groups.get(entity)
    if (bucket) bucket.push(scope)
    else groups.set(entity, [scope])
  }
  return [...groups].sort(([a], [b]) => a.localeCompare(b))
}

// ApiTokens mints and revokes credentials carrying draft 09's three axes.
//
// The form is deliberately wider than "name it and press create". A token
// with no boundary and no verb list is exactly the credential that put
// arbitrary command execution on every enrolled machine behind one bearer,
// so the boundary is required and the verbs are opt-in. `exec:fleet.device`
// sits apart and unchecked: it is the one scope whose misuse is not a data
// leak but a command running on someone's machine.
function ApiTokens({ me }: { me: Me }) {
  const memberships = me.orgs ?? []
  const catalogue = me.tokenScopes ?? []

  const [tokens, setTokens] = useState<ApiTokenInfo[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [name, setName] = useState('')
  const [orgId, setOrgId] = useState(memberships[0]?.orgId ?? '')
  const [projectId, setProjectId] = useState('')
  const [chosen, setChosen] = useState<string[]>([])
  const [fresh, setFresh] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      const [issued, readable] = await Promise.all([
        api.get<ApiTokenInfo[]>('/api/tokens'),
        api.get<Project[]>('/api/projects'),
      ])
      setTokens(issued)
      setProjects(readable)
    } catch {
      /* transient */
    }
  }, [])
  usePoll(load, 30000)

  const orgName = useCallback(
    (id: string) => memberships.find((m) => m.orgId === id)?.name ?? id,
    [memberships],
  )
  const projectName = useCallback(
    (id: string) => projects.find((p) => p.id === id)?.name ?? id,
    [projects],
  )

  // Narrowing is offered only inside the chosen tenant, because a project in
  // another org is not a narrower grant — it is a different one, and the hub
  // refuses it.
  const narrowable = useMemo(
    () => projects.filter((p) => p.orgId === orgId),
    [orgId, projects],
  )
  const safe = useMemo(
    () => scopeGroups(catalogue.filter((s) => !s.dangerous).map((s) => s.scope)),
    [catalogue],
  )
  const dangerous = useMemo(() => catalogue.filter((s) => s.dangerous), [catalogue])

  const toggle = (scope: string) =>
    setChosen((current) =>
      current.includes(scope) ? current.filter((s) => s !== scope) : [...current, scope],
    )

  const create = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      const r = await api.post<{ token: string }>('/api/tokens', {
        name,
        orgId,
        projectId,
        scopes: chosen,
      })
      setFresh(r.token)
      setName('')
      setProjectId('')
      setChosen([])
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not mint the token')
    }
  }

  const revoke = async (id: string) => {
    if (!confirm('Revoke this token? Anything using it will lose access.')) return
    try {
      await api.del(`/api/tokens/${id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not revoke the token')
    }
    load()
  }

  return (
    <section className={cardClass}>
      <h2 className="mb-1 text-lg font-medium text-zinc-100">API tokens</h2>
      <p className="mb-4 text-sm text-zinc-400">
        For the <code className="text-lime-300">initagent fleet</code> CLI and
        the MCP server — this is how your coding agents get hands on the fleet.
        A token never exceeds your own permissions, and it stops working the
        moment you leave the organization it names.
      </p>

      {memberships.length === 0 ? (
        <p className="mb-4 rounded-lg border border-amber-500/20 bg-amber-500/5 p-3 text-xs leading-5 text-amber-200/80">
          A token acts as a person inside an organization, and this session
          belongs to none. Create an organization first, or sign in with an
          account that is a member of one.
        </p>
      ) : (
        <form onSubmit={create} className="mb-5 flex flex-col gap-3">
          <div className="flex flex-col gap-2 sm:flex-row">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="token name, e.g. senior-agent"
              required
              className={`${inputClass} flex-1`}
            />
            <select
              value={orgId}
              onChange={(e) => {
                setOrgId(e.target.value)
                setProjectId('')
              }}
              className={inputClass}
              aria-label="Organization"
            >
              {memberships.map((m) => (
                <option key={m.orgId} value={m.orgId}>
                  {m.name}
                </option>
              ))}
            </select>
            <select
              value={projectId}
              onChange={(e) => setProjectId(e.target.value)}
              className={inputClass}
              aria-label="Project"
            >
              <option value="">every project in this organization</option>
              {narrowable.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>

          <fieldset className="rounded-xl border border-zinc-800 p-4">
            <legend className="eyebrow px-1">What it may do</legend>
            <div className="grid gap-x-6 gap-y-3 sm:grid-cols-2">
              {safe.map(([entity, group]) => (
                <div key={entity}>
                  <p className="mb-1 font-mono text-[11px] uppercase tracking-wider text-zinc-500">
                    {entity}
                  </p>
                  {group.map((scope) => (
                    <label key={scope} className="flex items-center gap-2 py-0.5 text-sm text-zinc-300">
                      <input
                        type="checkbox"
                        checked={chosen.includes(scope)}
                        onChange={() => toggle(scope)}
                        className="h-4 w-4 accent-lime-400"
                      />
                      <span className="font-mono text-xs">{scope.slice(0, scope.indexOf(':'))}</span>
                    </label>
                  ))}
                </div>
              ))}
            </div>

            {dangerous.length > 0 && (
              <div className="mt-4 rounded-lg border border-rose-500/25 bg-rose-500/5 p-3">
                <p className="mb-2 text-xs leading-5 text-rose-200/80">
                  Running a command is not reading data. Anything holding one
                  of these can execute code on the machines in scope.
                </p>
                {dangerous.map(({ scope }) => (
                  <label key={scope} className="flex items-center gap-2 py-0.5 text-sm text-rose-100">
                    <input
                      type="checkbox"
                      checked={chosen.includes(scope)}
                      onChange={() => toggle(scope)}
                      className="h-4 w-4 accent-rose-400"
                    />
                    <span className="font-mono text-xs">{scope}</span>
                  </label>
                ))}
              </div>
            )}
          </fieldset>

          <div className="flex items-center gap-3">
            <button className="btn-primary" disabled={chosen.length === 0}>
              Create
            </button>
            <span className="text-xs text-zinc-500">
              {chosen.length === 0
                ? 'Pick at least one thing it may do.'
                : `${chosen.length} scope${chosen.length === 1 ? '' : 's'} selected`}
            </span>
          </div>
        </form>
      )}

      {error && (
        <p className="mb-4 rounded-lg border border-rose-500/30 bg-rose-500/10 p-3 text-xs leading-5 text-rose-200">
          {error}
        </p>
      )}
      {fresh && (
        <div className="mb-4 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3">
          <p className="mb-2 text-xs text-emerald-300">
            Copy this now — it won't be shown again:
          </p>
          <code className="block overflow-x-auto whitespace-nowrap font-mono text-[13px] text-emerald-200">
            {fresh}
          </code>
        </div>
      )}
      <ul className="divide-y divide-zinc-800/60">
        {tokens.map((t) => (
          <li key={t.id} className="flex flex-wrap items-start justify-between gap-3 py-3">
            <div className="min-w-0">
              <p className="text-sm text-zinc-200">{t.name}</p>
              <p className="mt-0.5 text-xs text-zinc-500">
                {orgName(t.orgId)}
                {t.projectId ? ` · ${projectName(t.projectId)}` : ' · every project'}
              </p>
              <p className="mt-1 flex flex-wrap gap-1">
                {t.scopes.map((scope) => (
                  <span
                    key={scope}
                    className={`rounded px-1.5 py-0.5 font-mono text-[10px] ${
                      catalogue.find((s) => s.scope === scope)?.dangerous
                        ? 'bg-rose-500/10 text-rose-300'
                        : 'bg-zinc-800 text-zinc-400'
                    }`}
                  >
                    {scope}
                  </span>
                ))}
              </p>
            </div>
            <span className="flex items-center gap-4">
              <span className="text-xs text-zinc-500">
                {t.lastUsedAt ? `used ${timeAgo(t.lastUsedAt)}` : 'never used'}
              </span>
              <button
                onClick={() => revoke(t.id)}
                className="text-xs text-zinc-500 hover:text-rose-400"
              >
                Revoke
              </button>
            </span>
          </li>
        ))}
        {tokens.length === 0 && (
          <li className="py-2 text-sm text-zinc-500">No tokens yet.</li>
        )}
      </ul>
    </section>
  )
}

function Presets() {
  const [presets, setPresets] = useState<Preset[]>([])
  const [name, setName] = useState('')
  const [command, setCommand] = useState('')

  const load = useCallback(async () => {
    try {
      setPresets(await api.get<Preset[]>('/api/presets'))
    } catch {
      /* transient */
    }
  }, [])
  usePoll(load, 30000)

  const create = async (e: FormEvent) => {
    e.preventDefault()
    await api.post('/api/presets', { name, command })
    setName('')
    setCommand('')
    load()
  }

  const remove = async (id: number) => {
    await api.del(`/api/presets/${id}`)
    load()
  }

  return (
    <section className={cardClass}>
      <h2 className="mb-1 text-lg font-medium text-zinc-100">
        Launch presets
      </h2>
      <p className="mb-4 text-sm text-zinc-400">
        One-click commands in the Launch dialog. Add your favorite agents.
      </p>
      <form onSubmit={create} className="mb-4 flex flex-col gap-2 sm:flex-row">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="name, e.g. Aider"
          required
          className={`${inputClass} w-full sm:w-40`}
        />
        <input
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          placeholder="command, e.g. aider"
          className={`${inputClass} flex-1 font-mono`}
        />
        <button className="btn-primary">
          Add
        </button>
      </form>
      <ul className="divide-y divide-zinc-800/60">
        {presets.map((p) => (
          <li key={p.id} className="flex items-center justify-between py-2">
            <span className="text-sm text-zinc-200">{p.name}</span>
            <span className="flex items-center gap-4">
              <code className="font-mono text-xs text-zinc-400">
                {p.command || '(shell)'}
              </code>
              <button
                onClick={() => remove(p.id)}
                className="text-xs text-zinc-500 hover:text-rose-400"
              >
                Delete
              </button>
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

function McpHelp() {
  const origin = location.origin
  return (
    <section className={cardClass}>
      <h2 className="mb-1 text-lg font-medium text-zinc-100">
        Give an agent control of your fleet
      </h2>
      <p className="mb-4 text-sm text-zinc-400">
        Run a coding agent on any machine with the{' '}
        <code className="text-lime-300">initagent</code> binary and an API token,
        and it can see every device, launch worker agents, read their output,
        and steer them. For Claude Code:
      </p>
      <pre className="overflow-x-auto rounded-lg border border-zinc-700 bg-zinc-950 p-4 font-mono text-[13px] leading-relaxed text-zinc-300">
        {`initagent fleet login --hub ${origin} --token YOUR_API_TOKEN
claude mcp add initagent -- initagent mcp`}
      </pre>
      <p className="mt-3 text-xs text-zinc-500">
        Then ask it things like “launch claude in ~/projects/api on the
        homelab box and have it fix the failing tests.”
      </p>
      <p className="mt-2 text-xs text-zinc-500">
        An agent needs the device, task and terminal scopes to do that, plus{' '}
        <code className="font-mono text-zinc-400">exec:fleet.device</code> if
        you want it running commands directly. If a tool comes back refused,
        the hub names the scope the token is missing — mint a new one with it
        rather than widening the old.
      </p>

      <div className="mt-6 border-t border-zinc-800 pt-5">
        <h3 className="mb-1 text-sm font-medium text-zinc-100">
          Remote MCP (ChatGPT, Claude, Cursor)
        </h3>
        <p className="mb-3 text-sm text-zinc-400">
          Any client that supports remote MCP connectors can drive your fleet
          over HTTPS. Add this endpoint and paste an API token as the Bearer
          credential:
        </p>
        <pre className="overflow-x-auto rounded-lg border border-zinc-700 bg-zinc-950 p-4 font-mono text-[13px] leading-relaxed text-zinc-300">
          {`${origin.replace(/^http:/, 'https:')}/mcp`}
        </pre>
        <div className="mt-3 rounded-lg border border-amber-500/25 bg-amber-500/8 p-3 text-xs text-amber-200/90">
          ⚠️ This is a remote shell — the tools run commands and write files on
          your devices. Only expose it over HTTPS ({''}
          <code className="font-mono">--tls-domain</code> or a TLS proxy), and
          treat the API token like an SSH key. Revoke it above if it leaks.
        </div>
      </div>
    </section>
  )
}
