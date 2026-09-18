import { useTranslation } from 'react-i18next'
import { ApiError, localizeError } from '../api'

const publicPlans = '/plans'

export default function PlanWall({ error }: { error: unknown }) {
  const { t } = useTranslation()
  if (!(error instanceof ApiError) || error.code !== 'plan_limit') {
    return null
  }
  return (
    <div className="rounded-lg border border-line-2 bg-fill-2 px-3 py-3 text-sm text-fg">
      <p>{localizeError(error, t)}</p>
      <p className="mt-2 text-fg-muted">{t('errors.planLimitHint')}</p>
      <a href={publicPlans} className="mt-3 inline-block text-sm underline-offset-2 hover:underline">
        {t('errors.planLimitCta')}
      </a>
    </div>
  )
}

export function HubError({
  error,
  fallback,
  className = 'rounded-lg border border-fail/30 bg-fail/10 px-3 py-3 text-left text-sm text-fail-fg',
}: {
  error: unknown
  fallback: string
  className?: string
}) {
  if (!error) return null
  if (error instanceof ApiError && error.code === 'plan_limit') {
    return <PlanWall error={error} />
  }
  return (
    <p role="alert" className={className}>
      {planLimitMessage(error, fallback)}
    </p>
  )
}

export function isMissingGateway(error: unknown): boolean {
  const message = planLimitMessage(error, '')
  return message.includes('has no gateway') || message.includes('gateway URL is required')
}

export function planLimitMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error) return error.message
  return fallback
}
