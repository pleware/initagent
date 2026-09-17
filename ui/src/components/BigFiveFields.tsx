import { useTranslation } from 'react-i18next'
import type { Character } from '../types'

const TRAITS: { key: keyof Character; label: string }[] = [
  { key: 'openness', label: 'staff.openness' },
  { key: 'conscientiousness', label: 'staff.conscientiousness' },
  { key: 'extraversion', label: 'staff.extraversion' },
  { key: 'agreeableness', label: 'staff.agreeableness' },
  { key: 'neuroticism', label: 'staff.neuroticism' },
]

// The five 0..1 trait editors the admin staff form and the org override form
// share: one slider plus one numeric input per trait, so a value can be set
// by drag or typed exactly.
export default function BigFiveFields({
  value,
  onChange,
}: {
  value: Character
  onChange: (next: Character) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
      {TRAITS.map(({ key, label }) => (
        <TraitField
          key={key}
          label={t(label)}
          value={value[key]}
          onChange={(next) => onChange({ ...value, [key]: next })}
        />
      ))}
    </div>
  )
}

function TraitField({
  label,
  value,
  onChange,
}: {
  label: string
  value: number
  onChange: (next: number) => void
}) {
  const clamped = Number.isFinite(value) ? Math.min(1, Math.max(0, value)) : 0
  return (
    <label className="text-sm text-zinc-300">
      <span className="flex items-center justify-between gap-2">
        <span>{label}</span>
        <input
          type="number"
          min={0}
          max={1}
          step={0.05}
          value={Number.isFinite(value) ? value : 0}
          onChange={(e) => onChange(parseFloat(e.target.value))}
          className="w-20 rounded-md border border-white/10 bg-white/5 px-2 py-1 text-right font-mono text-xs text-zinc-100"
        />
      </span>
      <input
        type="range"
        min={0}
        max={1}
        step={0.05}
        value={clamped}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        className="mt-1.5 w-full accent-lime-400"
      />
    </label>
  )
}
