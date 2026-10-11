import { validateLightDefinition } from './validateDefinition.js'

export { getNightOpacity } from '../../utils/dayNight'

/** Cosmetic light anchored to the owner's displayed ground position. */
export interface LightDefinition {
  /** Radius in projected local pixels at zoom one. */
  radius: number
  offset?: readonly [number, number]
  tint?: number
  strength?: number
  /** Relative strength variation; the evaluated result is clamped to [0, 1]. */
  flicker?: number
}

export interface ResolvedLightDefinition {
  readonly radius: number
  readonly offsetX: number
  readonly offsetY: number
  readonly tint: number
  readonly strength: number
  readonly flicker: number
}

export interface LightMapLayout {
  readonly width: number
  readonly height: number
  readonly bytes: number
}

export const NIGHT_FLOOR = [0.24, 0.28, 0.36] as const
export const LIGHT_MAP_MAX_DIMENSION = 512
export const LIGHT_FLICKER_PERIOD_MS = 1000 / 15
export const LIGHT_RADIAL_SIZE = 256

export function resolveLightDefinition(definition: LightDefinition): ResolvedLightDefinition {
  validateLightDefinition(definition)
  return {
    radius: definition.radius,
    offsetX: definition.offset?.[0] ?? 0,
    offsetY: definition.offset?.[1] ?? 0,
    tint: definition.tint ?? 0xffffff,
    strength: definition.strength ?? 1,
    flicker: definition.flicker ?? 0,
  }
}

/** Physical RT dimensions, independent of canvas DPR; allocate only on size changes. */
export function getLightMapLayout(width: number, height: number): LightMapLayout {
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    throw new Error('Light-map dimensions must be positive and finite')
  }
  const scale = Math.min(0.5, LIGHT_MAP_MAX_DIMENSION / Math.max(width, height))
  const mapWidth = Math.max(1, Math.floor(width * scale))
  const mapHeight = Math.max(1, Math.floor(height * scale))
  return { width: mapWidth, height: mapHeight, bytes: mapWidth * mapHeight * 4 }
}

/** Legacy round light: a fully lit inner half and a linear fade to its radius. */
export function radialLightFalloff(normalizedDistance: number): number {
  return Math.max(0, Math.min(1, 2 * (1 - normalizedDistance)))
}

export function lightFlickerPhase(entityId: number): number {
  let hash = entityId >>> 0
  hash = Math.imul(hash ^ (hash >>> 16), 0x45d9f3b)
  hash = Math.imul(hash ^ (hash >>> 16), 0x45d9f3b)
  return ((hash ^ (hash >>> 16)) >>> 0) / 0x100000000 * Math.PI * 2
}

export function lightFlickerStrength(nowMs: number, strength: number, flicker: number, phase: number): number {
  if (flicker === 0) return strength
  const seconds = nowMs / 1000
  const wave = 0.65 * Math.sin(7.1 * seconds + phase) + 0.35 * Math.sin(11.7 * seconds + 1.7 * phase)
  return Math.max(0, Math.min(1, strength * (1 + flicker * wave)))
}

/** Per-channel multiplier; sources add before saturation and can only restore daytime. */
export function nightLightMultiplier(nightOpacity: number, light: number, floor: number): number {
  return 1 - nightOpacity * (1 - floor) * (1 - Math.max(0, Math.min(1, light)))
}

export function shouldRefreshLightMap(
  nowMs: number,
  lastRefreshMs: number,
  valid: boolean,
  geometryDirty: boolean,
  hasFlicker: boolean,
): boolean {
  return !valid || geometryDirty || (hasFlicker && nowMs - lastRefreshMs >= LIGHT_FLICKER_PERIOD_MS)
}

/** Premultiplied white RGBA avoids upload ambiguity and a separate source texture pass. */
export function createRadialLightPixels(size = LIGHT_RADIAL_SIZE): Uint8Array {
  const pixels = new Uint8Array(size * size * 4)
  const radius = size / 2
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const distance = Math.hypot(x + 0.5 - radius, y + 0.5 - radius) / radius
      const alpha = Math.round(radialLightFalloff(distance) * 255)
      const offset = (y * size + x) * 4
      pixels[offset] = alpha
      pixels[offset + 1] = alpha
      pixels[offset + 2] = alpha
      pixels[offset + 3] = alpha
    }
  }
  return pixels
}
