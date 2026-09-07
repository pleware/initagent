import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
  type PointerEvent,
} from 'react'
import { Canvas, useFrame, useThree } from '@react-three/fiber'
import * as THREE from 'three'
import {
  CAMERA_FOV,
  MOUSE_SMOOTHING,
  TILE_IMAGE_DEFAULTS,
  cameraZForHeight,
  coverPlaneSize,
  createPointerState,
  gridFromTileSize,
  influenceForce,
  rowsFromImage,
  type PointerState,
  type TileImage3DProps,
} from './tileImage.ts'

const SETTLE_EPS = 0.02

type CanvasProps = Omit<TileImage3DProps, 'className' | 'fallback' | 'disabled'> & {
  onReady?: () => void
  onFailure?: () => void
}

type GridBuffers = {
  count: number
  cols: number
  rows: number
  tileW: number
  tileH: number
  centersX: Float32Array
  centersY: Float32Array
  z: Float32Array
  rotX: Float32Array
  rotY: Float32Array
  uvOffset: Float32Array
  uvScale: Float32Array
}

function allocGrid(
  cols: number,
  rows: number,
  planeW: number,
  planeH: number,
): GridBuffers {
  const count = cols * rows
  const tileW = planeW / cols
  const tileH = planeH / rows
  const centersX = new Float32Array(count)
  const centersY = new Float32Array(count)
  const uvOffset = new Float32Array(count * 2)
  const uvScale = new Float32Array(count * 2)
  let i = 0
  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols; c++) {
      centersX[i] = -planeW / 2 + (c + 0.5) * tileW
      centersY[i] = planeH / 2 - (r + 0.5) * tileH
      uvOffset[i * 2] = c / cols
      uvOffset[i * 2 + 1] = 1 - (r + 1) / rows
      uvScale[i * 2] = 1 / cols
      uvScale[i * 2 + 1] = 1 / rows
      i++
    }
  }
  return {
    count,
    cols,
    rows,
    tileW,
    tileH,
    centersX,
    centersY,
    z: new Float32Array(count),
    rotX: new Float32Array(count),
    rotY: new Float32Array(count),
    uvOffset,
    uvScale,
  }
}

const ATLAS_VERTEX = /* glsl */ `
attribute vec2 uvOffset;
attribute vec2 uvScale;
varying vec2 vAtlasUv;
varying vec3 vWorldNormal;
varying vec3 vLocalNormal;

void main() {
  vAtlasUv = uv * uvScale * 0.996 + uvOffset + uvScale * 0.002;
  vLocalNormal = normal;
  vWorldNormal = normalize(normalMatrix * mat3(instanceMatrix) * normal);
  gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0);
}
`

const ATLAS_FRAGMENT = /* glsl */ `
uniform sampler2D map;
varying vec2 vAtlasUv;
varying vec3 vWorldNormal;
varying vec3 vLocalNormal;

void main() {
  bool front = normalize(vLocalNormal).z > 0.5;
  vec3 albedo = front ? texture2D(map, vAtlasUv).rgb : vec3(0.48, 0.49, 0.52);
  vec3 n = normalize(vWorldNormal);
  vec3 key = normalize(vec3(0.55, 0.35, 0.6));
  vec3 fill = normalize(vec3(-0.45, 0.15, 0.4));
  float lit = 0.22 + 0.62 * max(dot(n, key), 0.0) + 0.28 * max(dot(n, fill), 0.0);
  float shade = front ? (0.86 + 0.14 * max(dot(n, key), 0.0)) : lit;
  gl_FragColor = vec4(albedo * shade, 1.0);
}
`

function useImageTexture(src: string, onFailure?: () => void) {
  const [texture, setTexture] = useState<THREE.Texture | null>(null)
  const [size, setSize] = useState<{ w: number; h: number } | null>(null)
  const onFailureRef = useRef(onFailure)
  onFailureRef.current = onFailure

  useEffect(() => {
    let cancelled = false
    const loader = new THREE.TextureLoader()
    const tex = loader.load(
      src,
      (ready) => {
        if (cancelled) {
          ready.dispose()
          return
        }
        ready.colorSpace = THREE.NoColorSpace
        ready.minFilter = THREE.LinearFilter
        ready.magFilter = THREE.LinearFilter
        ready.generateMipmaps = false
        ready.needsUpdate = true
        const image = ready.image as { width?: number; height?: number }
        setTexture(ready)
        setSize({ w: image.width ?? 1, h: image.height ?? 1 })
      },
      undefined,
      () => {
        if (!cancelled) onFailureRef.current?.()
      },
    )
    return () => {
      cancelled = true
      tex.dispose()
      setTexture(null)
      setSize(null)
    }
  }, [src])

  return { texture, size }
}

