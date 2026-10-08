import type { SoundProfile } from '../types/soundDefs'
import {
  TILE_BROADLEAF_FOREST, TILE_CLAY, TILE_CONIFEROUS_FOREST, TILE_DEEP_WATER,
  TILE_DIRT, TILE_GRASS, TILE_HEATH, TILE_MOOR, TILE_MOUNTAIN, TILE_PLOWED,
  TILE_SAND, TILE_SHALLOW_WATER, TILE_STONE_PAVING, TILE_SWAMP_1, TILE_SWAMP_2,
  TILE_SWAMP_3, TILE_THICKET,
} from './tiles/tileIds'

export const DEFAULT_FOOTSTEP_SOUND_KEY = 'footstep'

export type FootstepTileConfig = Readonly<Partial<Record<number, Readonly<{
  soundKey?: string
  volume?: number
}>>>>

// Volume is absolute (0–1), before distance and user settings. Undefined inherits the sound profile.
// Tiles without a soundKey use the default recording, even when their volume is customized.
export const FOOTSTEP_TILES: FootstepTileConfig = Object.freeze({
  [TILE_DEEP_WATER]: { volume: undefined },
  [TILE_SHALLOW_WATER]: { soundKey: 'footstep_shallow_water', volume: undefined },
  [TILE_STONE_PAVING]: { volume: undefined },
  [TILE_PLOWED]: { soundKey: 'footstep_gravel', volume: undefined },
  [TILE_CONIFEROUS_FOREST]: { soundKey: 'footstep_forest_leaves', volume: undefined },
  [TILE_BROADLEAF_FOREST]: { soundKey: 'footstep_forest_leaves', volume: undefined },
  [TILE_THICKET]: { volume: undefined },
  [TILE_GRASS]: { volume: undefined },
  [TILE_HEATH]: { volume: undefined },
  [TILE_MOOR]: { volume: undefined },
  [TILE_SWAMP_1]: { volume: undefined },
  [TILE_SWAMP_2]: { volume: undefined },
  [TILE_SWAMP_3]: { volume: undefined },
  [TILE_DIRT]: { soundKey: 'footstep_gravel', volume: undefined },
  [TILE_CLAY]: { soundKey: 'footstep_gravel', volume: undefined },
  [TILE_SAND]: { volume: undefined },
  [TILE_MOUNTAIN]: { soundKey: 'footstep_stone', volume: undefined },
})

export function locomotionSoundKey(soundKey: string, tileType: number | undefined, tiles: FootstepTileConfig = FOOTSTEP_TILES): string {
  // Only footsteps use the ground material; other locomotion contacts keep their authored sound.
  if (soundKey !== DEFAULT_FOOTSTEP_SOUND_KEY || tileType === undefined) return soundKey
  return tiles[tileType]?.soundKey ?? DEFAULT_FOOTSTEP_SOUND_KEY
}

export function locomotionVolume(soundKey: string, tileType: number | undefined, tiles: FootstepTileConfig = FOOTSTEP_TILES): number | undefined {
  if (soundKey !== DEFAULT_FOOTSTEP_SOUND_KEY || tileType === undefined) return undefined
  return tiles[tileType]?.volume
}

export function validateFootstepTileConfig(tiles: FootstepTileConfig): void {
  for (const [tile, settings] of Object.entries(tiles)) {
    const tileType = Number(tile)
    if (!Number.isInteger(tileType) || tileType < 0 || tileType > 255 || !settings) {
      throw new Error(`Footstep configuration: invalid tile ${tile}`)
    }
    if (settings.volume !== undefined && (!Number.isFinite(settings.volume) || settings.volume < 0 || settings.volume > 1)) {
      throw new Error(`Footstep configuration: tile ${tile} volume must be finite and between 0 and 1`)
    }
  }
}

export function validateFootstepSoundProfiles(profiles: Readonly<Record<string, SoundProfile>>, tiles: FootstepTileConfig = FOOTSTEP_TILES): void {
  validateFootstepTileConfig(tiles)
  const keys = new Set([DEFAULT_FOOTSTEP_SOUND_KEY, ...Object.values(tiles).map(settings => settings?.soundKey ?? DEFAULT_FOOTSTEP_SOUND_KEY)])
  for (const key of keys) {
    if (profiles[key]?.mode !== 'local') throw new Error(`Footstep configuration: missing local sound ${key}`)
  }
}
