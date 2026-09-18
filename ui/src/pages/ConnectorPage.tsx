import { useCallback, useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, formatBytes, timeAgo } from '../api'
import { usePoll } from '../hooks'
import type { Connector, Session } from '../types'
import Terminal from '../components/Terminal'
import FileBrowser from '../components/FileBrowser'
import LaunchSessionModal from '../components/LaunchSessionModal'
import StatusBadge from '../components/StatusBadge'

export default function ConnectorPage() {
  const { id = '' } = useParams()
  const [searchParams] = useSearchParams()
  const [connector, setConnector] = useState<Connector | null>(null)
  const [sessions, setSessions] = useState<Session[]>([])
  const [active, setActive] = useState<string | null>(null)
  const requestedSession = searchParams.get('session')
  const [tab, setTab] = useState<'overview' | 'terminal' | 'files'>(requestedSession ? 'terminal' : 'overview')
  const [showLaunch, setShowLaunch] = useState(false)
  const { t } = useTranslation()

  const load = useCallback(async () => {
    try {
      const connectors = await api.get<Connector[]>('/api/connectors')
      const d = connectors.find((x) => x.id === id) ?? null
      setConnector(d)
      if (d?.online) {
        const s = await api.get<Session[]>(`/api/connectors/${id}/sessions`)
        setSessions(s)
        setActive((a) => a ?? requestedSession ?? s[0]?.name ?? null)
      }
    } catch {
      /* transient */
    }
  }, [id, requestedSession])

  usePoll(load, 10000)

  // Reset when navigating between connectors.
  useEffect(() => {
    setActive(null)
    setSessions([])
    setConnector(null)
    setTab(requestedSession ? 'terminal' : 'overview')
  }, [id, requestedSession])

  const newQuickTerminal = () => {
    // tmux new-session -A on the agent side creates it on first attach.
    const base = 'term'
    let n = 1
    while (sessions.some((s) => s.name === `${base}-${n}`)) n++
    const name = `${base}-${n}`
    setSessions((ss) => [
      ...ss,
      {
        name,
        kind: 'shell',
        status: 'idle',
        createdAt: Date.now() / 1000,
        lastActivity: Date.now() / 1000,
        attached: false,
        ephemeral: connector ? !connector.tmux : false,
      },
    ])
    setActive(name)
    setTab('terminal')
  }

  const killSession = async (name: string) => {
    if (!confirm(t('connectors.killConfirm', { name }))) return
    try {
      await api.del(`/api/connectors/${id}/sessions/${encodeURIComponent(name)}`)
    } catch {
      /* it may already be gone */
    }
    setSessions((ss) => ss.filter((s) => s.name !== name))
    setActive((a) => (a === name ? null : a))
    load()
  }

  if (connector === null) {
    return <div className="p-8 text-fg-subtle">{t('common.loading')}</div>
  }

  return (
    <div className="flex h-full min-h-[calc(100dvh-64px)] flex-col">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line-2 px-4 py-4 sm:px-7">
        <div className="flex items-center gap-3">
          <Link to="/" className="text-fg-faint hover:text-fg" aria-label={t('connectors.backToFleet')}>
            ←
          </Link>
          <span
            className={`h-2.5 w-2.5 rounded-full ${connector.online ? 'bg-ok' : 'bg-fg-faint'}`}
          />
          <h1 className="text-lg font-semibold tracking-tight text-fg-strong">{connector.name}</h1>
          <span className="font-mono text-xs text-fg-faint">
            {connector.os}/{connector.arch}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <TabButton active={tab === 'overview'} onClick={() => setTab('overview')}>
            {t('connectors.overview')}
          </TabButton>
          <TabButton active={tab === 'terminal'} onClick={() => setTab('terminal')}>
            {t('connectors.terminals')}
          </TabButton>
          <TabButton active={tab === 'files'} onClick={() => setTab('files')}>
            {t('connectors.files')}
          </TabButton>
        </div>
      </header>

      {!connector.online ? (
        <div className="flex flex-1 items-center justify-center text-fg-subtle">
          {t('connectors.offline')}
        </div>
      ) : tab === 'overview' ? (
        <ConnectorOverview connector={connector} onTerminal={() => setTab('terminal')} onFiles={() => setTab('files')} />
      ) : tab === 'files' ? (
        <FileBrowser connectorId={id} />
      ) : (
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="flex items-center gap-1 overflow-x-auto border-b border-line-2 bg-fill-1 px-3 py-2">
            {sessions.map((s) => (
              <div
                key={s.name}
                className={`group flex shrink-0 cursor-pointer items-center gap-2 rounded-lg px-3 py-1.5 text-sm transition ${
                  active === s.name
                    ? 'bg-fill-4 text-fg-strong'
                    : 'text-fg-subtle hover:bg-fill-3'
                }`}
                onClick={() => setActive(s.name)}
              >
                <StatusBadge status={s.status} kind={s.kind} />
                <span className="font-mono text-[13px]">{s.name}</span>
                {s.ephemeral && (
                  <span title={t('connectors.noTmux')}>
                    ⚡
                  </span>
                )}
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    killSession(s.name)
                  }}
                  className="hidden rounded p-0.5 text-fg-subtle hover:text-fail-fg group-hover:block"
                  title={t('connectors.killSession')}
                >
                  ✕
                </button>
              </div>
            ))}
            <button
              onClick={newQuickTerminal}
              className="shrink-0 rounded-lg px-3 py-1.5 text-sm text-fg-subtle hover:bg-fill-3 hover:text-fg"
              title={t('connectors.newTerminal')}
            >
              {t('connectors.addTerminal')}
            </button>
            <button
              onClick={() => setShowLaunch(true)}
              className="shrink-0 rounded-lg px-3 py-1.5 text-sm text-accent hover:bg-accent/10"
              title={t('connectors.launchAgentTitle')}
            >
              {t('connectors.launchAgent')}
            </button>
          </div>
          <div className="min-h-0 flex-1">
            {active ? (
              <Terminal
                key={`${id}:${active}`}
                connectorId={id}
                session={active}
                onExit={() => load()}
              />
            ) : (
              <div className="flex h-full items-center justify-center text-fg-subtle">
                {sessions.length === 0
                  ? t('connectors.noSessions')
                  : t('connectors.pickSession')}
              </div>
            )}
          </div>
        </div>
      )}

      {showLaunch && connector && (
        <LaunchSessionModal
          connectors={[connector]}
          onLaunched={(_, name) => {
            setShowLaunch(false)
            load().then(() => {
              setActive(name)
              setTab('terminal')
            })
          }}
          onClose={() => setShowLaunch(false)}
        />
      )}
    </div>
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-lg px-3 py-1.5 text-sm font-medium transition ${
        active
          ? 'bg-fill-4 text-fg-strong'
          : 'text-fg-subtle hover:bg-fill-3 hover:text-fg'
      }`}
    >
      {children}
    </button>
  )
}

function ConnectorOverview({ connector, onTerminal, onFiles }: { connector: Connector; onTerminal: () => void; onFiles: () => void }) {
  const { t } = useTranslation()
  const s = connector.stats
  const pct = (used = 0, total = 0) => total ? (used / total) * 100 : 0
  return (
    <div className="page-shell flex-1">
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="eyebrow mb-2">{t('connectors.machineHealth')}</p>
          <h2 className="text-2xl font-semibold tracking-[-0.035em] text-fg-strong">{connector.platform || connector.os} {connector.platformVersion}</h2>
          <p className="mt-2 font-mono text-xs text-fg-faint">{connector.hostname} · {connector.arch} · {t('connectors.agentVersion', { version: connector.agentVersion || t('connectors.unknown') })}</p>
        </div>
        <div className="flex gap-2"><button onClick={onTerminal} className="btn-primary">{t('connectors.openTerminal')}</button><button onClick={onFiles} className="btn-secondary">{t('connectors.browseFiles')}</button></div>
      </div>
      {s ? (
        <>
          <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <HealthMetric label="CPU" value={`${Math.round(s.cpuPercent)}%`} detail={`${s.cpuCores || '—'} ${t('connectors.logicalCores')}`} pct={s.cpuPercent} />
            <HealthMetric label={t('connectors.memory')} value={formatBytes(s.memUsed)} detail={`${formatBytes(s.memTotal)} ${t('connectors.total')}`} pct={pct(s.memUsed, s.memTotal)} />
            <HealthMetric label={t('connectors.disk')} value={formatBytes(s.diskTotal - s.diskUsed)} detail={t('connectors.available')} pct={pct(s.diskUsed, s.diskTotal)} />
            <HealthMetric label={t('connectors.load')} value={s.load1 ? s.load1.toFixed(2) : '—'} detail={`${s.load5?.toFixed(2) || '—'} / ${s.load15?.toFixed(2) || '—'} ${t('connectors.overTime')}`} />
          </section>
          <section className="surface mt-4 grid grid-cols-2 overflow-hidden rounded-2xl sm:grid-cols-4">
            <Detail label={t('connectors.networkReceived')} value={formatBytes(s.netRxBytes)} />
            <Detail label={t('connectors.networkSent')} value={formatBytes(s.netTxBytes)} />
            <Detail label={t('connectors.processes')} value={String(s.processCount || '—')} />
            <Detail label={t('connectors.lastContact')} value={timeAgo(connector.lastSeen)} />
          </section>
          <section className="mt-6 grid gap-4 lg:grid-cols-2">
            <div className="surface rounded-2xl p-5"><p className="eyebrow">{t('connectors.terminalContinuity')}</p><p className="mt-3 text-lg font-semibold text-fg-strong">{connector.tmux ? t('connectors.reconnectableReady') : connector.os === 'windows' ? t('connectors.windowsSessionsLive') : t('connectors.installTmux')}</p><p className="mt-2 text-sm leading-6 text-fg-subtle">{connector.tmux ? t('connectors.continuityOn') : t('connectors.continuityOff')}</p></div>
            <div className="surface rounded-2xl p-5"><p className="eyebrow">{t('connectors.kernel')}</p><p className="mt-3 break-words font-mono text-sm text-fg-soft">{connector.kernelVersion || t('connectors.kernelNotReported')}</p><p className="mt-2 text-sm text-fg-faint">{t('connectors.architecture')} {connector.arch}</p></div>
          </section>
        </>
      ) : <div className="surface rounded-2xl p-10 text-center text-sm text-fg-subtle">{t('connectors.waitingHealth')}</div>}
    </div>
  )
}

function HealthMetric({ label, value, detail, pct }: { label: string; value: string; detail: string; pct?: number }) {
  return <article className="surface rounded-2xl p-5"><p className="text-[11px] font-medium text-fg-faint">{label}</p><p className="data-number mt-3 text-3xl font-medium tracking-[-0.05em] text-fg-strong">{value}</p><p className="mt-1 text-xs text-fg-faint">{detail}</p>{pct !== undefined && <div className="mt-5 h-1 bg-fill-4"><div className="h-full bg-accent" style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} /></div>}</article>
}

function Detail({ label, value }: { label: string; value: string }) {
  return <div className="border-b border-r border-line-2 p-5 last:border-r-0 sm:border-b-0"><p className="text-[10px] font-medium uppercase tracking-wider text-fg-ghost">{label}</p><p className="data-number mt-2 text-sm text-fg-soft">{value}</p></div>
}
