/** Production marketing origin. */
export const PROD_SITE = 'https://initagent.dev'

/** Hosted control-plane hub. */
export const PROD_HUB = 'https://app.initagent.dev'

/** Marketing Vite (`site/`) while developing both apps. */
export const DEV_SITE_PORT = 5173

/** Cockpit Vite (`ui/`) while developing both apps. */
export const DEV_HUB_PORT = 5174

export function isLocalHost(hostname: string): boolean {
  return hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '[::1]'
}

function trimSlash(url: string): string {
  return url.replace(/\/$/, '')
}

function localOrigin(port: number): string {
  return `${window.location.protocol}//${window.location.hostname}:${port}`
}

/**
 * Marketing origin. On a laptop, guest chrome on the hub points at the
 * local site instead of initagent.dev. Override with VITE_SITE.
 */
export function siteOrigin(envSite?: string): string {
  const fromEnv = envSite?.trim()
  if (fromEnv) return trimSlash(fromEnv)
  if (typeof window !== 'undefined' && isLocalHost(window.location.hostname)) {
    return localOrigin(DEV_SITE_PORT)
  }
  return PROD_SITE
}

/**
 * Hub origin. On a laptop, Open app goes to the local cockpit instead of
 * app.initagent.dev. Override with VITE_HUB.
 */
export function hubOrigin(envHub?: string): string {
  const fromEnv = envHub?.trim()
  if (fromEnv) return trimSlash(fromEnv)
  if (typeof window !== 'undefined' && isLocalHost(window.location.hostname)) {
    return localOrigin(DEV_HUB_PORT)
  }
  return PROD_HUB
}
