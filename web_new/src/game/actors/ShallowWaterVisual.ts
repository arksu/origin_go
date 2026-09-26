import { Assets, Sprite, type Texture } from 'pixi.js'
import { SHALLOW_WATER } from './shallowWaterConfig'

export async function loadShallowWaterTexture(): Promise<Texture> {
  const texture = await Assets.load<Texture>(SHALLOW_WATER.textureURL)
  texture.source.scaleMode = 'nearest'
  return texture
}

/** Owns only the waterline transition and its noninteractive ripple sprite. */
export class ShallowWaterVisual {
  readonly sprite: Sprite
  private progress = 0
  private activity = 0
  private phase = 0
  private lastDistance = 0
  private lastUpdateMs: number | null = null

  constructor(texture: Texture) {
    this.sprite = new Sprite(texture)
    this.sprite.anchor.set(0.5, SHALLOW_WATER.rippleAnchorY)
    this.sprite.eventMode = 'none'
    // The body must occlude wave crests that cross its visible silhouette.
    this.sprite.zIndex = -1
    this.sprite.roundPixels = true
    this.sprite.visible = false
  }

  get immersionPx(): number {
    const eased = this.progress * this.progress * (3 - 2 * this.progress)
    return Math.round(eased * SHALLOW_WATER.immersionPx)
  }

  update(inWater: boolean, walking: boolean, distanceTiles: number, nowMs: number, reset = false): void {
    const elapsedMs = this.lastUpdateMs === null ? 0 : Math.max(0, nowMs - this.lastUpdateMs)
    this.lastUpdateMs = nowMs
    if (reset) {
      this.progress = 0
      this.activity = 0
    } else {
      const step = elapsedMs / SHALLOW_WATER.transitionMs
      this.progress = inWater ? Math.min(1, this.progress + step) : Math.max(0, this.progress - step)
      const settling = elapsedMs / SHALLOW_WATER.settlingMs
      this.activity = walking ? Math.min(1, this.activity + settling) : Math.max(0, this.activity - settling)
    }
    if (walking) this.phase += Math.max(0, distanceTiles - this.lastDistance)
    this.lastDistance = distanceTiles
    this.phase %= SHALLOW_WATER.pulseDistanceTiles
    const pulse = Math.sin(this.phase / SHALLOW_WATER.pulseDistanceTiles * Math.PI * 2)
    const scale = 1 + pulse * SHALLOW_WATER.pulseScale * this.activity
    this.sprite.width = SHALLOW_WATER.rippleWidth * scale
    this.sprite.height = SHALLOW_WATER.rippleHeight * scale
    this.sprite.alpha = this.progress * (SHALLOW_WATER.idleAlpha + this.activity * (SHALLOW_WATER.movingAlpha - SHALLOW_WATER.idleAlpha))
    this.sprite.visible = this.immersionPx > 0
  }
}
