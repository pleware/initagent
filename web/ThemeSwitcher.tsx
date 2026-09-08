import { useMemo, useState } from 'react'
import { cn } from 'cn'
import { SimpleSelect } from './components/SimpleSelect'
import {
  currentPreference,
  isThemeFamily,
  pickerFamilies,
  setTheme,
  themeFamilies,
  type ThemeFamily,
} from './theme'

/**
 * Family picker for the hub and the marketing site.
 * Same SimpleSelect as LanguageSwitcher and the hub forms.
 */
export function ThemeSwitcher({
  className = '',
  label,
  size = 'compact',
}: {
  className?: string
  label?: string
  size?: 'compact' | 'nav'
}) {
  const items = useMemo(
    () =>
      pickerFamilies().map((id) => ({
        label: themeFamilies[id].label,
        value: id,
      })),
    [],
  )
  const stored = currentPreference().family
  const initial = items.some((item) => item.value === stored) ? stored : 'legacy'
  const [family, setFamily] = useState<ThemeFamily>(initial)
  const labelled = Boolean(label)

  return (
    <div className={cn(labelled ? 'flex flex-col gap-1' : 'inline-block', className)}>
      <span className={labelled ? 'text-sm font-medium text-fg' : 'sr-only'}>
        {label ?? 'Theme'}
      </span>
      <SimpleSelect
        items={items}
        value={family}
        onValueChange={(next) => {
          if (!isThemeFamily(next)) return
          setFamily(next)
          setTheme({ family: next })
        }}
        size={size === 'compact' && !labelled ? 'sm' : 'default'}
        className={labelled ? 'w-full' : undefined}
      />
    </div>
  )
}

export default ThemeSwitcher
