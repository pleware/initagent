import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'

export type SimpleSelectItem = {
  value: string
  label: string
}

// Base UI rejects an empty item value. Callers may still pass "" for
// "any / unset"; we map it here so each screen does not invent a sentinel.
const EMPTY = '__empty__'

function encode(value: string): string {
  return value === '' ? EMPTY : value
}

function decode(value: string): string {
  return value === EMPTY ? '' : value
}

/**
 * One-line picker over the shared shadcn Select.
 * LanguageSwitcher and ThemeSwitcher use the same primitive; this is the
 * short form for the other hub surfaces so native <select> does not return.
 */
export function SimpleSelect({
  items,
  value,
  onValueChange,
  size = 'sm',
  disabled,
  className,
  'aria-label': ariaLabel,
}: {
  items: SimpleSelectItem[]
  value: string
  onValueChange: (value: string) => void
  size?: 'sm' | 'default'
  disabled?: boolean
  className?: string
  'aria-label'?: string
}) {
  return (
    <Select
      items={items.map((item) => ({ ...item, value: encode(item.value) }))}
      value={encode(value)}
      disabled={disabled}
      onValueChange={(next) => {
        if (next == null) return
        const decoded = decode(next)
        if (decoded === value) return
        onValueChange(decoded)
      }}
    >
      <SelectTrigger size={size} className={className} aria-label={ariaLabel}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        <SelectGroup>
          {items.map((item) => (
            <SelectItem key={encode(item.value)} value={encode(item.value)}>
              {item.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}
