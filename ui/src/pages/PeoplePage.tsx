import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { api, timeAgo } from '../api'
import DataTable from '../components/DataTable'
import { HubError } from '../components/PlanWall'
import type { Me, OrgInvite, OrgMember } from '../types'

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
  const [invites, setInvites] = useState<OrgInvite[]>([])
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState('member')
  const [inviteLink, setInviteLink] = useState('')
  const [error, setError] = useState('')
  const [inviteError, setInviteError] = useState<unknown>(null)
  const [busy, setBusy] = useState('')

  const current = memberships.find((m) => m.orgId === orgId)
  const myRole = current?.role ?? ''
  const canManage = myRole === 'owner' || myRole === 'admin'
  const roles = me.orgRoles ?? []

  const load = useCallback(async () => {
    if (!orgId) {
      setMembers([])
      setInvites([])
      return
    }
    try {
      setMembers(await api.get<OrgMember[]>(`/api/orgs/${orgId}/members`))
      if (canManage) {
        setInvites(await api.get<OrgInvite[]>(`/api/orgs/${orgId}/invites`))
      } else {
        setInvites([])
      }
      setError('')
    } catch (err) {
      setMembers([])
      setInvites([])
      setError(err instanceof Error ? err.message : t('admin.loadFailed'))
    }
  }, [orgId, canManage, t])

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

  const sendInvite = async () => {
    setInviteError(null)
    setInviteLink('')
    setBusy('invite')
    try {
      const created = await api.post<{ link: string }>(`/api/orgs/${orgId}/invites`, {
        email: inviteEmail,
        role: inviteRole,
      })
      setInviteEmail('')
      setInviteRole('member')
      setInviteLink(created.link)
      await load()
    } catch (err) {
      setInviteError(err)
    } finally {
      setBusy('')
    }
  }

  const revokeInvite = async (invite: OrgInvite) => {
    if (!window.confirm(t('people.confirmRevoke', { email: invite.email }))) return
    setBusy(invite.id)
    setInviteError(null)
    try {
      await api.del(`/api/orgs/${orgId}/invites/${invite.id}`)
      await load()
    } catch (err) {
      setInviteError(err)
    } finally {
      setBusy('')
    }
  }

  const copyLink = async (link: string) => {
    try {
      await navigator.clipboard.writeText(link)
    } catch {
      window.prompt(t('people.copyPrompt'), link)
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
            <SimpleSelect
              size="default"
              value={orgId}
              onValueChange={setOrgId}
              aria-label={t('admin.organization')}
              items={memberships.map((m) => ({ value: m.orgId, label: m.name }))}
            />
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
                <SimpleSelect
                  value={m.role}
                  disabled={busy === m.accountId}
                  onValueChange={(role) => changeRole(m.accountId, role)}
                  aria-label={`${t('people.role')}: ${m.email}`}
                  items={roles.map((role) => ({ value: role, label: role }))}
                />
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

      {canManage && (
        <section className="mt-10">
          <h2 className="text-lg font-semibold text-zinc-100">{t('people.inviteTitle')}</h2>
          <p className="mt-1 text-sm text-zinc-400">{t('people.inviteHint')}</p>
          <form
            className="mt-4 flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              void sendInvite()
            }}
          >
            <label className="min-w-56 flex-1 text-sm text-zinc-300">
              {t('people.email')}
              <input
                type="email"
                required
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-zinc-100"
              />
            </label>
            <label className="text-sm text-zinc-300">
              {t('people.role')}
              <div className="mt-1">
                <SimpleSelect
                  value={inviteRole}
                  onValueChange={setInviteRole}
                  aria-label={t('people.role')}
                  items={roles.map((role) => ({ value: role, label: role }))}
                />
              </div>
            </label>
            <button type="submit" disabled={busy === 'invite'} className="btn-primary">
              {busy === 'invite' ? t('common.loading') : t('people.sendInvite')}
            </button>
          </form>
          <div className="mt-4">
            <HubError error={inviteError} fallback={t('people.inviteFailed')} />
          </div>
          {inviteLink && (
            <p className="mt-3 text-sm text-zinc-300">
              {t('people.inviteLinkOnce')}{' '}
              <button
                type="button"
                onClick={() => void copyLink(inviteLink)}
                className="underline underline-offset-2 hover:text-zinc-100"
              >
                {t('people.copyLink')}
              </button>
            </p>
          )}
          {invites.length > 0 && (
            <DataTable
              rows={invites}
              rowKey={(inv) => inv.id}
              empty={null}
              columns={[
                {
                  header: t('people.email'),
                  cell: (inv) => <span className="text-zinc-200">{inv.email}</span>,
                },
                {
                  header: t('people.role'),
                  cell: (inv) => <span className="text-zinc-400">{inv.role}</span>,
                },
                {
                  header: t('people.expires'),
                  cell: (inv) => (
                    <span className="text-zinc-500">
                      {new Date(inv.expiresAt * 1000).toLocaleDateString()}
                    </span>
                  ),
                },
                {
                  header: '',
                  srHeader: t('people.actions'),
                  width: 'w-28',
                  cell: (inv) => (
                    <button
                      type="button"
                      onClick={() => void revokeInvite(inv)}
                      disabled={busy === inv.id}
                      className="text-xs text-zinc-500 hover:text-rose-400"
                    >
                      {t('people.revoke')}
                    </button>
                  ),
                },
              ]}
            />
          )}
        </section>
      )}
    </div>
  )
}
