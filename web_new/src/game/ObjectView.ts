import { Container, Sprite, Graphics, Text, Rectangle } from 'pixi.js'
import type { Spine } from '@esotericsoftware/spine-pixi-v8'
import { ResourceLoader, type ResourceDef, type LayerDef } from './ResourceLoader'
import { coordGame2Screen } from './utils/coordConvert'
import { type AABB, fromMinMax } from './culling/AABB'
import { OBJECT_BOUNDS_COLOR, OBJECT_BOUNDS_WIDTH, OBJECT_BOUNDS_ALPHA } from '@/constants/render'

export interface ObjectViewOptions {
  entityId: number
  typeId: number
  resourcePath: string
  position: { x: number; y: number }
  size: { x: number; y: number }
}

/**
 * ObjectView represents a visual game object with multi-layer rendering.
 * Supports: static sprites, shadow layers, Spine animations with 8-direction movement.
 */
export class ObjectView {
  readonly entityId: number
  readonly typeId: number

  private container: Container
  private debugText: Text | null = null
  private boundsGraphics: Graphics | null = null
  private placeholder: Graphics | null = null

  private position: { x: number; y: number }
  private size: { x: number; y: number }

  private resDef: ResourceDef | undefined
  private sprites: Sprite[] = []
  private spineAnimations: Array<Spine | undefined> = []
  private layerIndexMap: Map<number, number> = new Map() // layerIdx -> spineAnimations index
  private lastDir = 4 // default south
  private isDestroyed = false

  constructor(options: ObjectViewOptions) {
    this.entityId = options.entityId
    this.typeId = options.typeId
    this.position = options.position
    this.size = options.size

    this.container = new Container()
    this.container.sortableChildren = true

    this.resDef = ResourceLoader.getResourceDef(options.resourcePath)
    if (!this.resDef) {
      this.resDef = ResourceLoader.getResourceDef('unknown')
    }

    if (this.resDef) {
      this.buildLayers()
    } else {
      this.createPlaceholder()
    }

    this.updateScreenPosition()
    this.onStopped()
  }

  getContainer(): Container {
    return this.container
  }

  private buildLayers(): void {
    if (!this.resDef) return
    let spineIdx = 0
    for (let i = 0; i < this.resDef.layers.length; i++) {
      const layer = this.resDef.layers[i]
      if (!layer) continue
      if (layer.img) {
        this.addSpriteLayer(layer)
      }
      if (layer.spine) {
        const currentSpineIdx = spineIdx++
        this.layerIndexMap.set(i, currentSpineIdx)
        this.addSpineLayer(layer, currentSpineIdx)
      }
    }
  }

  private addSpriteLayer(layer: LayerDef): void {
    ResourceLoader.createSprite(layer, this.resDef!).then((spr) => {
      if (this.isDestroyed) {
        spr.destroy()
        return
      }
      if (layer.interactive) {
        this.setInteractive(spr)
      }
      this.sprites.push(spr)
      this.container.addChild(spr)
    })
  }

  private addSpineLayer(layer: LayerDef, spineIdx: number): void {
    ResourceLoader.loadSpine(layer, this.resDef!).then((spineAnim) => {
      if (this.isDestroyed) {
        spineAnim.destroy()
        return
      }
      // set default idle animation
      const dirs = layer.spine?.dirs
      const idleAnim = dirs?.['idle']?.[this.lastDir]
      if (idleAnim) {
        spineAnim.state.setAnimation(0, idleAnim, true)
      }
      this.spineAnimations[spineIdx] = spineAnim
      this.container.addChild(spineAnim)
    })
  }

  private createPlaceholder(): void {
    const ph = new Graphics()
    const color = this.typeId === 1 ? 0x0000ff : this.typeId === 6 ? 0xff0000 : 0x00ff00
    const s = 10
    ph.moveTo(-s, -s).lineTo(s, s).moveTo(s, -s).lineTo(-s, s)
    ph.stroke({ color, width: 3 })
    ph.hitArea = new Rectangle(-15, -15, 30, 30)
    ph.eventMode = 'static'
    ph.cursor = 'pointer'
    ph.on('pointerdown', () => this.onClick())
    this.placeholder = ph
    this.container.addChild(ph)
  }

  private setInteractive(target: Sprite | Container): void {
    target.eventMode = 'static'
    target.cursor = 'pointer'
    target.on('pointerdown', () => this.onClick())
  }

  /**
   * Called when the entity is moving in a direction (0-7).
   */
  onMoved(dir: number): void {
    this.lastDir = dir
    if (!this.resDef) return

    this.resDef.layers.forEach((layer, layerIdx) => {
      if (!layer.spine?.dirs) return
      const animName = layer.spine.dirs['walk']?.[dir]
      if (!animName) return

      const spineIdx = this.layerIndexMap.get(layerIdx)
      if (spineIdx == null) return
      const anim = this.spineAnimations[spineIdx]
      if (!anim) return

      const current = anim.state.getCurrent(0)?.animation?.name
      if (current !== animName) {
        anim.state.setAnimation(0, animName, true)
      }
    })
  }

