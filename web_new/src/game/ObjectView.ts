import { Container, Sprite, Graphics, Text, Texture, Rectangle } from 'pixi.js'
import type { Spine } from '@esotericsoftware/spine-pixi-v8'
import { ResourceLoader, type ResourceDef, type LayerDef } from './ResourceLoader'
import { coordGame2Screen } from './utils/coordConvert'
import { type AABB, fromMinMax } from './culling/AABB'
import { TEXTURE_WIDTH, TEXTURE_HEIGHT } from './tiles/Tile'
import {
  OBJECT_BOUNDS_COLOR,
  OBJECT_BOUNDS_WIDTH,
  OBJECT_BOUNDS_ALPHA,
  DROP_ITEM_TYPE_ID,
  DROP_ITEM_PAD,
  DROP_ITEM_GROUND_BIAS,
  DROP_ITEM_MIN_SCALE,
  DROP_ITEM_MAX_SCALE,
  RMB_PIXEL_ALPHA_THRESHOLD,
  HOVER_BORDER_COLOR,
  HOVER_BORDER_WIDTH,
  HOVER_BORDER_ALPHA,
} from '@/constants/render'
import { getSpriteAlphaMask, hitTestSpritePixel } from './PixelHitTest'

interface AnimatedFrameLayer {
  layer: LayerDef
  sprite: Sprite
  textures: Texture[]
  frameOffsets: Array<readonly [number, number] | number[] | undefined>
  frameCount: number
  fps: number
  loop: boolean
  groupKey: string
  currentFrame: number
}

export interface ObjectViewOptions {
  entityId: number
  typeId: number
  resourcePath: string
  position: { x: number; y: number }
  size: { x: number; y: number }
}

/**
 * ObjectView represents a visual game object with multi-layer rendering.
 * Supports: static sprites, frame sprites, shadow layers, and Spine animations.
 */
export class ObjectView {
  readonly entityId: number
  readonly typeId: number

  private container: Container
  private debugText: Text | null = null
  private boundsGraphics: Graphics | null = null
  private hoverGraphics: Graphics | null = null
  private placeholder: Graphics | null = null

  private position: { x: number; y: number }
  private size: { x: number; y: number }
  private screenOffsetX = 0
  private screenOffsetY = 0
  private screenPositionOverride: { x: number; y: number } | null = null
  private zIndexOverride: number | null = null
  private interactionSuppressed = false

  private resDef: ResourceDef | undefined
  private sprites: Sprite[] = []
  private shadowSprites: Sprite[] = []
  private interactiveSprites: Sprite[] = []
  private interactiveSpritesSorted: Sprite[] = []
  private interactiveOrderDirty = true
  private spineAnimations: Array<Spine | undefined> = []
  private layerIndexMap: Map<number, number> = new Map() // layerIdx -> spineAnimations index
  private animatedFrameLayers: AnimatedFrameLayer[] = []
  private hasFrameAnimation = false
  private animationStartMs = 0
  private lastDir = 4 // default south
  private isDestroyed = false
  private isDroppedItem = false
  private hasSpineLayers = false
  private isHovered = false
  private hoverBorderDirty = true
  private hoverBorderSignature = ''
  private shadowSuppressed = false

  constructor(options: ObjectViewOptions) {
    this.entityId = options.entityId
    this.typeId = options.typeId
    this.position = options.position
    this.size = options.size

    this.container = new Container()
    this.container.sortableChildren = true
    this.animationStartMs = typeof performance !== 'undefined' ? performance.now() : Date.now()

    if (this.typeId === DROP_ITEM_TYPE_ID) {
      this.isDroppedItem = true
      this.buildDroppedItem(options.resourcePath)
    } else {
      this.resDef = ResourceLoader.getResourceDef(options.resourcePath)
      if (!this.resDef) {
        this.resDef = ResourceLoader.getResourceDef('unknown')
      }

      if (this.resDef) {
        this.buildLayers()
      } else {
        this.createPlaceholder()
      }
    }

    this.updateScreenPosition()
    this.onStopped()
  }

  getContainer(): Container {
    return this.container
  }

  hasAnimatedFrames(): boolean {
    return this.hasFrameAnimation
  }

