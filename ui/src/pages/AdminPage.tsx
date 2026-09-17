import { useCallback, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api, timeAgo } from '../api'
import { usePoll } from '../hooks'
import DataTable from '../components/DataTable'
import Modal from '../components/Modal'
import BigFiveFields from '../components/BigFiveFields'
import type { Account, Character, KPISnapshot, Org, Staff } from '../types'

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
  const [staff, setStaff] = useState<Staff[] | null>(null)
  const [editor, setEditor] = useState<Staff | 'new' | null>(null)
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

      <div className="mt-8 mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium text-zinc-300">{t('admin.staff')}</h2>
        <button onClick={() => setEditor('new')} className="btn-secondary">
          {t('admin.newStaff')}
        </button>
      </div>
      <p className="mb-3 text-xs text-zinc-500">{t('admin.staffHint')}</p>
      <DataTable
        rows={staff}
        rowKey={(s) => s.id}
        empty={<p className="text-sm text-zinc-500">{t('admin.noStaff')}</p>}
        columns={[
          {
            header: t('staff.name'),
            cell: (s) => <span className="text-zinc-200">{s.name}</span>,
          },
          {
            header: t('staff.slug'),
            cell: (s) => (
              <span className="font-mono text-[12px] text-zinc-500">{s.slug}</span>
            ),
          },
          {
            header: t('staff.locale'),
            cell: (s) => <span className="text-zinc-400">{s.locale || '—'}</span>,
          },
          {
            header: t('staff.age'),
            cell: (s) => (
              <span className="text-zinc-400 tabular-nums">{s.age > 0 ? s.age : '—'}</span>
            ),
          },
          {
            header: t('staff.model'),
            cell: (s) => <span className="text-zinc-400">{s.model || '—'}</span>,
          },
          {
            header: t('staff.updated'),
            cell: (s) => (
              <span className="text-zinc-500">{timeAgo(s.updatedAt)}</span>
            ),
          },
          {
            header: '',
            srHeader: t('admin.people'),
            width: 'w-24',
            cell: (s) => (
              <button
                onClick={() => setEditor(s)}
                className="text-xs text-zinc-500 hover:text-zinc-200"
              >
                {t('common.edit')}
              </button>
            ),
          },
        ]}
      />

      {editor !== null && (
        <Modal
          title={
            editor === 'new' ? t('admin.newStaffTitle') : t('admin.editStaffTitle')
          }
          onClose={() => setEditor(null)}
          wide
        >
          <StaffForm
            staff={editor === 'new' ? null : editor}
            onClose={() => setEditor(null)}
            onSaved={() => {
              setEditor(null)
              void load()
            }}
          />
        </Modal>
      )}
    </div>
  )
}

// StaffForm creates or updates one canonical staff member. The slug is not a
// form field: on create it derives from the name, on edit the stored slug
// travels untouched — the hub keys the upsert on it, so inventing a new one
// in edit mode would mint a second row instead of updating.
function StaffForm({
  staff,
  onClose,
  onSaved,
}: {
  staff: Staff | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(staff?.name ?? '')
  const [locale, setLocale] = useState(staff?.locale ?? '')
  const [age, setAge] = useState(staff && staff.age > 0 ? String(staff.age) : '')
  const [model, setModel] = useState(staff?.model ?? '')
  const [brief, setBrief] = useState(staff?.brief ?? '')
  const [wordBudget, setWordBudget] = useState(
    staff && staff.wordBudget > 0 ? String(staff.wordBudget) : '',
  )
  const [bigFive, setBigFive] = useState<Character>(staff?.bigFive ?? neutralCharacter())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    const payload = {
      slug: staff ? staff.slug : slugify(name),
      name: name.trim(),
      locale: locale.trim(),
      age: Number(age) || 0,
      model: model.trim(),
      brief: brief.trim(),
      wordBudget: Number(wordBudget) || 0,
      bigFive,
    }
    try {
      if (staff) {
        await api.patch<Staff>(`/api/admin/staff/${staff.id}`, payload)
      } else {
        await api.post<Staff>('/api/admin/staff', payload)
      }
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('staff.saveFailed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-4">
      {error && (
        <p className="rounded-lg border border-rose-400/20 px-3 py-2 text-sm text-rose-400">
          {error}
        </p>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm text-zinc-300">
          {t('staff.name')}
          <input
            type="text"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-zinc-100"
          />
        </label>
        <label className="text-sm text-zinc-300">
          {t('staff.locale')}
          <input
            type="text"
            value={locale}
            onChange={(e) => setLocale(e.target.value)}
            placeholder="pl"
            className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-zinc-100"
          />
        </label>
        <label className="text-sm text-zinc-300">
          {t('staff.age')}
          <input
            type="number"
            min={0}
            value={age}
            onChange={(e) => setAge(e.target.value)}
            className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-zinc-100"
          />
        </label>
        <label className="text-sm text-zinc-300">
          {t('staff.wordBudget')}
          <input
            type="number"
            min={0}
            value={wordBudget}
            onChange={(e) => setWordBudget(e.target.value)}
            className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-zinc-100"
          />
        </label>
        <label className="text-sm text-zinc-300 sm:col-span-2">
          {t('staff.model')}
          <input
            type="text"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 font-mono text-[12px] text-zinc-100"
          />
        </label>
      </div>

      <section className="rounded-lg border border-white/10 p-4">
        <h3 className="text-sm font-medium text-zinc-200">{t('staff.bigFive')}</h3>
        <p className="mt-1 text-xs text-zinc-500">{t('staff.bigFiveHint')}</p>
        <div className="mt-3">
          <BigFiveFields value={bigFive} onChange={setBigFive} />
        </div>
      </section>

      <label className="text-sm text-zinc-300">
        {t('staff.brief')}
        <textarea
          rows={3}
          value={brief}
          onChange={(e) => setBrief(e.target.value)}
          className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-zinc-100"
        />
      </label>

      <div className="mt-2 flex items-center justify-end gap-3">
        <button type="button" onClick={onClose} className="btn-secondary">
          {t('common.cancel')}
        </button>
        <button type="submit" disabled={busy} className="btn-primary">
          {busy ? t('common.loading') : t('common.save')}
        </button>
      </div>
    </form>
  )
}

function neutralCharacter(): Character {
  return {
    openness: 0.5,
    conscientiousness: 0.5,
    extraversion: 0.5,
    agreeableness: 0.5,
    neuroticism: 0.5,
  }
}

function slugify(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
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
