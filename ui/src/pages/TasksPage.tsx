import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { api, forProject } from '../api'
import type { Connector, TaskLaunch, TaskView } from '../types'

const LAUNCH_MODES: readonly TaskLaunch[] = ['exec', 'process', 'send_keys']

export default function TasksPage() {
  const { t } = useTranslation()
  // Which project this page acts on travels in the URL, so a link to one
  // project's console stays a link. Absent means "the hub's only project",
  // which the hub resolves — the free plan never needs the parameter.
  const [searchParams] = useSearchParams()
  const projectId = searchParams.get('project') ?? undefined
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [command, setCommand] = useState('')
  const [connectorId, setConnectorId] = useState('')
  const [launch, setLaunch] = useState<TaskLaunch>('exec')
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<TaskView | null>(null)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      setConnectors(await api.get<Connector[]>(forProject('/api/connectors', projectId)))
    } catch {
      /* preserve the last snapshot during a short disconnect */
    }
  }, [projectId])

  useEffect(() => {
    load()
  }, [load])

  const online = connectors.filter((d) => d.online)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!command.trim() || submitting) return
    setSubmitting(true)
    setError('')
    setResult(null)
    try {
      const body: { command: string; connectorId?: string; launch: TaskLaunch } = {
        command: command.trim(),
        launch,
      }
      if (connectorId) body.connectorId = connectorId
      setResult(await api.post<TaskView>(forProject('/api/tasks', projectId), body))
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.generic'))
    } finally {
      setSubmitting(false)
    }
  }

  const done = result?.state === 'done'
  const failed = result?.state === 'failed'

  return (
    <div className="page-shell">
      <header className="mb-8">
        <p className="eyebrow mb-3">{t('tasks.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.045em] text-fg-strong sm:text-4xl">{t('tasks.title')}</h1>
        <p className="mt-3 text-sm text-fg-subtle">{t('tasks.subtitle')}</p>
      </header>

      <form onSubmit={submit} className="surface rounded-2xl p-5">
        <label htmlFor="task-command" className="text-[11px] font-medium text-fg-subtle">{t('tasks.commandLabel')}</label>
        <textarea
          id="task-command"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          rows={3}
          placeholder={t('tasks.commandPlaceholder')}
          className="mt-2 w-full rounded-lg border border-line-2 bg-fill-sunken px-3 py-2 font-mono text-sm text-fg placeholder:text-fg-faint focus:border-info/60 focus:outline-none"
        />
        <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
          <label className="flex items-center gap-2 text-[11px] font-medium text-fg-subtle">
            <span>{t('tasks.connectorLabel')}</span>
            <SimpleSelect
              value={connectorId}
              onValueChange={setConnectorId}
              aria-label={t('tasks.connectorLabel')}
              items={[
                { value: '', label: t('tasks.anyConnector') },
                ...online.map((d) => ({ value: d.id, label: d.name })),
              ]}
            />
          </label>
          <label className="flex items-center gap-2 text-[11px] font-medium text-fg-subtle">
            <span>{t('tasks.launchLabel')}</span>
            <SimpleSelect
              value={launch}
              onValueChange={(next) => {
                if (next === 'exec' || next === 'process' || next === 'send_keys') {
                  setLaunch(next)
                }
              }}
              aria-label={t('tasks.launchLabel')}
              items={LAUNCH_MODES.map((mode) => ({
                value: mode,
                label: t(`tasks.launchOption.${mode}`),
              }))}
            />
          </label>
          <button type="submit" disabled={submitting || !command.trim()} className="btn-primary sm:ml-auto">
            {submitting ? t('tasks.submitting') : t('tasks.submit')}
          </button>
        </div>
        <p className="mt-3 text-xs text-fg-subtle">{t(`tasks.launchHint.${launch}`)}</p>
        {online.length === 0 && !result && (
          <p className="mt-3 text-xs text-warn-fg/70">{t('tasks.noOnlineWorkers')}</p>
        )}
      </form>

      {error && <p className="mt-4 rounded-lg border border-fail/20 bg-fail/10 px-4 py-3 text-sm text-fail-fg">{error}</p>}

      {result && (
        <section className="surface mt-5 rounded-2xl p-5" aria-label={t('tasks.eyebrow')}>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <span className={`rounded px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider ${done ? 'bg-accent/10 text-accent' : failed ? 'bg-fail/10 text-fail-fg' : 'bg-fill-4 text-fg-soft'}`}>{result.state}</span>
            <Fact label={t('tasks.taskId')} value={result.id} />
            {result.launch && <Fact label={t('tasks.launch')} value={result.launch} />}
            <Fact label={t('tasks.exitCode')} value={String(result.exitCode)} />
            {result.reason && <Fact label={t('tasks.reason')} value={result.reason} />}
            {result.assignedWorkerId && <Fact label={t('tasks.connectorLabel')} value={result.assignedWorkerId} />}
          </div>
          {result.stdout && <pre className="mt-4 overflow-x-auto rounded-lg bg-fill-sunken p-3 font-mono text-xs text-fg">{result.stdout}</pre>}
          {result.stderr && <pre className="mt-2 overflow-x-auto rounded-lg bg-fill-sunken p-3 font-mono text-xs text-fail-fg">{result.stderr}</pre>}
        </section>
      )}
    </div>
  )
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span className="text-fg-faint">{label}</span>
      <span className="font-mono text-fg-soft">{value}</span>
    </span>
  )
}
