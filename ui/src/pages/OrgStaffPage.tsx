import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { useCurrentOrg } from '../current-org'
import OrgStaffForm from '../components/OrgStaffForm'
import type { Me, Staff } from '../types'

// The org-level editor for one staff member as a whole page (draft 25).
// TeamPage links here instead of opening a modal, so the form — sized for a
// wide dialog — keeps the page to itself behind a narrower column. Only an
// owner or admin may see it; a read-only member who reaches this URL
// directly gets the same refusal the hub would give, not the form.
export default function OrgStaffPage({ me }: { me: Me }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { orgId } = useCurrentOrg()
  const { staffId = '' } = useParams()
  // The same capability read as the Team screen: the membership in the
  // chosen organization carries the role, and only owner/admin may edit.
  const memberships = me.orgs ?? []
  const current = memberships.find((m) => m.orgId === orgId)
  const myRole = current?.role ?? ''
  const canManage = myRole === 'owner' || myRole === 'admin'
  const [staff, setStaff] = useState<Staff | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    if (!orgId || !canManage) return
    setError('')
    setNotFound(false)
    try {
      const roster = await api.get<Staff[]>(`/api/orgs/${orgId}/staff`)
      const found = roster.find((s) => s.id === staffId)
      if (found) {
        setStaff(found)
      } else {
        setStaff(null)
        setNotFound(true)
      }
    } catch (err) {
      setStaff(null)
      setError(err instanceof Error ? err.message : t('staff.loadFailed'))
    }
  }, [orgId, staffId, canManage, t])

  useEffect(() => {
    void load()
  }, [load])

  const title = staff
    ? t('staff.pageTitleOverride', { name: staff.name })
    : t('team.staff')

  return (
    <div className="page-shell">
      <div className="mb-6">
        <p className="eyebrow mb-3">{t('team.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
          {title}
        </h1>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      {!orgId ? (
        <div className="surface rounded-2xl p-12 text-center">
          <p className="mb-2 text-fg-soft">{t('team.noOrgTitle')}</p>
          <p className="text-sm text-fg-subtle">{t('team.noOrgHint')}</p>
        </div>
      ) : !canManage ? (
        <p className="text-sm text-fg-muted">{t('staff.notFound')}</p>
      ) : notFound ? (
        <p className="text-sm text-fg-muted">{t('staff.notFound')}</p>
      ) : staff === null ? (
        <p className="text-sm text-fg-muted">{t('common.loading')}</p>
      ) : (
        <div className="mx-auto max-w-3xl">
          <OrgStaffForm
            staff={staff}
            orgId={orgId}
            onClose={() => navigate('/team')}
            onSaved={() => navigate('/team')}
          />
        </div>
      )}
    </div>
  )
}
