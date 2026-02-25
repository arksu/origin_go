import { Application, Assets, Container, Graphics, Sprite, Texture, TextureStyle } from 'pixi.js'
import { Spine } from '@esotericsoftware/spine-pixi-v8'
import type { LayerDefLike, ResourceDefLike } from '@/types/objectEditor'

type RenderDisplay = Container | Graphics | Sprite | Spine

interface RenderedLayer {
  layerIndex: number
  kind: 'img' | 'frames' | 'spine' | 'placeholder'
  object: RenderDisplay
  zIndex: number
}

interface RenderOptions {
  selectedLayerIndex: number
  getFrameIndex: (layerIndex: number) => number
  getImageOverride: (layerIndex: number) => string | undefined
}

export class ObjectPreviewRenderer {
  private app: Application | null = null
  private world = new Container()
  private crosshair = new Graphics()
  private overlay = new Graphics()
  private renderedLayers: RenderedLayer[] = []
  private scale = 4
  private panX = 0
  private panY = 0
  private selectedLayerIndex = -1
  private renderToken = 0
  private dragEnabled = true
  private draggingLayerIndex = -1
  private dragStartClientX = 0
  private dragStartClientY = 0
  private onLayerClickCb: ((layerIndex: number) => void) | null = null
  private onLayerDragCb: ((layerIndex: number, dx: number, dy: number) => void) | null = null
  private boundDown = (e: PointerEvent) => this.onPointerDown(e)
  private boundMove = (e: PointerEvent) => this.onPointerMove(e)
  private boundUp = () => this.onPointerUp()
  private boundWheel = (e: WheelEvent) => this.onWheel(e)
  private canvasEl: HTMLCanvasElement | null = null
  private spineLoadLocks = new Map<string, Promise<void>>()

  async init(canvas: HTMLCanvasElement): Promise<void> {
    this.canvasEl = canvas
    this.app = new Application()
    TextureStyle.defaultOptions.scaleMode = 'nearest'
    await this.app.init({
      canvas,
      width: canvas.clientWidth || 400,
      height: canvas.clientHeight || 400,
      backgroundColor: 0x2a2a2a,
      antialias: false,
      resolution: 1,
    })

    this.world.sortableChildren = true
    this.app.stage.addChild(this.world)
    this.crosshair.zIndex = 999999
    this.overlay.zIndex = 1000000
    this.app.stage.addChild(this.crosshair)
    this.app.stage.addChild(this.overlay)

    this.applyCamera()
    this.drawCrosshair()

    canvas.addEventListener('pointerdown', this.boundDown)
    canvas.addEventListener('pointermove', this.boundMove)
    canvas.addEventListener('pointerup', this.boundUp)
    canvas.addEventListener('pointerleave', this.boundUp)
    canvas.addEventListener('wheel', this.boundWheel, { passive: false })
  }

  setOnLayerClick(cb: (layerIndex: number) => void): void {
    this.onLayerClickCb = cb
  }

  setOnLayerDrag(cb: (layerIndex: number, dx: number, dy: number) => void): void {
    this.onLayerDragCb = cb
  }

  setDragEnabled(enabled: boolean): void {
    this.dragEnabled = enabled
  }

  resize(width: number, height: number): void {
    if (!this.app) return
    this.app.renderer.resize(width, height)
    this.applyCamera()
    this.drawCrosshair()
    this.drawSelection()
  }

  resetView(): void {
    this.scale = 4
    this.panX = 0
    this.panY = 0
    this.applyCamera()
    this.drawCrosshair()
    this.drawSelection()
  }

  async renderResource(resource: ResourceDefLike | null, options: RenderOptions): Promise<void> {
    this.clearWorld()
    this.selectedLayerIndex = options.selectedLayerIndex
    this.drawSelection()
    if (!resource) return

    const token = ++this.renderToken
    for (let i = 0; i < resource.layers.length; i++) {
      const layer = resource.layers[i]!
      try {
        const rendered = await this.buildLayerDisplay(resource, layer, i, options)
        if (token !== this.renderToken) {
          this.destroyDisplay(rendered?.object)
          return
        }
        if (!rendered) continue
        this.renderedLayers.push(rendered)
        rendered.object.zIndex = rendered.zIndex
        this.world.addChild(rendered.object)
      } catch (error) {
        console.warn('[ObjectPreviewRenderer] Failed to build layer', i, error)
        const fallback = this.buildPlaceholder(resource, layer, i, 0xff00ff)
        if (token !== this.renderToken) {
          this.destroyDisplay(fallback.object)
          return
        }
        this.renderedLayers.push(fallback)
        this.world.addChild(fallback.object)
      }
    }
    this.drawSelection()
  }

