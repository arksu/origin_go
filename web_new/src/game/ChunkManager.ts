import { Container, Assets, Spritesheet } from 'pixi.js'
import { Chunk, type ChunkBuildResult } from './Chunk'
import { initTileSets } from './tiles/tileSetLoader'
import { setWorldParams, getChunkSize } from './tiles/Tile'
import { terrainManager } from './terrain'
import { cullingController } from './culling'
import {
  chunkCache,
  buildQueue,
  cacheMetrics,
  BuildPriority,
  type CachedChunk,
  type BuildTask,
  getNeighborBit,
  NeighborDirection,
  BORDER_REFRESH_DELAY_MS,
} from './cache'

interface PendingChunk {
  x: number
  y: number
  tiles: Uint8Array
  version: number
}

/**
 * ChunkManager with LRU cache and prioritized build queue.
 * Implements deferred border refresh to avoid rebuild cascades.
 */
export class ChunkManager {
  private container: Container
  private chunks: Map<string, Chunk> = new Map()
  private spritesheet: Spritesheet | null = null
  private initialized: boolean = false
  private pendingChunks: PendingChunk[] = []
  private objectsContainer: Container | null = null

  // Build token for cancellation
  private buildTokens: Map<string, number> = new Map()

  // Camera position for priority calculation
  private cameraX: number = 0
  private cameraY: number = 0

  // Border refresh debounce
  private borderRefreshPending: Set<string> = new Set()
  private borderRefreshTimeoutId: ReturnType<typeof setTimeout> | null = null

  constructor() {
    this.container = new Container()
    this.container.sortableChildren = true
  }

  getContainer(): Container {
    return this.container
  }

  setObjectsContainer(container: Container): void {
    this.objectsContainer = container
  }

  async init(): Promise<void> {
    if (this.initialized) {
      console.log('[ChunkManager] Already initialized')
      return
    }

    console.log('[ChunkManager] Initializing...')
    initTileSets()

    console.log('[ChunkManager] Loading spritesheet from /assets/game/tiles.json...')
    this.spritesheet = await Assets.load('/assets/game/tiles.json')
    console.log('[ChunkManager] Spritesheet loaded:', this.spritesheet ? 'OK' : 'FAILED')

    if (this.spritesheet) {
      const textureNames = Object.keys(this.spritesheet.textures)
      console.log('[ChunkManager] Available textures:', textureNames.length, 'first 5:', textureNames.slice(0, 5))
    }

    this.initialized = true

    if (this.objectsContainer && this.spritesheet) {
      terrainManager.init(this.objectsContainer, this.spritesheet)
      console.log('[ChunkManager] TerrainManager initialized')
    }

    console.log('[ChunkManager] Initialization complete')

    // Process any chunks that arrived before spritesheet was loaded
    if (this.pendingChunks.length > 0) {
      console.log(`[ChunkManager] Processing ${this.pendingChunks.length} pending chunks...`)
      for (const pending of this.pendingChunks) {
        this.loadChunkInternal(pending.x, pending.y, pending.tiles, pending.version)
      }
      this.pendingChunks = []
    }
  }

  setWorldParams(coordPerTile: number, chunkSize: number): void {
    setWorldParams(coordPerTile, chunkSize)
  }

  /**
   * Update camera position for priority calculation.
   */
  setCameraPosition(x: number, y: number): void {
    const prevX = this.cameraX
    const prevY = this.cameraY
    this.cameraX = x
    this.cameraY = y

    if (Math.abs(x - prevX) > 1 || Math.abs(y - prevY) > 1) {
      console.log(`[ChunkManager] Camera moved: (${x.toFixed(0)}, ${y.toFixed(0)})`)
    }
  }

