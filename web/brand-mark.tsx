import { useEffect, useState, type ReactNode } from 'react'
import { modeFromThemeId, type ThemeMode } from './theme/resolve'

/**
 * The product mark — the "eye" square — as a generated PNG. The source lives
 * on the pware umbrella and is staged into `public/brand/` at build time
 * (web/scripts/prepare-brand.mjs). The light/dark pick follows the resolved
 * `data-theme` id, the same signal the auth split background reads.
 *
 * `fallback` renders when the generated mark is absent — public CI has no
 * umbrella vendor tree — so a bare checkout keeps the committed placeholder.
 */

function readMode(): ThemeMode {
  return modeFromThemeId(document.documentElement.dataset.theme ?? '')
}

/** Re-reads the resolved theme mode and re-renders when `data-theme` moves. */
export function useBrandMode(): ThemeMode {
  const [mode, setMode] = useState<ThemeMode>(readMode)

  useEffect(() => {
    const root = document.documentElement
    const sync = () => setMode(readMode())
    const observer = new MutationObserver(sync)
    observer.observe(root, { attributes: true, attributeFilter: ['data-theme'] })
    return () => observer.disconnect()
  }, [])

  return mode
}

export function BrandMark({
  className,
  fallback,
}: {
  className?: string
  fallback?: ReactNode
}) {
  const mode = useBrandMode()
  const [failed, setFailed] = useState(false)
  const src =
    mode === 'light' ? '/brand/logo_square_white.png' : '/brand/logo_square_dark.png'

  useEffect(() => {
    setFailed(false)
  }, [src])

  if (failed && fallback != null) return <>{fallback}</>

  return (
    <img
      src={src}
      alt=""
      aria-hidden
      draggable={false}
      className={className}
      onError={() => setFailed(true)}
    />
  )
}
