import { FormEvent, useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { useCurrentOrg } from '../current-org'
import { HubError } from '../components/PlanWall'
import { PLAN_BY_SLUG, PLAN_ORDER, type PlanSlug } from '../lib/org-plans.gen'
import type { Me } from '../types'

type OrgBilling = {
  orgId: string
  plan: string
  people: number
  checkoutReady: boolean
  invoicesReady: boolean
  kind: string
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

type BillingField = 'kind' | 'name' | 'taxNo' | 'street' | 'city' | 'postCode' | 'country' | 'email'
type BillingErrors = Partial<Record<BillingField, string>>

// Mirrors billing.ValidNIP: 10 digits + the Polish checksum, ignoring
// spaces and dashes. Keeps the client from sending a bad NIP and showing
// the raw server error at the bottom of the page.
function validNip(raw: string): boolean {
  const digits: number[] = []
  for (const r of raw) {
    if (r === ' ' || r === '-') continue
    if (r < '0' || r > '9') return false
    digits.push(r.charCodeAt(0) - 48)
  }
  if (digits.length !== 10) return false
  const weights = [6, 5, 7, 2, 3, 4, 5, 6, 7]
  let sum = 0
  for (let i = 0; i < 9; i += 1) sum += digits[i] * weights[i]
  const check = sum % 11
  if (check === 10) return false
  return check === digits[9]
}

export default function PlansPage({ me }: { me: Me }) {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const memberships = me.orgs ?? []
  const { orgId } = useCurrentOrg()
  const [billing, setBilling] = useState<OrgBilling | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState('')
  const [fieldErrors, setFieldErrors] = useState<BillingErrors>({})
  const current = memberships.find((m) => m.orgId === orgId)
  const canPay = current?.role === 'owner' || current?.role === 'admin'

  // Same rules as billing.Buyer.Validate, translated for the form.
  const validate = (): BillingErrors => {
    const errors: BillingErrors = {}
    const isCompany = (billing?.kind ?? '') !== 'individual'
    const name = (billing?.name ?? '').trim()
    const street = (billing?.street ?? '').trim()
    const city = (billing?.city ?? '').trim()
    const postCode = (billing?.postCode ?? '').trim()
    const email = (billing?.email || me.email || '').trim()
    const country = (billing?.country || 'PL').trim().toUpperCase()
    const taxNo = (billing?.taxNo ?? '').trim()

    if (!name) errors.name = t('validation.required')
    if (!street) errors.street = t('validation.required')
    if (!city) errors.city = t('validation.required')
    if (!postCode) errors.postCode = t('validation.required')
    if (!email || !email.includes('@')) errors.email = t('validation.email')
    if (country.length !== 2) errors.country = t('plans.countryInvalid')
    if (isCompany && country === 'PL' && !validNip(taxNo)) errors.taxNo = t('plans.nipInvalid')
    return errors
  }

  // Update one field and clear its error as the user types.
  const update = (patch: Partial<Pick<OrgBilling, BillingField>>) => {
    setBilling((b) => (b ? { ...b, ...patch } : b))
    setFieldErrors((prev) => {
      const next = { ...prev }
      for (const key of Object.keys(patch) as BillingField[]) next[key] = undefined
      return next
    })
  }

  // Switching the buyer kind clears the NIP error: a private person has no NIP.
  const setKind = (kind: string) => {
    setBilling((b) => (b ? { ...b, kind } : b))
    setFieldErrors((prev) => ({ ...prev, taxNo: undefined }))
  }

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
    const errors = validate()
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      setError(null)
      return
    }
    setFieldErrors({})
    setBusy('save')
    setError(null)
    const isCompany = (billing.kind ?? '') !== 'individual'
    try {
      setBilling(await api.patch<OrgBilling>(`/api/orgs/${orgId}/billing`, {
        kind: isCompany ? 'company' : 'individual',
        name: billing.name,
        taxNo: isCompany ? billing.taxNo : '',
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
    const errors = validate()
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors)
      setError(null)
      return
    }
    setFieldErrors({})
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
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">{t('plans.title')}</h1>
        <p className="mt-4 max-w-xl text-sm text-fg-subtle">{t('plans.selfhost')}</p>
      </div>
    )
  }

  if (memberships.length === 0) {
    return (
      <div className="page-shell">
        <p className="eyebrow mb-3">{t('plans.eyebrow')}</p>
        <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">{t('plans.title')}</h1>
        <p className="mt-4 max-w-xl text-sm text-fg-subtle">{t('plans.noOrg')}</p>
      </div>
    )
  }

  const isCompany = (billing?.kind ?? '') !== 'individual'

  return (
    <div className="page-shell max-w-5xl">
      <p className="eyebrow mb-3">{t('plans.eyebrow')}</p>
      <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">{t('plans.title')}</h1>
      <p className="mt-2 max-w-2xl text-sm text-fg-muted">{t('plans.subtitle', { org: current?.name })}</p>

      {params.get('paid') === '1' && (
        <p className="mt-4 rounded-lg border border-accent/20 bg-accent/5 px-3 py-2 text-sm text-accent-fg">{t('plans.paid')}</p>
      )}
      {params.get('canceled') === '1' && (
        <p className="mt-4 rounded-lg border border-line-2 px-3 py-2 text-sm text-fg-muted">{t('plans.canceled')}</p>
      )}

      <div className="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {PLAN_ORDER.map((id) => {
          const cfg = PLAN_BY_SLUG[id]
          const currentPlan = billing?.plan === id
          const price = cfg.charge.kind === 'free' ? '0 €' : cfg.charge.kind === 'contact' ? t('plans.talk') : `${cfg.charge.eur} €`
          return (
            <article key={id} className={`flex flex-col rounded-2xl border p-5 ${currentPlan ? 'border-accent/35 bg-accent/5' : 'border-line-2 bg-fill-1'}`}>
              <h2 className="text-base font-semibold text-fg-strong">{LABELS[id]}</h2>
              <p className="mt-3 text-2xl font-semibold text-fg-strong">{price}</p>
              <p className="mt-1 text-xs text-fg-subtle">{cfg.charge.perPerson ? t('plans.perPerson') : id === 'free' ? t('plans.onePerson') : t('plans.contract')}</p>
              <ul className="mt-4 flex-1 space-y-1 text-sm text-fg-muted">
                {cfg.limits.people === 1 && <li>{t('plans.capPeople', { n: 1 })}</li>}
                {cfg.limits.projects > 0 && <li>{t('plans.capProjects', { n: cfg.limits.projects })}</li>}
                {cfg.limits.projects === 0 && <li>{t('plans.noProjectCap')}</li>}
                {cfg.limits.workersPerProject > 0 && <li>{t('plans.capMachines', { n: cfg.limits.workersPerProject })}</li>}
              </ul>
              {currentPlan && <p className="mt-4 text-xs font-medium text-accent">{t('plans.current')}</p>}
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
          <h2 className="text-lg font-semibold text-fg-strong">{t('plans.invoiceTitle')}</h2>
          <p className="text-sm text-fg-subtle">{t('plans.invoiceHint')}</p>

          <fieldset>
            <legend className="mb-2 text-sm text-fg-subtle">{t('plans.buyerKind')}</legend>
            <div className="grid gap-2 sm:grid-cols-2">
              <button
                type="button"
                onClick={() => setKind('company')}
                aria-pressed={isCompany}
                className={`rounded-lg border px-3 py-2 text-sm transition-colors ${
                  isCompany ? 'border-accent/40 bg-accent/10 text-fg-strong' : 'border-line-2 text-fg-muted hover:text-fg'
                }`}
              >
                {t('plans.kindCompany')}
              </button>
              <button
                type="button"
                onClick={() => setKind('individual')}
                aria-pressed={!isCompany}
                className={`rounded-lg border px-3 py-2 text-sm transition-colors ${
                  !isCompany ? 'border-accent/40 bg-accent/10 text-fg-strong' : 'border-line-2 text-fg-muted hover:text-fg'
                }`}
              >
                {t('plans.kindIndividual')}
              </button>
            </div>
          </fieldset>

          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={isCompany ? t('plans.company') : t('plans.personName')} value={billing?.name ?? ''} error={fieldErrors.name} onChange={(name) => update({ name })} />
            {isCompany && (
              <Field label={t('plans.nip')} value={billing?.taxNo ?? ''} error={fieldErrors.taxNo} onChange={(taxNo) => update({ taxNo })} />
            )}
            <Field label={t('plans.street')} value={billing?.street ?? ''} error={fieldErrors.street} onChange={(street) => update({ street })} className="sm:col-span-2" />
            <Field label={t('plans.postCode')} value={billing?.postCode ?? ''} error={fieldErrors.postCode} onChange={(postCode) => update({ postCode })} />
            <Field label={t('plans.city')} value={billing?.city ?? ''} error={fieldErrors.city} onChange={(city) => update({ city })} />
            <Field label={t('plans.country')} value={billing?.country || 'PL'} error={fieldErrors.country} onChange={(country) => update({ country })} />
            <Field label={t('plans.email')} value={billing?.email || me.email || ''} error={fieldErrors.email} onChange={(email) => update({ email })} />
          </div>
          <button type="submit" disabled={busy === 'save' || !billing} className="btn-secondary">
            {busy === 'save' ? t('common.loading') : t('plans.saveInvoice')}
          </button>
          {!billing?.checkoutReady && (
            <p className="text-sm text-fg-subtle">{t('plans.notWired')}</p>
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
  error,
  className = '',
}: {
  label: string
  value: string
  onChange: (value: string) => void
  error?: string
  className?: string
}) {
  return (
    <label className={`block text-sm text-fg-soft ${className}`}>
      <span className="mb-1 block">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={error ? true : undefined}
        className={`w-full rounded-lg border bg-fill-2 px-3 py-2 text-fg-strong ${
          error ? 'border-fail/60' : 'border-line-2'
        }`}
      />
      {error && (
        <span role="alert" className="mt-1 block text-xs text-fail-fg">
          {error}
        </span>
      )}
    </label>
  )
}
