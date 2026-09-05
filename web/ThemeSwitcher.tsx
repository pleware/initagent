import { useMemo, useState } from 'react'
import { cn } from 'cn'
import {
  currentPreference,
  isThemeFamily,
  pickerFamilies,
  setTheme,
  themeFamilies,
  type ThemeFamily,
} from './theme'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './ui/select'

/**
 * Family picker for the hub and the marketing site.
 * Shared Select from `web/ui` (shadcn base-nova).
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
      <Select
        items={items}
        value={family}
        onValueChange={(next) => {
          if (!isThemeFamily(next)) return
          setFamily(next)
          setTheme({ family: next })
        }}
      >
        <SelectTrigger
          size={size === 'compact' && !labelled ? 'sm' : 'default'}
          className={labelled ? 'w-full' : undefined}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          <SelectGroup>
            {items.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

export default ThemeSwitcher
