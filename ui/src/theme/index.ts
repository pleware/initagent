/**
 * Theme edge: storage, the document, and the OS media query.
 *
 * Every decision belongs to `./resolve.ts`. This file only performs it, and
 * tolerates the two things that can genuinely fail out here — storage refused
 * by the browser, and a stale or hand-edited stored value.
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

/** Read the stored preference, falling back on anything unusable. */
export function readPreference(): ThemePreference {
  let raw: string | null = null
  try {
    raw = localStorage.getItem(STORAGE_KEY)
  } catch {
    // Storage can be disabled outright (private mode, blocked third-party
    // context). A theme is not worth failing startup over.
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
    // Same as above: the choice still applies for this session.
  }
}

function systemPrefersDark(): boolean {
  return typeof matchMedia === 'function' && matchMedia(DARK_QUERY).matches
}

/**
 * Write the id to the document. `color-scheme` for native widgets is set by
 * the family's own stylesheet, so nothing else is needed here.
 */
export function applyTheme(
  id: ThemeId,
  root: HTMLElement = document.documentElement,
): void {
  root.dataset.theme = id
}

/** Resolve the current preference against the OS and apply it. */
function render(): ThemeId {
  const id = resolveThemeId(preference, systemPrefersDark())
  applyTheme(id)
  return id
}

/**
 * Adopt the stored preference and keep following the OS while the mode is
 * `system`. Call once, before first paint; `index.html` carries the fallback
 * id so there is no flash if this runs late.
 */
export function initTheme(): ThemeId {
  preference = readPreference()

  if (typeof matchMedia === 'function') {
    matchMedia(DARK_QUERY).addEventListener('change', () => {
      if (preference.mode === 'system') render()
    })
  }

  return render()
}

/** Change part of the preference, persist it, and repaint. */
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
