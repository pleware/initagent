import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { displayName } from '../../../web/brand.ts'
import { api } from '../api'
import type { Me, Project } from '../types'
import LanguageSwitcher from './LanguageSwitcher'
import ThemeSwitcher from './ThemeSwitcher'
import { isHostedOperator } from './Boarding'

const links = [
  { to: '/code', label: 'Code', icon: 'compose' },
  { to: '/tasks', label: 'Tasks', icon: 'terminal' },
  { to: '/fleet', label: 'Fleet', icon: 'grid' },
  { to: '/agents', label: 'Agent runs', icon: 'pulse' },
  { to: '/setup', label: 'Set up nodes', icon: 'spark' },
  { to: '/settings', label: 'Settings', icon: 'sliders' },
]

export type HubOutlet = {
  projects: Project[]
  setProjects: Dispatch<SetStateAction<Project[]>>
  projectsReady: boolean
  reloadProjects: () => void
}

export default function Layout({ me }: { me: Me }) {
  const { t } = useTranslation()
  const [projects, setProjects] = useState<Project[]>([])
  const [projectsReady, setProjectsReady] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)

  // Two hub surfaces, deliberately separate (draft 17): People is an
  // organization's own roster, Administration is this installation. The
  // second appears only for the operator — a hidden link is not the
  // permission, the endpoint behind it is.
  const hubLinks = [
    ...(me.offering === 'hosted' && me.orgs && me.orgs.length > 0
      ? [{ to: '/plans', label: t('nav.plans'), icon: 'plans' }]
      : []),
    ...(me.orgs && me.orgs.length > 0
      ? [{ to: '/people', label: t('nav.people'), icon: 'people' }]
      : []),
    ...(me.platformAdmin
      ? [{ to: '/admin', label: t('nav.administration'), icon: 'shield' }]
      : []),
  ]

  const loadProjects = useCallback(async () => {
    try {
      setProjects(await api.get<Project[]>('/api/projects'))
    } catch {
      /* preserve the current project rail during a short disconnect */
    } finally {
      setProjectsReady(true)
    }
  }, [])

  useEffect(() => {
    loadProjects()
    const timer = window.setInterval(loadProjects, 15_000)
    window.addEventListener('liveagent:projects-changed', loadProjects)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('liveagent:projects-changed', loadProjects)
    }
  }, [loadProjects])

  const logout = async () => {
    await api.post('/api/logout')
    window.location.assign('/login')
  }

  return (
    <div className="app-frame">
      <a href="#main-content" className="skip-link">Skip to content</a>

      <div className="mobile-bar">
        <button onClick={() => setMobileOpen(true)} aria-label="Open navigation" className="toolbar-button"><MenuIcon /></button>
        <Brand />
        <span className="ml-auto flex items-center gap-1.5 text-[10px] text-zinc-500"><span className="node-dot node-dot-online" />online</span>
      </div>

      {mobileOpen && <button className="sidebar-scrim" onClick={() => setMobileOpen(false)} aria-label="Close navigation" />}

      <aside className={`app-sidebar ${mobileOpen ? 'app-sidebar-open' : ''}`}>
        <div className="sidebar-head">
          <Brand />
          <span className="rounded border border-white/[0.08] px-1.5 py-0.5 font-mono text-[9px] text-zinc-600">fx</span>
          <button onClick={() => setMobileOpen(false)} className="ml-auto text-zinc-600 hover:text-white lg:hidden" aria-label="Close navigation">×</button>
        </div>

        <nav className="sidebar-nav" aria-label="Main navigation">
          {links.map((link) => (
            <NavLink
              key={link.to}
              to={link.to}
              end={link.to === '/code'}
              onClick={() => setMobileOpen(false)}
              className={({ isActive }) => `sidebar-link ${isActive ? 'sidebar-link-active' : ''}`}
            >
              <NavIcon name={link.icon} />
              <span>{link.label}</span>
            </NavLink>
          ))}
        </nav>

        {hubLinks.length > 0 && (
          <nav className="sidebar-nav" aria-label="Hub navigation">
            {hubLinks.map((link) => (
              <NavLink
                key={link.to}
                to={link.to}
                onClick={() => setMobileOpen(false)}
                className={({ isActive }) => `sidebar-link ${isActive ? 'sidebar-link-active' : ''}`}
              >
                <NavIcon name={link.icon} />
                <span>{link.label}</span>
              </NavLink>
            ))}
          </nav>
        )}

        <section className="sidebar-projects">
          <div className="sidebar-section-title">
            <span>Projects</span>
            {!isHostedOperator(me) && (
              <NavLink to="/code?new=1" onClick={() => setMobileOpen(false)} aria-label="Add project" className="sidebar-add">+</NavLink>
            )}
          </div>
          <div className="space-y-0.5">
            {projects.map((project) => (
              <NavLink
                key={project.id}
                to={`/code/${project.id}`}
                onClick={() => setMobileOpen(false)}
                className={({ isActive }) => `project-link ${isActive ? 'project-link-active' : ''}`}
              >
                <FolderIcon />
                <span className="truncate">{project.name}</span>
              </NavLink>
            ))}
            {projects.length === 0 && <p className="px-2 py-3 text-xs leading-5 text-zinc-700">No projects yet</p>}
          </div>
        </section>

        <footer className="sidebar-footer">
          <div className="operator-avatar" title={me.email} aria-hidden>
            {operatorInitials(me.email)}
          </div>
          <div className="min-w-0">
            <p className="truncate text-xs font-medium text-zinc-300">
              {me.orgs?.[0]?.name ?? me.email ?? 'Personal fleet'}
            </p>
            <p className="font-mono text-[9px] text-zinc-700">{me.version || 'development'}</p>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <ThemeSwitcher />
            <LanguageSwitcher persist />
            <button onClick={logout} className="text-[11px] text-zinc-600 hover:text-zinc-200">Log out</button>
          </div>
        </footer>
      </aside>

      <main id="main-content" className="app-content">
        <Outlet context={{ projects, setProjects, projectsReady, reloadProjects: loadProjects } satisfies HubOutlet} />
      </main>
    </div>
  )
}

