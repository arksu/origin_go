import type { SoundProfile } from '../types/soundDefs'
import {
  TILE_BROADLEAF_FOREST, TILE_CLAY, TILE_CONIFEROUS_FOREST,
  TILE_DIRT, TILE_MOUNTAIN, TILE_PLOWED, TILE_SHALLOW_WATER,
} from './tiles/tileIds'

export const DEFAULT_FOOTSTEP_SOUND_KEY = 'footstep'

export const FOOTSTEP_TILE_SOUND_KEYS: Readonly<Record<number, string>> = Object.freeze({
  [TILE_SHALLOW_WATER]: 'footstep_shallow_water',
  [TILE_MOUNTAIN]: 'footstep_stone',
  [TILE_DIRT]: 'footstep_gravel',
  [TILE_CLAY]: 'footstep_gravel',
  [TILE_PLOWED]: 'footstep_gravel',
  [TILE_CONIFEROUS_FOREST]: 'footstep_forest_leaves',
  [TILE_BROADLEAF_FOREST]: 'footstep_forest_leaves',
})

export function locomotionSoundKey(soundKey: string, tileType: number | undefined): string {
  // Only footsteps use the ground material; other locomotion contacts keep their authored sound.
  if (soundKey !== DEFAULT_FOOTSTEP_SOUND_KEY || tileType === undefined) return soundKey
  return FOOTSTEP_TILE_SOUND_KEYS[tileType] ?? DEFAULT_FOOTSTEP_SOUND_KEY
}

export function validateFootstepSoundProfiles(profiles: Readonly<Record<string, SoundProfile>>): void {
  const keys = new Set([DEFAULT_FOOTSTEP_SOUND_KEY, ...Object.values(FOOTSTEP_TILE_SOUND_KEYS)])
  for (const key of keys) {
    if (profiles[key]?.mode !== 'local') throw new Error(`Footstep configuration: missing local sound ${key}`)
  }
}
