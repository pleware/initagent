/**
 * Dev ports shared across the initagent tree. Single source for the numbers:
 * the Go side mirrors the hub/gateway in internal/brand, and the whole block
 * is documented in the binder docs/LOCAL-PORTS.md (initagent 21000–21999).
 */

/** The Go hub's HTTP listen port (`initagent serve`). Cockpit Vite proxies to it. */
export const HUB_HTTP_PORT = 21000

/** Marketing Vite (`site/`) dev port. */
export const DEV_SITE_PORT = 21003

/** Marketing Vite (`site/`) preview port. */
export const DEV_SITE_PREVIEW_PORT = 21013

/** Cockpit Vite (`ui/`) dev port. */
export const DEV_HUB_PORT = 21004

/** Cockpit Vite (`ui/`) preview port. */
export const DEV_HUB_PREVIEW_PORT = 21014
