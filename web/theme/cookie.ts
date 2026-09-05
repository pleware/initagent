/**
 * Guest theme cookie: one preference for the marketing site and the hub.
 *
 * Hosted names share `.initagent.dev`. localStorage cannot; this cookie can.
 * Not HttpOnly (the document must read it). Not `__Host-` (that forbids Domain).
 * Auth stays `initagent_auth` on `app.` only.
 */

import {
  defaultPreference,
  parsePreference,
  type ThemeId,
  type ThemePreference,
} from './resolve.ts'

/** Cookie name. Underscore, like `initagent_auth` (`05`). */
export const THEME_COOKIE = 'initagent_theme'

/** Previous per-origin key. Still written as a fallback; cookie wins on read. */
export const LEGACY_STORAGE_KEY = 'initagent.theme'

const YEAR_SECONDS = 60 * 60 * 24 * 365

export type ThemeCookieValue = ThemePreference & { id: ThemeId }

/** `Domain` for the guest theme cookie, or omit (host-only). */
export function themeCookieDomain(hostname: string): string | undefined {
  if (hostname === 'initagent.dev' || hostname.endsWith('.initagent.dev')) {
    return '.initagent.dev'
  }
  return undefined
}

export function encodeCookieValue(value: ThemeCookieValue): string {
  return encodeURIComponent(
    JSON.stringify({
      family: value.family,
      mode: value.mode,
      id: value.id,
    }),
  )
}

export function decodePreference(raw: string): ThemePreference {
  try {
    return parsePreference(JSON.parse(decodeURIComponent(raw)))
  } catch {
    return defaultPreference
  }
}

/** Assignment string for `document.cookie`. */
export function themeCookieAssignment(
  value: ThemeCookieValue,
  opts: { hostname: string; secure: boolean },
): string {
  const parts = [
    `${THEME_COOKIE}=${encodeCookieValue(value)}`,
    'Path=/',
    `Max-Age=${YEAR_SECONDS}`,
    'SameSite=Lax',
  ]
  const domain = themeCookieDomain(opts.hostname)
  if (domain) parts.push(`Domain=${domain}`)
  if (opts.secure) parts.push('Secure')
  return parts.join('; ')
}

export function readCookieValue(
  cookieHeader: string,
  name: string,
): string | null {
  if (!cookieHeader) return null
  const prefix = `${name}=`
  for (const part of cookieHeader.split(';')) {
    const trimmed = part.trim()
    if (trimmed.startsWith(prefix)) return trimmed.slice(prefix.length)
  }
  return null
}