  /**
   * Load a chunk from server. Uses cache if available and version matches.
   */
  loadChunk(x: number, y: number, tiles: Uint8Array, version: number = 0): void {
    console.log(`[ChunkManager] loadChunk(${x}, ${y}) tiles.length=${tiles.length}, version=${version}`)

    if (!this.spritesheet) {
      console.log(`[ChunkManager] Spritesheet not ready, buffering chunk (${x}, ${y})`)
      this.pendingChunks.push({ x, y, tiles, version })
      return
    }

    this.loadChunkInternal(x, y, tiles, version)
  }

  private loadChunkInternal(x: number, y: number, tiles: Uint8Array, version: number): void {
    if (!this.spritesheet) {
      console.warn('[ChunkManager] loadChunkInternal called but spritesheet not loaded!')
      return
    }

    const key = `${x},${y}`

    // Cancel any pending build for this chunk
    this.cancelBuild(key)

    // Check cache for valid entry
    if (chunkCache.hasValidEntry(key, version)) {
      const cached = chunkCache.get(key)
      if (cached) {
        console.log(`[ChunkManager] Cache hit for chunk ${key}`)
        this.attachFromCache(cached)
        return
      }
    }

    // Cache miss - enqueue build
    const priority = this.calculatePriority(x, y)
    const distance = this.calculateDistance(x, y)
    const buildToken = buildQueue.nextBuildToken()
    this.buildTokens.set(key, buildToken)

    const task: BuildTask = {
      chunkKey: key,
      x,
      y,
      tiles,
      version,
      priority,
      buildToken,
      distanceToCamera: distance,
      createdAt: performance.now(),
      isBorderRefresh: false,
    }

    // Build immediately on first load (not from cache)
    // Only use queue for border refresh tasks
    this.processBuildTask(task)
  }

  /**
   * Attach a chunk from cache without rebuilding.
   */
  private attachFromCache(cached: CachedChunk): void {
    const key = cached.key

    let chunk = this.chunks.get(key)
    if (!chunk) {
      chunk = new Chunk(cached.x, cached.y)
      this.chunks.set(key, chunk)
      this.container.addChild(chunk.getContainer())
    }

    // If GPU resources are cached, reuse them
    if (cached.gpu && cached.gpu.size > 0) {
      // Reattach GPU resources
      this.reattachGpuResources(chunk, cached)
    } else {
      // Need to rebuild from CPU cache
      this.rebuildFromCpuCache(chunk, cached)
    }

    chunk.visible = true
    this.registerSubchunksForCulling(chunk)

    // Update neighbor mask and check if border refresh needed
    this.updateNeighborMask(cached)
    if (cached.needsBorderRefresh) {
      this.scheduleBorderRefresh(key)
    }

    // Regenerate terrain
    terrainManager.generateTerrainForChunk(cached.x, cached.y, cached.tiles, cached.hasBordersOrCorners)

    // Notify neighbors about this chunk
    this.notifyNeighborsOfLoad(cached.x, cached.y)
  }

  /**
   * Reattach GPU resources from cache.
   */
  private reattachGpuResources(chunk: Chunk, cached: CachedChunk): void {
    // GPU cache reattachment - for now, rebuild since GPU resources
    // are tied to the chunk's container hierarchy
    this.rebuildFromCpuCache(chunk, cached)
  }

  /**
   * Rebuild chunk visuals from CPU cached geometry.
   */
  private rebuildFromCpuCache(chunk: Chunk, cached: CachedChunk): void {
    // For now, do a full rebuild since we need the spritesheet
    // In future, we can optimize by storing pre-built geometry
    if (!this.spritesheet) return

    this.unregisterSubchunksFromCulling(chunk)
    const neighborTiles = this.getNeighborTiles(cached.x, cached.y)
    chunk.buildTiles(cached.tiles, this.spritesheet, neighborTiles)
    this.registerSubchunksForCulling(chunk)
  }

