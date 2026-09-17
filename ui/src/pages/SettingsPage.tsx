import { FormEvent, useCallback, useMemo, useState, type ComponentType, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowClockwiseIcon, KeyIcon, PlugsConnectedIcon, RocketLaunchIcon, ShieldCheckIcon } from '@phosphor-icons/react'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@ia/web/ui/accordion'
import { api, timeAgo } from '../api'
import { usePoll } from '../hooks'
import type { ApiTokenInfo, Me, Preset, Project, UpdateStatus } from '../types'

export default function SettingsPage({ me }: { me: Me }) {
  const { t } = useTranslation()
  return (
    <div className="page-shell max-w-4xl">
      <p className="eyebrow mb-3">{t('settings.eyebrow')}</p>
      <h1 className="mb-8 text-3xl font-semibold tracking-[-0.04em] text-fg">{t('settings.title')}</h1>
      <Accordion className="gap-3" multiple>
        <SettingsPanel value="updates" icon={ArrowClockwiseIcon} title={t('settings.updates')}>
          <SoftwareUpdates />
        </SettingsPanel>
        <SettingsPanel value="tokens" icon={KeyIcon} title={t('settings.tokens')}>
          <ApiTokens me={me} />
        </SettingsPanel>
        {me.platformAdmin && (
          <SettingsPanel value="adminTokens" icon={ShieldCheckIcon} title={t('settings.adminTokens')}>
            <AdminTokens me={me} />
          </SettingsPanel>
        )}
        <SettingsPanel value="presets" icon={RocketLaunchIcon} title={t('settings.presets')}>
          <Presets />
        </SettingsPanel>
        <SettingsPanel value="mcp" icon={PlugsConnectedIcon} title={t('settings.mcp')}>
          <McpHelp />
        </SettingsPanel>
      </Accordion>
    </div>
  )
}

const inputClass =
  'rounded-lg border border-line-3 bg-canvas-sunken px-3 py-2 text-sm text-fg-strong outline-none focus:border-accent'

