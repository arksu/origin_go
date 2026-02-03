import { Container, Spritesheet } from 'pixi.js'
import { TerrainSpriteRenderer } from './TerrainSpriteRenderer'
import { getTerrainGenerator } from './TerrainRegistry'
import type { TerrainRenderContext } from './types'
import type { TerrainSubchunk, TerrainBuildTask } from './TerrainSubchunkTypes'
import { TerrainSubchunkState } from './TerrainSubchunkTypes'
import { TILE_WIDTH_HALF, TILE_HEIGHT_HALF, getChunkSize, getFullChunkSize } from '../tiles/Tile'
import { terrainBuildQueue } from './TerrainBuildQueue'
import { terrainMetrics } from './TerrainMetricsCollector'
import { terrainSpritePool } from './TerrainSpritePool'
import {
  TERRAIN_SUBCHUNK_DIVIDER,
  TERRAIN_VISIBLE_WIDTH_SUBCHUNKS,
  TERRAIN_VISIBLE_HEIGHT_SUBCHUNKS,
  TERRAIN_HIDE_WIDTH_SUBCHUNKS,
  TERRAIN_HIDE_HEIGHT_SUBCHUNKS,
} from '@/constants/terrain'

interface ChunkData {
  chunkX: number
  chunkY: number
  tiles: Uint8Array
  hasBordersOrCorners: boolean[][]
  epoch: number
}

export class TerrainManager {
  private renderer: TerrainSpriteRenderer | null = null

  // Chunk data storage
  private chunkData: Map<string, ChunkData> = new Map()

  // Subchunk state tracking
  private subchunks: Map<string, TerrainSubchunk> = new Map()

  // Epoch for cancellation
  private globalEpoch: number = 0
  private chunkEpochs: Map<string, number> = new Map()

  // Camera position for priority calculation
  private cameraSubchunkX: number = 0
  private cameraSubchunkY: number = 0
  private lastPriorityUpdateX: number = 0
  private lastPriorityUpdateY: number = 0

  init(objectsContainer: Container, spritesheet: Spritesheet): void {
    this.renderer = new TerrainSpriteRenderer(objectsContainer, spritesheet)
  }

  /**
   * Update camera position for visibility and priority calculation.
   * Call this from render loop.
   */
  setCameraPosition(gameX: number, gameY: number): void {
    // Convert game coords to subchunk coords directly
    // Game coords are in tile units, subchunk coords are in subchunk units
    const chunkSize = getFullChunkSize()
    const subchunkSize = chunkSize / TERRAIN_SUBCHUNK_DIVIDER

    const prevX = this.cameraSubchunkX
    const prevY = this.cameraSubchunkY

    this.cameraSubchunkX = gameX / subchunkSize
    this.cameraSubchunkY = gameY / subchunkSize

    // Recalculate queue priorities if camera moved significantly
    const dx = this.cameraSubchunkX - this.lastPriorityUpdateX
    const dy = this.cameraSubchunkY - this.lastPriorityUpdateY
    const distanceMoved = Math.sqrt(dx * dx + dy * dy)

    // Recalculate if moved more than 1 subchunk or if this is first update (both were 0)
    if (distanceMoved > 1 || (prevX === 0 && prevY === 0 && (this.cameraSubchunkX !== 0 || this.cameraSubchunkY !== 0))) {
      terrainBuildQueue.recalculatePriorities(this.cameraSubchunkX, this.cameraSubchunkY)
      this.lastPriorityUpdateX = this.cameraSubchunkX
      this.lastPriorityUpdateY = this.cameraSubchunkY
      console.log(`[TerrainManager] Recalculated priorities: camera=(${this.cameraSubchunkX.toFixed(2)},${this.cameraSubchunkY.toFixed(2)}), queue=${terrainBuildQueue.length}`)
    }
  }

