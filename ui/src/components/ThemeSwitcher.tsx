import { useState } from 'react'
import {
  currentPreference,
  pickerFamilies,
  setTheme,
  themeFamilies,
  type ThemeFamily,
} from '../theme'

export default function ThemeSwitcher({
  className = '',
  label,
  size = 'compact',
}: {
  className?: string
  label?: string
  size?: 'compact' | 'nav'
}) {
  const designed = pickerFamilies()
  const stored = currentPreference().family
  const initial = designed.includes(stored) ? stored : 'legacy'
  const [family, setFamily] = useState<ThemeFamily>(initial)
  const labelled = Boolean(label)
  const selectClass = labelled
    ? 'w-full appearance-none rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-sm text-fg hover:bg-fill-3 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus'
    : size === 'nav'
      ? 'appearance-none rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-[13.5px] text-fg-muted hover:bg-fill-3 hover:text-fg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus'
      : 'appearance-none rounded border border-line-2 bg-fill-2 py-1 pr-6 pl-2 text-[11px] text-fg hover:bg-fill-3 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus'

  return (
    <label className={`relative ${labelled ? 'block' : 'inline-block'} ${className}`}>
      <span className={labelled ? 'mb-1 block text-sm font-medium text-fg' : 'sr-only'}>
        {label ?? 'Theme'}
      </span>
      <select
        value={family}
        onChange={(event) => {
          const next = event.target.value as ThemeFamily
          setFamily(next)
          setTheme({ family: next })
        }}
        className={selectClass}
      >
        {designed.map((id) => (
          <option key={id} value={id}>
            {themeFamilies[id].label}
          </option>
        ))}
      </select>
    </label>
  )
}
