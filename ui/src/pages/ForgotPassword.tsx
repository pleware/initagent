import { FormEvent, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import AuthSplit, { AuthMark, authFieldClass } from '../components/AuthSplit'

export default function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [error, setError] = useState('')
  const [sent, setSent] = useState(false)
  const [busy, setBusy] = useState(false)
  const { t } = useTranslation()

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await api.post('/api/password/forgot', { email })
      setSent(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.generic'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthSplit skipTarget="#forgot-email">
      <AuthMark className="size-10" />
      <h1 className="mt-8 text-2xl/9 font-bold tracking-tight text-fg-strong">
        {t('auth.forgotTitle')}
      </h1>
      <p className="mt-2 text-sm/6 text-fg-muted">{t('auth.forgotHint')}</p>
      <div className="mt-10">
        {sent ? (
          <p className="text-sm text-fg">{t('auth.resetSent')}</p>
        ) : (
          <form onSubmit={submit} className="flex flex-col gap-6">
            <div>
              <label htmlFor="forgot-email" className="block text-sm/6 font-medium text-fg">
                {t('auth.email')}
              </label>
              <div className="mt-2">
                <input
                  id="forgot-email"
                  type="email"
                  autoFocus
                  autoComplete="username"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  className={authFieldClass}
                />
              </div>
            </div>
            {error && <p className="text-sm text-fail-fg">{error}</p>}
            <button type="submit" disabled={busy} className="btn-primary w-full">
              {busy ? t('common.loading') : t('auth.sendReset')}
            </button>
          </form>
        )}
        <Link
          to="/login"
          className="mt-6 block text-sm/6 font-semibold text-accent-fg hover:text-accent"
        >
          {t('auth.backToLogin')}
        </Link>
      </div>
    </AuthSplit>
  )
}