  /**
   * Enqueue terrain generation for a chunk.
   * Builds subchunks incrementally by frame budget.
   */
  generateTerrainForChunk(
    chunkX: number,
    chunkY: number,
    tiles: Uint8Array,
    hasBordersOrCorners: boolean[][],
  ): void {
    if (!this.renderer) {
      console.warn('[TerrainManager] Not initialized')
      return
    }

    const chunkKey = `${chunkX},${chunkY}`
    // console.log(`[TerrainManager] Enqueuing terrain for chunk ${chunkKey}, camera=(${this.cameraSubchunkX.toFixed(2)},${this.cameraSubchunkY.toFixed(2)})`)
    const epoch = ++this.globalEpoch
    this.chunkEpochs.set(chunkKey, epoch)

    // Cancel pending builds for this chunk
    terrainBuildQueue.cancelChunk(chunkKey)

    // Clear existing terrain for this chunk
    this.renderer.clearChunk(chunkKey)
    this.clearSubchunkStates(chunkKey)

    // Store chunk data for incremental building
    this.chunkData.set(chunkKey, {
      chunkX,
      chunkY,
      tiles,
      hasBordersOrCorners,
      epoch,
    })

    // Enqueue subchunk build tasks
    this.enqueueSubchunkBuilds(chunkKey, chunkX, chunkY, tiles, hasBordersOrCorners, epoch)
  }

  /**
   * Enqueue build tasks for all subchunks of a chunk.
   */
  private enqueueSubchunkBuilds(
    chunkKey: string,
    chunkX: number,
    chunkY: number,
    tiles: Uint8Array,
    hasBordersOrCorners: boolean[][],
    epoch: number,
  ): void {
    for (let cx = 0; cx < TERRAIN_SUBCHUNK_DIVIDER; cx++) {
      for (let cy = 0; cy < TERRAIN_SUBCHUNK_DIVIDER; cy++) {
        const subchunkKey = `${chunkKey}:${cx},${cy}`

        // Initialize subchunk state
        this.subchunks.set(subchunkKey, {
          key: subchunkKey,
          chunkKey,
          cx,
          cy,
          state: TerrainSubchunkState.NotBuilt,
          sprites: [],
          epoch,
        })

        // Calculate subchunk global position
        const subchunkGlobalX = chunkX * TERRAIN_SUBCHUNK_DIVIDER + cx
        const subchunkGlobalY = chunkY * TERRAIN_SUBCHUNK_DIVIDER + cy

        // Calculate distance to camera for priority
        const dx = subchunkGlobalX - this.cameraSubchunkX
        const dy = subchunkGlobalY - this.cameraSubchunkY
        const distanceToCamera = Math.sqrt(dx * dx + dy * dy)

        const task: TerrainBuildTask = {
          subchunkKey,
          chunkKey,
          chunkX,
          chunkY,
          cx,
          cy,
          tiles,
          hasBordersOrCorners,
          epoch,
          distanceToCamera,
          createdAt: performance.now(),
        }

        terrainBuildQueue.enqueue(task)
      }
    }
  }

  /**
   * Process build queue - call this from render loop.
   */
  update(): void {
    if (!this.renderer) return

    // Process build tasks within frame budget
    const tasks = terrainBuildQueue.getTasksForFrame()
    for (const task of tasks) {
      this.processBuildTask(task)
    }

    // Update visibility based on camera position
    this.updateSubchunkVisibility()
  }

  /**
   * Process a single build task.
   */
  private processBuildTask(task: TerrainBuildTask): void {
    const start = performance.now()

    // Check epoch for cancellation
    const currentEpoch = this.chunkEpochs.get(task.chunkKey)
    if (currentEpoch !== task.epoch) {
      terrainMetrics.recordSubchunkCanceled()
      return
    }

    const subchunk = this.subchunks.get(task.subchunkKey)
    if (!subchunk || subchunk.epoch !== task.epoch) {
      terrainMetrics.recordSubchunkCanceled()
      return
    }

    // Mark as building
    subchunk.state = TerrainSubchunkState.Building

    // Build the subchunk
    this.buildSubchunk(task)

    // Mark as built visible initially - visibility will be updated in updateSubchunkVisibility
    // Don't hide immediately to avoid all sprites going to pool on first build
    subchunk.state = TerrainSubchunkState.BuiltVisible

    const buildTime = performance.now() - start
    terrainBuildQueue.recordBuildTime(buildTime)
    terrainMetrics.recordSubchunkBuilt()
  }

