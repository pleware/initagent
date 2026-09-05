import { FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import GuestNav from '../components/GuestNav'

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
  const { t } = useTranslation()

  const mode = offeringLabel[offering]
  const createAccount = claimed && signup && registering
  const newPassword = createAccount || !claimed

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    // Confirm-password is a typo guard and stays on this side: the hub takes
    // one password, so there is nothing to disagree about on the wire.
    if (newPassword && password !== confirm) {
      setError('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      if (createAccount) {
        await api.post('/api/register', { email, password })
      } else if (claimed) {
        await api.post('/api/login', { email, password })
      } else {
        await api.post('/api/setup', { email, password, token, orgName })
      }
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <a href="#login-email" className="skip-link">
        {t('publicNav.skipToForm')}
      </a>
      <GuestNav />
      <div className="fixed inset-0 flex items-center justify-center px-4 pt-16">
      <form
        onSubmit={submit}
        className="surface w-full max-w-sm rounded-2xl p-8"
      >
        <div className="mb-2 flex items-center gap-3">
          <svg width="28" height="28" viewBox="0 0 100 100" aria-hidden>
            <circle cx="50" cy="50" r="42" className="fill-accent" />
            <circle cx="50" cy="50" r="18" className="fill-canvas" />
            <circle cx="50" cy="50" r="8" className="fill-accent-on" />
          </svg>
          <h1 className="text-xl font-semibold text-fg-strong">initagent</h1>
          {mode && (
            <span
              title={mode.hint}
              className="ml-auto rounded-full border border-line-2 px-2 py-0.5 text-xs font-medium text-fg-muted"
            >
              {mode.label}
            </span>
          )}
        </div>
        <p className="mb-6 text-sm text-fg-muted">
          {createAccount
            ? t('auth.createAccountHint')
            : claimed
              ? 'Sign in to reach your fleet.'
              : 'This hub has no owner yet. Claim it to create the admin account.'}
        </p>

        <label
          htmlFor="login-email"
          className="mb-1 block text-sm font-medium text-fg"
        >
          Email
        </label>
        <input
          id="login-email"
          type="email"
          autoFocus
          autoComplete="username"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
          className="mb-4 w-full rounded-lg border border-line-2 bg-canvas-sunken px-3 py-2 text-fg outline-none focus:border-accent"
        />

        <label
          htmlFor="login-password"
          className="mb-1 block text-sm font-medium text-fg"
        >
          Password
        </label>
        <input
          id="login-password"
          type="password"
          autoComplete={newPassword ? 'new-password' : 'current-password'}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          minLength={newPassword ? passwordMinLength : undefined}
          required
          className="mb-4 w-full rounded-lg border border-line-2 bg-canvas-sunken px-3 py-2 text-fg outline-none focus:border-accent"
        />

        {newPassword && (
          <>
            <label
              htmlFor="login-confirm"
              className="mb-1 block text-sm font-medium text-fg"
            >
              {t('auth.confirmPassword')}
            </label>
            <input
              id="login-confirm"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              required
              className="mb-4 w-full rounded-lg border border-line-2 bg-canvas-sunken px-3 py-2 text-fg outline-none focus:border-accent"
            />
            <p className="mb-4 text-xs text-fg-subtle">
              At least {passwordMinLength} characters.
            </p>
          </>
        )}

        {!claimed && (
          <>
            {offering === 'hosted' && (
              <>
                <label
                  htmlFor="login-org"
                  className="mb-1 block text-sm font-medium text-fg"
                >
                  {t('claim.organization')}
                </label>
                <input
                  id="login-org"
                  type="text"
                  autoComplete="organization"
                  value={orgName}
                  onChange={(e) => setOrgName(e.target.value)}
                  placeholder={t('claim.organizationPlaceholder')}
                  className="mb-2 w-full rounded-lg border border-line-2 bg-canvas-sunken px-3 py-2 text-fg outline-none focus:border-accent"
                />
                <p className="mb-4 text-xs text-fg-subtle">
                  {t('claim.organizationHint')}
                </p>
              </>
            )}

            <label
              htmlFor="login-token"
              className="mb-1 block text-sm font-medium text-fg"
            >
              Bootstrap token
            </label>
            <input
              id="login-token"
              type="text"
              autoComplete="off"
              spellCheck={false}
              value={token}
              onChange={(e) => setToken(e.target.value)}
              required
              className="mb-2 w-full rounded-lg border border-line-2 bg-canvas-sunken px-3 py-2 font-mono text-sm text-fg outline-none focus:border-accent"
            />
            <p className="mb-4 text-xs text-fg-subtle">
              The hub printed this to its log at start, and wrote it to{' '}
              <code className="text-fg-muted">bootstrap-token</code> in its
              data directory. It proves you are the operator of this hub rather
              than someone who found the address, and it stops working once the
              hub is claimed.
            </p>
          </>
        )}

        {error && <p className="mb-4 text-sm text-fail-fg">{error}</p>}
        <button type="submit" disabled={busy} className="btn-primary w-full">
          {busy
            ? 'Please wait…'
            : createAccount
              ? t('auth.createAccount')
              : claimed
                ? 'Log in'
                : 'Claim this hub'}
        </button>
        {claimed && signup && (
          <button
            type="button"
            className="mt-3 w-full text-sm text-fg-muted underline-offset-2 hover:text-fg hover:underline"
            onClick={() => {
              setRegistering((v) => !v)
              setError('')
              setConfirm('')
            }}
          >
            {registering ? t('auth.alreadyHaveAccount') : t('auth.needAccount')}
          </button>
        )}
      </form>
      </div>
    </>
  )
}
