import { Container, Assets, Spritesheet } from 'pixi.js'
import { Chunk } from './Chunk'
import { initTileSets } from './tiles/tileSetLoader'
import { setWorldParams, getChunkSize } from './tiles/Tile'
import { terrainManager } from './terrain'
import { cullingController } from './culling'
import type { ChunkEventIdentity } from '../network/ChunkStreamGuard'
import {
  chunkCache, buildQueue, cacheMetrics, BuildPriority,
  type CachedChunk, type BuildTask, BORDER_REFRESH_DELAY_MS,
} from './cache'

interface ChunkPayload {
  x: number
  y: number
  tiles: Uint8Array
  version: number
  identity: ChunkEventIdentity
  worldGeneration: number
}

export class ChunkManager {
  private container = new Container()
  private chunks = new Map<string, Chunk>()
  private activeChunks = new Map<string, ChunkPayload>()
  private spritesheet: Spritesheet | null = null
  private initialized = false
  private pendingChunks = new Map<string, ChunkPayload>()
  private objectsContainer: Container | null = null
  private buildTokens = new Map<string, number>()
  private cameraX = 0
  private cameraY = 0
  private worldGeneration = 0
  private lifecycleGeneration = 0
  private borderRefreshPending = new Set<string>()
  private borderRefreshTimeoutId: ReturnType<typeof setTimeout> | null = null

  constructor() {
    this.container.sortableChildren = true
    chunkCache.setDisposer(key => this.disposeChunk(key))
  }

  getContainer(): Container { return this.container }
  setObjectsContainer(container: Container): void { this.objectsContainer = container }

  async init(): Promise<void> {
    if (this.initialized) return
    const generation = this.lifecycleGeneration
    initTileSets()
    const spritesheet = await Assets.load<Spritesheet>('/assets/game/tiles.json')
    if (generation !== this.lifecycleGeneration) return
    this.spritesheet = spritesheet
    this.initialized = true
    if (this.objectsContainer) terrainManager.init(this.objectsContainer, spritesheet)
    const pending = [...this.pendingChunks.values()]
    this.pendingChunks.clear()
    for (const payload of pending) {
      if (this.isCurrent(payload)) this.loadChunkInternal(payload)
    }
  }

  setWorldParams(coordPerTile: number, chunkSize: number): void { setWorldParams(coordPerTile, chunkSize) }
  setCameraPosition(x: number, y: number): void { this.cameraX = x; this.cameraY = y }

  loadChunk(x: number, y: number, tiles: Uint8Array, version: number, identity: ChunkEventIdentity): void {
    const key = `${x},${y}`
    const payload = { x, y, tiles, version, identity, worldGeneration: this.worldGeneration }
    this.cancelBuild(key)
    this.activeChunks.set(key, payload)
    chunkCache.markActive(key)
    if (!this.spritesheet) this.pendingChunks.set(key, payload)
    else this.loadChunkInternal(payload)
    this.notifyNeighbors(x, y)
  }

  private isCurrent(payload: ChunkPayload): boolean {
    const current = this.activeChunks.get(`${payload.x},${payload.y}`)
    return payload.worldGeneration === this.worldGeneration
      && current?.identity.streamEpoch === payload.identity.streamEpoch
      && current?.identity.eventSeq === payload.identity.eventSeq
  }

  private loadChunkInternal(payload: ChunkPayload): void {
    if (!this.isCurrent(payload) || !this.spritesheet) return
    const key = `${payload.x},${payload.y}`
    const cached = chunkCache.get(key)
    const chunk = this.chunks.get(key)
    if (chunk && cached?.version === payload.version && this.neighborsMatch(cached)) {
      cached.needsBorderRefresh = false
      chunk.visible = true
      chunkCache.markActive(key)
      this.registerSubchunksForCulling(chunk)
      terrainManager.generateTerrainForChunk(payload.x, payload.y, payload.tiles, cached.hasBordersOrCorners)
      return
    }
    this.processBuildTask(this.createBuildTask(payload, false))
  }

