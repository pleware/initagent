export const LOCALES = [
  { value: 'en', label: 'EN' },
  { value: 'pl', label: 'PL' },
] as const

export type Locale = (typeof LOCALES)[number]['value']

/** Collapse a BCP 47 tag onto the two languages the hub stores. */
export function resolveLocale(code: string | undefined | null): Locale {
  const base = (code ?? 'en').trim().toLowerCase().split(/[-_]/)[0]
  return base === 'pl' ? 'pl' : 'en'
}

/** Carry the current language onto another origin (site → hub). */
export function withLangParam(url: string, locale: string): string {
  const next = new URL(url)
  next.searchParams.set('lng', resolveLocale(locale))
  return next.toString()
}