function TileGrid({
  src,
  columns = TILE_IMAGE_DEFAULTS.columns,
  rows: rowsProp,
  tileSize,
  radius = TILE_IMAGE_DEFAULTS.radius,
  depth = TILE_IMAGE_DEFAULTS.depth,
  gap = TILE_IMAGE_DEFAULTS.gap,
  rotationStrength = TILE_IMAGE_DEFAULTS.rotationStrength,
  easing = TILE_IMAGE_DEFAULTS.easing,
  pointerRef,
  onReady,
  onFailure,
}: CanvasProps & { pointerRef: MutableRefObject<PointerState> }) {
  const meshRef = useRef<THREE.InstancedMesh>(null)
  const dummy = useRef(new THREE.Object3D())
  const gridRef = useRef<GridBuffers | null>(null)
  const smooth = useRef({ x: 0, y: 0 })
  const armedRef = useRef(false)
  const settledRef = useRef(false)
  const readySent = useRef(false)
  const { size, camera } = useThree()
  const { texture, size: imageSize } = useImageTexture(src, onFailure)

  const plane = imageSize
    ? coverPlaneSize(size.width, size.height, imageSize.w, imageSize.h)
    : { width: size.width, height: size.height }
  const fromTile = tileSize ? gridFromTileSize(plane.width, plane.height, tileSize) : null
  const cols = fromTile?.cols ?? Math.max(1, columns)
  const rows = fromTile?.rows ?? (rowsProp ?? (imageSize ? rowsFromImage(cols, imageSize.w, imageSize.h) : 1))

  const grid = useMemo(
    () => allocGrid(cols, rows, plane.width, plane.height),
    [cols, rows, plane.width, plane.height],
  )
  gridRef.current = grid

  const geometry = useMemo(() => {
    const geo = new THREE.BoxGeometry(grid.tileW, grid.tileH, 1)
    geo.setAttribute('uvOffset', new THREE.InstancedBufferAttribute(grid.uvOffset, 2))
    geo.setAttribute('uvScale', new THREE.InstancedBufferAttribute(grid.uvScale, 2))
    return geo
  }, [grid])

  const material = useMemo(() => {
    if (!texture) return null
    return new THREE.ShaderMaterial({
      uniforms: { map: { value: texture } },
      vertexShader: ATLAS_VERTEX,
      fragmentShader: ATLAS_FRAGMENT,
      toneMapped: false,
    })
  }, [texture])

  useEffect(() => () => geometry.dispose(), [geometry])
  useEffect(() => () => material?.dispose(), [material])

  useEffect(() => {
    if (!(camera instanceof THREE.PerspectiveCamera)) return
    camera.fov = CAMERA_FOV
    camera.aspect = size.width / Math.max(size.height, 1)
    camera.position.set(0, 0, cameraZForHeight(Math.max(size.height, 1), CAMERA_FOV))
    camera.near = 1
    camera.far = camera.position.z + Math.max(depth * 8, 400)
    camera.updateProjectionMatrix()
  }, [camera, size.width, size.height, depth])

  useFrame(() => {
    const mesh = meshRef.current
    const buffers = gridRef.current
    if (!mesh || !buffers) return

    const pointer = pointerRef.current
    const viewW = pointer.viewW || size.width
    const viewH = pointer.viewH || size.height
    const targetX = pointer.active ? pointer.x - viewW / 2 : 0
    const targetY = pointer.active ? viewH / 2 - pointer.y : 0
    if (pointer.active && !armedRef.current) {
      smooth.current.x = targetX
      smooth.current.y = targetY
      armedRef.current = true
    }
    if (!pointer.active) armedRef.current = false
    smooth.current.x += (targetX - smooth.current.x) * MOUSE_SMOOTHING
    smooth.current.y += (targetY - smooth.current.y) * MOUSE_SMOOTHING

    const rotRad = THREE.MathUtils.degToRad(rotationStrength)
    const depthWorld = depth
    const obj = dummy.current
    let moving = false

    for (let i = 0; i < buffers.count; i++) {
      const dx = buffers.centersX[i] - smooth.current.x
      const dy = buffers.centersY[i] - smooth.current.y
      const distance = Math.sqrt(dx * dx + dy * dy)
      const force = pointer.active ? influenceForce(distance, radius) : 0
      const targetZ = force * depthWorld
      const nx = radius > 0 ? dx / radius : 0
      const ny = radius > 0 ? dy / radius : 0

      buffers.z[i] += (targetZ - buffers.z[i]) * easing
      buffers.rotX[i] += (-ny * force * rotRad - buffers.rotX[i]) * easing
      buffers.rotY[i] += (nx * force * rotRad - buffers.rotY[i]) * easing

      if (
        Math.abs(buffers.z[i]) > SETTLE_EPS ||
        Math.abs(buffers.rotX[i]) > 0.0005 ||
        Math.abs(buffers.rotY[i]) > 0.0005
      ) {
        moving = true
      }

      const tileMin = Math.min(buffers.tileW, buffers.tileH)
      const shrink = gap > 0 ? (force * gap) / tileMin : 0
      const restOverlap = (1 - Math.min(1, buffers.z[i] / 12)) * (1.5 / tileMin)
      const scale = Math.max(0.92, 1 - shrink + restOverlap)
      const thickness = Math.max(0.02, buffers.z[i])

      obj.position.set(buffers.centersX[i], buffers.centersY[i], thickness / 2)
      obj.rotation.set(buffers.rotX[i], buffers.rotY[i], 0)
      obj.scale.set(scale, scale, thickness)
      obj.updateMatrix()
      mesh.setMatrixAt(i, obj.matrix)
    }

    if (moving || !settledRef.current) {
      mesh.instanceMatrix.needsUpdate = true
      settledRef.current = !moving
    }

    if (texture && imageSize && !readySent.current) {
      readySent.current = true
      onReady?.()
    }
  })

  if (!texture || !imageSize || !material) return null

  return (
    <instancedMesh
      key={`${grid.cols}x${grid.rows}:${grid.tileW.toFixed(2)}x${grid.tileH.toFixed(2)}`}
      ref={meshRef}
      args={[geometry, material, grid.count]}
      frustumCulled={false}
    />
  )
}

