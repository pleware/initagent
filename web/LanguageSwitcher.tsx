import { useMemo } from 'react'
import { cn } from 'cn'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './ui/select'
import { LOCALES, resolveLocale, type Locale } from './locale'

/**
 * Language picker for the hub and the marketing site.
 * Shared Select from `web/ui` (shadcn base-nova), same shape as ThemeSwitcher.
 */
export function LanguageSwitcher({
  value,
  onChange,
  className = '',
  label,
  size = 'compact',
}: {
  value: string
  onChange: (locale: Locale) => void
  className?: string
  label?: string
  size?: 'compact' | 'nav'
}) {
  const items = useMemo(
    () => LOCALES.map((item) => ({ label: item.label, value: item.value })),
    [],
  )
  const current = resolveLocale(value)
  const labelled = Boolean(label)

  return (
    <div className={cn(labelled ? 'flex flex-col gap-1' : 'inline-block', className)}>
      <span className={labelled ? 'text-sm font-medium text-fg' : 'sr-only'}>
        {label ?? 'Language'}
      </span>
      <Select
        items={items}
        value={current}
        onValueChange={(next) => {
          const locale = resolveLocale(next)
          if (locale === current) return
          onChange(locale)
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

export default LanguageSwitcher