  private createBuildTask(payload: ChunkPayload, isBorderRefresh: boolean): BuildTask {
    const chunkKey = `${payload.x},${payload.y}`
    const buildToken = buildQueue.nextBuildToken()
    this.buildTokens.set(chunkKey, buildToken)
    return {
      ...payload, chunkKey, buildToken, isBorderRefresh,
      priority: isBorderRefresh ? BuildPriority.P1_NEARBY : BuildPriority.P0_VISIBLE,
      distanceToCamera: this.calculateDistance(payload.x, payload.y), createdAt: performance.now(),
    }
  }

  private processBuildTask(task: BuildTask): void {
    try {
      if (!this.isCurrent(task) || this.buildTokens.get(task.chunkKey) !== task.buildToken || !this.spritesheet) return
      let chunk = this.chunks.get(task.chunkKey)
      if (!chunk) {
        chunk = new Chunk(task.x, task.y)
        this.chunks.set(task.chunkKey, chunk)
        this.container.addChild(chunk.getContainer())
      }
      // Only a fully completed build may become a reusable cache entry.
      chunkCache.forget(task.chunkKey)
      this.unregisterSubchunksFromCulling(chunk)
      const neighborVersions = this.getNeighborVersions(task.x, task.y)
      const neighborTiles = this.getNeighborTiles(task.x, task.y)
      const started = performance.now()
      const result = chunk.buildTiles(task.tiles, this.spritesheet, neighborTiles)
      const elapsed = performance.now() - started
      if (!this.isCurrent(task)) { this.disposeChunk(task.chunkKey); return }
      chunk.visible = true
      this.registerSubchunksForCulling(chunk)
      chunkCache.set({
        x: task.x, y: task.y, key: task.chunkKey, tiles: task.tiles, version: task.version,
        hasBordersOrCorners: result.hasBordersOrCorners, neighborVersions, needsBorderRefresh: false,
        tilesBytes: task.tiles.byteLength, cpuBytes: 0, gpuBytes: 0,
        createdAt: performance.now(), retainedAt: null,
      })
      terrainManager.generateTerrainForChunk(task.x, task.y, task.tiles, result.hasBordersOrCorners)
      buildQueue.recordCpuBuildTime(elapsed)
      if (task.isBorderRefresh) cacheMetrics.recordBorderRefresh(elapsed)
    } catch (error) {
      this.disposeChunk(task.chunkKey)
      throw error
    }
  }

  private getNeighborVersions(x: number, y: number): Map<string, number> {
    const versions = new Map<string, number>()
    for (let dx = -1; dx <= 1; dx++) {
      for (let dy = -1; dy <= 1; dy++) {
        if (dx === 0 && dy === 0) continue
        const key = `${x + dx},${y + dy}`
        const neighbor = this.activeChunks.get(key)
        if (neighbor) versions.set(key, neighbor.version)
      }
    }
    return versions
  }

  private neighborsMatch(cached: CachedChunk): boolean {
    const current = this.getNeighborVersions(cached.x, cached.y)
    return current.size === cached.neighborVersions.size
      && [...current].every(([key, version]) => cached.neighborVersions.get(key) === version)
  }

  private notifyNeighbors(x: number, y: number): void {
    for (let dx = -1; dx <= 1; dx++) {
      for (let dy = -1; dy <= 1; dy++) {
        if (dx === 0 && dy === 0) continue
        const key = `${x + dx},${y + dy}`
        const cached = chunkCache.peek(key)
        if (!cached || this.neighborsMatch(cached)) continue
        cached.needsBorderRefresh = true
        // Hidden chunks are checked on reload, without extending their retention.
        if (this.activeChunks.has(key)) this.scheduleBorderRefresh(key)
      }
    }
  }

  private scheduleBorderRefresh(key: string): void {
    this.borderRefreshPending.add(key)
    if (this.borderRefreshTimeoutId != null) return
    this.borderRefreshTimeoutId = setTimeout(() => this.processBorderRefreshQueue(), BORDER_REFRESH_DELAY_MS)
  }