  /**
   * Process a build task - build the chunk.
   */
  private processBuildTask(task: BuildTask): void {
    const taskStart = performance.now()

    // Check if task is still valid
    const currentToken = this.buildTokens.get(task.chunkKey)
    if (currentToken !== task.buildToken) {
      console.log(`[ChunkManager] Build task for ${task.chunkKey} canceled (token mismatch)`)
      buildQueue.buildComplete()
      return
    }

    if (!this.spritesheet) {
      buildQueue.buildComplete()
      return
    }

    const key = task.chunkKey

    let chunk = this.chunks.get(key)
    if (!chunk) {
      chunk = new Chunk(task.x, task.y)
      this.chunks.set(key, chunk)
      this.container.addChild(chunk.getContainer())
    }

    // Unregister old subchunks before rebuild
    this.unregisterSubchunksFromCulling(chunk)

    const neighborTiles = this.getNeighborTiles(task.x, task.y)

    const buildStart = performance.now()
    const buildResult = chunk.buildTiles(task.tiles, this.spritesheet, neighborTiles)
    const buildTime = performance.now() - buildStart

    chunk.visible = true

    // Register new subchunks for culling
    this.registerSubchunksForCulling(chunk)

    // Generate terrain
    const terrainStart = performance.now()
    console.log(`[ChunkManager] Building terrain for chunk (${task.x},${task.y})`)
    terrainManager.generateTerrainForChunk(task.x, task.y, task.tiles, buildResult.hasBordersOrCorners)
    const terrainTime = performance.now() - terrainStart

    // Cache the chunk
    this.cacheChunk(task.x, task.y, task.tiles, task.version, buildResult, neighborTiles)

    // Record build time
    buildQueue.recordCpuBuildTime(buildTime)
    buildQueue.buildComplete()

    const totalTime = performance.now() - taskStart

    if (totalTime > 16 || buildTime > 8 || terrainTime > 8) {
      console.warn(`[ChunkManager] SLOW BUILD chunk ${key}: total=${totalTime.toFixed(2)}ms, build=${buildTime.toFixed(2)}ms, terrain=${terrainTime.toFixed(2)}ms`)
    } else {
      console.log(`[ChunkManager] Built chunk ${key}: total=${totalTime.toFixed(2)}ms, build=${buildTime.toFixed(2)}ms, terrain=${terrainTime.toFixed(2)}ms`)
    }

    // Notify neighbors (deferred border refresh instead of immediate rebuild)
    this.notifyNeighborsOfLoad(task.x, task.y)
  }

  /**
   * Cache a built chunk.
   */
  private cacheChunk(
    x: number,
    y: number,
    tiles: Uint8Array,
    version: number,
    buildResult: ChunkBuildResult,
    neighborTiles: Map<string, Uint8Array>,
  ): void {
    const key = `${x},${y}`
    const neighborsMask = this.calculateNeighborsMask(neighborTiles)

    const cached: CachedChunk = {
      x,
      y,
      key,
      version,
      tiles: tiles.slice(), // Copy to avoid mutation
      cpu: new Map(), // CPU geometry stored in Chunk, not extracted here for simplicity
      hasBordersOrCorners: buildResult.hasBordersOrCorners,
      neighborsMask,
      needsBorderRefresh: neighborsMask !== NeighborDirection.ALL,
      tilesBytes: tiles.byteLength,
      cpuBytes: 0, // Would need to extract from Chunk
      gpuBytes: 0, // Would need to track GPU buffer sizes
      createdAt: performance.now(),
      lastUsedAt: performance.now(),
    }

    chunkCache.set(cached)
  }

  /**
   * Calculate neighbor mask from available neighbor tiles.
   */
  private calculateNeighborsMask(neighborTiles: Map<string, Uint8Array>): number {
    // If we have all 8 neighbors, return full mask
    return neighborTiles.size >= 8 ? NeighborDirection.ALL : 0
  }

