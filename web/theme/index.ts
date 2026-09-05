/**
 * Theme edge: storage, the document, and the OS media query.
 * Shared by the cockpit and the marketing site.
 *
 * Guests persist to `initagent_theme` (Domain=.initagent.dev on hosted)
 * so the site and hub share one preference. localStorage remains a
 * same-origin fallback and a migration source.
 */

import {
  LEGACY_STORAGE_KEY,
  THEME_COOKIE,
  decodePreference,
  readCookieValue,
  themeCookieAssignment,
  type ThemeCookieValue,
} from './cookie'
import {
  defaultPreference,
  parsePreference,
  resolveThemeId,
  type ThemeChoice,
  type ThemeFamily,
  type ThemeId,
  type ThemePreference,
} from './resolve'

export * from './cookie'
export * from './resolve'

const DARK_QUERY = '(prefers-color-scheme: dark)'

let preference: ThemePreference = defaultPreference

function systemPrefersDark(): boolean {
  return typeof matchMedia === 'function' && matchMedia(DARK_QUERY).matches
}

function cookieValue(next: ThemePreference): ThemeCookieValue {
  return {
    family: next.family,
    mode: next.mode,
    id: resolveThemeId(next, systemPrefersDark()),
  }
}

function readLegacyStorage(): ThemePreference | null {
  try {
    const raw = localStorage.getItem(LEGACY_STORAGE_KEY)
    if (raw === null) return null
    return parsePreference(JSON.parse(raw))
  } catch {
    return null
  }
}

export function readPreference(): ThemePreference {
  if (typeof document !== 'undefined') {
    const raw = readCookieValue(document.cookie, THEME_COOKIE)
    if (raw !== null) return decodePreference(raw)
  }
  return readLegacyStorage() ?? defaultPreference
}

function storePreference(next: ThemePreference): void {
  const value = cookieValue(next)
  try {
    if (typeof document !== 'undefined' && typeof location !== 'undefined') {
      document.cookie = themeCookieAssignment(value, {
        hostname: location.hostname,
        secure: location.protocol === 'https:',
      })
    }
  } catch {
    // Cookie blocked; localStorage may still hold the session.
  }
  try {
    localStorage.setItem(
      LEGACY_STORAGE_KEY,
      JSON.stringify({ family: next.family, mode: next.mode }),
    )
  } catch {
    // Session still applies.
  }
}

export function applyTheme(
  id: ThemeId,
  root: HTMLElement = document.documentElement,
): void {
  root.dataset.theme = id
}

function render(): ThemeId {
  const id = resolveThemeId(preference, systemPrefersDark())
  applyTheme(id)
  return id
}

export function initTheme(): ThemeId {
  const foundCookie =
    typeof document !== 'undefined' &&
    readCookieValue(document.cookie, THEME_COOKIE) !== null
  preference = readPreference()
  if (foundCookie || readLegacyStorage() !== null) storePreference(preference)

  if (typeof matchMedia === 'function') {
    matchMedia(DARK_QUERY).addEventListener('change', () => {
      if (preference.mode === 'system') {
        render()
        storePreference(preference)
      }
    })
  }

  return render()
}

export function setTheme(next: {
  family?: ThemeFamily
  mode?: ThemeChoice
}): ThemeId {
  preference = {
    family: next.family ?? preference.family,
    mode: next.mode ?? preference.mode,
  }
  storePreference(preference)
  return render()
}

export function currentPreference(): ThemePreference {
  return preference
}
