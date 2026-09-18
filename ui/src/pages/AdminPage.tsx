import { useCallback, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, timeAgo } from '../api'
import { usePoll } from '../hooks'
import DataTable from '../components/DataTable'
import Modal from '../components/Modal'
import { slugify } from '../components/StaffEditor'
import type { Account, KPISnapshot, Org, Staff } from '../types'

// The operator's view of the installation they run: every account, every
// organization (drafts 08, 17).
//
// It stops at the roster size for each org. Reading who is inside a
// customer's organization is an org-level capability that running the hub
// does not grant — draft 09 has not decided whether a hub admin has any path
// into customer data, and a screen is a poor place to answer it by accident.
export default function AdminPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [accounts, setAccounts] = useState<Account[] | null>(null)
  const [orgs, setOrgs] = useState<Org[] | null>(null)
  const [kpis, setKpis] = useState<KPISnapshot | null>(null)
  const [staff, setStaff] = useState<Staff[] | null>(null)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [createBusy, setCreateBusy] = useState(false)
  const [createError, setCreateError] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      const [a, o, k, s] = await Promise.all([
        api.get<Account[]>('/api/admin/accounts'),
        api.get<Org[]>('/api/admin/orgs'),
        api.get<KPISnapshot>('/api/admin/kpis'),
        api.get<Staff[]>('/api/admin/staff'),
      ])
      setAccounts(a)
      setOrgs(o)
      setKpis(k)
      setStaff(s)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('admin.loadFailed'))
    }
  }, [t])

  usePoll(load, 30_000)

  // Create mints the row with only a slug and a name, then hands the rest
  // of the tuning to the staff page. The slug is derived from the name —
  // the hub keys the upsert on it, and the page echoes it on every PATCH.
  const create = async (e: FormEvent) => {
    e.preventDefault()
    const name = newName.trim()
    if (!name) return
    setCreateBusy(true)
    setCreateError('')
    try {
      const created = await api.post<Staff>('/api/admin/staff', {
        slug: slugify(name),
        name,
      })
      setCreating(false)
      setNewName('')
      navigate(`/admin/staff/${created.id}`)
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : t('staff.saveFailed'))
    } finally {
      setCreateBusy(false)
    }
  }

  return (
    <div className="page-shell">
      <div className="mb-6">
        <p className="eyebrow mb-3">{t('admin.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
          {t('admin.title')}
        </h1>
        <p className="mt-1 text-sm text-fg-muted">{t('admin.subtitle')}</p>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <h2 className="mb-1 text-sm font-medium text-fg-soft">{t('admin.kpis')}</h2>
      <p className="mb-3 text-xs text-fg-subtle">{t('admin.kpisHint')}</p>
      <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <KPI label={t('admin.ctaOpenApp')} value={kpis?.acquisition.ctaOpenApp} />
        <KPI label={t('admin.ctaSelfHost')} value={kpis?.acquisition.ctaSelfHost} />
        <KPI label={t('admin.signups')} value={kpis?.acquisition.signups} />
        <KPI label={t('admin.inviteRedeems')} value={kpis?.acquisition.inviteRedeems} />
        <KPI label={t('admin.signupRate')} value={pct(kpis?.acquisition.signupRate)} />
        <KPI label={t('admin.orgsWithProject')} value={kpis?.activation.orgsWithProject} />
        <KPI label={t('admin.orgsWithWorker')} value={kpis?.activation.orgsWithWorker} />
        <KPI label={t('admin.orgsWithTask')} value={kpis?.activation.orgsWithTask} />
        <KPI label={t('admin.timeToValue')} value={hours(kpis?.activation.timeToValueHours)} />
        <KPI label={t('admin.planLimitHits')} value={kpis?.hygiene.planLimitHits} />
        <KPI label={t('admin.idleWarned')} value={kpis?.hygiene.idleWarned} />
        <KPI label={t('admin.idleDeleted')} value={kpis?.hygiene.idleDeleted} />
        <KPI label={t('admin.d7')} value={ratio(kpis?.retention.d7Returned, kpis?.retention.d7Eligible, kpis?.retention.d7Rate)} />
        <KPI label={t('admin.d30')} value={ratio(kpis?.retention.d30Returned, kpis?.retention.d30Eligible, kpis?.retention.d30Rate)} />
        <KPI label={t('admin.paidOrgs')} value={kpis?.conversion.paidOrgs} />
        <KPI
          label={t('admin.mrr')}
          value={kpis?.conversion.mrrAvailable ? kpis.conversion.paidOrgs : t('admin.mrrPending')}
        />
        <KPI label={t('admin.onlineWorkers')} value={kpis?.cost.onlineWorkers} />
      </div>

      <h2 className="mb-3 text-sm font-medium text-fg-soft">
        {t('admin.accounts')}
      </h2>
      <DataTable
        rows={accounts}
        rowKey={(a) => a.id}
        empty={<p className="text-sm text-fg-subtle">{t('admin.noAccounts')}</p>}
        columns={[
          {
            header: t('admin.email'),
            cell: (a) => <span className="text-fg">{a.email}</span>,
          },
          {
            header: t('admin.role'),
            cell: (a) =>
              a.isAdmin ? (
                <span className="rounded-full border border-accent/30 px-2 py-0.5 text-xs text-accent">
                  {t('admin.platformAdmin')}
                </span>
              ) : (
                <span className="text-fg-subtle">{t('admin.member')}</span>
              ),
          },
          {
            header: t('admin.account'),
            cell: (a) => (
              <span className="font-mono text-[12px] text-fg-subtle">{a.id}</span>
            ),
          },
          {
            header: t('admin.created'),
            cell: (a) => (
              <span className="text-fg-subtle">{timeAgo(a.createdAt)}</span>
            ),
          },
        ]}
      />

      <h2 className="mt-8 mb-3 text-sm font-medium text-fg-soft">
        {t('admin.organizations')}
      </h2>
      <DataTable
        rows={orgs}
        rowKey={(o) => o.id}
        empty={<p className="text-sm text-fg-subtle">{t('admin.noOrgs')}</p>}
        columns={[
          {
            header: t('admin.name'),
            cell: (o) => <span className="text-fg">{o.name}</span>,
          },
          {
            header: t('admin.people'),
            cell: (o) => <span className="text-fg-soft">{o.members}</span>,
          },
          {
            header: t('admin.organization'),
            cell: (o) => (
              <span className="font-mono text-[12px] text-fg-subtle">{o.id}</span>
            ),
          },
          {
            header: t('admin.created'),
            cell: (o) => (
              <span className="text-fg-subtle">{timeAgo(o.createdAt)}</span>
            ),
          },
        ]}
      />

      <div className="mt-8 mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium text-fg-soft">{t('admin.staff')}</h2>
        <button onClick={() => setCreating(true)} className="btn-secondary">
          {t('admin.newStaff')}
        </button>
      </div>
      <p className="mb-3 text-xs text-fg-subtle">{t('admin.staffHint')}</p>
      <DataTable
        rows={staff}
        rowKey={(s) => s.id}
        empty={<p className="text-sm text-fg-subtle">{t('admin.noStaff')}</p>}
        columns={[
          {
            header: t('staff.name'),
            cell: (s) => <span className="text-fg">{s.name}</span>,
          },
          {
            header: t('staff.slug'),
            cell: (s) => (
              <span className="font-mono text-[12px] text-fg-subtle">{s.slug}</span>
            ),
          },
          {
            header: t('staff.locale'),
            cell: (s) => <span className="text-fg-muted">{s.locale || '—'}</span>,
          },
          {
            header: t('staff.age'),
            cell: (s) => (
              <span className="text-fg-muted tabular-nums">{s.age > 0 ? s.age : '—'}</span>
            ),
          },
          {
            header: t('staff.model'),
            cell: (s) => <span className="text-fg-muted">{s.model || '—'}</span>,
          },
          {
            header: t('staff.updated'),
            cell: (s) => (
              <span className="text-fg-subtle">{timeAgo(s.updatedAt)}</span>
            ),
          },
          {
            header: '',
            srHeader: t('admin.people'),
            width: 'w-24',
            cell: (s) => (
              <button
                onClick={() => navigate(`/admin/staff/${s.id}`)}
                className="text-xs text-fg-subtle hover:text-fg"
              >
                {t('common.edit')}
              </button>
            ),
          },
        ]}
      />

      {creating && (
        <Modal title={t('staff.createTitle')} onClose={() => setCreating(false)}>
          <form onSubmit={(e) => void create(e)} className="flex flex-col gap-4">
            <p className="text-xs text-fg-subtle">{t('staff.createHint')}</p>
            {createError && (
              <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
                {createError}
              </p>
            )}
            <label className="text-sm text-fg-soft">
              {t('staff.name')}
              <input
                type="text"
                required
                autoFocus
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
              />
            </label>
            <div className="mt-2 flex items-center justify-end gap-3">
              <button
                type="button"
                onClick={() => setCreating(false)}
                className="btn-secondary"
              >
                {t('common.cancel')}
              </button>
              <button
                type="submit"
                disabled={createBusy || !newName.trim()}
                className="btn-primary"
              >
                {createBusy ? t('common.loading') : t('common.add')}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}

function KPI({ label, value }: { label: string; value: string | number | undefined }) {
  return (
    <div className="rounded-lg border border-line-2 bg-fill-sunken px-3 py-2.5">
      <p className="text-[11px] tracking-wide text-fg-subtle uppercase">{label}</p>
      <p className="mt-1 text-lg font-medium text-fg-strong tabular-nums">
        {value ?? '—'}
      </p>
    </div>
  )
}

function pct(rate: number | undefined): string {
  if (rate == null) return '—'
  return `${Math.round(rate * 100)}%`
}

function hours(n: number | undefined): string {
  if (n == null) return '—'
  return n.toFixed(1)
}

function ratio(num: number | undefined, den: number | undefined, rate: number | undefined): string {
  if (den == null || num == null) return '—'
  if (den === 0) return '—'
  return `${num}/${den} (${pct(rate)})`
}
