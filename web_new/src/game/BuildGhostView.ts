import { Container, Graphics } from 'pixi.js'
import type { Spine } from '@esotericsoftware/spine-pixi-v8'
import { TERRAIN_BASE_Z_INDEX } from '@/constants/terrain'
import { ResourceLoader, type LayerDef, type ResourceDef } from './ResourceLoader'
import { coordGame2Screen } from './utils/coordConvert'

const DEFAULT_IDLE_DIR = 4
const GHOST_ALPHA = 0.55

export class BuildGhostView {
  private container: Container
  private isDestroyed = false
  private resDef: ResourceDef | undefined
  private spineAnimations: Array<Spine | undefined> = []
  private layerIndexMap: Map<number, number> = new Map()
  private lastDir = DEFAULT_IDLE_DIR

  constructor(resourcePath: string) {
    this.container = new Container()
    this.container.sortableChildren = true
    this.container.eventMode = 'none'
    this.container.alpha = GHOST_ALPHA

    this.resDef = ResourceLoader.getResourceDef(resourcePath.trim()) || ResourceLoader.getResourceDef('unknown')
    if (this.resDef) {
      this.buildLayers()
      this.onStopped()
    } else {
      this.createPlaceholder()
    }

    this.updatePosition(0, 0)
  }

  getContainer(): Container {
    return this.container
  }

  updatePosition(x: number, y: number): void {
    const screenPos = coordGame2Screen(x, y)
    this.container.x = screenPos.x
    this.container.y = screenPos.y
    this.container.zIndex = TERRAIN_BASE_Z_INDEX + this.container.y
  }

  destroy(): void {
    if (this.isDestroyed) return
    this.isDestroyed = true

    this.container.destroy({ children: true })
    this.spineAnimations = []
    this.layerIndexMap.clear()
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
    if (!this.resDef) return

    ResourceLoader.createSprite(layer, this.resDef).then((sprite) => {
      if (this.isDestroyed) {
        sprite.destroy()
        return
      }

      sprite.eventMode = 'none'
      this.container.addChild(sprite)
    })
  }

  private addSpineLayer(layer: LayerDef, spineIdx: number): void {
    if (!this.resDef) return

    ResourceLoader.loadSpine(layer, this.resDef).then((spineAnim) => {
      if (this.isDestroyed) {
        spineAnim.destroy()
        return
      }

      const idleAnim = layer.spine?.dirs?.idle?.[this.lastDir]
      if (idleAnim) {
        spineAnim.state.setAnimation(0, idleAnim, true)
      }

      spineAnim.eventMode = 'none'
      this.spineAnimations[spineIdx] = spineAnim
      this.container.addChild(spineAnim)
    }).catch((err: unknown) => {
      console.warn('[BuildGhostView] Failed to load spine layer', err)
    })
  }

  private onStopped(): void {
    if (!this.resDef) return

    this.resDef.layers.forEach((layer, layerIdx) => {
      if (!layer.spine?.dirs) return

      const animName = layer.spine.dirs.idle?.[this.lastDir]
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

  private createPlaceholder(): void {
    const marker = new Graphics()
    marker.eventMode = 'none'
    marker.circle(0, -12, 10)
    marker.fill({ color: 0xa8e7ff, alpha: 0.24 })
    marker.stroke({ color: 0xdff7ff, alpha: 0.8, width: 2 })
    marker.moveTo(-12, -12)
    marker.lineTo(12, -12)
    marker.moveTo(0, -26)
    marker.lineTo(0, 2)
    marker.stroke({ color: 0xdff7ff, alpha: 0.8, width: 1 })
    this.container.addChild(marker)
  }
}
