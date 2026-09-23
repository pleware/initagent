import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listVoices } from '../api'
import type { Voice } from '../types'

// voiceLanguageOrder is the picker's group order — pl_PL first (the
// product's home market), then en_US, then en_GB — mirroring the display
// order the hub serves. A language outside the three keeps its place after
// them.
const voiceLanguageOrder = ['pl_PL', 'en_US', 'en_GB']

// inheritVoice is the sentinel the org-override picker uses for "inherit
// the hub's value": choosing it omits voice from the payload instead of
// sending an empty string.
export const inheritVoice = '__inherit__'

function groupVoices(voices: Voice[]): { language: string; voices: Voice[] }[] {
  const byLanguage = new Map<string, Voice[]>()
  for (const voice of voices) {
    const list = byLanguage.get(voice.language)
    if (list) list.push(voice)
    else byLanguage.set(voice.language, [voice])
  }
  const order = [...voiceLanguageOrder]
  for (const language of byLanguage.keys()) {
    if (!order.includes(language)) order.push(language)
  }
  return order.flatMap((language) => {
    const list = byLanguage.get(language)
    return list ? [{ language, voices: list }] : []
  })
}

// VoiceSelect is the voice picker of the staff forms: a select grouped by
// language (pl_PL first), fed once by GET /api/voices. The catalog carries one
// non-Piper engine's voice as well, and that one is labelled through
// voices.engine.<id> instead of its raw id — so the picker reads
// "VoxCPM2 (48 kHz)" while the value it saves stays the id. A current value that
// is not in the catalog stays visible as a disabled fallback option, so the
// form still shows — and can save — what it holds.
//
// `override` switches to the org-override vocabulary: the first option
// inherits the hub's value (the caller omits the field) and the empty
// option clears it (the caller sends "").
export default function VoiceSelect({
  value,
  onChange,
  override = false,
  disabled = false,
}: {
  value: string
  onChange: (value: string) => void
  override?: boolean
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [voices, setVoices] = useState<Voice[]>([])

  useEffect(() => {
    let cancelled = false
    listVoices()
      .then((catalog) => {
        if (!cancelled) setVoices(catalog)
      })
      .catch(() => {
        // A catalog that fails to load leaves the fallback option as the
        // only choice; the form still saves whatever value it holds.
      })
    return () => {
      cancelled = true
    }
  }, [])

  const fallback =
    value !== '' &&
    value !== inheritVoice &&
    !voices.some((voice) => voice.name === value)

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      className="field-input mt-2"
    >
      {override && (
        <option value={inheritVoice}>{t('staff.voiceInherit')}</option>
      )}
      <option value="">{override ? t('staff.voiceClear') : t('staff.voiceNone')}</option>
      {fallback && (
        <option value={value} disabled>
          {value}
        </option>
      )}
      {groupVoices(voices).map((group) => (
        <optgroup
          key={group.language}
          label={t('voices.lang.' + group.language, {
            defaultValue: group.language,
          })}
        >
          {group.voices.map((voice) => (
            <option key={voice.name} value={voice.name}>
              {voice.engine
                ? t('voices.engine.' + voice.engine, { defaultValue: voice.name })
                : voice.name}
            </option>
          ))}
        </optgroup>
      ))}
    </select>
  )
}
