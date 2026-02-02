import { Sprite, type Spritesheet } from 'pixi.js'
import {
  MAX_TERRAIN_SPRITES_IN_POOL,
  TERRAIN_POOL_SHRINK_THRESHOLD,
  TERRAIN_POOL_SHRINK_TARGET,
} from '@/constants/terrain'

/**
 * Object pool for terrain sprites to avoid GC pressure from new Sprite() / destroy()
 */
export class TerrainSpritePool {
  private pool: Sprite[] = []
  private spritesheet: Spritesheet | null = null

  // Metrics
  private _createdTotal = 0
  private _acquired = 0
  private _released = 0

  init(spritesheet: Spritesheet): void {
    this.spritesheet = spritesheet
  }

  /**
   * Acquire a sprite from pool or create new one.
   * Sets texture, position, zIndex, and visible=true.
   */
  acquire(textureFrameId: string, x: number, y: number, zIndex: number): Sprite | null {
    if (!this.spritesheet) return null

    const texture = this.spritesheet.textures[textureFrameId]
    if (!texture) return null

    let sprite: Sprite

    if (this.pool.length > 0) {
      sprite = this.pool.pop()!
      sprite.texture = texture
    } else {
      sprite = new Sprite(texture)
      this._createdTotal++
    }

    sprite.x = x
    sprite.y = y
    sprite.zIndex = zIndex
    sprite.visible = true

    this._acquired++
    return sprite
  }

  /**
   * Release sprite back to pool.
   * Removes from parent, sets visible=false, optionally clears texture.
   */
  release(sprite: Sprite): void {
    if (sprite.parent) {
      sprite.parent.removeChild(sprite)
    }

    sprite.visible = false
    // Keep texture reference to avoid texture rebinding overhead
    // sprite.texture = Texture.EMPTY

    this._released++

    if (this.pool.length < MAX_TERRAIN_SPRITES_IN_POOL) {
      this.pool.push(sprite)
    } else {
      // Pool is full, destroy sprite
      sprite.destroy()
    }
  }

  /**
   * Release multiple sprites at once (batch operation).
   */
  releaseAll(sprites: Sprite[]): void {
    for (const sprite of sprites) {
      this.release(sprite)
    }
  }

  /**
   * Shrink pool if it exceeds threshold.
   * Call this periodically or on world reset.
   */
  shrinkIfNeeded(): void {
    if (this.pool.length > TERRAIN_POOL_SHRINK_THRESHOLD) {
      const toRemove = this.pool.length - TERRAIN_POOL_SHRINK_TARGET
      for (let i = 0; i < toRemove; i++) {
        const sprite = this.pool.pop()
        sprite?.destroy()
      }
    }
  }

  /**
   * Clear entire pool (on world reset or atlas change).
   */
  clear(): void {
    for (const sprite of this.pool) {
      sprite.destroy()
    }
    this.pool = []
  }

  /**
   * Get pool metrics.
   */
  getMetrics() {
    return {
      pooled: this.pool.length,
      createdTotal: this._createdTotal,
      acquired: this._acquired,
      released: this._released,
    }
  }

  /**
   * Reset metrics counters.
   */
  resetMetrics(): void {
    this._acquired = 0
    this._released = 0
  }

  destroy(): void {
    this.clear()
    this.spritesheet = null
  }
}

export const terrainSpritePool = new TerrainSpritePool()
