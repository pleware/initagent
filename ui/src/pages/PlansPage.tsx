import { FormEvent, useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { HubError } from '../components/PlanWall'
import { PLAN_BY_SLUG, PLAN_ORDER, PERSON_USD, type PlanSlug } from '../lib/org-plans.gen'
import type { Me } from '../types'

type OrgBilling = {
  orgId: string
  plan: string
  people: number
  checkoutReady: boolean
  invoicesReady: boolean
  name: string
  taxNo: string
  street: string
  city: string
  postCode: string
  country: string
  email: string
}

const LABELS: Record<PlanSlug, string> = {
  free: 'Free',
  starter: 'Starter',
  team: 'Team',
  enterprise: 'Enterprise',
}

export default function PlansPage({ me }: { me: Me }) {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const memberships = me.orgs ?? []
  const [orgId] = useState(memberships[0]?.orgId ?? '')
  const [billing, setBilling] = useState<OrgBilling | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState('')
  const current = memberships.find((m) => m.orgId === orgId)
  const canPay = current?.role === 'owner' || current?.role === 'admin'

  const load = useCallback(async () => {
    if (!orgId) {
      setBilling(null)
      return
    }
    try {
      setBilling(await api.get<OrgBilling>(`/api/orgs/${orgId}/billing`))
      setError(null)
    } catch (err) {
      setError(err)
    }
  }, [orgId])

  useEffect(() => {
    void load()
  }, [load])

  const saveBuyer = async (event: FormEvent) => {
    event.preventDefault()
    if (!billing) return
    setBusy('save')
    setError(null)
    try {
      setBilling(await api.patch<OrgBilling>(`/api/orgs/${orgId}/billing`, {
        name: billing.name,
        taxNo: billing.taxNo,
        street: billing.street,
        city: billing.city,
        postCode: billing.postCode,
        country: billing.country || 'PL',
        email: billing.email || me.email,
      }))
    } catch (err) {
      setError(err)
    } finally {
      setBusy('')
    }
  }

  const checkout = async (plan: PlanSlug) => {
    setBusy(plan)
    setError(null)
    try {
      const sess = await api.post<{ url: string }>(`/api/orgs/${orgId}/checkout`, { plan })
      window.location.assign(sess.url)
    } catch (err) {
      setError(err)
      setBusy('')
    }
  }

  if (me.offering !== 'hosted') {
    return (
      <div className="page-shell">
        <p className="eyebrow mb-3">{t('plans.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-zinc-100">{t('plans.title')}</h1>
        <p className="mt-4 max-w-xl text-sm text-zinc-500">{t('plans.selfhost')}</p>
      </div>
    )
  }

  if (memberships.length === 0) {
    return (
      <div className="page-shell">
        <p className="eyebrow mb-3">{t('plans.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-zinc-100">{t('plans.title')}</h1>
        <p className="mt-4 max-w-xl text-sm text-zinc-500">{t('plans.noOrg')}</p>
      </div>
    )
  }

  return (
    <div className="page-shell max-w-5xl">
      <p className="eyebrow mb-3">{t('plans.eyebrow')}</p>
      <h1 className="text-3xl font-semibold tracking-[-0.04em] text-zinc-100">{t('plans.title')}</h1>
      <p className="mt-2 max-w-2xl text-sm text-zinc-400">{t('plans.subtitle', { org: current?.name })}</p>

      {params.get('paid') === '1' && (
        <p className="mt-4 rounded-lg border border-lime-400/20 bg-lime-400/5 px-3 py-2 text-sm text-lime-200">{t('plans.paid')}</p>
      )}
      {params.get('canceled') === '1' && (
        <p className="mt-4 rounded-lg border border-white/10 px-3 py-2 text-sm text-zinc-400">{t('plans.canceled')}</p>
      )}

      <div className="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {PLAN_ORDER.map((id) => {
          const cfg = PLAN_BY_SLUG[id]
          const currentPlan = billing?.plan === id
          const price = cfg.charge.kind === 'free' ? '$0' : cfg.charge.kind === 'contact' ? t('plans.talk') : `$${PERSON_USD}`
          return (
            <article key={id} className={`flex flex-col rounded-2xl border p-5 ${currentPlan ? 'border-lime-400/35 bg-lime-400/5' : 'border-white/10 bg-white/[0.02]'}`}>
              <h2 className="text-base font-semibold text-zinc-100">{LABELS[id]}</h2>
              <p className="mt-3 text-2xl font-semibold text-white">{price}</p>
              <p className="mt-1 text-xs text-zinc-500">{cfg.charge.perPerson ? t('plans.perPerson') : id === 'free' ? t('plans.onePerson') : t('plans.contract')}</p>
              <ul className="mt-4 flex-1 space-y-1 text-sm text-zinc-400">
                {cfg.limits.people === 1 && <li>{t('plans.capPeople', { n: 1 })}</li>}
                {cfg.limits.projects > 0 && <li>{t('plans.capProjects', { n: cfg.limits.projects })}</li>}
                {cfg.limits.projects === 0 && <li>{t('plans.noProjectCap')}</li>}
                {cfg.limits.workersPerProject > 0 && <li>{t('plans.capMachines', { n: cfg.limits.workersPerProject })}</li>}
              </ul>
              {currentPlan && <p className="mt-4 text-xs font-medium text-lime-300">{t('plans.current')}</p>}
              {!currentPlan && canPay && id !== 'free' && id !== 'enterprise' && (
                <button
                  type="button"
                  disabled={busy !== '' || !billing?.checkoutReady}
                  onClick={() => void checkout(id)}
                  className="btn-primary mt-5"
                >
                  {busy === id ? t('common.loading') : t('plans.choose', { plan: LABELS[id] })}
                </button>
              )}
              {id === 'enterprise' && !currentPlan && (
                <a href="https://initagent.dev/plans" className="mt-5 text-center text-sm underline-offset-2 hover:underline">{t('plans.contact')}</a>
              )}
            </article>
          )
        })}
      </div>

      {canPay && (
        <form className="mt-10 max-w-2xl space-y-4" onSubmit={(e) => void saveBuyer(e)}>
          <h2 className="text-lg font-semibold text-zinc-100">{t('plans.invoiceTitle')}</h2>
          <p className="text-sm text-zinc-500">{t('plans.invoiceHint')}</p>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t('plans.company')} value={billing?.name ?? ''} onChange={(name) => setBilling((b) => b && { ...b, name })} />
            <Field label={t('plans.nip')} value={billing?.taxNo ?? ''} onChange={(taxNo) => setBilling((b) => b && { ...b, taxNo })} />
            <Field label={t('plans.street')} value={billing?.street ?? ''} onChange={(street) => setBilling((b) => b && { ...b, street })} className="sm:col-span-2" />
            <Field label={t('plans.postCode')} value={billing?.postCode ?? ''} onChange={(postCode) => setBilling((b) => b && { ...b, postCode })} />
            <Field label={t('plans.city')} value={billing?.city ?? ''} onChange={(city) => setBilling((b) => b && { ...b, city })} />
            <Field label={t('plans.country')} value={billing?.country || 'PL'} onChange={(country) => setBilling((b) => b && { ...b, country })} />
            <Field label={t('plans.email')} value={billing?.email || me.email || ''} onChange={(email) => setBilling((b) => b && { ...b, email })} />
          </div>
          <button type="submit" disabled={busy === 'save' || !billing} className="btn-secondary">
            {busy === 'save' ? t('common.loading') : t('plans.saveInvoice')}
          </button>
          {!billing?.checkoutReady && (
            <p className="text-sm text-zinc-500">{t('plans.notWired')}</p>
          )}
        </form>
      )}

      <div className="mt-6">
        <HubError error={error} fallback={t('plans.failed')} />
      </div>
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  className = '',
}: {
  label: string
  value: string
  onChange: (value: string) => void
  className?: string
}) {
  return (
    <label className={`text-sm text-zinc-300 ${className}`}>
      {label}
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="mt-1 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-zinc-100"
      />
    </label>
  )
}
