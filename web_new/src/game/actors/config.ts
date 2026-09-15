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
  cycleDistanceTiles: 1.20678051125,
  walkSamples: 8,
  maxResidentBytes: 128 * 1024 * 1024,
  maxOutputSlots: 128,
  secondaryUpdateMs: 1000 / 15,
} as const

const ROOT = '/assets/game/characters/male_commoner/realtime/'
export const COMMONER_MODEL = ROOT + 'commoner_meshy.glb'
// The old linen meshes use different bind matrices and cannot fit this rig.
// Populate this catalog with garments authored against the Meshy skeleton.
export const EQUIPMENT: Readonly<Record<string, { url: string; slot: string }>> = {}
export type EquipmentId = string
// Meshy supplied the wrap and belt welded into the character's base mesh.
export const DEFAULT_EQUIPMENT: readonly EquipmentId[] = []

export const ACTOR_PALETTE = [
  ['#392b1c', '#533425', '#7c4b31', '#a6693f', '#ca8d51', '#e6b06a', '#f4cc86', '#fbe0a5'],
  ['#19180f', '#2d2517', '#4d351e', '#76512d', '#966637', '#b1844b'],
  ['#392b1c', '#544930', '#81704d', '#a9996d', '#d3c08e', '#f0ddae'],
  ['#19180f', '#533425', '#f5e4c5', '#242721'],
] as const
