import { Sprite, Container, Spritesheet } from 'pixi.js'
import type { ITerrainRenderer } from './ITerrainRenderer'
import type { TerrainDrawCmd, TerrainRenderContext } from './types'
import { TILE_HEIGHT_HALF, TILE_WIDTH_HALF } from '../tiles/Tile'
import { coordGame2Screen } from '../utils/coordConvert'
import { cullingController } from '../culling'
import { type AABB, fromMinMax } from '../culling/AABB'

const BASE_Z_INDEX = 100

interface TerrainSpriteData {
  sprite: Sprite
  cullingKey: string
}

export class TerrainSpriteRenderer implements ITerrainRenderer {
  private objectsContainer: Container
  private spritesheet: Spritesheet
  private chunkSprites: Map<string, TerrainSpriteData[]> = new Map()
  private currentChunkKey: string = ''
  private terrainCounter: number = 0

  constructor(objectsContainer: Container, spritesheet: Spritesheet) {
    this.objectsContainer = objectsContainer
    this.spritesheet = spritesheet
  }

  setCurrentChunk(chunkKey: string): void {
    this.currentChunkKey = chunkKey
  }

  addTile(cmds: TerrainDrawCmd[], context: TerrainRenderContext): void {
    for (const cmd of cmds) {
      const texture = this.spritesheet.textures[cmd.textureFrameId]
      if (!texture) {
        continue
      }

      const sprite = new Sprite(texture)
      sprite.x = cmd.x
      sprite.y = cmd.y

      const screenPos = coordGame2Screen(context.tileX, context.tileY)
      sprite.zIndex = BASE_Z_INDEX + screenPos.y + TILE_HEIGHT_HALF + cmd.zOffset

      this.objectsContainer.addChild(sprite)

      // Generate unique key for this terrain sprite
      const cullingKey = `${this.currentChunkKey}:t${this.terrainCounter++}`

      // Compute bounds for culling (in objectsContainer local coordinates)
      const bounds = this.computeTerrainBounds(sprite, texture.width, texture.height)

      // Register with culling controller
      cullingController.registerTerrain(cullingKey, sprite, bounds)

      let sprites = this.chunkSprites.get(this.currentChunkKey)
      if (!sprites) {
        sprites = []
        this.chunkSprites.set(this.currentChunkKey, sprites)
      }
      sprites.push({ sprite, cullingKey })
    }
  }

  /**
   * Compute AABB bounds for a terrain sprite in objectsContainer coordinates.
   */
  private computeTerrainBounds(sprite: Sprite, width: number, height: number): AABB {
    // Sprite position is its top-left corner (default anchor is 0,0)
    const minX = sprite.x
    const minY = sprite.y
    const maxX = sprite.x + width
    const maxY = sprite.y + height

    // Add some padding for safety
    return fromMinMax(
      minX - TILE_WIDTH_HALF,
      minY - TILE_HEIGHT_HALF,
      maxX + TILE_WIDTH_HALF,
      maxY + TILE_HEIGHT_HALF,
    )
  }

  finalize(): void {
    // No-op for sprite renderer
  }

  destroy(): void {
    for (const spriteDataList of this.chunkSprites.values()) {
      for (const data of spriteDataList) {
        cullingController.unregisterTerrain(data.cullingKey)
        data.sprite.destroy()
      }
    }
    this.chunkSprites.clear()
  }

  getTerrainSpritesForChunk(chunkKey: string): Sprite[] {
    const dataList = this.chunkSprites.get(chunkKey)
    return dataList ? dataList.map(d => d.sprite) : []
  }

  clearChunk(chunkKey: string): void {
    const spriteDataList = this.chunkSprites.get(chunkKey)
    if (spriteDataList) {
      for (const data of spriteDataList) {
        cullingController.unregisterTerrain(data.cullingKey)
        this.objectsContainer.removeChild(data.sprite)
        data.sprite.destroy()
      }
      this.chunkSprites.delete(chunkKey)
    }
    // Also clear from culling controller by chunk prefix
    cullingController.clearTerrainForChunk(chunkKey)
  }
}