  private buildDroppedItem(resourcePath: string): void {
    const tileW = TEXTURE_WIDTH
    const tileH = TEXTURE_HEIGHT

    ResourceLoader.loadTexture(resourcePath).then((tex) => {
      if (this.isDestroyed) return

      if (tex === Texture.WHITE) {
        this.createDroppedItemFallback()
        return
      }

      const spr = new Sprite(tex)
      spr.anchor.set(0.5, 1.0)

      const texW = tex.width
      const texH = tex.height
      const fitW = tileW * (1 - DROP_ITEM_PAD)
      const fitH = tileH * (1 - DROP_ITEM_PAD)
      const scale = Math.min(
        Math.max(Math.min(fitW / texW, fitH / texH), DROP_ITEM_MIN_SCALE),
        DROP_ITEM_MAX_SCALE,
      )
      spr.scale.set(scale)

      spr.x = 0
      spr.y = tileH * DROP_ITEM_GROUND_BIAS

      this.setInteractive(spr)
      this.registerInteractiveSprite(spr)
      this.sprites.push(spr)
      this.container.addChild(spr)
    })
  }

  private createDroppedItemFallback(): void {
    const ph = new Graphics()
    const s = 8
    ph.rect(-s, -s * 2, s * 2, s * 2)
    ph.fill({ color: 0xff00ff, alpha: 0.6 })
    ph.stroke({ color: 0xffffff, width: 1 })
    ph.hitArea = new Rectangle(-15, -15, 30, 30)
    ph.eventMode = 'static'
    ph.cursor = 'pointer'
    ph.on('pointerdown', () => this.onClick())
    this.placeholder = ph
    this.container.addChild(ph)
  }

  private buildLayers(): void {
    if (!this.resDef) return
    let spineIdx = 0
    for (let i = 0; i < this.resDef.layers.length; i++) {
      const layer = this.resDef.layers[i]
      if (!layer) continue
      if (layer.img) {
        this.addSpriteLayer(layer)
        continue
      }
      if (Array.isArray(layer.frames) && layer.frames.length > 0) {
        this.addFrameLayer(layer)
        continue
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
      if (layer.shadow) {
        this.shadowSprites.push(spr)
        spr.visible = !this.shadowSuppressed
      }
      if (layer.interactive) {
        this.setInteractive(spr)
        this.registerInteractiveSprite(spr)
      }
      this.sprites.push(spr)
      this.container.addChild(spr)
    })
  }

