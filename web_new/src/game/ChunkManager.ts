import { Container, Assets, Spritesheet } from 'pixi.js'
import { Chunk } from './Chunk'
import { initTileSets } from './tiles/tileSetLoader'
import { setWorldParams } from './tiles/Tile'
import { terrainManager } from './terrain'

interface PendingChunk {
  x: number
  y: number
  tiles: Uint8Array
}

export class ChunkManager {
  private container: Container
  private chunks: Map<string, Chunk> = new Map()
  private spritesheet: Spritesheet | null = null
  private initialized: boolean = false
  private pendingChunks: PendingChunk[] = []
  private objectsContainer: Container | null = null

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
        this.loadChunkInternal(pending.x, pending.y, pending.tiles)
      }
      this.pendingChunks = []
    }
  }

  setWorldParams(coordPerTile: number, chunkSize: number): void {
    setWorldParams(coordPerTile, chunkSize)
  }

  loadChunk(x: number, y: number, tiles: Uint8Array): void {
    console.log(`[ChunkManager] loadChunk(${x}, ${y}) tiles.length=${tiles.length}`)

    if (!this.spritesheet) {
      console.log(`[ChunkManager] Spritesheet not ready, buffering chunk (${x}, ${y})`)
      this.pendingChunks.push({ x, y, tiles })
      return
    }

    this.loadChunkInternal(x, y, tiles)
  }

  private loadChunkInternal(x: number, y: number, tiles: Uint8Array): void {
    if (!this.spritesheet) {
      console.warn('[ChunkManager] loadChunkInternal called but spritesheet not loaded!')
      return
    }

    const key = `${x},${y}`

    let chunk = this.chunks.get(key)
    if (!chunk) {
      console.log(`[ChunkManager] Creating new chunk at ${key}`)
      chunk = new Chunk(x, y)
      this.chunks.set(key, chunk)
      this.container.addChild(chunk.getContainer())
    } else {
      console.log(`[ChunkManager] Reusing existing chunk at ${key}`)
    }

    const neighborTiles = this.getNeighborTiles(x, y)
    console.log(`[ChunkManager] Building tiles for chunk ${key}... (neighbors: ${neighborTiles.size})`)
    const buildResult = chunk.buildTiles(tiles, this.spritesheet, neighborTiles)
    chunk.visible = true
    console.log(`[ChunkManager] Chunk ${key} built and visible`)

    terrainManager.generateTerrainForChunk(x, y, tiles, buildResult.hasBordersOrCorners)

    // Rebuild neighbor chunks so they can use this chunk's tiles for borders/corners
    this.rebuildNeighborChunks(x, y)
  }

  private rebuildNeighborChunks(x: number, y: number): void {
    let rebuiltCount = 0
    for (let dx = -1; dx <= 1; dx++) {
      for (let dy = -1; dy <= 1; dy++) {
        if (dx === 0 && dy === 0) continue

        const neighborX = x + dx
        const neighborY = y + dy
        const neighborKey = `${neighborX},${neighborY}`
        const neighborChunk = this.chunks.get(neighborKey)

        if (neighborChunk && neighborChunk.getTiles()) {
          const neighborTiles = this.getNeighborTiles(neighborX, neighborY)
          neighborChunk.buildTiles(neighborChunk.getTiles()!, this.spritesheet!, neighborTiles)
          rebuiltCount++
        }
      }
    }

    if (rebuiltCount > 0) {
      console.log(`[ChunkManager] Rebuilt ${rebuiltCount} neighbor chunks for smooth borders`)
    }
  }

  unloadChunk(x: number, y: number): void {
    const key = `${x},${y}`
    const chunk = this.chunks.get(key)

    if (chunk) {
      chunk.visible = false
      terrainManager.clearChunk(x, y)
    }
  }

  removeChunk(x: number, y: number): void {
    const key = `${x},${y}`
    const chunk = this.chunks.get(key)

    if (chunk) {
      this.container.removeChild(chunk.getContainer())
      chunk.destroy()
      this.chunks.delete(key)
      terrainManager.clearChunk(x, y)
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

  clear(): void {
    for (const chunk of this.chunks.values()) {
      this.container.removeChild(chunk.getContainer())
      chunk.destroy()
    }
    this.chunks.clear()
  }

  destroy(): void {
    this.clear()
    terrainManager.destroy()
    this.container.destroy({ children: true })
    this.spritesheet = null
    this.initialized = false
    this.objectsContainer = null
  }
}
