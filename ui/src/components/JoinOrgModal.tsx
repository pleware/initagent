import { FormEvent, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import type { InvitePreview } from '../types'
import Modal from './Modal'

// The field takes either the bare token or the full invite link the People
// screen copies (<origin>/invite?token=…); a URL that fails to parse is a
// token.
function extractToken(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  try {
    return new URL(trimmed).searchParams.get('token') ?? ''
  } catch {
    return trimmed
  }
}

export default function JoinOrgModal({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [input, setInput] = useState('')
  const [token, setToken] = useState('')
  const [preview, setPreview] = useState<InvitePreview | null>(null)
  const [error, setError] = useState('')
  const [peeking, setPeeking] = useState(false)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    setPeeking(true)
    api
      .get<InvitePreview>(`/api/invite?token=${encodeURIComponent(token)}`)
      .then((got) => {
        if (cancelled) return
        setPreview(got)
        setError('')
      })
      .catch((err) => {
        if (cancelled) return
        setPreview(null)
        setError(
          err instanceof Error
            ? err.message
            : t('orgs.invalidToken', {
                defaultValue: 'This invite is invalid or has expired.',
              }),
        )
      })
      .finally(() => {
        if (!cancelled) setPeeking(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, t])

  const check = (event: FormEvent) => {
    event.preventDefault()
    const next = extractToken(input)
    if (!next) {
      setPreview(null)
      setError(t('orgs.emptyToken', { defaultValue: 'Paste an invite link or token.' }))
      return
    }
    setError('')
    setPreview(null)
    setToken(next)
  }

  const join = () => {
    navigate(`/invite?token=${encodeURIComponent(token)}`)
  }

  return (
    <Modal title={t('orgs.join', { defaultValue: 'Join organization' })} onClose={onClose}>
      <form onSubmit={check} className="px-6 pb-6">
        <p className="text-sm text-fg-muted">
          {t('orgs.joinHint', {
            defaultValue: 'Paste an invite link to join another organization.',
          })}
        </p>
        <label className="mt-4 block text-sm text-fg">
          {t('orgs.inviteLabel', { defaultValue: 'Invite link or token' })}
          <input
            autoFocus
            value={input}
            onChange={(event) => setInput(event.target.value)}
            placeholder={t('orgs.invitePlaceholder', {
              defaultValue: 'https://app.initagent.dev/invite?token=…',
            })}
            className="mt-1.5 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg placeholder:text-fg-faint"
          />
        </label>
        {error && (
          <p role="alert" className="mt-3 text-sm text-fail-fg">
            {error}
          </p>
        )}
        {peeking && <p className="mt-3 text-sm text-fg-muted">{t('common.loading')}</p>}
        {preview && !peeking && !error && (
          <p className="mt-3 rounded-lg border border-info/25 bg-info/10 px-3 py-2 text-sm text-fg">
            {t('orgs.joinPreview', {
              defaultValue: 'You are invited to {{org}} as {{role}}.',
              org: preview.orgName,
              role: preview.role,
            })}
          </p>
        )}
        <div className="mt-6 flex items-center justify-end gap-3">
          <button type="button" onClick={onClose} className="btn-secondary">
            {t('common.cancel')}
          </button>
          {preview && !peeking ? (
            <button type="button" onClick={join} className="btn-primary">
              {t('auth.joinOrg')}
            </button>
          ) : (
            <button type="submit" disabled={peeking} className="btn-primary">
              {t('orgs.check', { defaultValue: 'Check invite' })}
            </button>
          )}
        </div>
      </form>
    </Modal>
  )
}
