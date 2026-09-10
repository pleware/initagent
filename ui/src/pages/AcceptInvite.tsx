import { FormEvent, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import AuthSplit, { AuthMark, authFieldClass } from '../components/AuthSplit'
import { resolveLocale } from '../../../web/locale.ts'

type InvitePreview = {
  email: string
  orgName: string
  role: string
  expiresAt: number
}

export default function AcceptInvite({
  passwordMinLength,
  defaultEmail,
  onSuccess,
}: {
  passwordMinLength: number
  defaultEmail?: string
  onSuccess: () => void
}) {
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const [preview, setPreview] = useState<InvitePreview | null>(null)
  const [email, setEmail] = useState(defaultEmail ?? '')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const { t, i18n } = useTranslation()

  useEffect(() => {
    if (!token) {
      setError(t('auth.invalidInvite'))
      return
    }
    let cancelled = false
    api
      .get<InvitePreview>(`/api/invite?token=${encodeURIComponent(token)}`)
      .then((got) => {
        if (cancelled) return
        setPreview(got)
        setEmail((current) => current || got.email)
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : t('auth.invalidInvite'))
        }
      })
    return () => {
      cancelled = true
    }
  }, [token, t])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!token) {
      setError(t('auth.invalidInvite'))
      return
    }
    if (password !== confirm) {
      setError(t('auth.passwordsMismatch'))
      return
    }
    setBusy(true)
    try {
      const locale = resolveLocale(i18n.resolvedLanguage || i18n.language)
      await api.post('/api/invite/redeem', { token, email, password, locale })
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.generic'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthSplit skipTarget="#invite-email">
      <AuthMark className="size-10" />
      <h1 className="mt-8 text-2xl/9 font-bold tracking-tight text-fg-strong">
        {t('auth.inviteTitle')}
      </h1>
      <p className="mt-2 text-sm/6 text-fg-muted">
        {preview
          ? t('auth.inviteHintNamed', { org: preview.orgName, role: preview.role })
          : t('auth.inviteHint')}
      </p>
      <div className="mt-10">
        <form onSubmit={submit} className="flex flex-col gap-6">
          <div>
            <label htmlFor="invite-email" className="block text-sm/6 font-medium text-fg">
              {t('auth.email')}
            </label>
            <div className="mt-2">
              <input
                id="invite-email"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className={authFieldClass}
              />
            </div>
          </div>
          <div>
            <label htmlFor="invite-password" className="block text-sm/6 font-medium text-fg">
              {t('auth.password')}
            </label>
            <div className="mt-2">
              <input
                id="invite-password"
                type="password"
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
            <label htmlFor="invite-confirm" className="block text-sm/6 font-medium text-fg">
              {t('auth.confirmPassword')}
            </label>
            <div className="mt-2">
              <input
                id="invite-confirm"
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
            {busy ? t('common.loading') : t('auth.joinOrg')}
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