  /**
   * Update neighbor mask for a cached chunk.
   */
  private updateNeighborMask(cached: CachedChunk): void {
    let mask = 0

    for (let dx = -1; dx <= 1; dx++) {
      for (let dy = -1; dy <= 1; dy++) {
        if (dx === 0 && dy === 0) continue
        const neighborKey = `${cached.x + dx},${cached.y + dy}`
        if (this.chunks.has(neighborKey)) {
          mask |= getNeighborBit(dx, dy)
        }
      }
    }

    cached.neighborsMask = mask
    cached.needsBorderRefresh = mask !== NeighborDirection.ALL
  }

  /**
   * Notify neighbors that a chunk has loaded - schedule border refresh.
   */
  private notifyNeighborsOfLoad(x: number, y: number): void {
    for (let dx = -1; dx <= 1; dx++) {
      for (let dy = -1; dy <= 1; dy++) {
        if (dx === 0 && dy === 0) continue

        const neighborKey = `${x + dx},${y + dy}`
        const neighborChunk = this.chunks.get(neighborKey)

        if (neighborChunk && neighborChunk.getTiles()) {
          // Schedule border refresh instead of immediate rebuild
          this.scheduleBorderRefresh(neighborKey)
        }
      }
    }
  }

  /**
   * Schedule a border refresh for a chunk (debounced).
   */
  private scheduleBorderRefresh(chunkKey: string): void {
    this.borderRefreshPending.add(chunkKey)

    if (this.borderRefreshTimeoutId) {
      clearTimeout(this.borderRefreshTimeoutId)
    }

    this.borderRefreshTimeoutId = setTimeout(() => {
      this.processBorderRefreshQueue()
    }, BORDER_REFRESH_DELAY_MS)
  }

  /**
   * Process all pending border refresh tasks.
   */
  private processBorderRefreshQueue(): void {
    this.borderRefreshTimeoutId = null

    for (const chunkKey of this.borderRefreshPending) {
      const chunk = this.chunks.get(chunkKey)
      if (!chunk || !chunk.getTiles() || !this.spritesheet) continue

      const [x, y] = chunkKey.split(',').map(Number)
      const tiles = chunk.getTiles()!

      // Enqueue as low priority border refresh
      const buildToken = buildQueue.nextBuildToken()
      this.buildTokens.set(chunkKey, buildToken)

      const task: BuildTask = {
        chunkKey,
        x: x!,
        y: y!,
        tiles,
        version: 0, // Border refresh doesn't change version
        priority: BuildPriority.P1_NEARBY,
        buildToken,
        distanceToCamera: this.calculateDistance(x!, y!),
        createdAt: performance.now(),
        isBorderRefresh: true,
      }

      buildQueue.enqueue(task)
    }

    this.borderRefreshPending.clear()
  }

  /**
   * Calculate build priority based on distance to camera.
   */
  private calculatePriority(x: number, y: number): number {
    const distance = this.calculateDistance(x, y)

    // P0: chunks within 1 chunk distance (visible 2x2)
    if (distance <= 1.5) {
      return BuildPriority.P0_VISIBLE
    }
    // P1: chunks within 2 chunk distance
    if (distance <= 2.5) {
      return BuildPriority.P1_NEARBY
    }
    // P2: distant chunks
    return BuildPriority.P2_DISTANT
  }

  /**
   * Calculate distance from chunk to camera (in chunk units).
   */
  private calculateDistance(x: number, y: number): number {
    const chunkSize = getChunkSize()
    const cameraChunkX = Math.floor(this.cameraX / chunkSize)
    const cameraChunkY = Math.floor(this.cameraY / chunkSize)
    const dx = x - cameraChunkX
    const dy = y - cameraChunkY
    const distance = Math.sqrt(dx * dx + dy * dy)
    console.log(`[ChunkManager] Distance from chunk (${x},${y}) to camera (${cameraChunkX},${cameraChunkY}): ${distance.toFixed(2)}`)
    return distance
  }

  /**
   * Cancel a pending build for a chunk.
   */
  private cancelBuild(chunkKey: string): void {
    buildQueue.cancel(chunkKey)
    this.buildTokens.delete(chunkKey)
  }

