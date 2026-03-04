// Source of truth on server side:
// - internal/types/tile.go
// - cmd/mapgen/tile_ids.go

export const TILE_DEEP_WATER = 1
export const TILE_SHALLOW_WATER = 3
export const TILE_STONE_PAVING = 12
export const TILE_PLOWED = 14
export const TILE_CONIFEROUS_FOREST = 20
export const TILE_BROADLEAF_FOREST = 25
export const TILE_THICKET = 30
export const TILE_GRASS = 35
export const TILE_HEATH = 40
export const TILE_MOOR = 45
export const TILE_SWAMP_1 = 50
export const TILE_SWAMP_2 = 53
export const TILE_SWAMP_3 = 56
export const TILE_DIRT = 60
export const TILE_CLAY = 64
export const TILE_SAND = 68
export const TILE_MOUNTAIN = 120
export const TILE_VOID = 255

// Tile IDs that mapgen emits and must have a registered tileset.
// TILE_VOID is excluded — it is diagnostic-only and has no visual tileset.
export const RENDERABLE_TILE_IDS: readonly number[] = [
  TILE_DEEP_WATER,
  TILE_SHALLOW_WATER,
  TILE_STONE_PAVING,
  TILE_PLOWED,
  TILE_CONIFEROUS_FOREST,
  TILE_BROADLEAF_FOREST,
  TILE_THICKET,
  TILE_GRASS,
  TILE_HEATH,
  TILE_MOOR,
  TILE_SWAMP_1,
  TILE_SWAMP_2,
  TILE_SWAMP_3,
  TILE_DIRT,
  TILE_CLAY,
  TILE_SAND,
  TILE_MOUNTAIN,
]
