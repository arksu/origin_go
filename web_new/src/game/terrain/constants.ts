/**
 * Terrain system configuration constants
 */

// Subchunk configuration (terrain uses same DIVIDER as Chunk.ts)
export const TERRAIN_SUBCHUNK_DIVIDER = 4

// Incremental build budget
export const TERRAIN_BUILD_BUDGET_MS = 2
export const MAX_TERRAIN_SUBCHUNKS_PER_FRAME = 4

// Visibility radius (in subchunks)
export const TERRAIN_SHOW_RADIUS_SUBCHUNKS = 3
export const TERRAIN_HIDE_RADIUS_SUBCHUNKS = 4 // Hysteresis to prevent flickering

// Sprite pool configuration
export const MAX_TERRAIN_SPRITES_IN_POOL = 2000
export const TERRAIN_POOL_SHRINK_THRESHOLD = 3000 // Shrink pool if exceeds this
export const TERRAIN_POOL_SHRINK_TARGET = 2000

// Z-index base for terrain sprites
export const TERRAIN_BASE_Z_INDEX = 100