  /**
   * Process build queue - call this from render loop.
   */
  update(): void {
    const tasks = buildQueue.getTasksForFrame()
    for (const task of tasks) {
      this.processBuildTask(task)
    }
  }

  /**
   * Unload a chunk - hide but keep in cache.
   */
  unloadChunk(x: number, y: number): void {
    const key = `${x},${y}`
    const chunk = this.chunks.get(key)

    if (chunk) {
      // Hide chunk
      chunk.visible = false
      this.unregisterSubchunksFromCulling(chunk)
      terrainManager.clearChunk(x, y)

      // Cancel any pending builds
      this.cancelBuild(key)

      // Keep in cache - don't destroy GPU resources yet
      // Cache entry already exists from build, just update lastUsedAt
      const cached = chunkCache.get(key)
      if (cached) {
        cached.lastUsedAt = performance.now()
      }
    }
  }

  /**
   * Remove a chunk completely (not just unload).
   */
  removeChunk(x: number, y: number): void {
    const key = `${x},${y}`
    const chunk = this.chunks.get(key)

    if (chunk) {
      this.unregisterSubchunksFromCulling(chunk)
      this.container.removeChild(chunk.getContainer())
      chunk.destroy()
      this.chunks.delete(key)
      terrainManager.clearChunk(x, y)
      this.cancelBuild(key)
      chunkCache.evict(key)
    }
  }

  /**
   * Register all subchunks of a chunk for culling.
   */
  private registerSubchunksForCulling(chunk: Chunk): void {
    for (const subchunkData of chunk.getSubchunkDataList()) {
      cullingController.registerSubchunk(
        subchunkData.key,
        subchunkData.container,
        subchunkData.bounds,
      )
    }
  }

  /**
   * Unregister all subchunks of a chunk from culling.
   */
  private unregisterSubchunksFromCulling(chunk: Chunk): void {
    for (const subchunkData of chunk.getSubchunkDataList()) {
      cullingController.unregisterSubchunk(subchunkData.key)
    }
  }

  private getNeighborTiles(x: number, y: number): Map<string, Uint8Array> {
    const neighbors = new Map<string, Uint8Array>()

    for (let dx = -1; dx <= 1; dx++) {
      for (let dy = -1; dy <= 1; dy++) {
        if (dx === 0 && dy === 0) continue

        const neighborX = x + dx
        const neighborY = y + dy
        const key = `${neighborX},${neighborY}`
        const chunk = this.chunks.get(key)

        if (chunk) {
          const tiles = chunk.getTiles()
          if (tiles) {
            neighbors.set(key, tiles)
          }
        }
      }
    }

    return neighbors
  }

  getChunk(x: number, y: number): Chunk | undefined {
    return this.chunks.get(`${x},${y}`)
  }

  getLoadedChunksCount(): number {
    return this.chunks.size
  }

  /**
   * Clear all chunks and cache - for world reset.
   */
  clear(): void {
    // Stop all pending builds
    buildQueue.clear()
    this.buildTokens.clear()

    // Clear border refresh queue
    if (this.borderRefreshTimeoutId) {
      clearTimeout(this.borderRefreshTimeoutId)
      this.borderRefreshTimeoutId = null
    }
    this.borderRefreshPending.clear()

    // Destroy all chunks
    for (const chunk of this.chunks.values()) {
      this.container.removeChild(chunk.getContainer())
      chunk.destroy()
    }
    this.chunks.clear()

    // Clear cache
    chunkCache.clear()
  }

  destroy(): void {
    this.clear()
    terrainManager.destroy()
    chunkCache.destroy()
    this.container.destroy({ children: true })
    this.spritesheet = null
    this.initialized = false
    this.objectsContainer = null
  }

  /**
   * Get cache metrics for debugging.
   */
  getCacheMetrics() {
    return cacheMetrics.getMetrics()
  }
}
