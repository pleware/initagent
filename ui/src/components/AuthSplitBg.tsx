import { useEffect, useState } from 'react'
import { modeFromThemeId, type ThemeMode } from '../../../web/theme/resolve.ts'
import {
  authBgDark,
  authBgLight,
  type AuthBgAssets,
} from '../assets/auth/bg.gen.ts'

function readPaneMode(): ThemeMode {
  return modeFromThemeId(document.documentElement.dataset.theme ?? '')
}

function usePaneMode(): ThemeMode {
  const [mode, setMode] = useState<ThemeMode>(readPaneMode)

  useEffect(() => {
    const root = document.documentElement
    const sync = () => setMode(readPaneMode())
    const observer = new MutationObserver(sync)
    observer.observe(root, { attributes: true, attributeFilter: ['data-theme'] })
    return () => observer.disconnect()
  }, [])

  return mode
}

function Variant({ assets }: { assets: AuthBgAssets }) {
  const [ready, setReady] = useState(false)

  return (
    <>
      <img
        src={assets.lqip}
        alt=""
        className="absolute inset-0 size-full scale-110 object-cover object-center blur-2xl"
      />
      <picture>
        <source media="(min-width: 1024px)" type="image/avif" srcSet={assets.avif} />
        <source media="(min-width: 1024px)" type="image/webp" srcSet={assets.webp} />
        <source media="(min-width: 1024px)" type="image/jpeg" srcSet={assets.jpg} />
        <img
          src={assets.lqip}
          alt=""
          decoding="async"
          fetchPriority="high"
          onLoad={() => setReady(true)}
          className={`absolute inset-0 size-full object-cover object-center transition-opacity duration-500 ${
            ready ? 'opacity-100' : 'opacity-0'
          }`}
        />
      </picture>
    </>
  )
}

/** Right-pane art follows the resolved theme id, not the picker label:
 *  `*-light` (Corporate, Nord, …) → pware-bg; `*-dark` (Classic, Sunset, …) → pware-bg-dark. */
export default function AuthSplitBg() {
  const mode = usePaneMode()
  const assets = mode === 'light' ? authBgLight : (authBgDark ?? authBgLight)

  return (
    <div className="absolute inset-0">
      <div className="absolute inset-0 bg-sidebar" />
      <Variant key={mode} assets={assets} />
    </div>
  )
}