  private addSpineLayer(layer: LayerDef, spineIdx: number): void {
    this.hasSpineLayers = true
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

  private addFrameLayer(layer: LayerDef): void {
    if (!this.resDef || !Array.isArray(layer.frames) || layer.frames.length === 0) return

    const frames = layer.frames
    const fps = Number(layer.fps ?? 0)
    const isAnimated = Number.isFinite(fps) && fps > 0
    if (!isAnimated) {
      const firstFrame = frames[0]
      if (!firstFrame) return
      ResourceLoader.createFrameSprite(layer, this.resDef, firstFrame).then((spr) => {
        if (this.isDestroyed) {
          spr.destroy()
          return
        }
        if (layer.shadow) {
          this.shadowSprites.push(spr)
          spr.visible = !this.shadowSuppressed
        }
        if (layer.interactive) {
          this.setInteractive(spr)
          this.registerInteractiveSprite(spr)
        }
        this.sprites.push(spr)
        this.container.addChild(spr)
      })
      return
    }

    this.hasFrameAnimation = true
    Promise.all(frames.map((frame) => ResourceLoader.loadTexture(frame.img))).then((textures) => {
      if (this.isDestroyed || !this.resDef) {
        return
      }

      const initialFrame = frames[0]
      if (!initialFrame) return
      const sprite = new Sprite(textures[0] ?? Texture.WHITE)
      const initialPos = ResourceLoader.resolveLayerPosition(layer, this.resDef, initialFrame.offset)
      sprite.x = initialPos.x
      sprite.y = initialPos.y
      sprite.zIndex = ResourceLoader.resolveLayerZ(layer)

      if (layer.shadow) {
        this.shadowSprites.push(sprite)
        sprite.visible = !this.shadowSuppressed
      }
      if (layer.interactive) {
        this.setInteractive(sprite)
        this.registerInteractiveSprite(sprite)
      }
      this.sprites.push(sprite)
      this.container.addChild(sprite)

      const frameLayer: AnimatedFrameLayer = {
        layer,
        sprite,
        textures,
        frameOffsets: frames.map((frame) => frame.offset),
        frameCount: frames.length,
        fps,
        loop: layer.loop !== false,
        groupKey: `${fps}:${frames.length}`,
        currentFrame: 0,
      }
      this.animatedFrameLayers.push(frameLayer)

      const nowMs = typeof performance !== 'undefined' ? performance.now() : Date.now()
      this.updateAnimatedFrameLayer(frameLayer, this.computeFrameIndex(frameLayer, nowMs, new Map()))
    }).catch((err: unknown) => {
      console.warn('[ObjectView] Failed to load frame animation layer', err)
    })
  }

  updateAnimation(nowMs: number): void {
    if (!this.hasFrameAnimation || this.animatedFrameLayers.length === 0 || this.isDestroyed || !this.resDef) {
      return
    }

    const sharedSteps = new Map<string, number>()
    for (const frameLayer of this.animatedFrameLayers) {
      const nextFrame = this.computeFrameIndex(frameLayer, nowMs, sharedSteps)
      this.updateAnimatedFrameLayer(frameLayer, nextFrame)
    }
  }

  private computeFrameIndex(
    frameLayer: AnimatedFrameLayer,
    nowMs: number,
    sharedSteps: Map<string, number>,
  ): number {
    const cached = sharedSteps.get(frameLayer.groupKey)
    let step = cached
    if (step == null) {
      const elapsedMs = Math.max(0, nowMs - this.animationStartMs)
      step = Math.floor((elapsedMs * frameLayer.fps) / 1000)
      sharedSteps.set(frameLayer.groupKey, step)
    }

    if (frameLayer.loop) {
      return step % frameLayer.frameCount
    }
    return Math.min(step, frameLayer.frameCount - 1)
  }

  private updateAnimatedFrameLayer(frameLayer: AnimatedFrameLayer, frameIndex: number): void {
    if (!this.resDef || frameIndex === frameLayer.currentFrame) {
      return
    }

    frameLayer.currentFrame = frameIndex
    frameLayer.sprite.texture = frameLayer.textures[frameIndex] ?? Texture.WHITE
    const pos = ResourceLoader.resolveLayerPosition(frameLayer.layer, this.resDef, frameLayer.frameOffsets[frameIndex])
    frameLayer.sprite.x = pos.x
    frameLayer.sprite.y = pos.y

    if (frameLayer.layer.interactive) {
      this.hoverBorderDirty = true
      if (this.isHovered && this.hoverGraphics) {
        this.rebuildHoverBorder()
      }
    }
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
    if (this.isDroppedItem || !this.resDef) return

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
    if (this.isDroppedItem || !this.resDef) return

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
    if (this.screenPositionOverride) {
      this.container.x = this.screenPositionOverride.x
      this.container.y = this.screenPositionOverride.y
      return
    }
    const screenPos = coordGame2Screen(this.position.x, this.position.y)
    this.container.x = screenPos.x + this.screenOffsetX
    this.container.y = screenPos.y + this.screenOffsetY
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
    // For dropped items, use a larger hit area for easier clicking
    if (this.isDroppedItem) {
      const hitRadius = 15 // 30x30 hit area centered on position
      return (
        worldX >= this.position.x - hitRadius &&
        worldX <= this.position.x + hitRadius &&
        worldY >= this.position.y - hitRadius &&
        worldY <= this.position.y + hitRadius
      )
    }

    const halfSizeX = this.size.x / 2
    const halfSizeY = this.size.y / 2
    return (
      worldX >= this.position.x - halfSizeX &&
      worldX <= this.position.x + halfSizeX &&
      worldY >= this.position.y - halfSizeY &&
      worldY <= this.position.y + halfSizeY
    )
  }

  hitTestRmbScreenPoint(
    screenX: number,
    screenY: number,
    screenToWorld: (x: number, y: number) => { x: number; y: number }
  ): boolean {
    if (this.interactionSuppressed) {
      return false
    }

    const sortedSprites = this.getInteractiveSpritesForHitTest()
    for (const sprite of sortedSprites) {
      if (!sprite.visible || !sprite.renderable) continue
      if (hitTestSpritePixel(sprite, screenX, screenY, RMB_PIXEL_ALPHA_THRESHOLD)) {
        return true
      }
    }

    if (this.hitPlaceholderBounds(screenX, screenY)) {
      return true
    }

    // Spine is dynamic; keep bounds fallback for now.
    if (this.hasSpineLayers) {
      const worldPos = screenToWorld(screenX, screenY)
      return this.containsWorldPoint(worldPos.x, worldPos.y)
    }

    return false
  }

  private hitPlaceholderBounds(screenX: number, screenY: number): boolean {
    if (!this.placeholder || !this.placeholder.visible) return false

    const bounds = this.placeholder.getBounds()
    return (
      screenX >= bounds.minX &&
      screenX <= bounds.maxX &&
      screenY >= bounds.minY &&
      screenY <= bounds.maxY
    )
  }

  private getInteractiveSpritesForHitTest(): Sprite[] {
    if (!this.interactiveOrderDirty) {
      return this.interactiveSpritesSorted
    }

    this.interactiveSpritesSorted = this.interactiveSprites
      .slice()
      .sort((left, right) => {
        if (left.zIndex !== right.zIndex) {
          return right.zIndex - left.zIndex
        }

        // If zIndex is equal, prefer later children (rendered on top).
        return this.container.getChildIndex(right) - this.container.getChildIndex(left)
      })

    this.interactiveOrderDirty = false
    return this.interactiveSpritesSorted
  }

  private registerInteractiveSprite(sprite: Sprite): void {
    this.interactiveSprites.push(sprite)
    this.interactiveOrderDirty = true
    this.hoverBorderDirty = true
    if (this.isHovered && this.hoverGraphics) {
      this.rebuildHoverBorder()
    }
  }

  private computeHoverSignature(): string {
    const parts: string[] = []
    for (const sprite of this.getInteractiveSpritesForHitTest()) {
      const texture = sprite.texture
      const frame = texture.frame
      parts.push([
        texture.uid,
        frame.x,
        frame.y,
        frame.width,
        frame.height,
        sprite.visible ? 1 : 0,
        sprite.renderable ? 1 : 0,
      ].join(':'))
    }
    return parts.join('|')
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
    console.log(`[ObjectView] Clicked entity ${this.entityId} (typeId: ${this.typeId})`)
  }

  setHovered(hovered: boolean): void {
    if (this.isHovered === hovered) {
      if (hovered) {
        const signature = this.computeHoverSignature()
        if (signature !== this.hoverBorderSignature) {
          this.hoverBorderDirty = true
        }
      }
      if (hovered && this.hoverBorderDirty) {
        this.ensureHoverGraphics()
        this.rebuildHoverBorder()
      }
      return
    }

    this.isHovered = hovered

    if (!hovered) {
      if (this.hoverGraphics) {
        this.hoverGraphics.visible = false
      }
      return
    }

    this.ensureHoverGraphics()
    this.rebuildHoverBorder()
  }

  private ensureHoverGraphics(): void {
    if (this.hoverGraphics) {
      this.hoverGraphics.visible = true
      return
    }

    this.hoverGraphics = new Graphics()
    this.hoverGraphics.zIndex = 9999
    this.hoverGraphics.eventMode = 'none'
    this.container.addChild(this.hoverGraphics)
  }

  private rebuildHoverBorder(): void {
    if (!this.hoverGraphics) return

    this.hoverGraphics.clear()

    const drewSpriteOutline = this.drawInteractiveSpriteHoverOutline(this.hoverGraphics)
    if (!drewSpriteOutline) {
      this.hoverGraphics.setStrokeStyle({
        width: HOVER_BORDER_WIDTH,
        color: HOVER_BORDER_COLOR,
        alpha: HOVER_BORDER_ALPHA,
      })
      this.drawFallbackHoverOutline(this.hoverGraphics)
      this.hoverGraphics.stroke()
    } else {
      this.hoverGraphics.fill({ color: HOVER_BORDER_COLOR, alpha: HOVER_BORDER_ALPHA })
    }
    this.hoverGraphics.visible = true
    this.hoverBorderDirty = false
    this.hoverBorderSignature = this.computeHoverSignature()
  }

  private drawInteractiveSpriteHoverOutline(graphics: Graphics): boolean {
    const occupied = new Set<string>()
    const sprites = this.getInteractiveSpritesForHitTest()

    for (const sprite of sprites) {
      if (!sprite.visible || !sprite.renderable) continue

      const mask = getSpriteAlphaMask(sprite)
      if (!mask) continue

      const orig = sprite.texture.orig
      const localTransform = sprite.localTransform

      for (let py = 0; py < mask.height; py++) {
        for (let px = 0; px < mask.width; px++) {
          const bitIndex = py * mask.width + px
          const byte = mask.bits[bitIndex >> 3] ?? 0
          const isOpaque = (byte & (1 << (bitIndex & 7))) !== 0
          if (!isOpaque) continue

          const localOrigX = (px + 0.5) / mask.resolution - sprite.anchor.x * orig.width
          const localOrigY = (py + 0.5) / mask.resolution - sprite.anchor.y * orig.height

          const ox = Math.round(localTransform.a * localOrigX + localTransform.c * localOrigY + localTransform.tx)
          const oy = Math.round(localTransform.b * localOrigX + localTransform.d * localOrigY + localTransform.ty)

          occupied.add(`${ox},${oy}`)
        }
      }
    }

    if (occupied.size === 0) {
      return false
    }

    const boundaryByY = new Map<number, number[]>()

    for (const key of occupied) {
      const comma = key.indexOf(',')
      const x = Number(key.slice(0, comma))
      const y = Number(key.slice(comma + 1))

      const top = occupied.has(`${x},${y - 1}`)
      const right = occupied.has(`${x + 1},${y}`)
      const bottom = occupied.has(`${x},${y + 1}`)
      const left = occupied.has(`${x - 1},${y}`)
      if (top && right && bottom && left) continue

      const row = boundaryByY.get(y)
      if (row) {
        row.push(x)
      } else {
        boundaryByY.set(y, [x])
      }
    }

    for (const [y, row] of boundaryByY) {
      if (row.length === 0) continue
      row.sort((a, b) => a - b)
      const first = row[0]
      if (first == null) continue
      let start = first
      let prev = first

      for (let i = 1; i < row.length; i++) {
        const value = row[i]
        if (value == null) continue
        if (value === prev + 1) {
          prev = value
          continue
        }

        graphics.rect(start, y, prev - start + 1, 1)
        start = value
        prev = value
      }

      graphics.rect(start, y, prev - start + 1, 1)
    }

    return true
  }

  private drawFallbackHoverOutline(graphics: Graphics): void {
    if (this.placeholder) {
      const bounds = this.placeholder.getBounds()
      const localTopLeft = this.container.toLocal({ x: bounds.minX, y: bounds.minY })
      const localBottomRight = this.container.toLocal({ x: bounds.maxX, y: bounds.maxY })
      graphics.rect(
        localTopLeft.x,
        localTopLeft.y,
        localBottomRight.x - localTopLeft.x,
        localBottomRight.y - localTopLeft.y,
      )
      return
    }

    if (this.size.x > 0 && this.size.y > 0) {
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

      graphics.moveTo(localCorners[0]?.x || 0, localCorners[0]?.y || 0)
      graphics.lineTo(localCorners[1]?.x || 0, localCorners[1]?.y || 0)
      graphics.lineTo(localCorners[2]?.x || 0, localCorners[2]?.y || 0)
      graphics.lineTo(localCorners[3]?.x || 0, localCorners[3]?.y || 0)
      graphics.lineTo(localCorners[0]?.x || 0, localCorners[0]?.y || 0)
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

  setVisualScreenOffset(dx: number, dy: number): void {
    if (this.screenOffsetX === dx && this.screenOffsetY === dy) {
      return
    }
    this.screenOffsetX = dx
    this.screenOffsetY = dy
    this.updateScreenPosition()
    this.updateBoundsGraphics()
  }

  clearVisualScreenOffset(): void {
    this.setVisualScreenOffset(0, 0)
  }

  setScreenPositionOverride(x: number, y: number): void {
    const current = this.screenPositionOverride
    if (current && current.x === x && current.y === y) {
      return
    }
    this.screenPositionOverride = { x, y }
    this.updateScreenPosition()
    this.updateBoundsGraphics()
  }

  clearScreenPositionOverride(): void {
    if (this.screenPositionOverride == null) {
      return
    }
    this.screenPositionOverride = null
    this.updateScreenPosition()
    this.updateBoundsGraphics()
  }

  setZIndexOverride(zIndex: number | null): void {
    if (this.zIndexOverride === zIndex) {
      return
    }
    this.zIndexOverride = zIndex
    if (zIndex != null) {
      this.container.zIndex = zIndex
    }
  }

  getZIndexOverride(): number | null {
    return this.zIndexOverride
  }

  setInteractionSuppressed(suppressed: boolean): void {
    this.interactionSuppressed = suppressed
  }

  setShadowSuppressed(suppressed: boolean): void {
    if (this.shadowSuppressed === suppressed) {
      return
    }
    this.shadowSuppressed = suppressed
    for (const spr of this.shadowSprites) {
      spr.visible = !suppressed
    }
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
    if (this.hoverGraphics) {
      this.hoverGraphics.destroy()
      this.hoverGraphics = null
    }
    if (this.placeholder) {
      this.placeholder.destroy()
    }
    this.animatedFrameLayers = []
    this.container.destroy({ children: true })
  }
}
