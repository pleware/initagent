import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { api, timeAgo } from '../api'
import { useHubEvents, usePoll } from '../hooks'
import type { Connector, FleetSession } from '../types'
import LaunchSessionModal from '../components/LaunchSessionModal'
import StatusBadge from '../components/StatusBadge'

// AgentsPage is the fleet-wide view: every session on every machine, with
// coding agents front and center.
export default function AgentsPage() {
  const [sessions, setSessions] = useState<FleetSession[] | null>(null)
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [showLaunch, setShowLaunch] = useState(false)
  const [showAll, setShowAll] = useState(false)
  const navigate = useNavigate()
  const { t } = useTranslation()

  const load = useCallback(async () => {
    try {
      const [s, d] = await Promise.all([
        api.get<FleetSession[]>('/api/agents'),
        api.get<Connector[]>('/api/connectors'),
      ])
      setSessions(s)
      setConnectors(d)
    } catch {
      /* transient */
    }
  }, [])

  usePoll(load, 8000)
  useHubEvents((e) => {
    if (e.type === 'sessions.changed') load()
  })

  const agents = (sessions ?? []).filter(
    (s) => showAll || (s.kind && s.kind !== 'shell'),
  )

  return (
    <div className="page-shell">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <p className="eyebrow mb-3">{t('agents.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">{t('agents.title')}</h1>
          <p className="mt-1 text-sm text-fg-muted">
            {t('agents.subtitle')}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <label className="flex cursor-pointer items-center gap-2 text-sm text-fg-muted">
            <input
              type="checkbox"
              checked={showAll}
              onChange={(e) => setShowAll(e.target.checked)}
              className="accent-accent"
            />
            {t('agents.showTerminals')}
          </label>
          <button
            onClick={() => setShowLaunch(true)}
            className="btn-primary"
          >
            ▸ {t('agents.launch')}
          </button>
        </div>
      </div>

      {sessions === null ? (
        <p className="text-fg-subtle">{t('common.loading')}</p>
      ) : agents.length === 0 ? (
        <div className="surface rounded-2xl p-12 text-center">
          <p className="mb-2 text-fg-soft">{t('agents.empty')}</p>
          <p className="text-sm text-fg-subtle">
            {t('agents.emptyHint')}
          </p>
        </div>
      ) : (
        <div className="surface overflow-hidden rounded-2xl">
          <table className="w-full text-sm">
            <thead className="bg-canvas text-left text-xs text-fg-subtle">
              <tr>
                <th className="px-4 py-2.5 font-medium">{t('agents.session')}</th>
                <th className="px-4 py-2.5 font-medium">{t('agents.connector')}</th>
                <th className="px-4 py-2.5 font-medium">{t('agents.kind')}</th>
                <th className="px-4 py-2.5 font-medium">{t('agents.status')}</th>
                <th className="px-4 py-2.5 font-medium">{t('agents.lastActivity')}</th>
                <th className="w-24 px-4 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {agents.map((s) => (
                <tr
                  key={`${s.connectorId}:${s.name}`}
                  className="cursor-pointer border-t border-line-1 transition hover:bg-fill-1"
                  onClick={() => navigate(`/connectors/${s.connectorId}`)}
                >
                  <td className="px-4 py-3 font-mono text-[13px] text-fg">
                    {s.name}
                  </td>
                  <td className="px-4 py-3 text-fg-soft">{s.connectorName}</td>
                  <td className="px-4 py-3 text-fg-muted">
                    {s.kind || t('agents.terminal')}
                  </td>
                  <td className="px-4 py-3">
                    <span className="flex items-center gap-2">
                      <StatusBadge status={s.status} kind={s.kind} />
                      <span
                        className={
                          s.status === 'working'
                            ? 'text-ok'
                            : 'text-fg-muted'
                        }
                      >
                        {s.status}
                      </span>
                    </span>
                  </td>
                  <td className="px-4 py-3 text-fg-subtle">
                    {timeAgo(s.lastActivity)}
                  </td>
                  <td className="px-4 py-3 text-right text-xs text-accent">
                    {t('agents.open')} →
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showLaunch && (
        <LaunchSessionModal
          connectors={connectors}
          onLaunched={(connectorId) => {
            setShowLaunch(false)
            navigate(`/connectors/${connectorId}`)
          }}
          onClose={() => setShowLaunch(false)}
        />
      )}
    </div>
  )
}