function SettingsPanel({
  value,
  icon: Icon,
  title,
  children,
}: {
  value: string
  icon: ComponentType<{ className?: string }>
  title: string
  children: ReactNode
}) {
  return (
    <AccordionItem value={value} className="surface flex w-full flex-col overflow-hidden rounded-2xl not-last:border-b-0">
      <AccordionTrigger className="w-full items-center rounded-none px-5 py-4 hover:bg-fill-5 hover:no-underline">
        <span className="flex min-w-0 items-center gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line-3 bg-fill-3 text-fg-soft">
            <Icon className="size-4" />
          </span>
          <span className="text-base font-medium text-fg">{title}</span>
        </span>
      </AccordionTrigger>
      <AccordionContent className="w-full px-5 pb-5">
        {children}
      </AccordionContent>
    </AccordionItem>
  )
}

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
    <div>
      <div className="flex flex-col gap-4">
        <p className="max-w-xl text-sm leading-6 text-fg-muted">
          Stable releases are checksum-verified, tested before replacement, and keep one previous version ready for rollback.
          {status?.updateAvailable ? (
            <span className="ml-2 rounded-full bg-accent/10 px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-accent">Update ready</span>
          ) : null}
        </p>
        <label className="flex items-center gap-3 text-sm text-fg-soft">
          <span>Auto-update</span>
          <input
            type="checkbox"
            checked={status?.autoUpdate ?? true}
            disabled={!status?.managed}
            onChange={(event) => setAuto(event.target.checked)}
            className="h-4 w-4 accent-accent"
          />
        </label>
      </div>

      <div className="mt-5 grid gap-px overflow-hidden rounded-xl border border-line-2 bg-sidebar sm:grid-cols-3">
        <div className="bg-canvas-sunken/70 p-4"><p className="eyebrow">Installed</p><p className="mt-2 font-mono text-sm text-fg">{status?.currentVersion || 'Loading…'}</p></div>
        <div className="bg-canvas-sunken/70 p-4"><p className="eyebrow">Stable release</p><p className="mt-2 font-mono text-sm text-fg">{status?.latestVersion || 'Not checked'}</p></div>
        <div className="bg-canvas-sunken/70 p-4"><p className="eyebrow">Connector fleet</p><p className="mt-2 text-sm text-fg">{status ? `${status.fleetTotal - status.fleetOutdated}/${status.fleetTotal} current` : 'Loading…'}</p></div>
      </div>

      {!status?.managed && status && (
        <p className="mt-4 rounded-lg border border-warn/20 bg-warn/5 p-3 text-xs leading-5 text-warn-fg/80">
          This is a standalone/debug run. It can check releases, but automatic replacement is enabled after installing initagent as a background service. You can also run <code className="font-mono">initagent update</code> manually.
        </p>
      )}
      {(message || status?.error) && <p className="mt-4 text-xs leading-5 text-fg-muted">{message || status?.error}</p>}

      <div className="mt-5 flex flex-wrap gap-2">
        <button className="btn-secondary" disabled={!!busy} onClick={() => run('check')}>{busy === 'check' ? 'Checking…' : 'Check now'}</button>
        {status?.updateAvailable && status.managed && <button className="btn-primary" disabled={!!busy} onClick={() => run('install')}>{busy === 'install' ? 'Installing…' : `Install ${status.latestVersion}`}</button>}
        {status?.rollbackVersion && status.managed && <button className="btn-secondary" disabled={!!busy} onClick={() => run('rollback')}>Restore {status.rollbackVersion}</button>}
      </div>
      {status?.lastChecked ? <p className="mt-3 text-[11px] text-fg-faint">Last checked {timeAgo(status.lastChecked)} · managed agents retry automatically and update to the hub release.</p> : null}
    </div>
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
// so the boundary is required and the verbs are opt-in. `exec:fleet.connector`
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
    <div>
      <p className="mb-4 text-sm text-fg-muted">
        For the <code className="text-accent">initagent fleet</code> CLI and
        the MCP server — this is how your coding agents get hands on the fleet.
        A token never exceeds your own permissions, and it stops working the
        moment you leave the organization it names.
      </p>

      {memberships.length === 0 ? (
        <p className="mb-4 rounded-lg border border-warn/20 bg-warn/5 p-3 text-xs leading-5 text-warn-fg/80">
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
            <SimpleSelect
              size="default"
              value={orgId}
              onValueChange={(next) => {
                setOrgId(next)
                setProjectId('')
              }}
              className="min-w-40"
              aria-label="Organization"
              items={memberships.map((m) => ({ value: m.orgId, label: m.name }))}
            />
            <SimpleSelect
              size="default"
              value={projectId}
              onValueChange={setProjectId}
              className="min-w-40"
              aria-label="Project"
              items={[
                { value: '', label: 'every project in this organization' },
                ...narrowable.map((p) => ({ value: p.id, label: p.name })),
              ]}
            />
          </div>

          <fieldset className="rounded-xl border border-line-2 p-4">
            <legend className="eyebrow px-1">What it may do</legend>
            <div className="grid gap-x-6 gap-y-3 sm:grid-cols-2">
              {safe.map(([entity, group]) => (
                <div key={entity}>
                  <p className="mb-1 font-mono text-[11px] uppercase tracking-wider text-fg-subtle">
                    {entity}
                  </p>
                  {group.map((scope) => (
                    <label key={scope} className="flex items-center gap-2 py-0.5 text-sm text-fg-soft">
                      <input
                        type="checkbox"
                        checked={chosen.includes(scope)}
                        onChange={() => toggle(scope)}
                        className="h-4 w-4 accent-accent"
                      />
                      <span className="font-mono text-xs">{scope.slice(0, scope.indexOf(':'))}</span>
                    </label>
                  ))}
                </div>
              ))}
            </div>

            {dangerous.length > 0 && (
              <div className="mt-4 rounded-lg border border-fail/25 bg-fail/5 p-3">
                <p className="mb-2 text-xs leading-5 text-fail-fg/80">
                  Running a command is not reading data. Anything holding one
                  of these can execute code on the machines in scope.
                </p>
                {dangerous.map(({ scope }) => (
                  <label key={scope} className="flex items-center gap-2 py-0.5 text-sm text-fail-fg">
                    <input
                      type="checkbox"
                      checked={chosen.includes(scope)}
                      onChange={() => toggle(scope)}
                      className="h-4 w-4 accent-fail"
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
            <span className="text-xs text-fg-subtle">
              {chosen.length === 0
                ? 'Pick at least one thing it may do.'
                : `${chosen.length} scope${chosen.length === 1 ? '' : 's'} selected`}
            </span>
          </div>
        </form>
      )}

      {error && (
        <p className="mb-4 rounded-lg border border-fail/30 bg-fail/10 p-3 text-xs leading-5 text-fail-fg">
          {error}
        </p>
      )}
      {fresh && (
        <div className="mb-4 rounded-lg border border-ok/30 bg-ok/10 p-3">
          <p className="mb-2 text-xs text-ok">
            Copy this now — it won't be shown again:
          </p>
          <code className="block overflow-x-auto whitespace-nowrap font-mono text-[13px] text-ok">
            {fresh}
          </code>
        </div>
      )}
      <ul className="divide-y divide-line-2/60">
        {tokens.map((t) => (
          <li key={t.id} className="flex flex-wrap items-start justify-between gap-3 py-3">
            <div className="min-w-0">
              <p className="text-sm text-fg">{t.name}</p>
              <p className="mt-0.5 text-xs text-fg-subtle">
                {orgName(t.orgId)}
                {t.projectId ? ` · ${projectName(t.projectId)}` : ' · every project'}
              </p>
              <p className="mt-1 flex flex-wrap gap-1">
                {t.scopes.map((scope) => (
                  <span
                    key={scope}
                    className={`rounded px-1.5 py-0.5 font-mono text-[10px] ${
                      catalogue.find((s) => s.scope === scope)?.dangerous
                        ? 'bg-fail/10 text-fail-fg'
                        : 'bg-sidebar text-fg-muted'
                    }`}
                  >
                    {scope}
                  </span>
                ))}
              </p>
            </div>
            <span className="flex items-center gap-4">
              <span className="text-xs text-fg-subtle">
                {t.lastUsedAt ? `used ${timeAgo(t.lastUsedAt)}` : 'never used'}
              </span>
              <button
                onClick={() => revoke(t.id)}
                className="text-xs text-fg-subtle hover:text-fail-fg"
              >
                Revoke
              </button>
            </span>
          </li>
        ))}
        {tokens.length === 0 && (
          <li className="py-2 text-sm text-fg-subtle">No tokens yet.</li>
        )}
      </ul>
    </div>
  )
}

// AdminTokens mints and revokes the installation-scoped credentials only
// the platform operator may hold. Unlike ApiTokens there is no org or
// project axis: these tokens are the installation's, so the list is flat —
// name, scope badges, last use, revoke.
function AdminTokens({ me }: { me: Me }) {
  const { t } = useTranslation()
  const catalogue = me.adminTokenScopes ?? []

  const [tokens, setTokens] = useState<ApiTokenInfo[]>([])
  const [name, setName] = useState('')
  const [chosen, setChosen] = useState<string[]>([])
  const [fresh, setFresh] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      setTokens(await api.get<ApiTokenInfo[]>('/api/admin/tokens'))
    } catch {
      /* transient */
    }
  }, [])
  usePoll(load, 30000)

  const toggle = (scope: string) =>
    setChosen((current) =>
      current.includes(scope) ? current.filter((s) => s !== scope) : [...current, scope],
    )

  const create = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      const r = await api.post<{ token: string }>('/api/admin/tokens', {
        name,
        scopes: chosen,
      })
      setFresh(r.token)
      setName('')
      setChosen([])
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.adminTokensPanel.createError'))
    }
  }

  const revoke = async (id: string) => {
    if (!confirm(t('settings.adminTokensPanel.revokeConfirm'))) return
    try {
      await api.del(`/api/admin/tokens/${id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.adminTokensPanel.revokeError'))
    }
    load()
  }

  return (
    <div>
      <p className="mb-4 rounded-lg border border-warn/25 bg-warn/8 p-3 text-xs leading-5 text-warn-fg/90">
        {t('settings.adminTokensPanel.warning')}
      </p>

      <form onSubmit={create} className="mb-5 flex flex-col gap-3">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('settings.adminTokensPanel.namePlaceholder')}
          required
          className={`${inputClass} sm:max-w-72`}
        />
        <fieldset className="rounded-xl border border-line-2 p-4">
          <legend className="eyebrow px-1">{t('settings.adminTokensPanel.scopesLegend')}</legend>
          <div className="flex flex-col gap-1">
            {catalogue.map((entry) => (
              <label key={entry.scope} className="flex items-start gap-2 py-0.5 text-sm text-fg-soft">
                <input
                  type="checkbox"
                  checked={chosen.includes(entry.scope)}
                  onChange={() => toggle(entry.scope)}
                  className="mt-0.5 h-4 w-4 accent-accent"
                />
                <span className="min-w-0">
                  <span className="block font-mono text-xs">{entry.scope}</span>
                  <span className="block text-xs text-fg-subtle">{entry.description}</span>
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        <div className="flex items-center gap-3">
          <button className="btn-primary" disabled={chosen.length === 0 || name.trim() === ''}>
            {t('settings.adminTokensPanel.create')}
          </button>
          <span className="text-xs text-fg-subtle">
            {chosen.length === 0
              ? t('settings.adminTokensPanel.pickOne')
              : t('settings.adminTokensPanel.selected', { count: chosen.length })}
          </span>
        </div>
      </form>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/30 bg-fail/10 p-3 text-xs leading-5 text-fail-fg">
          {error}
        </p>
      )}
      {fresh && (
        <div className="mb-4 rounded-lg border border-ok/30 bg-ok/10 p-3">
          <p className="mb-2 text-xs text-ok">{t('settings.adminTokensPanel.copyNow')}</p>
          <code className="block overflow-x-auto whitespace-nowrap font-mono text-[13px] text-ok">
            {fresh}
          </code>
        </div>
      )}
      <ul className="divide-y divide-line-2/60">
        {tokens.map((token) => (
          <li key={token.id} className="flex flex-wrap items-start justify-between gap-3 py-3">
            <div className="min-w-0">
              <p className="text-sm text-fg">{token.name}</p>
              <p className="mt-1 flex flex-wrap gap-1">
                {token.scopes.map((scope) => (
                  <span
                    key={scope}
                    className="rounded bg-sidebar px-1.5 py-0.5 font-mono text-[10px] text-fg-muted"
                  >
                    {scope}
                  </span>
                ))}
              </p>
            </div>
            <span className="flex items-center gap-4">
              <span className="text-xs text-fg-subtle">
                {token.lastUsedAt
                  ? t('settings.adminTokensPanel.used', { ago: timeAgo(token.lastUsedAt) })
                  : t('settings.adminTokensPanel.neverUsed')}
              </span>
              <button
                onClick={() => revoke(token.id)}
                className="text-xs text-fg-subtle hover:text-fail-fg"
              >
                {t('settings.adminTokensPanel.revoke')}
              </button>
            </span>
          </li>
        ))}
        {tokens.length === 0 && (
          <li className="py-2 text-sm text-fg-subtle">{t('settings.adminTokensPanel.none')}</li>
        )}
      </ul>
    </div>
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
    <div>
      <p className="mb-4 text-sm text-fg-muted">
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
      <ul className="divide-y divide-line-2/60">
        {presets.map((p) => (
          <li key={p.id} className="flex items-center justify-between py-2">
            <span className="text-sm text-fg">{p.name}</span>
            <span className="flex items-center gap-4">
              <code className="font-mono text-xs text-fg-muted">
                {p.command || '(shell)'}
              </code>
              <button
                onClick={() => remove(p.id)}
                className="text-xs text-fg-subtle hover:text-fail-fg"
              >
                Delete
              </button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function McpHelp() {
  const origin = location.origin
  return (
    <div>
      <p className="mb-4 text-sm text-fg-muted">
        Run a coding agent on any machine with the{' '}
        <code className="text-accent">initagent</code> binary and an API token,
        and it can see every connector, launch worker agents, read their output,
        and steer them. For Claude Code:
      </p>
      <pre className="overflow-x-auto rounded-lg border border-line-3 bg-canvas-sunken p-4 font-mono text-[13px] leading-relaxed text-fg-soft">
        {`initagent fleet login --hub ${origin} --token YOUR_API_TOKEN
claude mcp add initagent -- initagent mcp`}
      </pre>
      <p className="mt-3 text-xs text-fg-subtle">
        Then ask it things like “launch claude in ~/projects/api on the
        homelab box and have it fix the failing tests.”
      </p>
      <p className="mt-2 text-xs text-fg-subtle">
        An agent needs the connector, task and terminal scopes to do that, plus{' '}
        <code className="font-mono text-fg-muted">exec:fleet.connector</code> if
        you want it running commands directly. If a tool comes back refused,
        the hub names the scope the token is missing — mint a new one with it
        rather than widening the old.
      </p>

      <div className="mt-6 border-t border-line-2 pt-5">
        <h3 className="mb-1 text-sm font-medium text-fg-strong">
          Remote MCP (ChatGPT, Claude, Cursor)
        </h3>
        <p className="mb-3 text-sm text-fg-muted">
          Any client that supports remote MCP connectors can drive your fleet
          over HTTPS. Add this endpoint and paste an API token as the Bearer
          credential:
        </p>
        <pre className="overflow-x-auto rounded-lg border border-line-3 bg-canvas-sunken p-4 font-mono text-[13px] leading-relaxed text-fg-soft">
          {`${origin.replace(/^http:/, 'https:')}/mcp`}
        </pre>
        <div className="mt-3 rounded-lg border border-warn/25 bg-warn/8 p-3 text-xs text-warn-fg/90">
          ⚠️ This is a remote shell — the tools run commands and write files on
          your connectors. Only expose it over HTTPS ({''}
          <code className="font-mono">--tls-domain</code> or a TLS proxy), and
          treat the API token like an SSH key. Revoke it above if it leaks.
        </div>
      </div>
    </div>
  )
}
