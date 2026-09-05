import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import AuthSplitBg from './AuthSplitBg'
import GuestNav from './GuestNav'

export const authFieldClass =
  'block w-full rounded-control bg-fill-sunken px-3 py-1.5 text-base text-fg outline outline-1 -outline-offset-1 outline-line-2 placeholder:text-fg-ghost focus:outline-2 focus:-outline-offset-2 focus:outline-accent sm:text-sm/6'

export function AuthMark({ className = 'size-10' }: { className?: string }) {
  return (
    <svg viewBox="0 0 100 100" aria-hidden className={className}>
      <circle cx="50" cy="50" r="42" className="fill-accent" />
      <circle cx="50" cy="50" r="18" className="fill-canvas" />
      <circle cx="50" cy="50" r="8" className="fill-accent-on" />
    </svg>
  )
}

/** Split-screen chrome for sign-in, create-account, claim, and password reset.
 *  GuestNav stays on top. The right pane is generated progressive art. */
export default function AuthSplit({
  skipTarget,
  children,
}: {
  skipTarget: string
  children: ReactNode
}) {
  const { t } = useTranslation()

  return (
    <>
      <a href={skipTarget} className="skip-link">
        {t('publicNav.skipToForm')}
      </a>
      <GuestNav />
      <div className="flex h-full min-h-0 pt-16">
        <div className="flex flex-1 flex-col justify-center overflow-y-auto bg-canvas px-4 py-12 sm:px-6 lg:flex-none lg:px-20 xl:px-24">
          <div className="mx-auto w-full max-w-sm lg:w-96">{children}</div>
        </div>
        <div className="relative hidden w-0 flex-1 overflow-hidden lg:block" aria-hidden>
          <AuthSplitBg />
        </div>
      </div>
    </>
  )
}