  /**
   * Build terrain for a single subchunk.
   */
  private buildSubchunk(task: TerrainBuildTask): void {
    if (!this.renderer) return

    const chunkSize = getChunkSize()
    const subchunkSize = chunkSize / TERRAIN_SUBCHUNK_DIVIDER

    const startTileX = task.cx * subchunkSize
    const startTileY = task.cy * subchunkSize

    this.renderer.setCurrentSubchunk(task.subchunkKey)

    for (let tx = 0; tx < subchunkSize; tx++) {
      for (let ty = 0; ty < subchunkSize; ty++) {
        const localX = startTileX + tx
        const localY = startTileY + ty

        if (task.hasBordersOrCorners[localX]?.[localY]) {
          continue
        }

        const idx = localY * chunkSize + localX
        const tileType = task.tiles[idx]
        if (tileType === undefined) continue

        const generator = getTerrainGenerator(tileType)
        if (!generator) continue

        const globalTileX = task.chunkX * chunkSize + localX
        const globalTileY = task.chunkY * chunkSize + localY

        const anchorScreenX =
          globalTileX * TILE_WIDTH_HALF - globalTileY * TILE_WIDTH_HALF + TILE_WIDTH_HALF
        const anchorScreenY =
          globalTileX * TILE_HEIGHT_HALF + globalTileY * TILE_HEIGHT_HALF + TILE_HEIGHT_HALF

        const cmds = generator.generate(globalTileX, globalTileY, anchorScreenX, anchorScreenY)
        if (cmds && cmds.length > 0) {
          const context: TerrainRenderContext = {
            tileX: globalTileX,
            tileY: globalTileY,
            anchorScreenX,
            anchorScreenY,
          }
          this.renderer.addTile(cmds, context)
        }
      }
    }
  }

  /**
   * Update subchunk visibility based on camera position.
   * Uses rectangular visibility area with hysteresis.
   */
  private updateSubchunkVisibility(): void {
    if (!this.renderer) return

    let hiddenCount = 0
    let visibleCount = 0
    let shownCount = 0

    // Calculate visibility rect around camera
    const halfVisibleWidth = TERRAIN_VISIBLE_WIDTH_SUBCHUNKS / 2
    const halfVisibleHeight = TERRAIN_VISIBLE_HEIGHT_SUBCHUNKS / 2
    const halfHideWidth = TERRAIN_HIDE_WIDTH_SUBCHUNKS / 2
    const halfHideHeight = TERRAIN_HIDE_HEIGHT_SUBCHUNKS / 2

    const visibleMinX = this.cameraSubchunkX - halfVisibleWidth
    const visibleMaxX = this.cameraSubchunkX + halfVisibleWidth
    const visibleMinY = this.cameraSubchunkY - halfVisibleHeight
    const visibleMaxY = this.cameraSubchunkY + halfVisibleHeight

    const hideMinX = this.cameraSubchunkX - halfHideWidth
    const hideMaxX = this.cameraSubchunkX + halfHideWidth
    const hideMinY = this.cameraSubchunkY - halfHideHeight
    const hideMaxY = this.cameraSubchunkY + halfHideHeight

    for (const [subchunkKey, subchunk] of this.subchunks) {
      if (subchunk.state === TerrainSubchunkState.NotBuilt ||
        subchunk.state === TerrainSubchunkState.Building) {
        continue
      }

      // Parse subchunk global position
      const chunkData = this.chunkData.get(subchunk.chunkKey)
      if (!chunkData) continue

      const subchunkGlobalX = chunkData.chunkX * TERRAIN_SUBCHUNK_DIVIDER + subchunk.cx
      const subchunkGlobalY = chunkData.chunkY * TERRAIN_SUBCHUNK_DIVIDER + subchunk.cy

      const isVisible = subchunk.state === TerrainSubchunkState.BuiltVisible

      // Check if within visible rect
      const inVisibleRect = subchunkGlobalX >= visibleMinX && subchunkGlobalX <= visibleMaxX &&
        subchunkGlobalY >= visibleMinY && subchunkGlobalY <= visibleMaxY

      // Check if within hide rect (hysteresis)
      const inHideRect = subchunkGlobalX >= hideMinX && subchunkGlobalX <= hideMaxX &&
        subchunkGlobalY >= hideMinY && subchunkGlobalY <= hideMaxY

      if (inVisibleRect && !isVisible) {
        // Show - need to rebuild since sprites were returned to pool
        console.log(`[TerrainManager] Showing subchunk ${subchunkKey}, subchunkGlobal=(${subchunkGlobalX},${subchunkGlobalY}), camera=(${this.cameraSubchunkX.toFixed(2)},${this.cameraSubchunkY.toFixed(2)})`)
        const data = this.chunkData.get(subchunk.chunkKey)
        if (data) {
          // Calculate distance to camera for priority
          const dx = subchunkGlobalX - this.cameraSubchunkX
          const dy = subchunkGlobalY - this.cameraSubchunkY
          const distanceToCamera = Math.sqrt(dx * dx + dy * dy)

          const task: TerrainBuildTask = {
            subchunkKey,
            chunkKey: subchunk.chunkKey,
            chunkX: data.chunkX,
            chunkY: data.chunkY,
            cx: subchunk.cx,
            cy: subchunk.cy,
            tiles: data.tiles,
            hasBordersOrCorners: data.hasBordersOrCorners,
            epoch: subchunk.epoch,
            distanceToCamera,
            createdAt: performance.now(),
          }
          // Rebuild immediately for show (high priority)
          this.buildSubchunk(task)
          subchunk.state = TerrainSubchunkState.BuiltVisible
          shownCount++
        }
      } else if (!inHideRect && isVisible) {
        // Hide - outside hide rect
        //console.log(`[TerrainManager] Hiding subchunk ${subchunkKey}, subchunkGlobal=(${subchunkGlobalX},${subchunkGlobalY}), camera=(${this.cameraSubchunkX.toFixed(2)},${this.cameraSubchunkY.toFixed(2)})`)
        this.renderer.hideSubchunk(subchunkKey)
        subchunk.state = TerrainSubchunkState.BuiltHidden
        hiddenCount++
      } else if (isVisible) {
        visibleCount++
      }
    }

    // if (hiddenCount > 0 || shownCount > 0) {
    // console.log(`[TerrainManager] Visibility: visible=${visibleCount}, shown=${shownCount}, hidden=${hiddenCount}, camera=(${this.cameraSubchunkX.toFixed(2)},${this.cameraSubchunkY.toFixed(2)})`)
    // }
  }


