import { Component, lazy, Suspense, useEffect, useState, type ErrorInfo, type ReactNode } from 'react'
import { shouldUseTileEffect, TILE_IMAGE_DEFAULTS, type TileImage3DProps } from './tileImage.ts'

const TileImageCanvas = lazy(() => import('./TileImageCanvas.tsx'))

class CanvasErrorBoundary extends Component<
  { onError: () => void; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false }

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true }
  }

  componentDidCatch(_error: Error, _info: ErrorInfo) {
    this.props.onError()
  }

  render() {
    if (this.state.failed) return null
    return this.props.children
  }
}

function StaticImage({ src, className }: { src: string; className?: string }) {
  return <img src={src} alt="" decoding="async" className={className ?? 'absolute inset-0 size-full object-cover'} />
}

export function TileImage3D({
  src,
  className,
  fallback,
  columns = TILE_IMAGE_DEFAULTS.columns,
  rows,
  tileSize,
  radius = TILE_IMAGE_DEFAULTS.radius,
  depth = TILE_IMAGE_DEFAULTS.depth,
  gap = TILE_IMAGE_DEFAULTS.gap,
  rotationStrength = TILE_IMAGE_DEFAULTS.rotationStrength,
  easing = TILE_IMAGE_DEFAULTS.easing,
  disabled,
}: TileImage3DProps) {
  const [enhanced, setEnhanced] = useState(false)
  const [failed, setFailed] = useState(false)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    const sync = () => setEnhanced(shouldUseTileEffect(disabled))
    sync()
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)')
    const pointer = window.matchMedia('(pointer: coarse)')
    motion.addEventListener('change', sync)
    pointer.addEventListener('change', sync)
    return () => {
      motion.removeEventListener('change', sync)
      pointer.removeEventListener('change', sync)
    }
  }, [disabled])

  useEffect(() => {
    setFailed(false)
    setReady(false)
  }, [src])

  const showCanvas = enhanced && !failed
  const fallbackNode = fallback ?? <StaticImage src={src} />

  return (
    <div className={className ? `relative overflow-hidden ${className}` : 'relative overflow-hidden'}>
      <div className={ready ? 'invisible' : undefined} aria-hidden={ready || undefined}>
        {fallbackNode}
      </div>
      {showCanvas ? (
        <CanvasErrorBoundary onError={() => setFailed(true)}>
          <Suspense fallback={null}>
            <TileImageCanvas
              src={src}
              columns={columns}
              rows={rows}
              tileSize={tileSize}
              radius={radius}
              depth={depth}
              gap={gap}
              rotationStrength={rotationStrength}
              easing={easing}
              onReady={() => setReady(true)}
              onFailure={() => setFailed(true)}
            />
          </Suspense>
        </CanvasErrorBoundary>
      ) : null}
    </div>
  )
}
