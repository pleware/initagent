import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, timeAgo } from '../api'
import DataTable from '../components/DataTable'
import type { Me, OrgMember } from '../types'

// An organization's own people, managed by its owner or admin (draft 25).
//
// This is the same question the administration screen asks — who is here,
// and what may they do — at a different boundary, which is why it shares the
// table and the vocabulary and not a screen. A customer's owner never sees
// another organization from here.
//
// The rules live on the hub, not in this form: an admin cannot make an owner,
// and an organization cannot lose its last one. The screen submits and shows
// what came back, so there is one place those rules can be wrong.
export default function PeoplePage({
  me,
  onChanged,
}: {
  me: Me
  onChanged: () => void
}) {
  const { t } = useTranslation()
  const memberships = me.orgs ?? []
  const [orgId, setOrgId] = useState(memberships[0]?.orgId ?? '')
  const [members, setMembers] = useState<OrgMember[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')

  const current = memberships.find((m) => m.orgId === orgId)
  const myRole = current?.role ?? ''
  const canManage = myRole === 'owner' || myRole === 'admin'
  const roles = me.orgRoles ?? []

  const load = useCallback(async () => {
    if (!orgId) {
      setMembers([])
      return
    }
    try {
      setMembers(await api.get<OrgMember[]>(`/api/orgs/${orgId}/members`))
      setError('')
    } catch (err) {
      setMembers([])
      setError(err instanceof Error ? err.message : t('admin.loadFailed'))
    }
  }, [orgId, t])

  useEffect(() => {
    load()
  }, [load])

  const changeRole = async (accountId: string, role: string) => {
    setBusy(accountId)
    setError('')
    try {
      await api.patch(`/api/orgs/${orgId}/members/${accountId}`, { role })
      await load()
      // My own role may have changed, and with it what this screen offers.
      if (accountId === me.accountId) onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('people.roleChangeFailed'))
    } finally {
      setBusy('')
    }
  }

  const remove = async (member: OrgMember) => {
    const leaving = member.accountId === me.accountId
    const question = leaving
      ? t('people.confirmLeave', { org: current?.name })
      : t('people.confirmRemove', { email: member.email, org: current?.name })
    if (!window.confirm(question)) return
    setBusy(member.accountId)
    setError('')
    try {
      await api.del(`/api/orgs/${orgId}/members/${member.accountId}`)
      if (leaving) {
        onChanged()
      } else {
        await load()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('people.removeFailed'))
    } finally {
      setBusy('')
    }
  }

  const rename = async () => {
    const name = window.prompt(t('people.renamePrompt'), current?.name ?? '')
    if (name === null || name.trim() === '') return
    try {
      await api.patch(`/api/orgs/${orgId}`, { name })
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('people.renameFailed'))
    }
  }

  if (memberships.length === 0) {
    return (
      <div className="page-shell">
        <div className="mb-6">
          <p className="eyebrow mb-3">{t('people.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-zinc-100">
            {t('people.title')}
          </h1>
        </div>
        <div className="surface rounded-2xl p-12 text-center">
          <p className="mb-2 text-zinc-300">{t('people.noOrgTitle')}</p>
          <p className="text-sm text-zinc-500">{t('people.noOrgHint')}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="page-shell">
      <div className="mb-6 flex items-end justify-between">
        <div>
          <p className="eyebrow mb-3">{t('people.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-zinc-100">
            {t('people.title')}
          </h1>
          <p className="mt-1 text-sm text-zinc-400">
            {t('people.subtitle', { org: current?.name, role: myRole })}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {memberships.length > 1 && (
            <select
              value={orgId}
              onChange={(e) => setOrgId(e.target.value)}
              aria-label={t('admin.organization')}
              className="rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-100"
            >
              {memberships.map((m) => (
                <option key={m.orgId} value={m.orgId}>
                  {m.name}
                </option>
              ))}
            </select>
          )}
          {canManage && (
            <button onClick={rename} className="btn-secondary">
              {t('people.rename')}
            </button>
          )}
        </div>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-rose-400/20 px-3 py-2 text-sm text-rose-400">
          {error}
        </p>
      )}

      <DataTable
        rows={members}
        rowKey={(m) => m.accountId}
        empty={<p className="text-sm text-zinc-500">{t('people.empty')}</p>}
        columns={[
          {
            header: t('people.email'),
            cell: (m) => (
              <span className="text-zinc-200">
                {m.email}
                {m.accountId === me.accountId && (
                  <span className="ml-2 text-xs text-zinc-500">
                    {t('people.you')}
                  </span>
                )}
              </span>
            ),
          },
          {
            header: t('people.role'),
            cell: (m) =>
              canManage ? (
                <select
                  value={m.role}
                  disabled={busy === m.accountId}
                  onChange={(e) => changeRole(m.accountId, e.target.value)}
                  aria-label={`${t('people.role')}: ${m.email}`}
                  className="rounded-lg border border-zinc-700 bg-zinc-950 px-2 py-1 text-sm text-zinc-100"
                >
                  {roles.map((role) => (
                    <option key={role} value={role}>
                      {role}
                    </option>
                  ))}
                </select>
              ) : (
                <span className="text-zinc-400">{m.role}</span>
              ),
          },
          {
            header: t('people.joined'),
            cell: (m) => (
              <span className="text-zinc-500">{timeAgo(m.createdAt)}</span>
            ),
          },
          {
            header: '',
            srHeader: t('people.actions'),
            width: 'w-28',
            cell: (m) =>
              // Leaving needs no administrative right, so the button is here
              // for your own row whatever your role is.
              canManage || m.accountId === me.accountId ? (
                <button
                  onClick={() => remove(m)}
                  disabled={busy === m.accountId}
                  className="text-xs text-zinc-500 hover:text-rose-400"
                >
                  {m.accountId === me.accountId
                    ? t('people.leave')
                    : t('people.remove')}
                </button>
              ) : null,
          },
        ]}
      />

      <p className="mt-6 text-xs leading-5 text-zinc-600">
        {t('people.inviteMissing')}
      </p>
    </div>
  )
}
