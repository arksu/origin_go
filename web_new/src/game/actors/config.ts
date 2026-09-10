export const ACTOR_RENDER = {
  cellSize: 128,
  anchorX: 64,
  anchorY: 116,
  supersampling: 2,
  orthoHeight: 1.94 * 128 / 96,
  cameraHeight: (116 - 64) / (96 / 1.94) / Math.cos(Math.PI / 6),
  cycleDistanceTiles: 0.765424409,
  walkSamples: 8,
  maxResidentBytes: 128 * 1024 * 1024,
  maxOutputSlots: 128,
  secondaryUpdateMs: 1000 / 15,
} as const

const ROOT = '/assets/game/characters/male_commoner/realtime/'
export const COMMONER_MODEL = ROOT + 'commoner.glb'
export const EQUIPMENT = {
  linen_wrap: { url: ROOT + 'linen_wrap.glb', slot: 'legs' },
  linen_belt: { url: ROOT + 'linen_belt.glb', slot: 'waist' },
} as const
export type EquipmentId = keyof typeof EQUIPMENT
export const DEFAULT_EQUIPMENT: readonly EquipmentId[] = ['linen_wrap', 'linen_belt']

export const ACTOR_PALETTE = [
  ['#392b1c', '#533425', '#7c4b31', '#a6693f', '#ca8d51', '#e6b06a', '#f4cc86', '#fbe0a5'],
  ['#19180f', '#2d2517', '#4d351e', '#76512d', '#966637', '#b1844b'],
  ['#392b1c', '#544930', '#81704d', '#a9996d', '#d3c08e', '#f0ddae'],
  ['#19180f', '#533425', '#f5e4c5', '#242721'],
] as const
