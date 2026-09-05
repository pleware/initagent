import { FormEvent, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import AuthSplit, { AuthMark, authFieldClass } from '../components/AuthSplit'

export default function ResetPassword({
  passwordMinLength,
  onSuccess,
}: {
  passwordMinLength: number
  onSuccess: () => void
}) {
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const { t } = useTranslation()

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!token) {
      setError(t('auth.invalidReset'))
      return
    }
    if (password !== confirm) {
      setError(t('auth.passwordsMismatch'))
      return
    }
    setBusy(true)
    try {
      await api.post('/api/password/reset', { token, password })
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.generic'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthSplit skipTarget="#reset-password">
      <AuthMark className="size-10" />
      <h1 className="mt-8 text-2xl/9 font-bold tracking-tight text-fg-strong">
        {t('auth.resetTitle')}
      </h1>
      <p className="mt-2 text-sm/6 text-fg-muted">{t('auth.resetHint')}</p>
      <div className="mt-10">
        <form onSubmit={submit} className="flex flex-col gap-6">
          <div>
            <label htmlFor="reset-password" className="block text-sm/6 font-medium text-fg">
              {t('auth.password')}
            </label>
            <div className="mt-2">
              <input
                id="reset-password"
                type="password"
                autoFocus
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={passwordMinLength}
                required
                className={authFieldClass}
              />
            </div>
          </div>
          <div>
            <label htmlFor="reset-confirm" className="block text-sm/6 font-medium text-fg">
              {t('auth.confirmPassword')}
            </label>
            <div className="mt-2">
              <input
                id="reset-confirm"
                type="password"
                autoComplete="new-password"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                required
                className={authFieldClass}
              />
            </div>
            <p className="mt-2 text-xs text-fg-subtle">
              {t('validation.minLength', { min: passwordMinLength })}
            </p>
          </div>
          {error && <p className="text-sm text-fail-fg">{error}</p>}
          <button type="submit" disabled={busy || !token} className="btn-primary w-full">
            {busy ? t('common.loading') : t('auth.setPassword')}
          </button>
        </form>
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
