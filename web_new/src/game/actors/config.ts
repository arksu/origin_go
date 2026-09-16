import { LOCOMOTION_STOP_MS, LOCOMOTION_TRANSITION_MS } from '../movementTiming'

export const ACTOR_RENDER = {
  cameraElevation: Math.PI / 6,
  facingHysteresis: 3 * Math.PI / 180,
  facingConfirmMs: 120,
  facingHoldMs: 250,
  cellSize: 128,
  anchorX: 64,
  anchorY: 116,
  supersampling: 2,
  orthoHeight: 1.94 * 128 / 96,
  cameraHeight: (116 - 64) / (96 / 1.94) / Math.cos(Math.PI / 6),
  cycleDistanceTiles: 1.677975879375,
  walkSamples: 8,
  locomotionBlendMs: LOCOMOTION_TRANSITION_MS,
  locomotionStopMs: LOCOMOTION_STOP_MS,
  maxResidentBytes: 128 * 1024 * 1024,
  maxOutputSlots: 128,
} as const

/**
 * Runtime contract for the future Settings window. These values only control
 * how frequently 3D poses are copied into the Pixi texture; movement simulation
 * and world interpolation still run at the display frame rate.
 */
export interface ActorRenderSettings {
  mode: 'hybrid3d' | 'baked8'
  localAnimationFps: number
  remoteAnimationFps: number
  turnDurationMs: number
  renderStationaryChangesImmediately: boolean
}

export const DEFAULT_ACTOR_RENDER_SETTINGS: Readonly<ActorRenderSettings> = Object.freeze({
  mode: 'hybrid3d',
  localAnimationFps: 60,
  remoteAnimationFps: 20,
  turnDurationMs: 500,
  renderStationaryChangesImmediately: true,
})

export function resolveActorRenderSettings(overrides: Partial<ActorRenderSettings> = {}): Readonly<ActorRenderSettings> {
  const settings = { ...DEFAULT_ACTOR_RENDER_SETTINGS, ...overrides }
  for (const [name, rate] of Object.entries({ localAnimationFps: settings.localAnimationFps, remoteAnimationFps: settings.remoteAnimationFps })) {
    if (!Number.isFinite(rate) || rate < 1 || rate > 120) throw new Error(`${name} must be between 1 and 120`)
  }
  if (settings.mode !== 'hybrid3d' && settings.mode !== 'baked8') throw new Error('mode must be hybrid3d or baked8')
  if (!Number.isFinite(settings.turnDurationMs) || settings.turnDurationMs < 1 || settings.turnDurationMs > 5000) throw new Error('turnDurationMs must be between 1 and 5000')
  if (typeof settings.renderStationaryChangesImmediately !== 'boolean') throw new Error('renderStationaryChangesImmediately must be boolean')
  return Object.freeze(settings)
}

const ROOT = '/assets/game/characters/male_commoner/realtime/'
export const COMMONER_MODEL = ROOT + 'commoner_meshy.glb'
export { EQUIPMENT, DEFAULT_EQUIPMENT } from './equipment'

export const ACTOR_PALETTE = [
  ['#392b1c', '#533425', '#7c4b31', '#a6693f', '#ca8d51', '#e6b06a', '#f4cc86', '#fbe0a5'],
  ['#19180f', '#2d2517', '#4d351e', '#76512d', '#966637', '#b1844b'],
  ['#392b1c', '#544930', '#81704d', '#a9996d', '#d3c08e', '#f0ddae'],
  ['#19180f', '#533425', '#f5e4c5', '#242721'],
] as const
