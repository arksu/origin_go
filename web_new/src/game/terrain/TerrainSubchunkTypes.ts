import type { Sprite } from 'pixi.js'

/**
 * Terrain subchunk state
 */
export const enum TerrainSubchunkState {
  NotBuilt = 0,
  Building = 1,
  BuiltHidden = 2,
  BuiltVisible = 3,
}

/**
 * Terrain subchunk data
 */
export interface TerrainSubchunk {
  key: string // "${chunkKey}:${cx},${cy}"
  chunkKey: string
  cx: number
  cy: number
  state: TerrainSubchunkState
  sprites: Sprite[]
  epoch: number // For cancellation
}

/**
 * Build task for terrain subchunk
 */
export interface TerrainBuildTask {
  subchunkKey: string
  chunkKey: string
  chunkX: number
  chunkY: number
  cx: number
  cy: number
  tiles: Uint8Array
  hasBordersOrCorners: boolean[][]
  epoch: number
  distanceToCamera: number
  createdAt: number
}

/**
 * Terrain metrics for debugging
 */
export interface TerrainMetrics {
  spritesActive: number
  spritesPooled: number
  spritesCreatedTotal: number
  subchunksQueued: number
  subchunksDone: number
  subchunksCanceled: number
  buildMsAvg: number
  buildMsP95: number
  clearMsAvg: number
  clearMsP95: number
}