  private async buildLayerDisplay(
    resource: ResourceDefLike,
    layer: LayerDefLike,
    layerIndex: number,
    options: RenderOptions,
  ): Promise<RenderedLayer | null> {
    const zIndex = layer.shadow ? -1000 + layerIndex : (layer.z ?? layerIndex)

    if (typeof layer.img === 'string') {
      const source = options.getImageOverride(layerIndex) ?? `/assets/game/${layer.img}`
      const sprite = new Sprite(await this.loadTexture(source))
      const [x, y] = this.computeLayerPosition(resource, layer, [0, 0])
      sprite.position.set(x, y)
      return { layerIndex, kind: 'img', object: sprite, zIndex }
    }

    if (Array.isArray(layer.frames) && layer.frames.length > 0) {
      const frameIndex = Math.max(0, Math.min(options.getFrameIndex(layerIndex), layer.frames.length - 1))
      const frame = layer.frames[frameIndex]!
      const sprite = new Sprite(await this.loadTexture(`/assets/game/${frame.img}`))
      const [x, y] = this.computeLayerPosition(resource, layer, [
        Number(frame.offset?.[0] ?? 0),
        Number(frame.offset?.[1] ?? 0),
      ])
      sprite.position.set(x, y)
      return { layerIndex, kind: 'frames', object: sprite, zIndex }
    }

    if (layer.spine) {
      const spine = await this.loadSpine(resource, layer)
      if (!spine) {
        return this.buildPlaceholder(resource, layer, layerIndex, 0x6699ff)
      }
      const [x, y] = this.computeLayerPosition(resource, layer, [0, 0])
      spine.position.set(x, y)
      return { layerIndex, kind: 'spine', object: spine, zIndex }
    }

    return this.buildPlaceholder(resource, layer, layerIndex, 0xffaa00)
  }

  private computeLayerPosition(resource: ResourceDefLike, layer: LayerDefLike, extraOffset: [number, number]): [number, number] {
    const rootOffsetX = Number(resource.offset?.[0] ?? 0)
    const rootOffsetY = Number(resource.offset?.[1] ?? 0)
    const layerOffsetX = Number(layer.offset?.[0] ?? 0)
    const layerOffsetY = Number(layer.offset?.[1] ?? 0)
    return [
      -rootOffsetX + layerOffsetX + extraOffset[0],
      -rootOffsetY + layerOffsetY + extraOffset[1],
    ]
  }

  private buildPlaceholder(
    resource: ResourceDefLike,
    layer: LayerDefLike,
    layerIndex: number,
    color: number,
  ): RenderedLayer {
    const g = new Graphics()
    const [x, y] = this.computeLayerPosition(resource, layer, [0, 0])
    g.rect(x, y, 32, 32)
    g.stroke({ color, width: 1 })
    g.moveTo(x, y).lineTo(x + 32, y + 32)
    g.moveTo(x + 32, y).lineTo(x, y + 32)
    g.stroke({ color, width: 1 })
    return {
      layerIndex,
      kind: 'placeholder',
      object: g,
      zIndex: layer.shadow ? -1000 + layerIndex : (layer.z ?? layerIndex),
    }
  }

  private async loadTexture(source: string): Promise<Texture> {
    try {
      return await Assets.load<Texture>(source)
    } catch (error) {
      console.warn('[ObjectPreviewRenderer] Failed texture load', source, error)
      return Texture.WHITE
    }
  }

