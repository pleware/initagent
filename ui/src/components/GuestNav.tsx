import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import ThemeSwitcher from './ThemeSwitcher'

/** Public marketing origin. The unauthenticated hub chrome points back
 *  here so a guest can leave the login card the same way they arrived. */
const SITE = 'https://initagent.dev'
const REPO = 'https://github.com/pleware/initagent'

const LINKS = [
  { key: 'how', href: `${SITE}/#how` },
  { key: 'agents', href: `${SITE}/#agents` },
  { key: 'code', href: `${SITE}/#code` },
] as const

/**
 * Restricted public bar for sign-in, create-account, and first-run.
 * Same order and labels as the marketing nav, minus Open app — this
 * origin already is the hub.
 */
export default function GuestNav() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <header className="fixed inset-x-0 top-0 z-40 border-b border-line-2 bg-canvas/80 backdrop-blur-xl">
      <nav className="mx-auto flex h-16 max-w-[1240px] items-center gap-8 px-5 lg:px-8">
        <a href={SITE} className="flex shrink-0 items-center gap-2.5 text-fg-strong">
          <EyeIcon />
          <span className="text-[15px] font-semibold tracking-tight">
            {t('publicNav.home')}
          </span>
        </a>

        <div className="hidden flex-1 items-center gap-7 md:flex">
          {LINKS.map((link) => (
            <a
              key={link.href}
              href={link.href}
              className="text-[13.5px] text-fg-muted transition-colors hover:text-fg"
            >
              {t(`publicNav.${link.key}`)}
            </a>
          ))}
        </div>

        <div className="ml-auto hidden items-center gap-3 md:flex">
          <ThemeSwitcher size="nav" />
          <a
            href={REPO}
            target="_blank"
            rel="noreferrer"
            className="flex items-center gap-2 rounded-lg px-3 py-2 text-[13.5px] text-fg-muted transition-colors hover:bg-fill-3 hover:text-fg"
          >
            <GithubIcon />
            {t('publicNav.source')}
          </a>
          <a
            href={`${SITE}/#install`}
            className="rounded-lg px-3 py-2 text-[13.5px] text-fg-muted transition-colors hover:bg-fill-3 hover:text-fg"
          >
            {t('publicNav.selfHost')}
          </a>
        </div>

        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          aria-label={open ? t('publicNav.closeMenu') : t('publicNav.openMenu')}
          className="ml-auto rounded-lg p-2 text-fg-muted transition-colors hover:text-fg md:hidden"
        >
          {open ? <CloseIcon /> : <MenuIcon />}
        </button>
      </nav>

      {open && (
        <div className="border-t border-line-2 bg-canvas/95 px-5 py-4 backdrop-blur-xl md:hidden">
          <div className="flex flex-col gap-1">
            {LINKS.map((link) => (
              <a
                key={link.href}
                href={link.href}
                onClick={() => setOpen(false)}
                className="rounded-lg px-2 py-3 text-[15px] text-fg-muted hover:bg-fill-3 hover:text-fg"
              >
                {t(`publicNav.${link.key}`)}
              </a>
            ))}
            <ThemeSwitcher className="px-2 py-2" size="nav" />
            <a
              href={REPO}
              target="_blank"
              rel="noreferrer"
              className="rounded-lg px-2 py-3 text-[15px] text-fg-muted hover:bg-fill-3 hover:text-fg"
            >
              {t('publicNav.source')}
            </a>
            <a
              href={`${SITE}/#install`}
              onClick={() => setOpen(false)}
              className="rounded-lg px-2 py-3 text-[15px] text-fg-muted hover:bg-fill-3 hover:text-fg"
            >
              {t('publicNav.selfHost')}
            </a>
          </div>
        </div>
      )}
    </header>
  )
}

function EyeIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" className="text-accent" aria-hidden>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

function GithubIcon() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      <path d="M12 2C6.48 2 2 6.58 2 12.26c0 4.52 2.87 8.36 6.84 9.72.5.1.68-.22.68-.49 0-.24-.01-.87-.01-1.71-2.78.62-3.37-1.37-3.37-1.37-.45-1.18-1.11-1.5-1.11-1.5-.91-.64.07-.63.07-.63 1 .07 1.53 1.06 1.53 1.06.9 1.57 2.36 1.12 2.94.86.09-.67.35-1.12.63-1.38-2.22-.26-4.56-1.14-4.56-5.07 0-1.12.39-2.03 1.03-2.75-.1-.26-.45-1.3.1-2.7 0 0 .84-.27 2.75 1.05A9.3 9.3 0 0 1 12 6.84c.85 0 1.71.12 2.51.35 1.9-1.32 2.74-1.05 2.74-1.05.55 1.4.2 2.44.1 2.7.64.72 1.03 1.63 1.03 2.75 0 3.94-2.34 4.8-4.58 5.06.36.32.68.95.68 1.92 0 1.38-.01 2.49-.01 2.83 0 .27.18.6.69.49A10.05 10.05 0 0 0 22 12.26C22 6.58 17.52 2 12 2Z" />
    </svg>
  )
}

function MenuIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
      <path d="M4 7h16M4 12h16M4 17h16" />
    </svg>
  )
}

function CloseIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  )
}
