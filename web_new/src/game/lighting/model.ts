import { validateLightDefinition } from './validateDefinition.js'

export { getNightOpacity } from '../../utils/dayNight'

/** Cosmetic light anchored to the owner's displayed ground position. */
export interface LightDefinition {
  /** Radius in projected local pixels at zoom one. */
  radius: number
  offset?: readonly [number, number]
  tint?: number
  strength?: number
  /** Relative strength variation (clamped to [0, 1]); radius varies by a quarter as much. */
  flicker?: number
  /** Fast noise frequency in Hz; slow noise runs at one quarter of this rate. Default: 4. */
  flickerHz?: number
  /** Fast noise share in [0, 1]; higher values produce sharper changes. Default: 0.25. */
  flickerFastWeight?: number
  /** Maximum horizontal/vertical halo displacement in local pixels. Default: [0, 0]. */
  sway?: readonly [number, number]
  /** Sway noise frequency in Hz. Default: 1. */
  swayHz?: number
}

export interface ResolvedLightDefinition {
  readonly radius: number
  readonly offsetX: number
  readonly offsetY: number
  readonly tint: number
  readonly strength: number
  readonly flicker: number
  readonly flickerHz: number
  readonly flickerFastWeight: number
  readonly swayX: number
  readonly swayY: number
  readonly swayHz: number
}

export interface LightMapLayout {
  readonly width: number
  readonly height: number
  readonly bytes: number
}

export const NIGHT_FLOOR = [0.24, 0.28, 0.36] as const
export const LIGHT_MAP_MAX_DIMENSION = 512
export const LIGHT_FLICKER_PERIOD_MS = 1000 / 15
export const LIGHT_FLICKER_RADIUS_RATIO = 0.25
export const LIGHT_RADIAL_SIZE = 256
const DEFAULT_FLICKER_HZ = 4
const DEFAULT_FLICKER_FAST_WEIGHT = 0.25
const DEFAULT_SWAY_HZ = 1

export function resolveLightDefinition(definition: LightDefinition): ResolvedLightDefinition {
  validateLightDefinition(definition)
  return {
    radius: definition.radius,
    offsetX: definition.offset?.[0] ?? 0,
    offsetY: definition.offset?.[1] ?? 0,
    tint: definition.tint ?? 0xffffff,
    strength: definition.strength ?? 1,
    flicker: definition.flicker ?? 0,
    flickerHz: definition.flickerHz ?? DEFAULT_FLICKER_HZ,
    flickerFastWeight: definition.flickerFastWeight ?? DEFAULT_FLICKER_FAST_WEIGHT,
    swayX: definition.sway?.[0] ?? 0,
    swayY: definition.sway?.[1] ?? 0,
    swayHz: definition.swayHz ?? DEFAULT_SWAY_HZ,
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

function hashLightNoise(value: number): number {
  let hash = value >>> 0
  hash = Math.imul(hash ^ (hash >>> 16), 0x45d9f3b)
  hash = Math.imul(hash ^ (hash >>> 16), 0x45d9f3b)
  return (hash ^ (hash >>> 16)) >>> 0
}

export function lightFlickerSeed(entityId: number): number {
  return hashLightNoise(entityId)
}

function smoothLightNoise(position: number, seed: number): number {
  const cell = Math.floor(position)
  const fraction = position - cell
  const blend = fraction * fraction * (3 - 2 * fraction)
  const start = hashLightNoise(seed ^ cell) / 0xffffffff * 2 - 1
  const end = hashLightNoise(seed ^ (cell + 1)) / 0xffffffff * 2 - 1
  return start + (end - start) * blend
}

/** Stateless two-band value noise, shared by brightness and radius on each map refresh. */
export function lightFlickerAmount(
  nowMs: number, flicker: number, seed: number,
  frequencyHz = DEFAULT_FLICKER_HZ, fastWeight = DEFAULT_FLICKER_FAST_WEIGHT,
): number {
  if (flicker === 0) return 0
  const seconds = nowMs / 1000
  return flicker * ((1 - fastWeight) * smoothLightNoise(seconds * (frequencyHz / 4), seed) +
    fastWeight * smoothLightNoise(seconds * frequencyHz, seed ^ 0x9e3779b9))
}

/** Separate noise seeds let the halo drift independently of brightness and of its other axis. */
export function lightSwayOffset(nowMs: number, amplitude: number, seed: number, frequencyHz = DEFAULT_SWAY_HZ): number {
  if (amplitude === 0) return 0
  return amplitude * smoothLightNoise(nowMs / 1000 * frequencyHz, seed)
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
  hasAnimation: boolean,
): boolean {
  return !valid || geometryDirty || (hasAnimation && nowMs - lastRefreshMs >= LIGHT_FLICKER_PERIOD_MS)
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
