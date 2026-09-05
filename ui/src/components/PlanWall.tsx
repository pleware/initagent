import { ApiError } from '../api'

const publicPlans = 'https://initagent.dev/plans'

export default function PlanWall({ error }: { error: unknown }) {
  if (!(error instanceof ApiError) || error.code !== 'plan_limit') {
    return null
  }
  return (
    <div className="rounded-lg border border-white/[0.08] bg-white/[0.03] px-3 py-3 text-sm text-fg">
      <p>{error.message}</p>
      <p className="mt-2 text-fg-muted">You pay for people. Machines stay yours. We never host workers.</p>
      <a href={publicPlans} className="mt-3 inline-block text-sm underline-offset-2 hover:underline">
        See plans
      </a>
    </div>
  )
}

export function HubError({ error, fallback, className = 'text-sm text-fail-fg' }: { error: unknown; fallback: string; className?: string }) {
  if (!error) return null
  if (error instanceof ApiError && error.code === 'plan_limit') {
    return <PlanWall error={error} />
  }
  return <p className={className}>{planLimitMessage(error, fallback)}</p>
}

export function planLimitMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error) return error.message
  return fallback
}
