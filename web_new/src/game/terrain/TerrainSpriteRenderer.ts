import { Sprite, Container, Spritesheet } from 'pixi.js'
import type { ITerrainRenderer } from './ITerrainRenderer'
import type { TerrainDrawCmd, TerrainRenderContext } from './types'
import { TILE_HEIGHT_HALF } from '../tiles/Tile'
import { terrainSpritePool } from './TerrainSpritePool'
import { terrainMetrics } from './TerrainMetricsCollector'
import { TERRAIN_BASE_Z_INDEX } from '@/constants/terrain'

interface TerrainSpriteData {
  sprite: Sprite
  subchunkKey: string
}

/**
 * Sprite-based terrain renderer with object pooling.
 * Uses anchorScreenY directly for zIndex calculation (no coordGame2Screen call).
 * Does NOT register individual sprites with culling - uses bulk clearTerrainForSubchunk.
 */
export class TerrainSpriteRenderer implements ITerrainRenderer {
  private objectsContainer: Container
  // Map: subchunkKey -> sprites
  private subchunkSprites: Map<string, TerrainSpriteData[]> = new Map()
  private currentSubchunkKey: string = ''
  private _activeSpritesCount: number = 0

  constructor(objectsContainer: Container, spritesheet: Spritesheet) {
    this.objectsContainer = objectsContainer
    terrainSpritePool.init(spritesheet)
  }

  setCurrentSubchunk(subchunkKey: string): void {
    this.currentSubchunkKey = subchunkKey
  }

  /**
   * Add terrain sprites for a tile.
   * Uses anchorScreenY from context for zIndex (optimization: no coordGame2Screen call).
   */
  addTile(cmds: TerrainDrawCmd[], context: TerrainRenderContext): void {
    for (const cmd of cmds) {
      // Use anchorScreenY directly for zIndex calculation (spec item 4)
      const zIndex = TERRAIN_BASE_Z_INDEX + context.anchorScreenY + TILE_HEIGHT_HALF + cmd.zOffset

      const sprite = terrainSpritePool.acquire(cmd.textureFrameId, cmd.x, cmd.y, zIndex)
      if (!sprite) continue

      this.objectsContainer.addChild(sprite)
      this._activeSpritesCount++

      let sprites = this.subchunkSprites.get(this.currentSubchunkKey)
      if (!sprites) {
        sprites = []
        this.subchunkSprites.set(this.currentSubchunkKey, sprites)
      }
      sprites.push({ sprite, subchunkKey: this.currentSubchunkKey })
    }

    terrainMetrics.setActiveSprites(this._activeSpritesCount)
  }

  finalize(): void {
    // No-op for sprite renderer
  }

  /**
   * Hide sprites for a subchunk (return to pool).
   */
  hideSubchunk(subchunkKey: string): void {
    const sprites = this.subchunkSprites.get(subchunkKey)
    if (!sprites) return

    const start = performance.now()
    for (const data of sprites) {
      this.objectsContainer.removeChild(data.sprite)
      terrainSpritePool.release(data.sprite)
      this._activeSpritesCount--
    }
    this.subchunkSprites.delete(subchunkKey)
    terrainMetrics.recordClearTime(performance.now() - start)
    terrainMetrics.setActiveSprites(this._activeSpritesCount)
  }

  /**
   * Clear all sprites for a subchunk (return to pool).
   * This is the single contract for cleanup - no individual unregister.
   */
  clearSubchunk(subchunkKey: string): void {
    this.hideSubchunk(subchunkKey)
  }

  /**
   * Clear all sprites for a chunk (all subchunks).
   */
  clearChunk(chunkKey: string): void {
    const start = performance.now()
    const prefix = chunkKey + ':'
    const keysToDelete: string[] = []

    for (const key of this.subchunkSprites.keys()) {
      if (key.startsWith(prefix)) {
        keysToDelete.push(key)
      }
    }

    for (const key of keysToDelete) {
      const sprites = this.subchunkSprites.get(key)
      if (sprites) {
        for (const data of sprites) {
          terrainSpritePool.release(data.sprite)
          this._activeSpritesCount--
        }
        this.subchunkSprites.delete(key)
      }
    }

    terrainMetrics.recordClearTime(performance.now() - start)
    terrainMetrics.setActiveSprites(this._activeSpritesCount)
  }

  /**
   * Get sprites for a subchunk.
   */
  getSpritesForSubchunk(subchunkKey: string): Sprite[] {
    const dataList = this.subchunkSprites.get(subchunkKey)
    return dataList ? dataList.map(d => d.sprite) : []
  }

  /**
   * Check if subchunk has sprites built.
   */
  hasSubchunk(subchunkKey: string): boolean {
    return this.subchunkSprites.has(subchunkKey)
  }

  /**
   * Get active sprites count.
   */
  getActiveSpritesCount(): number {
    return this._activeSpritesCount
  }

  destroy(): void {
    for (const sprites of this.subchunkSprites.values()) {
      for (const data of sprites) {
        terrainSpritePool.release(data.sprite)
      }
    }
    this.subchunkSprites.clear()
    this._activeSpritesCount = 0
    terrainSpritePool.destroy()
  }
}
