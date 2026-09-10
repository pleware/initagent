import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, timeAgo } from '../api'
import { usePoll } from '../hooks'
import DataTable from '../components/DataTable'
import type { Account, KPISnapshot, Org } from '../types'

// The operator's view of the installation they run: every account, every
// organization (drafts 08, 17).
//
// It stops at the roster size for each org. Reading who is inside a
// customer's organization is an org-level capability that running the hub
// does not grant — draft 09 has not decided whether a hub admin has any path
// into customer data, and a screen is a poor place to answer it by accident.
export default function AdminPage() {
  const { t } = useTranslation()
  const [accounts, setAccounts] = useState<Account[] | null>(null)
  const [orgs, setOrgs] = useState<Org[] | null>(null)
  const [kpis, setKpis] = useState<KPISnapshot | null>(null)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      const [a, o, k] = await Promise.all([
        api.get<Account[]>('/api/admin/accounts'),
        api.get<Org[]>('/api/admin/orgs'),
        api.get<KPISnapshot>('/api/admin/kpis'),
      ])
      setAccounts(a)
      setOrgs(o)
      setKpis(k)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('admin.loadFailed'))
    }
  }, [t])

  usePoll(load, 30_000)

  return (
    <div className="page-shell">
      <div className="mb-6">
        <p className="eyebrow mb-3">{t('admin.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-zinc-100">
          {t('admin.title')}
        </h1>
        <p className="mt-1 text-sm text-zinc-400">{t('admin.subtitle')}</p>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-rose-400/20 px-3 py-2 text-sm text-rose-400">
          {error}
        </p>
      )}

      <h2 className="mb-1 text-sm font-medium text-zinc-300">{t('admin.kpis')}</h2>
      <p className="mb-3 text-xs text-zinc-500">{t('admin.kpisHint')}</p>
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

      <h2 className="mb-3 text-sm font-medium text-zinc-300">
        {t('admin.accounts')}
      </h2>
      <DataTable
        rows={accounts}
        rowKey={(a) => a.id}
        empty={<p className="text-sm text-zinc-500">{t('admin.noAccounts')}</p>}
        columns={[
          {
            header: t('admin.email'),
            cell: (a) => <span className="text-zinc-200">{a.email}</span>,
          },
          {
            header: t('admin.role'),
            cell: (a) =>
              a.isAdmin ? (
                <span className="rounded-full border border-lime-400/30 px-2 py-0.5 text-xs text-lime-300">
                  {t('admin.platformAdmin')}
                </span>
              ) : (
                <span className="text-zinc-500">{t('admin.member')}</span>
              ),
          },
          {
            header: t('admin.account'),
            cell: (a) => (
              <span className="font-mono text-[12px] text-zinc-500">{a.id}</span>
            ),
          },
          {
            header: t('admin.created'),
            cell: (a) => (
              <span className="text-zinc-500">{timeAgo(a.createdAt)}</span>
            ),
          },
        ]}
      />

      <h2 className="mt-8 mb-3 text-sm font-medium text-zinc-300">
        {t('admin.organizations')}
      </h2>
      <DataTable
        rows={orgs}
        rowKey={(o) => o.id}
        empty={<p className="text-sm text-zinc-500">{t('admin.noOrgs')}</p>}
        columns={[
          {
            header: t('admin.name'),
            cell: (o) => <span className="text-zinc-200">{o.name}</span>,
          },
          {
            header: t('admin.people'),
            cell: (o) => <span className="text-zinc-300">{o.members}</span>,
          },
          {
            header: t('admin.organization'),
            cell: (o) => (
              <span className="font-mono text-[12px] text-zinc-500">{o.id}</span>
            ),
          },
          {
            header: t('admin.created'),
            cell: (o) => (
              <span className="text-zinc-500">{timeAgo(o.createdAt)}</span>
            ),
          },
        ]}
      />
    </div>
  )
}

function KPI({ label, value }: { label: string; value: string | number | undefined }) {
  return (
    <div className="rounded-lg border border-white/8 bg-zinc-950/40 px-3 py-2.5">
      <p className="text-[11px] tracking-wide text-zinc-500 uppercase">{label}</p>
      <p className="mt-1 text-lg font-medium text-zinc-100 tabular-nums">
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