function BindToContainer({ container }: { container: HTMLElement }) {
  const setSize = useThree((state) => state.setSize)

  useLayoutEffect(() => {
    const apply = () => {
      const { width, height } = container.getBoundingClientRect()
      if (width < 1 || height < 1) return
      setSize(width, height, false)
      const canvas = container.querySelector('canvas')
      if (canvas instanceof HTMLCanvasElement) {
        canvas.style.width = '100%'
        canvas.style.height = '100%'
      }
    }
    apply()
    const observer = new ResizeObserver(apply)
    observer.observe(container)
    window.addEventListener('resize', apply)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', apply)
    }
  }, [container, setSize])

  return null
}

export default function TileImageCanvas({
  src,
  columns,
  rows,
  tileSize,
  radius,
  depth,
  gap,
  rotationStrength,
  easing,
  onReady,
  onFailure,
}: CanvasProps) {
  const pointerRef = useRef<PointerState>(createPointerState())
  const [container, setContainer] = useState<HTMLDivElement | null>(null)

  const updatePointer = useCallback((event: PointerEvent<HTMLDivElement>, active: boolean) => {
    const rect = event.currentTarget.getBoundingClientRect()
    const next = pointerRef.current
    next.x = event.clientX - rect.left
    next.y = event.clientY - rect.top
    next.viewW = rect.width
    next.viewH = rect.height
    next.active = active
  }, [])

  return (
    <div ref={setContainer} className="absolute inset-0 size-full">
      <Canvas
        camera={{ fov: CAMERA_FOV, position: [0, 0, 800], near: 1, far: 4000 }}
        dpr={1}
        flat
        linear
        resize={{ debounce: 0, scroll: false, offsetSize: true }}
        gl={{ antialias: false, alpha: true, powerPreference: 'high-performance', stencil: false }}
        style={{ width: '100%', height: '100%', display: 'block', pointerEvents: 'none' }}
      >
        {container ? <BindToContainer container={container} /> : null}
        <TileGrid
          src={src}
          columns={columns}
          rows={rows}
          tileSize={tileSize}
          radius={radius}
          depth={depth}
          gap={gap}
          rotationStrength={rotationStrength}
          easing={easing}
          pointerRef={pointerRef}
          onReady={onReady}
          onFailure={onFailure}
        />
      </Canvas>
      <div
        className="absolute inset-0"
        onPointerEnter={(event) => updatePointer(event, true)}
        onPointerMove={(event) => updatePointer(event, true)}
        onPointerLeave={(event) => updatePointer(event, false)}
      />
    </div>
  )
}
