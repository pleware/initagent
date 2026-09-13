/**
 * Stages the pware brand marks into an app's `public/brand/` so the site and
 * the cockpit reference them at stable URLs (`/brand/logo_square_white.png`,
 * …). The master PNGs live on the pware umbrella — `vendor/pware/initagent/`
 * beside this workspace — and are never committed into the product checkout
 * (see `initagent-workspace/vendor/pware/README.md` and the binder contract
 * `docs/BRAND-MARKS.md`).
 *
 * Called from each app's Vite build. When the umbrella vendor tree is absent
 * (public CI, a bare clone) this is a deliberate no-op: the apps keep their
 * committed placeholder favicon and marks, and nothing throws.
 */
import { copyFile, mkdir } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

// initagent/web/scripts → pware-workspace/vendor/pware/initagent.
// Override with VENDOR_DIR when the umbrella lives somewhere else.
const VENDOR_DIR =
  process.env.VENDOR_DIR ??
  join(here, '..', '..', '..', '..', 'vendor', 'pware', 'initagent')

// The six role filenames from the brand contract. The square pair is the
// favicon + narrow-navbar mark; the lockup pair is wide chrome; the favicon
// pair is currently a copy of the square pair.
const ROLES = [
  'logo_on_dark.png',
  'logo_on_white.png',
  'logo_square_white.png',
  'logo_square_dark.png',
  'logo_favicon_white.png',
  'logo_favicon_dark.png',
]

/** Copy the master marks into `<outDir>/brand/`. Returns what it staged. */
export async function prepareBrand(outDir) {
  const dest = join(outDir, 'brand')
  if (!existsSync(VENDOR_DIR)) return { staged: 0, missing: true }

  await mkdir(dest, { recursive: true })
  let staged = 0
  for (const role of ROLES) {
    const from = join(VENDOR_DIR, role)
    if (!existsSync(from)) continue
    await copyFile(from, join(dest, role))
    staged += 1
  }
  return { staged, missing: false }
}