// operatorInitials turns an email into two uppercase letters.
// john.doe@x → JD; alice@x → AL; j@acme.com → JA. Always two characters.
function operatorInitials(email?: string): string {
  const trimmed = email?.trim() ?? ''
  const at = trimmed.indexOf('@')
  const localRaw = at >= 0 ? trimmed.slice(0, at) : trimmed
  const domain = at >= 0 ? trimmed.slice(at + 1) : ''
  const local = localRaw.split('+')[0] ?? ''
  const parts = local.split(/[._-]+/).map(lettersOf).filter((part) => part.length > 0)

  if (parts.length >= 2) {
    return twoLetters(parts[0][0], parts[parts.length - 1][0])
  }
  const word = parts[0] ?? ''
  if (word.length >= 2) {
    return twoLetters(word[0], word[1])
  }
  if (word.length === 1) {
    const host = lettersOf(domain.split('.')[0] ?? '')
    return twoLetters(word[0], host[0] ?? word[0])
  }
  const domainLetters = lettersOf(domain)
  if (domainLetters.length >= 2) {
    return twoLetters(domainLetters[0], domainLetters[1])
  }
  if (domainLetters.length === 1) {
    return twoLetters(domainLetters[0], domainLetters[0])
  }
  return 'IA'
}

function lettersOf(value: string): string {
  return [...value].filter((ch) => /\p{L}/u.test(ch)).join('')
}

function twoLetters(first: string, second: string): string {
  return (first + second).toUpperCase()
}

function Brand() {
  return (
    <NavLink to="/code" className="flex items-center gap-2 text-zinc-100">
      <span className="brand-mark"><span>⌁</span></span>
      <span className="text-sm font-semibold tracking-[-0.025em]">{displayName}</span>
    </NavLink>
  )
}

function MenuIcon() {
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden><path d="M4 7h16M4 12h16M4 17h16" /></svg>
}

function FolderIcon() {
  return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h8.5A1.5 1.5 0 0 1 21 8.5v9a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5v-11Z" /></svg>
}

function NavIcon({ name }: { name: string }) {
  const common = { width: 15, height: 15, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.8 } as const
  if (name === 'compose') return <svg {...common} aria-hidden><path d="M13.5 5H6a2 2 0 0 0-2 2v11a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-7.5M16.5 3.5l4 4L11 17l-4.5 1 1-4.5 9-10Z" /></svg>
  if (name === 'terminal') return <svg {...common} aria-hidden><rect x="3" y="4" width="18" height="16" rx="2" /><path d="m7 9 3 3-3 3M13 15h4" /></svg>
  if (name === 'pulse') return <svg {...common} aria-hidden><path d="M3 12h4l2.2-6 4.2 12 2.3-6H21" /></svg>
  if (name === 'spark') return <svg {...common} aria-hidden><path d="m12 3 1.2 4.1L17 9l-3.8 1.9L12 15l-1.2-4.1L7 9l3.8-1.9L12 3ZM5 16l.7 2.3L8 19.5l-2.3 1.2L5 23l-.7-2.3L2 19.5l2.3-1.2L5 16Z" /></svg>
  if (name === 'sliders') return <svg {...common} aria-hidden><path d="M4 6h10M18 6h2M4 12h2M10 12h10M4 18h7M15 18h5" /><circle cx="16" cy="6" r="2" /><circle cx="8" cy="12" r="2" /><circle cx="13" cy="18" r="2" /></svg>
  if (name === 'plans') return <svg {...common} aria-hidden><rect x="3" y="6" width="18" height="13" rx="2" /><path d="M7 10h6M7 14h10" /></svg>
  if (name === 'people') return <svg {...common} aria-hidden><path d="M16 19v-1.5a3.5 3.5 0 0 0-3.5-3.5h-5A3.5 3.5 0 0 0 4 17.5V19M20 19v-1.5a3.5 3.5 0 0 0-2.6-3.4M14.5 5.6a3 3 0 0 1 0 5.8" /><circle cx="10" cy="8" r="3" /></svg>
  if (name === 'shield') return <svg {...common} aria-hidden><path d="M12 3l7 3v5.5c0 4.2-2.9 7.6-7 9.5-4.1-1.9-7-5.3-7-9.5V6l7-3Z" /><path d="m9 12 2 2 4-4" /></svg>
  return <svg {...common} aria-hidden><rect x="3" y="3" width="7" height="7" rx="1.5" /><rect x="14" y="3" width="7" height="7" rx="1.5" /><rect x="3" y="14" width="7" height="7" rx="1.5" /><rect x="14" y="14" width="7" height="7" rx="1.5" /></svg>
}
