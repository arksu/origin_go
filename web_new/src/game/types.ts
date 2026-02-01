export interface DebugInfo {
  fps: number
  cameraX: number
  cameraY: number
  zoom: number
  viewportWidth: number
  viewportHeight: number
  lastClickScreenX: number
  lastClickScreenY: number
  lastClickWorldX: number
  lastClickWorldY: number
  objectsCount: number
  chunksLoaded: number
  // Movement metrics
  rttMs?: number
  jitterMs?: number
  timeOffsetMs?: number
  interpolationDelayMs?: number
  moveEntityCount?: number
  totalSnapCount?: number
  totalIgnoredOutOfOrder?: number
  totalBufferUnderrun?: number
  // Culling metrics
  subchunksTotal?: number
  subchunksVisible?: number
  subchunksCulled?: number
  terrainTotal?: number
  terrainVisible?: number
  terrainCulled?: number
  objectsVisibleCulling?: number
  objectsCulled?: number
  cullingTimeMs?: number
  // Cache metrics
  cacheEntries?: number
  cacheHitRate?: number
  cacheBytesKb?: number
  buildQueueLength?: number
  buildAvgMs?: number
}

export interface ScreenPoint {
  x: number
  y: number
}
