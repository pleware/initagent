import { FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import type { Device, Me, Project } from '../types'
import ProjectModal from './ProjectModal'

// Boarding is the empty-state funnel after hosted signup or a self-host
// claim (26). Name the company if it is still the generic default, then
// create the first project. Skip-ahead is allowed; leaving the name as
// default just asks again next time. The hosted platform operator is
// not this funnel — they keep the inherited empty workspace.

export function isHostedOperator(me: Me): boolean {
  return me.platformAdmin === true && me.offering === 'hosted'
}

export function orgNeedsName(me: Me): boolean {
  const name = me.orgs?.[0]?.name
  const unnamed = me.defaultOrgName || 'default'
  return Boolean(name && name === unnamed)
}

export default function Boarding({
  me,
  devices,
  onMeChanged,
  onProjectCreated,
}: {
  me: Me
  devices: Device[]
  onMeChanged: () => Promise<void> | void
  onProjectCreated: (project: Project) => void
}) {
  const { t } = useTranslation()
  const org = me.orgs?.[0]
  const [step, setStep] = useState<'name' | 'project'>(orgNeedsName(me) ? 'name' : 'project')
  const [orgName, setOrgName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [showModal, setShowModal] = useState(false)

  const saveName = async (event: FormEvent) => {
    event.preventDefault()
    if (!org) return
    const trimmed = orgName.trim()
    if (trimmed === '') {
      setStep('project')
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.patch(`/api/orgs/${org.orgId}`, { name: trimmed })
      await onMeChanged()
      setStep('project')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t('errors.generic'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="code-empty">
      <div className="code-empty-mark"><span>ia</span></div>
      <p className="eyebrow mt-7">{t('boarding.eyebrow')}</p>
      {step === 'name' ? (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-white">
            {t('boarding.nameTitle')}
          </h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-zinc-500">
            {t('boarding.nameHint')}
          </p>
          <form onSubmit={saveName} className="mt-6 w-full max-w-sm space-y-3">
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
            {error && <p className="text-sm text-rose-400">{error}</p>}
            <button type="submit" disabled={busy} className="btn-primary w-full">
              {busy ? t('common.loading') : t('boarding.continue')}
            </button>
            <button
              type="button"
              className="w-full text-sm text-zinc-400 underline-offset-2 hover:text-zinc-200 hover:underline"
              onClick={() => setStep('project')}
            >
              {t('boarding.skipName')}
            </button>
          </form>
        </>
      ) : (
        <>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em] text-white">
            {t('boarding.projectTitle')}
          </h1>
          <p className="mt-3 max-w-lg text-center text-sm leading-6 text-zinc-500">
            {t('boarding.projectHint')}
          </p>
          <button onClick={() => setShowModal(true)} className="btn-primary mt-6">
            {t('boarding.addProject')}
          </button>
          <p className="mt-4 max-w-sm text-center text-xs leading-5 text-zinc-600">
            {t('boarding.skipProject')}
          </p>
        </>
      )}
      {showModal && (
        <ProjectModal
          devices={devices}
          onClose={() => setShowModal(false)}
          onSaved={(saved) => {
            setShowModal(false)
            onProjectCreated(saved)
          }}
        />
      )}
    </div>
  )
}
