import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import type { Membership } from './types'

// CurrentOrg is the one cockpit-wide answer to "which organization are we
// looking at". Team, Plans, and the project rail all read it; the sidebar
// Organizations section is its only switcher. The choice is remembered in
// localStorage under initagent.currentOrg, but only while that id is still a
// membership — the moment an organization stops being mine, the cockpit
// falls back to the first one.

const STORAGE_KEY = 'initagent.currentOrg'

type CurrentOrgValue = {
  orgId: string
  setOrgId: (orgId: string) => void
}

const CurrentOrgContext = createContext<CurrentOrgValue | null>(null)

export function CurrentOrgProvider({
  orgs,
  children,
}: {
  orgs: Membership[]
  children: ReactNode
}) {
  const [orgId, setOrgIdState] = useState<string>(() => {
    const stored = window.localStorage.getItem(STORAGE_KEY) ?? ''
    return orgs.some((m) => m.orgId === stored) ? stored : (orgs[0]?.orgId ?? '')
  })

  // A membership can disappear without a reload (invite revoked, removed,
  // org deleted): fall back to the first org instead of looking at an
  // organization that is no longer mine.
  useEffect(() => {
    if (orgs.some((m) => m.orgId === orgId)) return
    setOrgIdState(orgs[0]?.orgId ?? '')
  }, [orgs, orgId])

  const setOrgId = useCallback((next: string) => {
    setOrgIdState(next)
    try {
      window.localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // Storage unavailable (private mode, quota): the in-memory choice
      // still holds for this session.
    }
  }, [])

  const value = useMemo(() => ({ orgId, setOrgId }), [orgId, setOrgId])

  return (
    <CurrentOrgContext.Provider value={value}>{children}</CurrentOrgContext.Provider>
  )
}

export function useCurrentOrg(): CurrentOrgValue {
  const value = useContext(CurrentOrgContext)
  if (value === null) {
    throw new Error('useCurrentOrg must be used inside CurrentOrgProvider')
  }
  return value
}