  /**
   * Called when the entity stops moving.
   */
  onStopped(): void {
    if (!this.resDef) return

    this.resDef.layers.forEach((layer, layerIdx) => {
      if (!layer.spine?.dirs) return
      const animName = layer.spine.dirs['idle']?.[this.lastDir]
      if (!animName) return

      const spineIdx = this.layerIndexMap.get(layerIdx)
      if (spineIdx == null) return
      const anim = this.spineAnimations[spineIdx]
      if (!anim) return

      const current = anim.state.getCurrent(0)?.animation?.name
      if (current !== animName) {
        anim.state.setAnimation(0, animName, true)
      }
    })
  }

  /**
   * Update game position and convert to screen coordinates.
   */
  updatePosition(x: number, y: number): void {
    this.position.x = x
    this.position.y = y
    this.updateScreenPosition()
    this.updateBoundsGraphics()
  }

  private updateScreenPosition(): void {
    const screenPos = coordGame2Screen(this.position.x, this.position.y)
    this.container.x = screenPos.x
    this.container.y = screenPos.y
  }

  getDepthY(): number {
    return this.position.y + this.size.y
  }

  computeScreenBounds(): AABB {
    const cx = this.container.x
    const cy = this.container.y
    const halfWidth = Math.max(this.size.x, this.size.y) * 2 + 64
    const halfHeight = Math.max(this.size.x, this.size.y) + 128

    return fromMinMax(cx - halfWidth, cy - halfHeight, cx + halfWidth, cy)
  }

  getPosition(): { x: number; y: number } {
    return { x: this.position.x, y: this.position.y }
  }

  containsWorldPoint(worldX: number, worldY: number): boolean {
    const halfSizeX = this.size.x / 2
    const halfSizeY = this.size.y / 2
    return (
      worldX >= this.position.x - halfSizeX &&
      worldX <= this.position.x + halfSizeX &&
      worldY >= this.position.y - halfSizeY &&
      worldY <= this.position.y + halfSizeY
    )
  }

  setDebugMode(enabled: boolean): void {
    if (enabled && !this.debugText) {
      this.debugText = new Text({
        text: `E:${this.entityId}\nT:${this.typeId}`,
        style: {
          fontSize: 10,
          fill: 0xffffff,
          stroke: { color: 0x000000, width: 2 },
        },
      })
      this.debugText.anchor.set(0.5, 1)
      this.debugText.y = -40
      this.container.addChild(this.debugText)
    } else if (!enabled && this.debugText) {
      this.container.removeChild(this.debugText)
      this.debugText.destroy()
      this.debugText = null
    }
  }

  onClick(): void {
    console.log(`[ObjectView] Clicked entity ${this.entityId}`)
  }

  setHovered(hovered: boolean): void {
    const tint = hovered ? 0xcccccc : 0xffffff
    for (const spr of this.sprites) {
      spr.tint = tint
    }
  }

  setBoundsVisible(visible: boolean): void {
    if (visible && !this.boundsGraphics) {
      this.createBoundsGraphics()
    } else if (!visible && this.boundsGraphics) {
      this.removeBoundsGraphics()
    }
  }

  isBoundsVisible(): boolean {
    return this.boundsGraphics !== null
  }

  private createBoundsGraphics(): void {
    if (this.boundsGraphics) return
    this.boundsGraphics = new Graphics()
    this.updateBoundsGraphics()
    this.container.addChild(this.boundsGraphics)
  }

  private removeBoundsGraphics(): void {
    if (this.boundsGraphics) {
      this.container.removeChild(this.boundsGraphics)
      this.boundsGraphics.destroy()
      this.boundsGraphics = null
    }
  }

  private updateBoundsGraphics(): void {
    if (!this.boundsGraphics) return
    this.boundsGraphics.clear()
    if (this.size.x === 0 || this.size.y === 0) return

    const halfWidthX = this.size.x / 2
    const halfHeightY = this.size.y / 2
    const corners = [
      { x: this.position.x - halfWidthX, y: this.position.y - halfHeightY },
      { x: this.position.x + halfWidthX, y: this.position.y - halfHeightY },
      { x: this.position.x + halfWidthX, y: this.position.y + halfHeightY },
      { x: this.position.x - halfWidthX, y: this.position.y + halfHeightY },
    ]
    const screenCorners = corners.map(c => coordGame2Screen(c.x, c.y))
    const containerScreenPos = coordGame2Screen(this.position.x, this.position.y)
    const localCorners = screenCorners.map(s => ({
      x: s.x - containerScreenPos.x,
      y: s.y - containerScreenPos.y,
    }))

    this.boundsGraphics.setStrokeStyle({
      width: OBJECT_BOUNDS_WIDTH,
      color: OBJECT_BOUNDS_COLOR,
      alpha: OBJECT_BOUNDS_ALPHA,
    })
    this.boundsGraphics.moveTo(localCorners[0]?.x || 0, localCorners[0]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[1]?.x || 0, localCorners[1]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[2]?.x || 0, localCorners[2]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[3]?.x || 0, localCorners[3]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[0]?.x || 0, localCorners[0]?.y || 0)
    this.boundsGraphics.stroke()
  }

  destroy(): void {
    if (this.isDestroyed) return
    this.isDestroyed = true
    this.removeBoundsGraphics()
    for (const spr of this.sprites) {
      spr.destroy()
    }
    for (const spine of this.spineAnimations) {
      spine?.destroy()
    }
    if (this.placeholder) {
      this.placeholder.destroy()
    }
    this.container.destroy({ children: true })
  }
}
