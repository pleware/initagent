import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  THEME_COOKIE,
  decodePreference,
  encodeCookieValue,
  readCookieValue,
  themeCookieAssignment,
  themeCookieDomain,
} from './cookie.ts'

test('themeCookieDomain shares hosted site and hub', () => {
  const cases: [string, string | undefined][] = [
    ['initagent.dev', '.initagent.dev'],
    ['app.initagent.dev', '.initagent.dev'],
    ['www.initagent.dev', '.initagent.dev'],
    ['localhost', undefined],
    ['127.0.0.1', undefined],
    ['[::1]', undefined],
    ['hub.example.com', undefined],
    ['initagent.dev.evil.com', undefined],
    ['notinitagent.dev', undefined],
  ]
  for (const [hostname, domain] of cases) {
    assert.equal(themeCookieDomain(hostname), domain, hostname)
  }
})

test('themeCookieAssignment is Lax, not HttpOnly, Domain only on hosted', () => {
  const value = {
    family: 'nord' as const,
    mode: 'system' as const,
    id: 'nord-light' as const,
  }
  const hosted = themeCookieAssignment(value, {
    hostname: 'app.initagent.dev',
    secure: true,
  })
  assert.match(hosted, new RegExp(`^${THEME_COOKIE}=`))
  assert.match(hosted, /Path=\//)
  assert.match(hosted, /SameSite=Lax/)
  assert.match(hosted, /Secure/)
  assert.match(hosted, /Domain=\.initagent\.dev/)
  assert.doesNotMatch(hosted, /HttpOnly/)

  const local = themeCookieAssignment(value, {
    hostname: 'localhost',
    secure: false,
  })
  assert.doesNotMatch(local, /Domain=/)
  assert.doesNotMatch(local, /Secure/)
})

test('readCookieValue matches the whole name', () => {
  const json = encodeCookieValue({
    family: 'sunset',
    mode: 'dark',
    id: 'sunset-dark',
  })
  const header = `other=1; ${THEME_COOKIE}=${json}; keep=yes`
  assert.equal(readCookieValue(header, THEME_COOKIE), json)
  assert.equal(readCookieValue('', THEME_COOKIE), null)
  assert.equal(readCookieValue(`x${THEME_COOKIE}=no`, THEME_COOKIE), null)
})

test('decodePreference reads family and mode and ignores id', () => {
  const raw = encodeCookieValue({
    family: 'corporate',
    mode: 'system',
    id: 'corporate-light',
  })
  assert.deepEqual(decodePreference(raw), {
    family: 'corporate',
    mode: 'system',
  })
  assert.deepEqual(decodePreference('%7B%7D'), {
    family: 'legacy',
    mode: 'system',
  })
  assert.deepEqual(decodePreference('not-json'), {
    family: 'legacy',
    mode: 'system',
  })
})
