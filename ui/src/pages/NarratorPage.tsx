import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, ApiError } from '../api'
import StaffEditor from '../components/StaffEditor'
import type { Staff } from '../types'

// The platform operator's full-page editor for one box's narrator (58):
// the box-scoped staff row the box presents to its people. BoxesPage links
// here instead of opening the old read-only preview, so the whole editor
// gets the whole page. The identity is context, not a field: the hub keys
// the row on the narrator slug st_b_pi, so the PATCH sends only the nine
// editable fields — no slug, no scope, no box id.
export default function NarratorPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const [narrator, setNarrator] = useState<Staff | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    if (!id) {
      setNotFound(true)
      return
    }
    try {
      setNarrator(await api.get<Staff>(`/api/boxes/${id}/narrator`))
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setNotFound(true)
      } else {
        setError(err instanceof Error ? err.message : t('narrator.loadFailed'))
      }
    }
  }, [id, t])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="page-shell">
      <div className="mb-6">
        <p className="eyebrow mb-3">{t('boxes.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
          {t('narrator.pageTitle')}
        </h1>
        <p className="mt-1 text-sm text-fg-muted">{t('narrator.subtitle')}</p>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      {notFound ? (
        <p className="text-sm text-fg-muted">{t('narrator.notFound')}</p>
      ) : narrator === null ? (
        <p className="text-sm text-fg-muted">{t('common.loading')}</p>
      ) : (
        <>
          <p className="mb-4 text-sm text-fg-muted">
            <span className="font-mono text-[12px] text-fg-subtle">{narrator.slug}</span>
            <span className="mx-2 text-fg-faint">·</span>
            <span className="font-mono text-[12px] text-fg-subtle">{id}</span>
          </p>
          <StaffEditor
            staff={narrator}
            submit={(fields) =>
              api.patch<Staff>(`/api/boxes/${id}/narrator`, fields)
            }
            onSaved={() => navigate('/boxes')}
            onCancel={() => navigate('/boxes')}
          />
        </>
      )}
    </div>
  )
}
