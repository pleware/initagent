import { FormEvent, useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { usePoll } from '../hooks'
import type { Device, Me, Project, ProjectTemplate, TaskView } from '../types'
import { HubError } from './PlanWall'

// Boarding is the empty-state funnel after hosted signup or a self-host
// claim (26). Name the company if it is still the generic default, pick a
// shipped template, create the first project, bind a repo when the contract
// is change, enroll a worker, then run one task. Skip-ahead is allowed at
// each step. The hosted platform operator is not this funnel.

type Step = 'name' | 'template' | 'project' | 'repo' | 'worker' | 'task'

export function isHostedOperator(me: Me): boolean {
  return me.platformAdmin === true && me.offering === 'hosted'
}

export function orgNeedsName(me: Me): boolean {
  const name = me.orgs?.[0]?.name
  const unnamed = me.defaultOrgName || 'default'
  return Boolean(name && name === unnamed)
}

const firstTaskCommand = 'echo hello from initagent'

export default function Boarding({
  me,
  devices,
  onMeChanged,
  onFinished,
}: {
  me: Me
  devices: Device[]
  onMeChanged: () => Promise<void> | void
  onFinished: (project: Project) => void
}) {
  const { t } = useTranslation()
  const org = me.orgs?.[0]
  const [step, setStep] = useState<Step>(orgNeedsName(me) ? 'name' : 'template')
  const [orgName, setOrgName] = useState('')
  const [templates, setTemplates] = useState<ProjectTemplate[]>([])
  const [templateId, setTemplateId] = useState('software')
  const [projectName, setProjectName] = useState('')
  const [project, setProject] = useState<Project | null>(null)
  const [repoRemote, setRepoRemote] = useState('')
  const [fleet, setFleet] = useState<Device[]>(devices)
  const [command, setCommand] = useState('')
  const [windowsCommand, setWindowsCommand] = useState('')
  const [platform, setPlatform] = useState<'unix' | 'windows'>('unix')
  const [copied, setCopied] = useState(false)
  const [task, setTask] = useState<TaskView | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)

  const selected = templates.find((item) => item.id === templateId) ?? templates.find((item) => item.live)

  const loadFleet = useCallback(async () => {
    try {
      setFleet(await api.get<Device[]>('/api/devices'))
    } catch {
      /* keep the last snapshot */
    }
  }, [])

  usePoll(loadFleet, step === 'worker' || step === 'task' ? 4000 : 30_000)

  useEffect(() => {
    setFleet(devices)
  }, [devices])

  useEffect(() => {
    api.get<ProjectTemplate[]>('/api/templates').then((list) => {
      setTemplates(list)
      const live = list.find((item) => item.live)
      if (live) setTemplateId(live.id)
    }).catch((cause) => {
      setError(cause)
    })
  }, [t])

  useEffect(() => {
    if (step !== 'worker') return
    api
      .post<{ command: string; windowsCommand: string }>('/api/enroll-tokens')
      .then((offer) => {
        setCommand(offer.command)
        setWindowsCommand(offer.windowsCommand)
      })
      .catch((cause) => {
        setError(cause)
      })
  }, [step, t])

  const online = fleet.filter((device) => device.online)
  const joined = online.length > 0

  const afterProject = (saved: Project) => {
    setProject(saved)
    const tmpl = templates.find((item) => item.id === saved.templateId) ?? selected
    if (tmpl?.needsRepo) {
      setStep('repo')
      return
    }
    setStep(joined ? 'task' : 'worker')
  }

  const saveName = async (event: FormEvent) => {
    event.preventDefault()
    if (!org) return
    const trimmed = orgName.trim()
    if (trimmed === '') {
      setStep('template')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await api.patch(`/api/orgs/${org.orgId}`, { name: trimmed })
      await onMeChanged()
      setStep('template')
    } catch (cause) {
      setError(cause)
    } finally {
      setBusy(false)
    }
  }

  const createProject = async (event: FormEvent) => {
    event.preventDefault()
    if (!selected?.live || !projectName.trim()) return
    setBusy(true)
    setError(null)
    try {
      const saved = await api.post<Project>('/api/projects', {
        name: projectName.trim(),
        templateId: selected.id,
      })
      window.dispatchEvent(new Event('liveagent:projects-changed'))
      afterProject(saved)
    } catch (cause) {
      setError(cause)
    } finally {
      setBusy(false)
    }
  }

  const saveRepo = async (event: FormEvent) => {
    event.preventDefault()
    if (!project || !repoRemote.trim()) return
    setBusy(true)
    setError(null)
    try {
      const saved = await api.patch<Project>(`/api/projects/${project.id}`, {
        repoRemote: repoRemote.trim(),
      })
      setProject(saved)
      setStep(joined ? 'task' : 'worker')
    } catch (cause) {
      setError(cause)
    } finally {
      setBusy(false)
    }
  }

  const runFirstTask = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(null)
    setTask(null)
    try {
      setTask(await api.post<TaskView>('/api/tasks', { command: firstTaskCommand }))
    } catch (cause) {
      setError(cause)
    } finally {
      setBusy(false)
    }
  }

  const finish = () => {
    if (project) onFinished(project)
  }

  const skipRepo = () => setStep(joined ? 'task' : 'worker')
  const skipWorker = () => setStep('task')

  const copy = async () => {
    const text = platform === 'windows' ? windowsCommand : command
    if (!text) return
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="code-empty">
      <div className="code-empty-mark"><span>ia</span></div>
      <p className="eyebrow mt-7">{t('boarding.eyebrow')}</p>
      {step === 'name' && (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-fg">{t('boarding.nameTitle')}</h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-fg-muted">{t('boarding.nameHint')}</p>
          <form onSubmit={saveName} className="mt-6 w-full max-w-sm flex flex-col gap-3">
            <label className="block text-left">
              <span className="field-label">{t('boarding.organization')}</span>
              <input
                value={orgName}
                onChange={(e) => setOrgName(e.target.value)}
                autoFocus
                maxLength={120}
                placeholder={me.defaultOrgName || 'default'}
                className="field-input mt-2"
              />
            </label>
            {error ? <HubError error={error} fallback={t('errors.generic')} /> : null}
            <button type="submit" disabled={busy} className="btn-primary w-full">
              {busy ? t('common.loading') : t('boarding.continue')}
            </button>
            <button type="button" className="w-full text-sm text-fg-muted underline-offset-2 hover:text-fg hover:underline" onClick={() => { setError(null); setStep('template') }}>
              {t('boarding.skipName')}
            </button>
          </form>
        </>
      )}
      {step === 'template' && (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-fg">{t('boarding.templateTitle')}</h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-fg-muted">{t('boarding.templateHint')}</p>
          <div className="mt-6 grid w-full max-w-lg grid-cols-1 gap-2 sm:grid-cols-2" role="radiogroup" aria-label={t('boarding.templateTitle')}>
            {templates.map((tmpl) => {
              const selectedNow = templateId === tmpl.id
              return (
                <button
                  key={tmpl.id}
                  type="button"
                  role="radio"
                  aria-checked={selectedNow}
                  disabled={!tmpl.live}
                  onClick={() => tmpl.live && setTemplateId(tmpl.id)}
                  className={`rounded-xl border px-4 py-3 text-left ${
                    tmpl.live
                      ? selectedNow
                        ? 'border-accent/40 bg-accent/10 text-fg'
                        : 'border-line-2 bg-fill-2 text-fg-soft hover:border-line-3'
                      : 'cursor-not-allowed border-line-1 bg-fill-1 text-fg-ghost'
                  }`}
                >
                  <span className="block text-sm font-semibold">{tmpl.label}</span>
                  {!tmpl.live && <span className="mt-1 block text-[11px] uppercase tracking-wider">{t('boarding.comingSoon')}</span>}
                </button>
              )
            })}
          </div>
          {error ? <HubError error={error} fallback={t('errors.generic')} /> : null}
          <button
            type="button"
            className="btn-primary mt-6"
            disabled={!selected?.live}
            onClick={() => { setError(null); setStep('project') }}
          >
            {t('boarding.continue')}
          </button>
        </>
      )}
      {step === 'project' && (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-fg">{t('boarding.projectTitle')}</h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-fg-muted">{t('boarding.projectHint')}</p>
          <form onSubmit={createProject} className="mt-6 w-full max-w-sm flex flex-col gap-3">
            <label className="block text-left">
              <span className="field-label">{t('boarding.projectName')}</span>
              <input
                value={projectName}
                onChange={(e) => setProjectName(e.target.value)}
                autoFocus
                maxLength={80}
                placeholder={t('boarding.projectPlaceholder')}
                className="field-input mt-2"
              />
            </label>
            {error ? <HubError error={error} fallback={t('errors.generic')} /> : null}
            <button type="submit" disabled={busy || !projectName.trim()} className="btn-primary w-full">
              {busy ? t('common.loading') : t('boarding.createProject')}
            </button>
            <p className="text-center text-xs leading-5 text-fg-ghost">{t('boarding.skipProject')}</p>
          </form>
        </>
      )}
      {step === 'repo' && (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-fg">{t('boarding.repoTitle')}</h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-fg-muted">{t('boarding.repoHint')}</p>
          <form onSubmit={saveRepo} className="mt-6 w-full max-w-sm flex flex-col gap-3">
            <label className="block text-left">
              <span className="field-label">{t('boarding.repoRemote')}</span>
              <input
                value={repoRemote}
                onChange={(e) => setRepoRemote(e.target.value)}
                autoFocus
                placeholder={t('boarding.repoPlaceholder')}
                className="field-input mt-2 font-mono text-xs"
              />
            </label>
            {error ? <HubError error={error} fallback={t('errors.generic')} /> : null}
            <button type="submit" disabled={busy || !repoRemote.trim()} className="btn-primary w-full">
              {busy ? t('common.loading') : t('boarding.continue')}
            </button>
            <button type="button" className="w-full text-sm text-fg-muted underline-offset-2 hover:text-fg hover:underline" onClick={skipRepo}>
              {t('boarding.skipRepo')}
            </button>
          </form>
        </>
      )}
      {step === 'worker' && (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-fg">{t('boarding.workerTitle')}</h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-fg-muted">{t('boarding.workerHint')}</p>
          <div className="mt-6 w-full max-w-xl">
            {error ? <HubError error={error} fallback={t('errors.generic')} /> : null}
            <div className="mb-3 inline-flex rounded-lg border border-line-2 bg-fill-sunken p-1">
              <button type="button" onClick={() => setPlatform('unix')} className={`rounded-md px-3 py-1.5 text-sm ${platform === 'unix' ? 'bg-fill-4 text-fg' : 'text-fg-muted hover:text-fg'}`}>
                {t('boarding.unix')}
              </button>
              <button type="button" onClick={() => setPlatform('windows')} className={`rounded-md px-3 py-1.5 text-sm ${platform === 'windows' ? 'bg-fill-4 text-fg' : 'text-fg-muted hover:text-fg'}`}>
                {t('boarding.windows')}
              </button>
            </div>
            <div className="flex items-stretch gap-2">
              <code className="flex-1 overflow-x-auto whitespace-nowrap rounded-lg border border-line-2 bg-fill-sunken p-3 font-mono text-[13px] text-ok-fg">
                {(platform === 'windows' ? windowsCommand : command) || t('common.loading')}
              </code>
              <button type="button" onClick={copy} disabled={!(platform === 'windows' ? windowsCommand : command)} className="btn-secondary shrink-0">
                {copied ? t('boarding.copied') : t('boarding.copy')}
              </button>
            </div>
            <p className="mt-3 text-xs text-fg-ghost">{t('boarding.enrollExpire')}</p>
            {joined ? (
              <div className="mt-4 flex items-center justify-between rounded-lg border border-ok/30 bg-ok/10 p-3">
                <span className="text-sm font-medium text-ok-fg">{t('boarding.enrollJoined')}</span>
                <button type="button" className="btn-primary" onClick={() => { setError(null); setStep('task') }}>
                  {t('boarding.continue')}
                </button>
              </div>
            ) : (
              <p className="mt-4 flex items-center gap-2 text-sm text-fg-muted">
                <span className="h-2 w-2 animate-pulse rounded-full bg-ok" />
                {t('boarding.enrollWait')}
              </p>
            )}
            <button type="button" className="mt-4 w-full text-sm text-fg-muted underline-offset-2 hover:text-fg hover:underline" onClick={skipWorker}>
              {t('boarding.skipWorker')}
            </button>
          </div>
        </>
      )}
      {step === 'task' && (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-fg">{t('boarding.taskTitle')}</h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-fg-muted">{t('boarding.taskHint')}</p>
          <form onSubmit={runFirstTask} className="mt-6 w-full max-w-sm flex flex-col gap-3">
            <code className="rounded-lg border border-line-2 bg-fill-sunken px-3 py-2 font-mono text-xs text-fg-soft">{firstTaskCommand}</code>
            {error ? <HubError error={error} fallback={t('errors.generic')} /> : null}
            {task && (
              <p className="text-sm text-fg-soft">
                {task.state}
                {task.stdout ? ` — ${task.stdout.trim()}` : ''}
              </p>
            )}
            <button type="submit" disabled={busy || online.length === 0} className="btn-primary w-full">
              {busy ? t('boarding.taskRunning') : t('boarding.runTask')}
            </button>
            <button type="button" className="w-full text-sm text-fg-muted underline-offset-2 hover:text-fg hover:underline" onClick={finish}>
              {project ? t('boarding.done') : t('boarding.skipTask')}
            </button>
            {online.length === 0 && <p className="text-center text-xs text-fg-ghost">{t('tasks.noOnlineWorkers')}</p>}
          </form>
        </>
      )}
    </div>
  )
}
