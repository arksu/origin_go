import {
  TILE_BROADLEAF_FOREST, TILE_CLAY, TILE_CONIFEROUS_FOREST, TILE_DEEP_WATER,
  TILE_DIRT, TILE_GRASS, TILE_HEATH, TILE_MOOR, TILE_MOUNTAIN, TILE_PLOWED,
  TILE_SAND, TILE_SHALLOW_WATER, TILE_STONE_PAVING, TILE_SWAMP_1,
  TILE_SWAMP_2, TILE_SWAMP_3, TILE_THICKET,
} from '../tiles/tileIds'

export type MinimapColor = readonly [red: number, green: number, blue: number]

// Match the map generator palette, including the types its PNG exporter omits.
export const MINIMAP_FALLBACK_COLORS: Readonly<Record<number, MinimapColor>> = {
  [TILE_DEEP_WATER]: [14, 49, 100],
  [TILE_SHALLOW_WATER]: [47, 117, 182],
  [TILE_STONE_PAVING]: [132, 132, 132],
  [TILE_PLOWED]: [95, 64, 43],
  [TILE_CONIFEROUS_FOREST]: [43, 88, 43],
  [TILE_BROADLEAF_FOREST]: [62, 123, 60],
  [TILE_THICKET]: [52, 105, 48],
  [TILE_GRASS]: [104, 164, 78],
  [TILE_HEATH]: [151, 143, 83],
  [TILE_MOOR]: [112, 107, 85],
  [TILE_SWAMP_1]: [83, 106, 82],
  [TILE_SWAMP_2]: [74, 94, 69],
  [TILE_SWAMP_3]: [63, 81, 76],
  [TILE_DIRT]: [127, 96, 66],
  [TILE_CLAY]: [158, 124, 93],
  [TILE_SAND]: [217, 196, 127],
  [TILE_MOUNTAIN]: [176, 192, 192],
}

export const MINIMAP_UNKNOWN_COLOR: MinimapColor = [255, 0, 255]
