/**
 * Theme edge: storage, the document, and the OS media query.
 * Shared by the cockpit and the marketing site.
 */

import {
  defaultPreference,
  parsePreference,
  resolveThemeId,
  type ThemeChoice,
  type ThemeFamily,
  type ThemeId,
  type ThemePreference,
} from './resolve'

export * from './resolve'

const STORAGE_KEY = 'initagent.theme'
const DARK_QUERY = '(prefers-color-scheme: dark)'

let preference: ThemePreference = defaultPreference

export function readPreference(): ThemePreference {
  let raw: string | null = null
  try {
    raw = localStorage.getItem(STORAGE_KEY)
  } catch {
    return defaultPreference
  }
  if (raw === null) return defaultPreference

  try {
    return parsePreference(JSON.parse(raw))
  } catch {
    return defaultPreference
  }
}

function storePreference(next: ThemePreference): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    // Session still applies.
  }
}

function systemPrefersDark(): boolean {
  return typeof matchMedia === 'function' && matchMedia(DARK_QUERY).matches
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
  preference = readPreference()

  if (typeof matchMedia === 'function') {
    matchMedia(DARK_QUERY).addEventListener('change', () => {
      if (preference.mode === 'system') render()
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
