import { Sprite, Container, Spritesheet } from 'pixi.js'
import type { ITerrainRenderer } from './ITerrainRenderer'
import type { TerrainDrawCmd, TerrainRenderContext } from './types'
import { TILE_HEIGHT_HALF } from '../tiles/Tile'
import { coordGame2Screen } from '../utils/coordConvert'

const BASE_Z_INDEX = 100

export class TerrainSpriteRenderer implements ITerrainRenderer {
  private objectsContainer: Container
  private spritesheet: Spritesheet
  private chunkSprites: Map<string, Sprite[]> = new Map()
  private currentChunkKey: string = ''

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

      let sprites = this.chunkSprites.get(this.currentChunkKey)
      if (!sprites) {
        sprites = []
        this.chunkSprites.set(this.currentChunkKey, sprites)
      }
      sprites.push(sprite)
    }
  }

  finalize(): void {
    // No-op for sprite renderer
  }

  destroy(): void {
    for (const sprites of this.chunkSprites.values()) {
      for (const sprite of sprites) {
        sprite.destroy()
      }
    }
    this.chunkSprites.clear()
  }

  getTerrainSpritesForChunk(chunkKey: string): Sprite[] {
    return this.chunkSprites.get(chunkKey) ?? []
  }

  clearChunk(chunkKey: string): void {
    const sprites = this.chunkSprites.get(chunkKey)
    if (sprites) {
      for (const sprite of sprites) {
        this.objectsContainer.removeChild(sprite)
        sprite.destroy()
      }
      this.chunkSprites.delete(chunkKey)
    }
  }
}
