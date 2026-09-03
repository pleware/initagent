import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import type { Device, TaskView } from '../types'

export default function TasksPage() {
  const { t } = useTranslation()
  const [devices, setDevices] = useState<Device[]>([])
  const [command, setCommand] = useState('')
  const [deviceId, setDeviceId] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<TaskView | null>(null)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      setDevices(await api.get<Device[]>('/api/devices'))
    } catch {
      /* preserve the last snapshot during a short disconnect */
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const online = devices.filter((d) => d.online)

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!command.trim() || submitting) return
    setSubmitting(true)
    setError('')
    setResult(null)
    try {
      const body: { command: string; deviceId?: string } = { command: command.trim() }
      if (deviceId) body.deviceId = deviceId
      setResult(await api.post<TaskView>('/api/tasks', body))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
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
        <h1 className="text-3xl font-semibold tracking-[-0.045em] text-white sm:text-4xl">{t('tasks.title')}</h1>
        <p className="mt-3 text-sm text-zinc-500">{t('tasks.subtitle')}</p>
      </header>

      <form onSubmit={submit} className="surface rounded-2xl p-5">
        <label htmlFor="task-command" className="text-[11px] font-medium text-zinc-500">{t('tasks.commandLabel')}</label>
        <textarea
          id="task-command"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          rows={3}
          placeholder={t('tasks.commandPlaceholder')}
          className="mt-2 w-full rounded-lg border border-white/[0.08] bg-black/30 px-3 py-2 font-mono text-sm text-white placeholder:text-zinc-600 focus:border-blue-500/60 focus:outline-none"
        />
        <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-center">
          <label className="flex items-center gap-2 text-[11px] font-medium text-zinc-500">
            <span>{t('tasks.deviceLabel')}</span>
            <select
              value={deviceId}
              onChange={(e) => setDeviceId(e.target.value)}
              className="rounded-lg border border-white/[0.08] bg-white/[0.025] px-2 py-1.5 text-sm text-white focus:outline-none"
            >
              <option value="">{t('tasks.anyDevice')}</option>
              {online.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
          </label>
          <button type="submit" disabled={submitting || !command.trim()} className="btn-primary sm:ml-auto">
            {submitting ? t('tasks.submitting') : t('tasks.submit')}
          </button>
        </div>
        {online.length === 0 && !result && (
          <p className="mt-3 text-xs text-amber-200/70">{t('tasks.noOnlineWorkers')}</p>
        )}
      </form>

      {error && <p className="mt-4 rounded-lg border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200">{error}</p>}

      {result && (
        <section className="surface mt-5 rounded-2xl p-5" aria-label={t('tasks.eyebrow')}>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <span className={`rounded px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider ${done ? 'bg-lime-400/10 text-lime-300' : failed ? 'bg-rose-400/10 text-rose-300' : 'bg-white/[0.06] text-zinc-300'}`}>{result.state}</span>
            <Fact label={t('tasks.taskId')} value={result.id} />
            <Fact label={t('tasks.exitCode')} value={String(result.exitCode)} />
            {result.reason && <Fact label={t('tasks.reason')} value={result.reason} />}
            {result.assignedWorkerId && <Fact label={t('tasks.deviceLabel')} value={result.assignedWorkerId} />}
          </div>
          {result.stdout && <pre className="mt-4 overflow-x-auto rounded-lg bg-black/30 p-3 font-mono text-xs text-zinc-200">{result.stdout}</pre>}
          {result.stderr && <pre className="mt-2 overflow-x-auto rounded-lg bg-black/30 p-3 font-mono text-xs text-rose-300">{result.stderr}</pre>}
        </section>
      )}
    </div>
  )
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span className="text-zinc-600">{label}</span>
      <span className="font-mono text-zinc-300">{value}</span>
    </span>
  )
}
