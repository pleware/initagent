import { useEffect, useState } from 'react'
import { listLocales } from '../api'
import type { Locale } from '../types'

// LocaleSelect is the staff form's language picker, fed once by
// GET /api/locales. A current value that is not in the catalog stays visible
// as a disabled fallback option, so the form still shows — and can save —
// whatever value it holds.
export default function LocaleSelect({
  value,
  onChange,
  disabled = false,
}: {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}) {
  const [locales, setLocales] = useState<Locale[]>([])

  useEffect(() => {
    let cancelled = false
    listLocales()
      .then((catalog) => {
        if (!cancelled) setLocales(catalog)
      })
      .catch(() => {
        // A catalog that fails to load leaves the fallback option as the
        // only choice; the form still saves whatever value it holds.
      })
    return () => {
      cancelled = true
    }
  }, [])

  const fallback = value !== '' && !locales.some((l) => l.code === value)

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
    >
      <option value="" />
      {fallback && (
        <option value={value} disabled>
          {value}
        </option>
      )}
      {locales.map((l) => (
        <option key={l.code} value={l.code}>
          {l.name}
        </option>
      ))}
    </select>
  )
}
