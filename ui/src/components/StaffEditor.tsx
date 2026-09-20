import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import BigFiveFields from './BigFiveFields'
import VoiceSelect from './VoiceSelect'
import LocaleSelect from './LocaleSelect'
import type { Character, Staff } from '../types'

// The nine fields of a canonical staff member. A name in `readonlyFields`
// renders read-only, a name in `disabledFields` renders disabled — one
// union serves both because a field a caller wants to lock is always
// locked by name.
export type StaffField =
  | 'name'
  | 'locale'
  | 'age'
  | 'wordBudget'
  | 'avatarModel3d'
  | 'voice'
  | 'biologicalGender'
  | 'bigFive'
  | 'brief'
  | 'soulCore'

// StaffFields is the editable subset of Staff the form owns. The slug is
// not a form field: on create the caller derives it from the name, on edit
// the stored slug travels untouched — the hub keys the upsert on it, so
// inventing a new one in edit mode would mint a second row instead of
// updating.
export interface StaffFields {
  name: string
  locale: string
  age: number
  wordBudget: number
  avatarModel3d: string
  voice: string
  biologicalGender: string
  bigFive: Character
  brief: string
  soulCore: string
}

// StaffEditor creates or updates one canonical staff member (draft 17's
// admin roster). The parent owns the transport: `submit` receives the
// parsed fields and resolves with the saved Staff, `onSaved` fires after
// it resolves, `onCancel` closes without saving. AdminPage uses it for the
// platform catalogue; the org override screen is its second consumer.
export default function StaffEditor({
  staff,
  readonlyFields = [],
  disabledFields = [],
  submit,
  onSaved,
  onCancel,
}: {
  staff: Staff | null
  readonlyFields?: StaffField[]
  disabledFields?: StaffField[]
  submit: (fields: StaffFields) => Promise<Staff>
  onSaved: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(staff?.name ?? '')
  const [locale, setLocale] = useState(staff?.locale ?? '')
  const [age, setAge] = useState(staff && staff.age > 0 ? String(staff.age) : '')
  const [avatarModel3d, setAvatarModel3d] = useState(staff?.avatarModel3d ?? '')
  const [brief, setBrief] = useState(staff?.brief ?? '')
  const [wordBudget, setWordBudget] = useState(
    staff && staff.wordBudget > 0 ? String(staff.wordBudget) : '',
  )
  const [soulCore, setSoulCore] = useState(staff?.soulCore ?? '')
  const [voice, setVoice] = useState(staff?.voice ?? '')
  const [biologicalGender, setBiologicalGender] = useState(staff?.biologicalGender ?? '')
  const [bigFive, setBigFive] = useState<Character>(staff?.bigFive ?? neutralCharacter())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const lock = (field: StaffField) => ({
    readOnly: readonlyFields.includes(field),
    disabled: disabledFields.includes(field),
  })

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await submit({
        name: name.trim(),
        locale: locale.trim(),
        age: Number(age) || 0,
        wordBudget: Number(wordBudget) || 0,
        avatarModel3d: avatarModel3d.trim(),
        voice: voice.trim(),
        biologicalGender: biologicalGender.trim(),
        bigFive,
        brief: brief.trim(),
        soulCore: soulCore.trim(),
      })
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('staff.saveFailed'))
    } finally {
      setBusy(false)
    }
  }

  // Range inputs have no native read-only; locking the whole trait grid
  // disables it, which is the honest rendering of a read-only composite.
  const bigFiveLocked =
    readonlyFields.includes('bigFive') || disabledFields.includes('bigFive')
  const bigFiveEditor = (
    <div className="mt-3">
      <BigFiveFields value={bigFive} onChange={setBigFive} />
    </div>
  )

  return (
    <form onSubmit={(e) => void handleSubmit(e)} className="flex flex-col gap-4">
      {error && (
        <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm text-fg-soft">
          {t('staff.name')}
          <input
            type="text"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
            {...lock('name')}
          />
        </label>
        <label className="text-sm text-fg-soft">
          {t('staff.locale')}
          <LocaleSelect
            value={locale}
            onChange={setLocale}
            disabled={
              readonlyFields.includes('locale') || disabledFields.includes('locale')
            }
          />
        </label>
        <label className="text-sm text-fg-soft">
          {t('staff.biologicalGender')}
          <select
            value={biologicalGender}
            onChange={(e) => setBiologicalGender(e.target.value)}
            disabled={
              readonlyFields.includes('biologicalGender') ||
              disabledFields.includes('biologicalGender')
            }
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
          >
            <option value="" />
            <option value="male">{t('staff.genderMale')}</option>
            <option value="female">{t('staff.genderFemale')}</option>
          </select>
        </label>
        <label className="text-sm text-fg-soft">
          {t('staff.age')}
          <input
            type="number"
            min={0}
            value={age}
            onChange={(e) => setAge(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
            {...lock('age')}
          />
        </label>
        <label className="text-sm text-fg-soft">
          {t('staff.wordBudget')}
          <input
            type="number"
            min={0}
            value={wordBudget}
            onChange={(e) => setWordBudget(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
            {...lock('wordBudget')}
          />
        </label>
        <label className="text-sm text-fg-soft">
          {t('staff.avatarModel3d')}
          <input
            type="text"
            value={avatarModel3d}
            onChange={(e) => setAvatarModel3d(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 font-mono text-[12px] text-fg-strong"
            {...lock('avatarModel3d')}
          />
        </label>
        <label className="block">
          <span className="field-label">{t('staff.voice')}</span>
          <VoiceSelect
            value={voice}
            onChange={setVoice}
            disabled={
              readonlyFields.includes('voice') || disabledFields.includes('voice')
            }
          />
        </label>
      </div>

      <section className="rounded-lg border border-line-2 p-4">
        <h3 className="text-sm font-medium text-fg">{t('staff.bigFive')}</h3>
        <p className="mt-1 text-xs text-fg-subtle">{t('staff.bigFiveHint')}</p>
        {bigFiveLocked ? <fieldset disabled className="contents">{bigFiveEditor}</fieldset> : bigFiveEditor}
      </section>

      <label className="text-sm text-fg-soft">
        {t('staff.brief')}
        <textarea
          rows={3}
          value={brief}
          onChange={(e) => setBrief(e.target.value)}
          className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
          {...lock('brief')}
        />
      </label>

      <label className="block">
        <span className="field-label">{t('staff.soulCore')}</span>
        <textarea
          rows={3}
          value={soulCore}
          onChange={(e) => setSoulCore(e.target.value)}
          className="field-input mt-2"
          {...lock('soulCore')}
        />
      </label>

      <div className="mt-2 flex items-center justify-end gap-3">
        <button type="button" onClick={onCancel} className="btn-secondary">
          {t('common.cancel')}
        </button>
        <button type="submit" disabled={busy} className="btn-primary">
          {busy ? t('common.loading') : t('common.save')}
        </button>
      </div>
    </form>
  )
}

// neutralCharacter is the OCEAN profile a new staff member starts from:
// every trait at the midpoint, so nothing is assumed and nothing is blank.
export function neutralCharacter(): Character {
  return {
    openness: 0.5,
    conscientiousness: 0.5,
    extraversion: 0.5,
    agreeableness: 0.5,
    neuroticism: 0.5,
  }
}

// slugify turns a display name into the slug the hub keys a staff row on.
export function slugify(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}
