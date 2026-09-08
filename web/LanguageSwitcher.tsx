import { useMemo } from 'react'
import { cn } from 'cn'
import { SimpleSelect } from './components/SimpleSelect'
import { LOCALES, resolveLocale, type Locale } from './locale'

/**
 * Language picker for the hub and the marketing site.
 * Same SimpleSelect as the rest of the chrome.
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
      <SimpleSelect
        items={items}
        value={current}
        onValueChange={(next) => onChange(resolveLocale(next))}
        size={size === 'compact' && !labelled ? 'sm' : 'default'}
        className={labelled ? 'w-full' : undefined}
      />
    </div>
  )
}

export default LanguageSwitcher
