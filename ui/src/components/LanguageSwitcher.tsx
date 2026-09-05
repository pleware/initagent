import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { LanguageSwitcher as SharedLanguageSwitcher } from '../../../web/LanguageSwitcher.tsx'
import { resolveLocale, type Locale } from '../../../web/locale.ts'

export { resolveLocale }
export type { Locale }

/**
 * Cockpit language picker. Guests keep the choice in the browser; a
 * signed-in person also writes it onto the account so the next session
 * (and reset mail) match.
 */
export default function LanguageSwitcher({
  persist = false,
  className,
  size = 'compact',
}: {
  persist?: boolean
  className?: string
  size?: 'compact' | 'nav'
}) {
  const { i18n } = useTranslation()
  const value = resolveLocale(i18n.resolvedLanguage || i18n.language)

  const handleChange = (locale: Locale) => {
    void i18n.changeLanguage(locale)
    document.documentElement.lang = locale
    if (!persist) return
    void api.patch('/api/me', { locale }).catch(() => {
      /* local choice already applied; next login restores the stored value */
    })
  }

  return (
    <SharedLanguageSwitcher
      value={value}
      onChange={handleChange}
      className={className}
      size={size}
    />
  )
}
