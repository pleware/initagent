import { FormEvent, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import AuthSplit, { AuthMark, authFieldClass } from '../components/AuthSplit'
import { resolveLocale } from '../../../web/locale.ts'

// The hub reports which offering it is running as, and the screen says so.
// An operator should be able to see what they are about to type a credential
// into, and a hosted hub that fell back to self-host is otherwise invisible
// from the outside.
const offeringLabel: Record<string, { label: string; hint: string }> = {
  hosted: {
    label: 'Hosted',
    hint: 'This is a managed initagent hub.',
  },
  selfhost: {
    label: 'Self-hosted',
    hint: 'This hub runs on your own infrastructure.',
  },
}

export default function Login({
  claimed,
  offering,
  passwordMinLength,
  signup,
  onSuccess,
}: {
  claimed: boolean
  offering: string
  passwordMinLength: number
  signup: boolean
  onSuccess: () => void
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [token, setToken] = useState('')
  const [orgName, setOrgName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [registering, setRegistering] = useState(false)
  const { t, i18n } = useTranslation()

  const mode = offeringLabel[offering]
  const createAccount = claimed && signup && registering
  const newPassword = createAccount || !claimed
  const hostedOauth = offering === 'hosted' && claimed

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    // Confirm-password is a typo guard and stays on this side: the hub takes
    // one password, so there is nothing to disagree about on the wire.
    if (newPassword && password !== confirm) {
      setError(t('auth.passwordsMismatch'))
      return
    }
    setBusy(true)
    const locale = resolveLocale(i18n.resolvedLanguage || i18n.language)
    try {
      if (createAccount) {
        await api.post('/api/register', { email, password, locale })
      } else if (claimed) {
        await api.post('/api/login', { email, password })
      } else {
        await api.post('/api/setup', { email, password, token, orgName, locale })
      }
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  const title = createAccount
    ? t('auth.createTitle')
    : claimed
      ? t('auth.signInTitle')
      : t('auth.claimHub')

  return (
    <AuthSplit skipTarget="#login-email">
      <div>
        <div className="flex items-center gap-3">
          <AuthMark className="size-10" />
          {mode && (
            <span
              title={mode.hint}
              className="ml-auto rounded-full border border-line-2 px-2 py-0.5 text-xs font-medium text-fg-muted"
            >
              {mode.label}
            </span>
          )}
        </div>
        <h1 className="mt-8 text-2xl/9 font-bold tracking-tight text-fg-strong">{title}</h1>
        {claimed && signup ? (
          <p className="mt-2 text-sm/6 text-fg-muted">
            {registering ? t('auth.alreadyMember') : t('auth.notAMember')}{' '}
            <button
              type="button"
              className="font-semibold text-accent-fg hover:text-accent"
              onClick={() => {
                setRegistering((v) => !v)
                setError('')
                setConfirm('')
              }}
            >
              {registering ? t('auth.signInInstead') : t('auth.freeAccount')}
            </button>
          </p>
        ) : (
          <p className="mt-2 text-sm/6 text-fg-muted">
            {createAccount
              ? t('auth.createAccountHint')
              : claimed
                ? t('auth.signInHint')
                : t('auth.claimHint')}
          </p>
        )}
        {claimed && signup && !registering && (
          <p className="mt-1 text-sm/6 text-fg-subtle">{t('auth.freeAccountHint')}</p>
        )}
      </div>

      <div className="mt-10">
        <form onSubmit={submit} className="flex flex-col gap-6">
          <div>
            <label htmlFor="login-email" className="block text-sm/6 font-medium text-fg">
              {t('auth.email')}
            </label>
            <div className="mt-2">
              <input
                id="login-email"
                name="email"
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

          <div>
            <label htmlFor="login-password" className="block text-sm/6 font-medium text-fg">
              {t('auth.password')}
            </label>
            <div className="mt-2">
              <input
                id="login-password"
                name="password"
                type="password"
                autoComplete={newPassword ? 'new-password' : 'current-password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={newPassword ? passwordMinLength : undefined}
                required
                className={authFieldClass}
              />
            </div>
          </div>

          {newPassword && (
            <div>
              <label htmlFor="login-confirm" className="block text-sm/6 font-medium text-fg">
                {t('auth.confirmPassword')}
              </label>
              <div className="mt-2">
                <input
                  id="login-confirm"
                  type="password"
                  autoComplete="new-password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  required
                  className={authFieldClass}
                />
              </div>
              <p className="mt-2 text-xs text-fg-subtle">{t('auth.passwordMin', { min: passwordMinLength })}</p>
            </div>
          )}

          {!claimed && (
            <>
              {offering === 'hosted' && (
                <div>
                  <label htmlFor="login-org" className="block text-sm/6 font-medium text-fg">
                    {t('claim.organization')}
                  </label>
                  <div className="mt-2">
                    <input
                      id="login-org"
                      type="text"
                      autoComplete="organization"
                      value={orgName}
                      onChange={(e) => setOrgName(e.target.value)}
                      placeholder={t('claim.organizationPlaceholder')}
                      className={authFieldClass}
                    />
                  </div>
                  <p className="mt-2 text-xs text-fg-subtle">{t('claim.organizationHint')}</p>
                </div>
              )}

              <div>
                <label htmlFor="login-token" className="block text-sm/6 font-medium text-fg">
                  {t('auth.bootstrapToken')}
                </label>
                <div className="mt-2">
                  <input
                    id="login-token"
                    type="text"
                    autoComplete="off"
                    spellCheck={false}
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                    required
                    className={`${authFieldClass} font-mono`}
                  />
                </div>
                <p className="mt-2 text-xs text-fg-subtle">{t('auth.bootstrapHint')}</p>
              </div>
            </>
          )}

          {error && <p className="text-sm text-fail-fg">{error}</p>}

          {claimed && (
            <div className="flex items-center justify-end">
              <Link
                to="/forgot"
                className="text-sm/6 font-semibold text-accent-fg hover:text-accent"
              >
                {t('auth.forgotPassword')}
              </Link>
            </div>
          )}

          <button type="submit" disabled={busy} className="btn-primary w-full">
            {busy
              ? t('auth.pleaseWait')
              : createAccount
                ? t('auth.createAccount')
                : claimed
                  ? t('auth.login')
                  : t('auth.claimHub')}
          </button>
        </form>

        {hostedOauth && (
          <div className="mt-10">
            <div className="relative">
              <div aria-hidden="true" className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-line-2" />
              </div>
              <div className="relative flex justify-center text-sm/6 font-medium">
                <span className="bg-canvas px-6 text-fg-muted">{t('auth.continueWith')}</span>
              </div>
            </div>

            <div className="mt-6 grid grid-cols-2 gap-4">
              <button
                type="button"
                className="flex w-full items-center justify-center gap-3 rounded-control bg-fill-3 px-3 py-2 text-sm font-semibold text-fg ring-1 ring-inset ring-line-2 hover:bg-fill-4"
                onClick={() => setError(t('auth.oauthUnavailable'))}
              >
                <GoogleMark />
                <span className="text-sm/6 font-semibold">Google</span>
              </button>
              <button
                type="button"
                className="flex w-full items-center justify-center gap-3 rounded-control bg-fill-3 px-3 py-2 text-sm font-semibold text-fg ring-1 ring-inset ring-line-2 hover:bg-fill-4"
                onClick={() => setError(t('auth.oauthUnavailable'))}
              >
                <GitHubMark />
                <span className="text-sm/6 font-semibold">GitHub</span>
              </button>
            </div>
          </div>
        )}
      </div>
    </AuthSplit>
  )
}

function GoogleMark() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden className="size-5">
      <path
        d="M12.0003 4.75C13.7703 4.75 15.3553 5.36002 16.6053 6.54998L20.0303 3.125C17.9502 1.19 15.2353 0 12.0003 0C7.31028 0 3.25527 2.69 1.28027 6.60998L5.27028 9.70498C6.21525 6.86002 8.87028 4.75 12.0003 4.75Z"
        fill="#EA4335"
      />
      <path
        d="M23.49 12.275C23.49 11.49 23.415 10.73 23.3 10H12V14.51H18.47C18.18 15.99 17.34 17.25 16.08 18.1L19.945 21.1C22.2 19.01 23.49 15.92 23.49 12.275Z"
        fill="#4285F4"
      />
      <path
        d="M5.26498 14.2949C5.02498 13.5699 4.88501 12.7999 4.88501 11.9999C4.88501 11.1999 5.01998 10.4299 5.26498 9.7049L1.275 6.60986C0.46 8.22986 0 10.0599 0 11.9999C0 13.9399 0.46 15.7699 1.28 17.3899L5.26498 14.2949Z"
        fill="#FBBC05"
      />
      <path
        d="M12.0004 24.0001C15.2404 24.0001 17.9654 22.935 19.9454 21.095L16.0804 18.095C15.0054 18.82 13.6204 19.245 12.0004 19.245C8.8704 19.245 6.21537 17.135 5.2654 14.29L1.27539 17.385C3.25539 21.31 7.3104 24.0001 12.0004 24.0001Z"
        fill="#34A853"
      />
    </svg>
  )
}

function GitHubMark() {
  return (
    <svg viewBox="0 0 20 20" aria-hidden className="size-5 fill-fg">
      <path
        d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z"
        clipRule="evenodd"
        fillRule="evenodd"
      />
    </svg>
  )
}
