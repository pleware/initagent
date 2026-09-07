import type { ReactNode } from 'react'

export interface TileImage3DProps {
  src: string
  className?: string
  fallback?: ReactNode
  columns?: number
  rows?: number
  tileSize?: number
  radius?: number
  depth?: number
  gap?: number
  rotationStrength?: number
  easing?: number
  disabled?: boolean
}

export const TILE_IMAGE_DEFAULTS = {
  columns: 30,
  radius: 200,
  depth: 35,
  gap: 0,
  rotationStrength: 3,
  easing: 0.08,
} as const

export const CAMERA_FOV = 35
export const MOUSE_SMOOTHING = 0.12

export type PointerState = {
  x: number
  y: number
  viewW: number
  viewH: number
  active: boolean
}

export function createPointerState(): PointerState {
  return { x: 0, y: 0, viewW: 1, viewH: 1, active: false }
}

export function isWebGLAvailable(): boolean {
  try {
    const canvas = document.createElement('canvas')
    return Boolean(canvas.getContext('webgl2') || canvas.getContext('webgl'))
  } catch {
    return false
  }
}

export function shouldUseTileEffect(disabled?: boolean): boolean {
  if (disabled || typeof window === 'undefined') return false
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return false
  if (window.matchMedia('(pointer: coarse)').matches) return false
  return isWebGLAvailable()
}

export function influenceForce(distance: number, radius: number): number {
  if (radius <= 0 || distance >= radius) return 0
  const t = 1 - distance / radius
  return t * t
}

export function cameraZForHeight(height: number, fovDeg: number): number {
  return height / 2 / Math.tan((fovDeg * Math.PI) / 360)
}

export function coverPlaneSize(
  viewW: number,
  viewH: number,
  imageW: number,
  imageH: number,
): { width: number; height: number } {
  const scale = Math.max(viewW / Math.max(imageW, 1), viewH / Math.max(imageH, 1))
  return { width: imageW * scale, height: imageH * scale }
}

export function rowsFromImage(columns: number, imageW: number, imageH: number): number {
  return Math.max(1, Math.round(columns * (imageH / imageW)))
}

export function gridFromTileSize(
  planeW: number,
  planeH: number,
  tileSize: number,
): { cols: number; rows: number } {
  const size = Math.max(1, tileSize)
  return {
    cols: Math.max(1, Math.round(planeW / size)),
    rows: Math.max(1, Math.round(planeH / size)),
  }
}
