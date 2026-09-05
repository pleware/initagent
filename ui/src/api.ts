// Thin fetch wrapper for the hub API. All calls are same-origin and rely on
// the session cookie; 401s bounce the user to the login screen via the
// listener App.tsx registers here.

let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export class ApiError extends Error {
  status: number
  code?: string
  wall?: string
  limit?: number
  constructor(status: number, message: string, code?: string, wall?: string, limit?: number) {
    super(message)
    this.status = status
    this.code = code
    this.wall = wall
    this.limit = limit
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (res.status === 401 && path !== '/api/login' && !path.startsWith('/api/password/')) {
    onUnauthorized?.()
  }
  if (!res.ok) {
    let msg = res.statusText
    let code: string | undefined
    let wall: string | undefined
    let limit: number | undefined
    try {
      const data = await res.json()
      if (data.error) msg = data.error
      if (typeof data.code === 'string') code = data.code
      if (typeof data.wall === 'string') wall = data.wall
      if (typeof data.limit === 'number') limit = data.limit
    } catch {
      /* not json */
    }
    throw new ApiError(res.status, msg, code, wall, limit)
  }
  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}

// forProject names which project a gateway-bound call acts on. The hub routes
// to that project's own gateway, so the parameter is placement, not a filter.
// Omitting it is correct whenever the hub has exactly one project, which is
// self-host and the free plan; the hub resolves it and refuses to guess when
// there are two.
export function forProject(path: string, projectId?: string): string {
  if (!projectId) return path
  const separator = path.includes('?') ? '&' : '?'
  return `${path}${separator}project=${encodeURIComponent(projectId)}`
}

// Remote execution entry point used by libfx's typed browser-workspace
// adapter. AbortSignal cancellation stops the browser request immediately;
// the device-side timeout remains the hard upper bound for a command.
export async function execProject(
  projectId: string,
  command: string,
  timeoutMs: number,
  signal?: AbortSignal,
) {
  const res = await fetch(`/api/projects/${encodeURIComponent(projectId)}/exec`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ command, timeoutMs }),
    signal,
  })
  if (res.status === 401) onUnauthorized?.()
  if (!res.ok) {
    let message = res.statusText
    try {
      const data = await res.json()
      if (data.error) message = data.error
    } catch {
      /* response was not JSON */
    }
    throw new ApiError(res.status, message)
  }
  return res.json()
}

export function wsURL(path: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}${path}`
}

export function formatBytes(n: number): string {
  if (!n && n !== 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 10 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

export function timeAgo(unixSec: number): string {
  if (!unixSec) return 'never'
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unixSec))
  if (s < 5) return 'just now'
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}