  /**
   * Clear subchunk states for a chunk.
   */
  private clearSubchunkStates(chunkKey: string): void {
    const prefix = chunkKey + ':'
    const keysToDelete: string[] = []
    for (const key of this.subchunks.keys()) {
      if (key.startsWith(prefix)) {
        keysToDelete.push(key)
      }
    }
    for (const key of keysToDelete) {
      this.subchunks.delete(key)
    }
  }

  /**
   * Clear terrain for a chunk.
   */
  clearChunk(chunkX: number, chunkY: number): void {
    if (!this.renderer) return

    const chunkKey = `${chunkX},${chunkY}`

    // Cancel pending builds
    terrainBuildQueue.cancelChunk(chunkKey)

    // Clear renderer sprites
    this.renderer.clearChunk(chunkKey)

    // Clear state
    this.clearSubchunkStates(chunkKey)
    this.chunkData.delete(chunkKey)
    this.chunkEpochs.delete(chunkKey)
  }

  /**
   * Get terrain metrics.
   */
  getMetrics() {
    return terrainMetrics.getMetrics()
  }

  /**
   * Reset world - clear all and shrink pool.
   */
  resetWorld(): void {
    if (this.renderer) {
      for (const chunkKey of this.chunkData.keys()) {
        this.renderer.clearChunk(chunkKey)
      }
    }

    terrainBuildQueue.clear()
    this.subchunks.clear()
    this.chunkData.clear()
    this.chunkEpochs.clear()
    this.globalEpoch = 0

    terrainSpritePool.shrinkIfNeeded()
  }

  destroy(): void {
    this.resetWorld()
    if (this.renderer) {
      this.renderer.destroy()
      this.renderer = null
    }
  }
}

export const terrainManager = new TerrainManager()
