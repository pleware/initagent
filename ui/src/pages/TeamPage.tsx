import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { api, timeAgo } from '../api'
import { useCurrentOrg } from '../current-org'
import DataTable from '../components/DataTable'
import { HubError } from '../components/PlanWall'
import type { Me, OrgInvite, OrgMember, Staff } from '../types'

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
export default function TeamPage({
  me,
  onChanged,
}: {
  me: Me
  onChanged: () => void
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const memberships = me.orgs ?? []
  // The sidebar Organizations section is the org switcher; this screen
  // reads the same cockpit-wide choice instead of keeping its own.
  const { orgId } = useCurrentOrg()
  const [members, setMembers] = useState<OrgMember[] | null>(null)
  const [invites, setInvites] = useState<OrgInvite[]>([])
  const [staff, setStaff] = useState<Staff[] | null>(null)
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
      setStaff([])
      return
    }
    try {
      setMembers(await api.get<OrgMember[]>(`/api/orgs/${orgId}/members`))
      setStaff(await api.get<Staff[]>(`/api/orgs/${orgId}/staff`))
      if (canManage) {
        setInvites(await api.get<OrgInvite[]>(`/api/orgs/${orgId}/invites`))
      } else {
        setInvites([])
      }
      setError('')
    } catch (err) {
      setMembers([])
      setInvites([])
      setStaff([])
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
      setError(err instanceof Error ? err.message : t('team.roleChangeFailed'))
    } finally {
      setBusy('')
    }
  }

  const remove = async (member: OrgMember) => {
    const leaving = member.accountId === me.accountId
    const question = leaving
      ? t('team.confirmLeave', { org: current?.name })
      : t('team.confirmRemove', { email: member.email, org: current?.name })
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
      setError(err instanceof Error ? err.message : t('team.removeFailed'))
    } finally {
      setBusy('')
    }
  }

  const resetStaff = async (s: Staff) => {
    if (!window.confirm(t('team.resetOverrideConfirm', { name: s.name }))) return
    setBusy(s.id)
    setError('')
    try {
      await api.del(`/api/orgs/${orgId}/staff/${s.id}/override`)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('team.resetFailed'))
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
    if (!window.confirm(t('team.confirmRevoke', { email: invite.email }))) return
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
      window.prompt(t('team.copyPrompt'), link)
    }
  }

  const rename = async () => {
    const name = window.prompt(t('team.renamePrompt'), current?.name ?? '')
    if (name === null || name.trim() === '') return
    try {
      await api.patch(`/api/orgs/${orgId}`, { name })
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('team.renameFailed'))
    }
  }

  if (memberships.length === 0) {
    return (
      <div className="page-shell">
        <div className="mb-6">
          <p className="eyebrow mb-3">{t('team.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
            {t('team.title')}
          </h1>
        </div>
        <div className="surface rounded-2xl p-12 text-center">
          <p className="mb-2 text-fg-soft">{t('team.noOrgTitle')}</p>
          <p className="text-sm text-fg-subtle">{t('team.noOrgHint')}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="page-shell">
      <div className="mb-6 flex items-end justify-between">
        <div>
          <p className="eyebrow mb-3">{t('team.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
            {t('team.title')}
          </h1>
          <p className="mt-1 text-sm text-fg-muted">
            {t('team.subtitle', { org: current?.name, role: myRole })}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {canManage && (
            <button onClick={rename} className="btn-secondary">
              {t('team.rename')}
            </button>
          )}
        </div>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <DataTable
        rows={members}
        rowKey={(m) => m.accountId}
        empty={<p className="text-sm text-fg-subtle">{t('team.empty')}</p>}
        columns={[
          {
            header: t('team.email'),
            cell: (m) => (
              <span className="text-fg">
                {m.email}
                {m.accountId === me.accountId && (
                  <span className="ml-2 text-xs text-fg-subtle">
                    {t('team.you')}
                  </span>
                )}
              </span>
            ),
          },
          {
            header: t('team.role'),
            cell: (m) =>
              canManage ? (
                <SimpleSelect
                  value={m.role}
                  disabled={busy === m.accountId}
                  onValueChange={(role) => changeRole(m.accountId, role)}
                  aria-label={`${t('team.role')}: ${m.email}`}
                  items={roles.map((role) => ({ value: role, label: t('orgs.roles.' + role, { defaultValue: role }) }))}
                />
              ) : (
                <span className="text-fg-muted">
                  {t('orgs.roles.' + m.role, { defaultValue: m.role })}
                </span>
              ),
          },
          {
            header: t('team.joined'),
            cell: (m) => (
              <span className="text-fg-subtle">{timeAgo(m.createdAt)}</span>
            ),
          },
          {
            header: '',
            srHeader: t('team.actions'),
            width: 'w-28',
            cell: (m) =>
              // Leaving needs no administrative right, so the button is here
              // for your own row whatever your role is.
              canManage || m.accountId === me.accountId ? (
                <button
                  onClick={() => remove(m)}
                  disabled={busy === m.accountId}
                  className="text-xs text-fg-subtle hover:text-fail-fg"
                >
                  {m.accountId === me.accountId
                    ? t('team.leave')
                    : t('team.remove')}
                </button>
              ) : null,
          },
        ]}
      />

      {canManage && (
        <section className="mt-10">
          <h2 className="text-lg font-semibold text-fg-strong">{t('team.inviteTitle')}</h2>
          <p className="mt-1 text-sm text-fg-muted">{t('team.inviteHint')}</p>
          <form
            className="mt-4 flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              void sendInvite()
            }}
          >
            <label className="min-w-56 flex-1 text-sm text-fg-soft">
              {t('team.email')}
              <input
                type="email"
                required
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
              />
            </label>
            <label className="text-sm text-fg-soft">
              {t('team.role')}
              <div className="mt-1">
                <SimpleSelect
                  value={inviteRole}
                  onValueChange={setInviteRole}
                  aria-label={t('team.role')}
                  items={roles.map((role) => ({ value: role, label: t('orgs.roles.' + role, { defaultValue: role }) }))}
                />
              </div>
            </label>
            <button type="submit" disabled={busy === 'invite'} className="btn-primary">
              {busy === 'invite' ? t('common.loading') : t('team.sendInvite')}
            </button>
          </form>
          <div className="mt-4">
            <HubError error={inviteError} fallback={t('team.inviteFailed')} />
          </div>
          {inviteLink && (
            <p className="mt-3 text-sm text-fg-soft">
              {t('team.inviteLinkOnce')}{' '}
              <button
                type="button"
                onClick={() => void copyLink(inviteLink)}
                className="underline underline-offset-2 hover:text-fg-strong"
              >
                {t('team.copyLink')}
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
                  header: t('team.email'),
                  cell: (inv) => <span className="text-fg">{inv.email}</span>,
                },
                {
                  header: t('team.role'),
                  cell: (inv) => (
                    <span className="text-fg-muted">
                      {t('orgs.roles.' + inv.role, { defaultValue: inv.role })}
                    </span>
                  ),
                },
                {
                  header: t('team.expires'),
                  cell: (inv) => (
                    <span className="text-fg-subtle">
                      {new Date(inv.expiresAt * 1000).toLocaleDateString()}
                    </span>
                  ),
                },
                {
                  header: '',
                  srHeader: t('team.actions'),
                  width: 'w-28',
                  cell: (inv) => (
                    <button
                      type="button"
                      onClick={() => void revokeInvite(inv)}
                      disabled={busy === inv.id}
                      className="text-xs text-fg-subtle hover:text-fail-fg"
                    >
                      {t('team.revoke')}
                    </button>
                  ),
                },
              ]}
            />
          )}
        </section>
      )}

      <section className="mt-10">
        <h2 className="text-lg font-semibold text-fg-strong">{t('team.staff')}</h2>
        <p className="mt-1 text-sm text-fg-muted">
          {t('team.staffHint', { org: current?.name })}
        </p>
        <div className="mt-4">
          <DataTable
            rows={staff}
            rowKey={(s) => s.id}
            empty={<p className="text-sm text-fg-subtle">{t('team.staffEmpty')}</p>}
            columns={[
              {
                header: t('staff.name'),
                cell: (s) => (
                  <span className="text-fg">
                    {s.name}{' '}
                    <span className="ml-1 rounded-full border border-accent/30 px-2 py-0.5 text-xs text-accent">
                      {t('team.staffBadge')}
                    </span>
                  </span>
                ),
              },
              {
                header: t('staff.avatarModel3d'),
                cell: (s) => <span className="text-fg-muted">{s.avatarModel3d || '—'}</span>,
              },
              {
                header: t('staff.wordBudget'),
                cell: (s) => (
                  <span className="text-fg-muted tabular-nums">
                    {s.wordBudget > 0 ? s.wordBudget : '—'}
                  </span>
                ),
              },
              ...(canManage
                ? [
                    {
                      header: '',
                      srHeader: t('team.actions'),
                      width: 'w-28',
                      cell: (s: Staff) => (
                        <div className="flex items-center gap-3">
                          <button
                            onClick={() => navigate(`/team/staff/${s.id}`)}
                            className="text-xs text-fg-subtle hover:text-fg"
                          >
                            {t('common.edit')}
                          </button>
                          <button
                            onClick={() => void resetStaff(s)}
                            disabled={busy === s.id}
                            className="text-xs text-fg-subtle hover:text-fail-fg"
                          >
                            {t('team.resetOverride')}
                          </button>
                        </div>
                      ),
                    },
                  ]
                : []),
            ]}
          />
        </div>
      </section>
    </div>
  )
}
