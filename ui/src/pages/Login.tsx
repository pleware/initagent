import { FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'

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
  onSuccess,
}: {
  claimed: boolean
  offering: string
  passwordMinLength: number
  onSuccess: () => void
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [token, setToken] = useState('')
  const [orgName, setOrgName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const { t } = useTranslation()

  const mode = offeringLabel[offering]

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    // Confirm-password is a typo guard and stays on this side: the hub takes
    // one password, so there is nothing to disagree about on the wire.
    if (!claimed && password !== confirm) {
      setError('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      if (claimed) {
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
    <div className="flex min-h-full items-center justify-center p-4">
      <form
        onSubmit={submit}
        className="surface w-full max-w-sm rounded-2xl p-8"
      >
        <div className="mb-2 flex items-center gap-3">
          <svg width="28" height="28" viewBox="0 0 100 100" aria-hidden>
            <circle cx="50" cy="50" r="42" className="fill-lime-300" />
            <circle cx="50" cy="50" r="18" className="fill-zinc-950" />
            <circle cx="50" cy="50" r="8" className="fill-lime-100" />
          </svg>
          <h1 className="text-xl font-semibold text-zinc-100">initagent</h1>
          {mode && (
            <span
              title={mode.hint}
              className="ml-auto rounded-full border border-zinc-700 px-2 py-0.5 text-xs font-medium text-zinc-400"
            >
              {mode.label}
            </span>
          )}
        </div>
        <p className="mb-6 text-sm text-zinc-400">
          {claimed
            ? 'Sign in to reach your fleet.'
            : 'This hub has no owner yet. Claim it to create the admin account.'}
        </p>

        <label
          htmlFor="login-email"
          className="mb-1 block text-sm font-medium text-zinc-300"
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
          className="mb-4 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-zinc-100 outline-none focus:border-lime-500"
        />

        <label
          htmlFor="login-password"
          className="mb-1 block text-sm font-medium text-zinc-300"
        >
          Password
        </label>
        <input
          id="login-password"
          type="password"
          autoComplete={claimed ? 'current-password' : 'new-password'}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          minLength={claimed ? undefined : passwordMinLength}
          required
          className="mb-4 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-zinc-100 outline-none focus:border-lime-500"
        />

        {!claimed && (
          <>
            <label
              htmlFor="login-confirm"
              className="mb-1 block text-sm font-medium text-zinc-300"
            >
              Confirm password
            </label>
            <input
              id="login-confirm"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              required
              className="mb-4 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-zinc-100 outline-none focus:border-lime-500"
            />
            <p className="mb-4 text-xs text-zinc-500">
              At least {passwordMinLength} characters.
            </p>

            {offering === 'hosted' && (
              <>
                <label
                  htmlFor="login-org"
                  className="mb-1 block text-sm font-medium text-zinc-300"
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
                  className="mb-2 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-zinc-100 outline-none focus:border-lime-500"
                />
                <p className="mb-4 text-xs text-zinc-500">
                  {t('claim.organizationHint')}
                </p>
              </>
            )}

            <label
              htmlFor="login-token"
              className="mb-1 block text-sm font-medium text-zinc-300"
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
              className="mb-2 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-lime-500"
            />
            <p className="mb-4 text-xs text-zinc-500">
              The hub printed this to its log at start, and wrote it to{' '}
              <code className="text-zinc-400">bootstrap-token</code> in its
              data directory. It proves you are the operator of this hub rather
              than someone who found the address, and it stops working once the
              hub is claimed.
            </p>
          </>
        )}

        {error && <p className="mb-4 text-sm text-rose-400">{error}</p>}
        <button type="submit" disabled={busy} className="btn-primary w-full">
          {busy ? 'Please wait…' : claimed ? 'Log in' : 'Claim this hub'}
        </button>
      </form>
    </div>
  )
}
