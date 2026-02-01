import type { Container, MeshGeometry } from 'pixi.js'
import type { AABB } from '../culling/AABB'

/**
 * CPU geometry data for a subchunk (positions, uvs, indices)
 */
export interface SubchunkCpuGeometry {
  positions: Float32Array
  uvs: Float32Array
  indices: Uint32Array
}

/**
 * GPU resources for a subchunk
 */
export interface SubchunkGpuResources {
  geometry: MeshGeometry
  container: Container
  bounds: AABB
}

/**
 * Cached chunk entry with all levels of data
 */
export interface CachedChunk {
  // Identity
  x: number
  y: number
  key: string
  version: number

  // Tile data
  tiles: Uint8Array

  // CPU geometry per subchunk
  cpu: Map<string, SubchunkCpuGeometry>

  // GPU resources per subchunk (optional, level C cache)
  gpu?: Map<string, SubchunkGpuResources>

  // Border/corner data for terrain exclusion
  hasBordersOrCorners: boolean[][]

  // Neighbor state for border refresh
  neighborsMask: number // Bitmask of known neighbors (8 bits for 8 directions)
  needsBorderRefresh: boolean

  // Size tracking
  tilesBytes: number
  cpuBytes: number
  gpuBytes: number

  // Timestamps
  createdAt: number
  lastUsedAt: number
}

/**
 * Build task for the queue
 */
export interface BuildTask {
  chunkKey: string
  x: number
  y: number
  tiles: Uint8Array
  version: number
  priority: number
  buildToken: number
  distanceToCamera: number
  createdAt: number
  isBorderRefresh: boolean
}

/**
 * Cache metrics for debugging
 */
export interface CacheMetrics {
  // Cache stats
  entries: number
  hits: number
  misses: number
  hitRate: number
  bytesTotal: number
  bytesCpu: number
  bytesGpu: number
  bytesTiles: number

  // Eviction stats
  evictionsLru: number
  evictionsTtl: number
  evictionsVersionMismatch: number

  // Build stats
  buildQueueLength: number
  canceledBuilds: number
  cpuBuildMsAvg: number
  gpuUploadMsAvg: number

  // Border refresh stats
  borderRefreshCount: number
  borderRefreshMsAvg: number
}

/**
 * Neighbor direction bitmask values
 */
export const NeighborDirection = {
  TOP_LEFT: 1 << 0,     // (-1, -1)
  TOP: 1 << 1,          // (0, -1)
  TOP_RIGHT: 1 << 2,    // (1, -1)
  LEFT: 1 << 3,         // (-1, 0)
  RIGHT: 1 << 4,        // (1, 0)
  BOTTOM_LEFT: 1 << 5,  // (-1, 1)
  BOTTOM: 1 << 6,       // (0, 1)
  BOTTOM_RIGHT: 1 << 7, // (1, 1)
  ALL: 0xFF,
} as const

/**
 * Map delta coordinates to neighbor direction bit
 */
export function getNeighborBit(dx: number, dy: number): number {
  const idx = (dy + 1) * 3 + (dx + 1)
  // Index mapping: 0=TL, 1=T, 2=TR, 3=L, 4=center(skip), 5=R, 6=BL, 7=B, 8=BR
  const bits = [
    NeighborDirection.TOP_LEFT,
    NeighborDirection.TOP,
    NeighborDirection.TOP_RIGHT,
    NeighborDirection.LEFT,
    0, // center
    NeighborDirection.RIGHT,
    NeighborDirection.BOTTOM_LEFT,
    NeighborDirection.BOTTOM,
    NeighborDirection.BOTTOM_RIGHT,
  ]
  return bits[idx] ?? 0
}
