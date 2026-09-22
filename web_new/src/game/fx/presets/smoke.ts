import type { ParticleConfig, Range } from '../ParticleEmitter'
import { validateFxDefinition } from '../validateDefinition.js'

export interface FxDefinition {
  preset: 'smoke'
  texture?: string
  zIndex?: number
  linger?: boolean
  /** Local-pixel adjustment from this preset's smoke source. */
  offset?: readonly [number, number]
  params?: {
    density?: number
    riseSpeed?: number
    tint?: number
    puffSize?: number
    sway?: number
    windResponse?: Range
  }
}

export function smokePreset(definition: FxDefinition): ParticleConfig {
  validateFxDefinition(definition)
  const { density = 1, riseSpeed = 1, tint = 0x9a9a9a, puffSize = 1, sway = 1, windResponse = [0.6, 1.4] } = definition.params ?? {}
  const [offsetX, offsetY] = definition.offset ?? [0, 0]
  return {
    texture: definition.texture ?? 'fx/smoke_puff.png',
    zIndex: definition.zIndex ?? 2,
    // Smoke should finish its visible puffs after an object changes to an unlit state.
    // Effects that must vanish with their owner can still explicitly opt out.
    linger: definition.linger ?? true,
    // Tuned against all seven campfire frames: fade-in overlaps the flame tip.
    position: { x: offsetX, y: -42 + offsetY },
    spread: { x: 3, y: 2 },
    rate: 5 * density,
    maxParticles: Math.min(128, Math.max(1, Math.ceil(24 * density))),
    lifetimeMs: [2200, 3400],
    speed: [16 * riseSpeed, 23 * riseSpeed],
    angle: [-Math.PI * 0.55, -Math.PI * 0.45],
    buoyancy: 9 * riseSpeed,
    drag: 0.35,
    windResponse,
    scale: { start: 0.5 * puffSize, end: 1.7 * puffSize },
    alpha: { start: 0, peak: 0.38, end: 0, fadeInMs: 350, fadeAwayMs: 1600 },
    tint,
    rotation: [-0.25, 0.25],
    spin: [-0.2, 0.2],
    wander: { speed: 4 * sway, frequency: 2.4 },
  }
}
