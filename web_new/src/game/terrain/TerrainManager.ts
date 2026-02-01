import { Container, Spritesheet } from 'pixi.js'
import { TerrainSpriteRenderer } from './TerrainSpriteRenderer'
import { getTerrainGenerator } from './TerrainRegistry'
import type { TerrainRenderContext } from './types'
import { TILE_WIDTH_HALF, TILE_HEIGHT_HALF, getChunkSize } from '../Tile'

export class TerrainManager {
  private renderer: TerrainSpriteRenderer | null = null

  init(objectsContainer: Container, spritesheet: Spritesheet): void {
    this.renderer = new TerrainSpriteRenderer(objectsContainer, spritesheet)
  }

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

    const chunkSize = getChunkSize()
    const chunkKey = `${chunkX},${chunkY}`

    this.renderer.clearChunk(chunkKey)
    this.renderer.setCurrentChunk(chunkKey)

    let terrainCount = 0

    for (let tx = 0; tx < chunkSize; tx++) {
      for (let ty = 0; ty < chunkSize; ty++) {
        if (hasBordersOrCorners[tx]?.[ty]) {
          continue
        }

        const idx = ty * chunkSize + tx
        const tileType = tiles[idx]
        if (tileType === undefined) continue

        const generator = getTerrainGenerator(tileType)
        if (!generator) continue

        const globalTileX = chunkX * chunkSize + tx
        const globalTileY = chunkY * chunkSize + ty

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
          terrainCount++
        }
      }
    }

    if (terrainCount > 0) {
      console.log(`[TerrainManager] Generated ${terrainCount} terrain objects for chunk ${chunkKey}`)
    }
  }

  clearChunk(chunkX: number, chunkY: number): void {
    if (!this.renderer) return
    const chunkKey = `${chunkX},${chunkY}`
    this.renderer.clearChunk(chunkKey)
  }

  destroy(): void {
    if (this.renderer) {
      this.renderer.destroy()
      this.renderer = null
    }
  }
}

export const terrainManager = new TerrainManager()
