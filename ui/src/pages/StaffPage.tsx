import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, ApiError } from '../api'
import StaffEditor from '../components/StaffEditor'
import type { Staff } from '../types'

// The platform operator's full-page editor for one canonical staff member
// (drafts 08, 17). AdminPage links here instead of opening a modal, so the
// whole editor gets the whole page. The slug is context, not a field: the
// hub keys the upsert on it, so the PATCH echoes the stored slug instead of
// inventing a new one — a changed slug would mint a second row.
export default function StaffPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const [staff, setStaff] = useState<Staff | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    if (!id) {
      setNotFound(true)
      return
    }
    try {
      setStaff(await api.get<Staff>(`/api/admin/staff/${encodeURIComponent(id)}`))
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setNotFound(true)
      } else {
        setError(err instanceof Error ? err.message : t('staff.loadFailed'))
      }
    }
  }, [id, t])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="page-shell">
      <div className="mb-6">
        <p className="eyebrow mb-3">{t('admin.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
          {t('staff.pageTitle')}
        </h1>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      {notFound ? (
        <p className="text-sm text-fg-muted">{t('staff.notFound')}</p>
      ) : staff === null ? (
        <p className="text-sm text-fg-muted">{t('common.loading')}</p>
      ) : (
        <>
          <p className="mb-4 text-sm text-fg-muted">
            <span className="font-mono text-[12px] text-fg-subtle">{staff.slug}</span>
          </p>
          <StaffEditor
            staff={staff}
            submit={(fields) =>
              api.patch<Staff>(`/api/admin/staff/${encodeURIComponent(staff.id)}`, {
                ...fields,
                slug: staff.slug,
              })
            }
            onSaved={() => navigate('/admin')}
            onCancel={() => navigate('/admin')}
          />
        </>
      )}
    </div>
  )
}