  private processBorderRefreshQueue(): void {
    if (this.borderRefreshTimeoutId != null) clearTimeout(this.borderRefreshTimeoutId)
    this.borderRefreshTimeoutId = null
    for (const key of [...this.borderRefreshPending]) {
      this.borderRefreshPending.delete(key)
      const payload = this.activeChunks.get(key)
      const cached = chunkCache.peek(key)
      if (!payload || !cached || !this.spritesheet || this.neighborsMatch(cached)) continue
      const task = this.createBuildTask(payload, true)
      if (!buildQueue.enqueue(task)) this.scheduleBorderRefresh(key)
    }
  }

  private calculateDistance(x: number, y: number): number {
    const chunkSize = getChunkSize()
    return Math.hypot(x - Math.floor(this.cameraX / chunkSize), y - Math.floor(this.cameraY / chunkSize))
  }

  private cancelBuild(key: string): void {
    buildQueue.cancel(key)
    this.buildTokens.delete(key)
    this.borderRefreshPending.delete(key)
    this.pendingChunks.delete(key)
  }

  update(): void {
    for (const task of buildQueue.getTasksForFrame()) {
      try { this.processBuildTask(task) }
      finally { buildQueue.buildComplete() }
    }
  }

  unloadChunk(x: number, y: number, _identity: ChunkEventIdentity): void {
    const key = `${x},${y}`
    this.activeChunks.delete(key)
    this.cancelBuild(key)
    terrainManager.clearChunk(x, y)
    const chunk = this.chunks.get(key)
    if (chunk) {
      chunk.visible = false
      this.unregisterSubchunksFromCulling(chunk)
      if (chunkCache.peek(key)) chunkCache.retain(key)
      else this.disposeChunk(key)
    }
    this.notifyNeighbors(x, y)
  }

  removeChunk(x: number, y: number): void { this.disposeChunk(`${x},${y}`) }

  private disposeChunk(key: string): void {
    const chunk = this.chunks.get(key)
    this.cancelBuild(key)
    chunkCache.forget(key)
    const wasActive = this.activeChunks.delete(key)
    if (chunk) {
      this.unregisterSubchunksFromCulling(chunk)
      this.container.removeChild(chunk.getContainer())
      chunk.destroy()
      this.chunks.delete(key)
      terrainManager.clearChunk(chunk.x, chunk.y)
      if (wasActive) this.notifyNeighbors(chunk.x, chunk.y)
    }
  }

  private registerSubchunksForCulling(chunk: Chunk): void {
    for (const subchunk of chunk.getSubchunkDataList()) {
      cullingController.registerSubchunk(subchunk.key, subchunk.container, subchunk.bounds)
    }
  }

  private unregisterSubchunksFromCulling(chunk: Chunk): void {
    for (const subchunk of chunk.getSubchunkDataList()) cullingController.unregisterSubchunk(subchunk.key)
  }

  private getNeighborTiles(x: number, y: number): Map<string, Uint8Array> {
    const neighbors = new Map<string, Uint8Array>()
    for (const key of this.getNeighborVersions(x, y).keys()) {
      neighbors.set(key, this.activeChunks.get(key)!.tiles)
    }
    return neighbors
  }

  getChunk(x: number, y: number): Chunk | undefined { return this.chunks.get(`${x},${y}`) }
  getLoadedChunksCount(): number { return this.activeChunks.size }

  clear(): void {
    this.worldGeneration++
    this.activeChunks.clear()
    this.pendingChunks.clear()
    buildQueue.clear()
    this.buildTokens.clear()
    if (this.borderRefreshTimeoutId != null) clearTimeout(this.borderRefreshTimeoutId)
    this.borderRefreshTimeoutId = null
    this.borderRefreshPending.clear()
    for (const key of [...this.chunks.keys()]) this.disposeChunk(key)
    chunkCache.clear()
  }

  destroy(): void {
    this.lifecycleGeneration++
    this.clear()
    terrainManager.destroy()
    chunkCache.destroy()
    this.container.destroy({ children: true })
    this.spritesheet = null
    this.initialized = false
    this.objectsContainer = null
  }

  getCacheMetrics() { return cacheMetrics.getMetrics() }
}