  private async loadSpine(resource: ResourceDefLike, layer: LayerDefLike): Promise<Spine | null> {
    const spineDef = layer.spine
    if (!spineDef) return null
    const basePath = `/assets/game/${spineDef.file}`
    const dataAlias = `${spineDef.file}-data`
    const atlasAlias = `${spineDef.file}-atlas`

    try {
      let loading = this.spineLoadLocks.get(spineDef.file)
      if (!loading) {
        if (!Assets.resolver.hasKey(dataAlias)) {
          Assets.add({ alias: dataAlias, src: `${basePath}.json` })
          Assets.add({ alias: atlasAlias, src: `${basePath}.atlas` })
        }
        loading = Assets.load([dataAlias, atlasAlias])
          .then(() => undefined)
          .finally(() => {
            this.spineLoadLocks.delete(spineDef.file)
          })
        this.spineLoadLocks.set(spineDef.file, loading)
      }
      await loading

      const spine = Spine.from({ skeleton: dataAlias, atlas: atlasAlias, autoUpdate: true })
      if (spineDef.scale != null) {
        spine.scale = spineDef.scale
      }
      if (spineDef.skin) {
        spine.skeleton.setSkinByName(spineDef.skin)
      }
      const idle = spineDef.dirs?.idle?.[4] ?? spine.state.data.skeletonData.animations[0]?.name
      if (idle) {
        spine.state.setAnimation(0, idle, true)
      }

      const [x, y] = this.computeLayerPosition(resource, layer, [0, 0])
      spine.position.set(x, y)
      return spine
    } catch (error) {
      console.warn('[ObjectPreviewRenderer] Spine preview failed', error)
      return null
    }
  }

  private applyCamera(): void {
    if (!this.app) return
    const cx = this.app.canvas.width / 2
    const cy = this.app.canvas.height / 2
    this.world.pivot.set(0, 0)
    this.world.position.set(cx + this.panX, cy + this.panY)
    this.world.scale.set(this.scale)
  }

  private drawCrosshair(): void {
    if (!this.app) return
    const w = this.app.canvas.width
    const h = this.app.canvas.height
    const cx = w / 2 + this.panX
    const cy = h / 2 + this.panY
    this.crosshair.clear()
    this.crosshair.moveTo(cx, 0).lineTo(cx, h)
    this.crosshair.stroke({ color: 0x555555, width: 1 })
    this.crosshair.moveTo(0, cy).lineTo(w, cy)
    this.crosshair.stroke({ color: 0x555555, width: 1 })
  }

  private drawSelection(): void {
    this.overlay.clear()
    const selected = this.renderedLayers.find((layer) => layer.layerIndex === this.selectedLayerIndex)
    if (!selected) return
    const bounds = selected.object.getBounds()
    this.overlay.rect(bounds.x - 1, bounds.y - 1, bounds.width + 2, bounds.height + 2)
    this.overlay.stroke({ color: 0x00ff66, width: 1 })
  }

  private clearWorld(): void {
    this.renderToken++
    for (const layer of this.renderedLayers) {
      this.world.removeChild(layer.object)
      this.destroyDisplay(layer.object)
    }
    this.renderedLayers = []
  }

  private destroyDisplay(display?: RenderDisplay | null): void {
    if (!display) return
    if ('destroy' in display && typeof display.destroy === 'function') {
      display.destroy()
    }
  }

  private getCanvasLocalFromClient(clientX: number, clientY: number): { x: number; y: number } | null {
    if (!this.canvasEl) return null
    const rect = this.canvasEl.getBoundingClientRect()
    return { x: clientX - rect.left, y: clientY - rect.top }
  }

  private pickLayerAtClient(clientX: number, clientY: number): RenderedLayer | null {
    const local = this.getCanvasLocalFromClient(clientX, clientY)
    if (!local) return null
    const worldX = (local.x - (this.app?.canvas.width ?? 0) / 2 - this.panX) / this.scale
    const worldY = (local.y - (this.app?.canvas.height ?? 0) / 2 - this.panY) / this.scale

    const ordered = [...this.renderedLayers].sort((a, b) => a.zIndex - b.zIndex)
    for (let i = ordered.length - 1; i >= 0; i--) {
      const layer = ordered[i]!
      const bounds = layer.object.getBounds()
      // Bounds are in screen coords, so test against canvas local.
      if (
        local.x >= bounds.x &&
        local.x <= bounds.x + bounds.width &&
        local.y >= bounds.y &&
        local.y <= bounds.y + bounds.height
      ) {
        return layer
      }
    }

    // Fallback hit using world coordinates for very small graphics
    for (let i = ordered.length - 1; i >= 0; i--) {
      const layer = ordered[i]!
      const localPos = layer.object.getLocalBounds()
      const x = layer.object.x ?? 0
      const y = layer.object.y ?? 0
      if (
        worldX >= x + localPos.x &&
        worldX <= x + localPos.x + localPos.width &&
        worldY >= y + localPos.y &&
        worldY <= y + localPos.y + localPos.height
      ) {
        return layer
      }
    }
    return null
  }

