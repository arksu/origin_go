import { Assets, Sprite, type Spritesheet, type Texture } from 'pixi.js'
import { SHALLOW_WATER } from './shallowWaterConfig'

export async function loadShallowWaterTextures(): Promise<readonly Texture[]> {
  const sheet = await Assets.load<Spritesheet>(SHALLOW_WATER.textureURL)
  const textures = sheet.animations.ripples
  if (!textures || textures.length !== SHALLOW_WATER.frameCount) {
    throw new Error(`Shallow-water atlas must contain ${SHALLOW_WATER.frameCount} ripple frames`)
  }
  for (const texture of textures) texture.source.scaleMode = 'nearest'
  return textures
}

/** Owns only the waterline transition and its noninteractive ripple sprite. */
export class ShallowWaterVisual {
  readonly sprite: Sprite
  private progress = 0
  private activity = 0
  private phase = 0
  private animationElapsedMs = 0
  private lastDistance = 0
  private lastUpdateMs: number | null = null

  constructor(private readonly textures: readonly Texture[]) {
    if (textures.length === 0) throw new Error('Shallow-water animation requires at least one texture')
    if (SHALLOW_WATER.framesPerSecond <= 0) throw new Error('Shallow-water frame rate must be positive')
    this.sprite = new Sprite(textures[0])
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
    this.sprite.visible = this.immersionPx > 0
    const loopMs = this.textures.length * 1000 / SHALLOW_WATER.framesPerSecond
    this.animationElapsedMs = this.sprite.visible ? (this.animationElapsedMs + elapsedMs) % loopMs : 0
    const frameIndex = Math.floor(this.animationElapsedMs * SHALLOW_WATER.framesPerSecond / 1000)
    this.sprite.texture = this.textures[frameIndex]!
    this.sprite.width = SHALLOW_WATER.rippleWidth * scale
    this.sprite.height = SHALLOW_WATER.rippleHeight * scale
    this.sprite.alpha = this.progress * (SHALLOW_WATER.idleAlpha + this.activity * (SHALLOW_WATER.movingAlpha - SHALLOW_WATER.idleAlpha))
  }
}
