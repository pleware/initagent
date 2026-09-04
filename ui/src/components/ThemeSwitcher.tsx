import { useState } from 'react'
import {
  currentPreference,
  pickerFamilies,
  setTheme,
  themeFamilies,
  type ThemeFamily,
} from '../theme'

export default function ThemeSwitcher({ className = '' }: { className?: string }) {
  const designed = pickerFamilies()
  const stored = currentPreference().family
  const initial = designed.includes(stored) ? stored : 'legacy'
  const [family, setFamily] = useState<ThemeFamily>(initial)

  return (
    <label className={`relative inline-block ${className}`}>
      <span className="sr-only">Theme</span>
      <select
        value={family}
        onChange={(event) => {
          const next = event.target.value as ThemeFamily
          setFamily(next)
          setTheme({ family: next })
        }}
        className="appearance-none rounded border border-line-2 bg-fill-2 py-1 pr-6 pl-2 text-[11px] text-fg hover:bg-fill-3 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
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