  private onPointerDown(e: PointerEvent): void {
    let picked: RenderedLayer | null = null

    if (this.selectedLayerIndex >= 0) {
      const selectedLayer = this.renderedLayers.find((layer) => layer.layerIndex === this.selectedLayerIndex) ?? null
      if (selectedLayer && this.isLayerHitAtClient(selectedLayer, e.clientX, e.clientY)) {
        picked = selectedLayer
      }
    }

    if (!picked) {
      picked = this.pickLayerAtClient(e.clientX, e.clientY)
    }
    if (!picked) return
    if (picked.layerIndex !== this.selectedLayerIndex) return
    this.onLayerClickCb?.(picked.layerIndex)
    if (!this.dragEnabled) return
    if (picked.kind !== 'img') return
    this.draggingLayerIndex = picked.layerIndex
    this.dragStartClientX = e.clientX
    this.dragStartClientY = e.clientY
  }

  private isLayerHitAtClient(layer: RenderedLayer, clientX: number, clientY: number): boolean {
    const local = this.getCanvasLocalFromClient(clientX, clientY)
    if (!local) return false

    const bounds = layer.object.getBounds()
    if (
      local.x >= bounds.x &&
      local.x <= bounds.x + bounds.width &&
      local.y >= bounds.y &&
      local.y <= bounds.y + bounds.height
    ) {
      return true
    }

    const worldX = (local.x - (this.app?.canvas.width ?? 0) / 2 - this.panX) / this.scale
    const worldY = (local.y - (this.app?.canvas.height ?? 0) / 2 - this.panY) / this.scale
    const localBounds = layer.object.getLocalBounds()
    const x = layer.object.x ?? 0
    const y = layer.object.y ?? 0
    return (
      worldX >= x + localBounds.x &&
      worldX <= x + localBounds.x + localBounds.width &&
      worldY >= y + localBounds.y &&
      worldY <= y + localBounds.y + localBounds.height
    )
  }

  private onPointerMove(e: PointerEvent): void {
    if (this.draggingLayerIndex < 0) return
    const dx = e.clientX - this.dragStartClientX
    const dy = e.clientY - this.dragStartClientY
    const gridDx = Math.round(dx / this.scale)
    const gridDy = Math.round(dy / this.scale)
    if (gridDx === 0 && gridDy === 0) return
    this.dragStartClientX += gridDx * this.scale
    this.dragStartClientY += gridDy * this.scale
    this.onLayerDragCb?.(this.draggingLayerIndex, gridDx, gridDy)
  }

  private onPointerUp(): void {
    this.draggingLayerIndex = -1
  }

  private onWheel(e: WheelEvent): void {
    e.preventDefault()
    const delta = e.deltaY > 0 ? -0.25 : 0.25
    this.scale = Math.max(1, Math.min(16, this.scale + delta))
    this.applyCamera()
    this.drawCrosshair()
    this.drawSelection()
  }

  destroy(): void {
    this.clearWorld()
    if (this.canvasEl) {
      this.canvasEl.removeEventListener('pointerdown', this.boundDown)
      this.canvasEl.removeEventListener('pointermove', this.boundMove)
      this.canvasEl.removeEventListener('pointerup', this.boundUp)
      this.canvasEl.removeEventListener('pointerleave', this.boundUp)
      this.canvasEl.removeEventListener('wheel', this.boundWheel)
    }
    this.canvasEl = null
    this.crosshair.destroy()
    this.overlay.destroy()
    this.world.destroy({ children: true })
    this.app?.destroy(true)
    this.app = null
  }
}
