import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import BigFiveFields from './BigFiveFields'
import VoiceSelect, { inheritVoice } from './VoiceSelect'
import type { Character, Staff } from '../types'

// OrgStaffForm writes this org's override of one staff member: only the
// overridable fields travel, and every org keeps tuning its own copy — the
// canonical row stays the installation's. Inherited fields render read-only
// so the boundary between "the hub's" and "ours" stays visible in the form.
export default function OrgStaffForm({
  staff,
  orgId,
  onClose,
  onSaved,
}: {
  staff: Staff
  orgId: string
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(staff.name)
  const [age, setAge] = useState(staff.age > 0 ? String(staff.age) : '')
  const [soulOverride, setSoulOverride] = useState(staff.soulOverride ?? '')
  const [voice, setVoice] = useState(staff.voice)
  const [bigFive, setBigFive] = useState<Character>(staff.bigFive)
  const [brief, setBrief] = useState(staff.brief)
  const [avatarModel3d, setAvatarModel3d] = useState(staff.avatarModel3d)
  const [wordBudget, setWordBudget] = useState(
    staff.wordBudget > 0 ? String(staff.wordBudget) : '',
  )
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    // Mirror the hub's checks before anything travels: the name keeps at
    // least two characters after trimming, and age is never negative.
    const nextName = name.trim()
    if (nextName.length < 2) {
      setError(t('validation.minLength', { min: 2 }))
      return
    }
    const nextAge = Number(age) || 0
    if (nextAge < 0) {
      setError(t('staff.ageInvalid'))
      return
    }
    // The override body carries only the fields the user actually changed:
    // an absent field means "inherit the hub's value", and echoing the
    // untouched fields back as concrete values would overwrite the canonical
    // row for this org.
    const payload: {
      name?: string
      age?: number
      soulOverride?: string
      voice?: string
      bigFive?: Character
      brief?: string
      avatarModel3d?: string
      wordBudget?: number
    } = {}
    if (nextName !== staff.name) payload.name = nextName
    if (nextAge !== staff.age) payload.age = nextAge
    const nextSoulOverride = soulOverride.trim()
    if (nextSoulOverride !== (staff.soulOverride ?? '')) payload.soulOverride = nextSoulOverride
    const nextVoice = voice.trim()
    if (nextVoice !== inheritVoice && nextVoice !== staff.voice) payload.voice = nextVoice
    if (!sameBigFive(bigFive, staff.bigFive)) payload.bigFive = bigFive
    const nextBrief = brief.trim()
    if (nextBrief !== staff.brief) payload.brief = nextBrief
    const nextAvatarModel3d = avatarModel3d.trim()
    if (nextAvatarModel3d !== staff.avatarModel3d) payload.avatarModel3d = nextAvatarModel3d
    const nextWordBudget = Number(wordBudget) || 0
    if (nextWordBudget !== staff.wordBudget) payload.wordBudget = nextWordBudget

    if (Object.keys(payload).length === 0) {
      onSaved()
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.patch(`/api/orgs/${orgId}/staff/${staff.id}`, payload)
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('team.overrideFailed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-4">
      {error && (
        <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <div className="rounded-lg border border-line-2 p-4">
        <p className="text-xs text-fg-subtle">{t('staff.inherited')}</p>
        <dl className="mt-2 grid grid-cols-3 gap-3 text-sm">
          <div>
            <dt className="text-xs text-fg-subtle">{t('staff.locale')}</dt>
            <dd className="mt-0.5 text-fg">{staff.locale || '—'}</dd>
          </div>
          <div className="col-span-2">
            <dt className="text-xs text-fg-subtle">{t('staff.soulCore')}</dt>
            <dd className="mt-0.5 whitespace-pre-wrap text-fg">
              {staff.soulCore || '—'}
            </dd>
          </div>
        </dl>
      </div>

      <p className="text-xs text-fg-subtle">{t('team.overrideHint')}</p>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="block">
          <span className="field-label">{t('staff.name')}</span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="field-input mt-2"
          />
        </label>
        <label className="block">
          <span className="field-label">{t('staff.age')}</span>
          <input
            type="number"
            min={0}
            value={age}
            onChange={(e) => setAge(e.target.value)}
            className="field-input mt-2"
          />
        </label>
      </div>

      <label className="block">
        <span className="field-label">{t('staff.soulOverride')}</span>
        <textarea
          rows={3}
          value={soulOverride}
          onChange={(e) => setSoulOverride(e.target.value)}
          className="field-input mt-2"
        />
      </label>

      <label className="block">
        <span className="field-label">{t('staff.voice')}</span>
        <VoiceSelect value={voice} onChange={setVoice} override />
      </label>

      <section className="rounded-lg border border-line-2 p-4">
        <h3 className="text-sm font-medium text-fg">{t('staff.bigFive')}</h3>
        <p className="mt-1 text-xs text-fg-subtle">{t('staff.bigFiveHint')}</p>
        <div className="mt-3">
          <BigFiveFields value={bigFive} onChange={setBigFive} />
        </div>
      </section>

      <label className="text-sm text-fg-soft">
        {t('staff.brief')}
        <textarea
          rows={3}
          value={brief}
          onChange={(e) => setBrief(e.target.value)}
          className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
        />
      </label>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm text-fg-soft">
          {t('staff.wordBudget')}
          <input
            type="number"
            min={0}
            value={wordBudget}
            onChange={(e) => setWordBudget(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
          />
        </label>
        <label className="text-sm text-fg-soft">
          {t('staff.avatarModel3d')}
          <input
            type="text"
            value={avatarModel3d}
            onChange={(e) => setAvatarModel3d(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 font-mono text-[12px] text-fg-strong"
          />
        </label>
      </div>

      <div className="mt-2 flex items-center justify-end gap-3">
        <button type="button" onClick={onClose} className="btn-secondary">
          {t('common.cancel')}
        </button>
        <button type="submit" disabled={busy} className="btn-primary">
          {busy ? t('common.loading') : t('common.save')}
        </button>
      </div>
    </form>
  )
}

function sameBigFive(a: Character, b: Character): boolean {
  return (
    a.openness === b.openness &&
    a.conscientiousness === b.conscientiousness &&
    a.extraversion === b.extraversion &&
    a.agreeableness === b.agreeableness &&
    a.neuroticism === b.neuroticism
  )
}
