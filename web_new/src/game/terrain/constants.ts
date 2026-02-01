/**
 * Terrain system configuration constants
 */

// Subchunk configuration (terrain uses same DIVIDER as Chunk.ts)
export const TERRAIN_SUBCHUNK_DIVIDER = 8

// Incremental build budget
export const TERRAIN_BUILD_BUDGET_MS = 2
export const MAX_TERRAIN_SUBCHUNKS_PER_FRAME = 4

// Visibility rect (in subchunks) - defines rectangular area around camera
// Width and height of visible area (camera is at center)
export const TERRAIN_VISIBLE_WIDTH_SUBCHUNKS = 7 // 2.5 subchunks on each side
export const TERRAIN_VISIBLE_HEIGHT_SUBCHUNKS = 7 // 2.5 subchunks on each side
// Hysteresis rect - larger area to prevent flickering
export const TERRAIN_HIDE_WIDTH_SUBCHUNKS = 7.5 // 3.5 subchunks on each side
export const TERRAIN_HIDE_HEIGHT_SUBCHUNKS = 7.5 // 3.5 subchunks on each side

// Sprite pool configuration
export const MAX_TERRAIN_SPRITES_IN_POOL = 20000
export const TERRAIN_POOL_SHRINK_THRESHOLD = 30000 // Shrink pool if exceeds this
export const TERRAIN_POOL_SHRINK_TARGET = 20000

// Z-index base for terrain sprites
export const TERRAIN_BASE_Z_INDEX = 100
