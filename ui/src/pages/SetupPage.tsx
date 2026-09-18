import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { api } from '../api'
import { usePoll } from '../hooks'
import type { Connector, SetupOverview, SetupTool } from '../types'

const toolMarks: Record<string, string> = {
  node: 'JS',
  codex: 'CX',
  claude: 'CL',
  gemini: 'GM',
  tailscale: 'TS',
}

export default function SetupPage() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [connectorId, setConnectorId] = useState('')
  const [setup, setSetup] = useState<SetupOverview | null>(null)
  const [error, setError] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const navigate = useNavigate()
  const { t } = useTranslation()

  const loadConnectors = useCallback(async () => {
    const result = await api.get<Connector[]>('/api/connectors')
    setConnectors(result)
    setConnectorId((current) => {
      if (current && result.some((d) => d.id === current && d.online)) return current
      return result.find((d) => d.online)?.id ?? ''
    })
  }, [])
  usePoll(loadConnectors, 15000)

  const loadSetup = useCallback(async () => {
    if (!connectorId) {
      setSetup(null)
      return
    }
    setRefreshing(true)
    setError('')
    try {
      setSetup(await api.get<SetupOverview>(`/api/connectors/${connectorId}/setup`))
    } catch (e) {
      setError(e instanceof Error ? e.message : t('setup.inspectError'))
    } finally {
      setRefreshing(false)
    }
  }, [connectorId])

  useEffect(() => {
    loadSetup()
  }, [loadSetup])

  const selected = useMemo(() => connectors.find((d) => d.id === connectorId), [connectors, connectorId])
  const core = setup?.tools.filter((t) => ['codex', 'claude', 'gemini'].includes(t.id)) ?? []
  const readyCount = core.filter((t) => t.installed).length

  const launchSetup = async (name: string, command: string, kind = 'setup') => {
    if (!connectorId || !command) return
    setError('')
    const session = `${name}-${Date.now().toString().slice(-6)}`
    try {
      await api.post(`/api/connectors/${connectorId}/sessions`, {
        name: session,
        command,
        kind,
      })
      navigate(`/connectors/${connectorId}?session=${encodeURIComponent(session)}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : t('setup.launchError'))
    }
  }

  return (
    <div className="page-shell">
      <section className="mb-9 grid gap-6 lg:grid-cols-[1fr_auto] lg:items-end">
        <div>
          <p className="eyebrow mb-3">{t('setup.eyebrow')}</p>
          <h1 className="max-w-3xl text-3xl font-semibold tracking-[-0.045em] text-fg-strong sm:text-4xl">
            {t('setup.title')}
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-fg-muted">
            {t('setup.subtitle')}
          </p>
        </div>
        <div className="surface flex min-w-72 items-center gap-3 rounded-xl p-3">
          <span className={`h-2 w-2 rounded-full ${selected?.online ? 'bg-ok shadow-glow-ok' : 'bg-fg-ghost'}`} />
          <SimpleSelect
            size="default"
            value={connectorId}
            onValueChange={setConnectorId}
            className="min-w-0 flex-1"
            aria-label={t('setup.connectorLabel')}
            items={connectors
              .filter((d) => d.online)
              .map((d) => ({ value: d.id, label: `${d.name} · ${d.os}/${d.arch}` }))}
          />
          <button onClick={loadSetup} disabled={refreshing || !connectorId} className="text-xs font-medium text-fg-subtle hover:text-fg-strong">
            {refreshing ? t('setup.checking') : t('common.refresh')}
          </button>
        </div>
      </section>

      {error && <div className="mb-5 rounded-lg border border-fail/20 bg-fail/[0.07] px-4 py-3 text-sm text-fail-fg">{error}</div>}

      <section className="mb-5 grid gap-4 rounded-2xl border border-info/15 bg-info/[0.045] p-5 sm:grid-cols-[auto_1fr_auto] sm:items-center sm:p-6">
        <span className="grid h-11 w-11 place-items-center rounded-xl border border-info/20 bg-info/10 font-mono text-sm font-semibold text-info-fg-strong">fx</span>
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold text-fg">{t('setup.fxBuiltIn')}</h2>
            <span className="rounded bg-ok/10 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-ok">{t('setup.fxReady')}</span>
          </div>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-fg-subtle">{t('setup.fxHint')}</p>
        </div>
        <button onClick={() => navigate('/code')} className="btn-secondary">{t('setup.openCode')}</button>
      </section>

      {!connectorId ? (
        <EmptyState />
      ) : setup === null ? (
        <SetupSkeleton />
      ) : (
        <>
          <section className="surface mb-5 grid gap-5 rounded-2xl p-5 sm:grid-cols-[1fr_auto] sm:items-center sm:p-6">
            <div>
              <div className="mb-2 flex items-center gap-2">
                <span className="eyebrow">{t('setup.agentPack')}</span>
                <span className="text-xs text-fg-faint">{setup.os}/{setup.arch}</span>
              </div>
              <h2 className="text-xl font-semibold tracking-tight text-fg-strong">
                {readyCount === 3 ? t('setup.coreInstalled') : t('setup.coreReadyCount', { count: readyCount })}
              </h2>
              <p className="mt-1 max-w-xl text-sm leading-6 text-fg-subtle">
                {t('setup.agentPackHint')}
              </p>
            </div>
            <button
              onClick={() => launchSetup('agent-pack', setup.bundleCommand)}
              className="btn-primary"
            >
              {readyCount === 3 ? t('setup.repairAll') : t('setup.installAll')}
            </button>
          </section>

          <section className="grid gap-4 lg:grid-cols-2">
            {setup.tools.map((tool) => (
              <ToolCard key={tool.id} tool={tool} onLaunch={launchSetup} />
            ))}
          </section>

          <section className="mt-7 border-l border-accent/30 pl-5">
            <h2 className="text-sm font-semibold text-fg">{t('setup.legacySignInTitle')}</h2>
            <p className="mt-1 max-w-3xl text-sm leading-6 text-fg-subtle">
              {t('setup.legacySignInHint')}
            </p>
          </section>
        </>
      )}
    </div>
  )
}

function ToolCard({
  tool,
  onLaunch,
}: {
  tool: SetupTool
  onLaunch: (name: string, command: string, kind?: string) => void
}) {
  const { t } = useTranslation()
  const connected = tool.auth === 'connected' || tool.auth === 'not-required'
  const isRemote = tool.id === 'tailscale'
  return (
    <article className={`surface rounded-2xl p-5 sm:p-6 ${isRemote ? 'lg:col-span-2' : ''}`}>
      <div className="flex items-start gap-4">
        <div className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-fill-4 font-mono text-[11px] font-bold tracking-wider text-accent-fg">
          {toolMarks[tool.id]}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="font-semibold tracking-tight text-fg-strong">{tool.name}</h2>
            <Status installed={tool.installed} connected={connected} auth={tool.auth} />
          </div>
          <p className="mt-1 text-sm leading-6 text-fg-subtle">{tool.description}</p>
          {tool.version && <p className="mt-2 truncate font-mono text-[11px] text-fg-faint">{tool.version}</p>}
        </div>
      </div>
      <div className="mt-5 flex flex-wrap items-center gap-2 border-t border-line-2 pt-4">
        <button onClick={() => onLaunch(`install-${tool.id}`, tool.installCommand)} className={tool.installed ? 'btn-secondary' : 'btn-primary'}>
          {tool.installed ? t('setup.updateOrRepair') : t('setup.install')}
        </button>
        {tool.authCommand && tool.installed && (
          <button onClick={() => onLaunch(`login-${tool.id}`, tool.authCommand!, tool.id)} className="btn-secondary">
            {isRemote ? (connected ? t('setup.refreshPrivateAccess') : t('setup.connectPrivately')) : connected ? t('setup.openAgain') : t('setup.signIn')}
          </button>
        )}
        <a href={tool.docsUrl} target="_blank" rel="noreferrer" className="ml-auto text-xs font-medium text-fg-faint hover:text-fg-soft">
          {t('setup.officialGuide')} ↗
        </a>
      </div>
      {tool.note && <p className="mt-3 text-xs leading-5 text-fg-faint">{tool.note}</p>}
    </article>
  )
}

function Status({ installed, connected, auth }: { installed: boolean; connected: boolean; auth: string }) {
  const { t } = useTranslation()
  const label = !installed
    ? t('setup.status.notInstalled')
    : auth === 'not-required'
      ? t('setup.status.ready')
      : connected
        ? t('setup.status.connected')
        : auth === 'ready'
          ? t('setup.status.signInNeeded')
          : t('setup.status.installed')
  const color = !installed ? 'bg-fg-ghost text-fg-muted' : connected ? 'bg-accent/10 text-accent' : 'bg-warn/10 text-warn-fg'
  return <span className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${color}`}>{label}</span>
}

function EmptyState() {
  const { t } = useTranslation()
  return <div className="surface rounded-2xl p-12 text-center"><p className="font-medium text-fg">{t('setup.noOnlineMachines')}</p><p className="mt-2 text-sm text-fg-subtle">{t('setup.noOnlineMachinesHint')}</p></div>
}

function SetupSkeleton() {
  return <div className="grid gap-4 lg:grid-cols-2">{[0, 1, 2, 3].map((x) => <div key={x} className="surface rounded-2xl p-6"><div className="skeleton h-5 w-32" /><div className="skeleton mt-4 h-3 w-full" /><div className="skeleton mt-2 h-3 w-2/3" /><div className="skeleton mt-7 h-9 w-28" /></div>)}</div>
}
