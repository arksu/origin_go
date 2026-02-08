import { Application, Container, Sprite, Graphics, Assets, type Spritesheet } from 'pixi.js'
import type { TerrainVariant } from '@/types/terrain'

export interface LayerSprite {
  sprite: Sprite
  highlight: Graphics
  layerIndex: number
}

export class TerrainRenderer {
  private app: Application | null = null
  private container: Container = new Container()
  private crosshair: Graphics = new Graphics()
  private layerSprites: LayerSprite[] = []
  private spritesheet: Spritesheet | null = null
  private selectedLayerIndex = -1
  private scale = 4

  private onLayerClickCallback: ((layerIndex: number) => void) | null = null
  private isDragging = false
  private dragLayerIndex = -1
  private dragStartX = 0
  private dragStartY = 0
  private onDragMoveCallback: ((layerIndex: number, dx: number, dy: number) => void) | null = null

  async init(canvas: HTMLCanvasElement): Promise<void> {
    this.app = new Application()
    await this.app.init({
      canvas,
      width: canvas.clientWidth,
      height: canvas.clientHeight,
      backgroundColor: 0x2a2a2a,
      antialias: false,
      resolution: 1,
    })

    this.container.sortableChildren = true
    this.app.stage.addChild(this.container)

    this.crosshair.zIndex = 999999
    this.app.stage.addChild(this.crosshair)

    this.applyScale()
    this.drawCrosshair()

    canvas.addEventListener('pointerdown', this.onPointerDown.bind(this))
    canvas.addEventListener('pointermove', this.onPointerMove.bind(this))
    canvas.addEventListener('pointerup', this.onPointerUp.bind(this))
    canvas.addEventListener('wheel', this.onWheel.bind(this), { passive: false })
  }

  async loadSpritesheet(jsonPath: string): Promise<void> {
    this.spritesheet = await Assets.load<Spritesheet>(jsonPath)
  }

  setOnLayerClick(cb: (layerIndex: number) => void): void {
    this.onLayerClickCallback = cb
  }

  setOnDragMove(cb: (layerIndex: number, dx: number, dy: number) => void): void {
    this.onDragMoveCallback = cb
  }

  renderVariant(
    variant: TerrainVariant,
    visibility: (layerIdx: number) => boolean,
    offsets: (layerIdx: number) => { dx: number; dy: number },
    selectedLayer: number,
  ): void {
    this.clear()
    if (!this.spritesheet) return

    this.selectedLayerIndex = selectedLayer

    const centerX = (this.app?.canvas.width ?? 400) / 2
    const centerY = (this.app?.canvas.height ?? 400) / 2

    const anchorX = centerX
    const anchorY = centerY

    for (let i = 0; i < variant.layers.length; i++) {
      const layer = variant.layers[i]!
      const visible = visibility(i)
      const offset = offsets(i)

      const dx = -(variant.offset[0] ?? 0) + (layer.offset[0] ?? 0) + offset.dx
      const dy = -(variant.offset[1] ?? 0) + (layer.offset[1] ?? 0) + offset.dy

      const textureFrameId = layer.img
      const texture = this.spritesheet.textures[textureFrameId]
      if (!texture) continue

      const sprite = new Sprite(texture)
      sprite.x = anchorX + dx
      sprite.y = anchorY + dy
      sprite.zIndex = i
      sprite.visible = visible
      sprite.eventMode = 'static'
      sprite.cursor = 'pointer'

      const highlight = new Graphics()
      this.updateHighlight(highlight, sprite, i === selectedLayer)
      highlight.visible = visible
      highlight.zIndex = i

      this.container.addChild(sprite)
      this.container.addChild(highlight)

      this.layerSprites.push({ sprite, highlight, layerIndex: i })
    }
  }

  updateSelection(selectedLayer: number): void {
    this.selectedLayerIndex = selectedLayer
    for (const ls of this.layerSprites) {
      this.updateHighlight(ls.highlight, ls.sprite, ls.layerIndex === selectedLayer)
    }
  }

  updateLayerVisibility(layerIndex: number, visible: boolean): void {
    const ls = this.layerSprites.find((l) => l.layerIndex === layerIndex)
    if (ls) {
      ls.sprite.visible = visible
      ls.highlight.visible = visible
    }
  }

  updateLayerPosition(
    layerIndex: number,
    variant: TerrainVariant,
    offset: { dx: number; dy: number },
  ): void {
    const ls = this.layerSprites.find((l) => l.layerIndex === layerIndex)
    if (!ls) return

    const layer = variant.layers[layerIndex]
    if (!layer) return

    const centerX = (this.app?.canvas.width ?? 400) / 2
    const centerY = (this.app?.canvas.height ?? 400) / 2

    const dx = -(variant.offset[0] ?? 0) + (layer.offset[0] ?? 0) + offset.dx
    const dy = -(variant.offset[1] ?? 0) + (layer.offset[1] ?? 0) + offset.dy

    ls.sprite.x = centerX + dx
    ls.sprite.y = centerY + dy
    this.updateHighlight(ls.highlight, ls.sprite, ls.layerIndex === this.selectedLayerIndex)
  }

  resize(width: number, height: number): void {
    if (this.app) {
      this.app.renderer.resize(width, height)
      this.applyScale()
      this.drawCrosshair()
    }
  }

  private applyScale(): void {
    if (!this.app) return
    const cx = this.app.canvas.width / 2
    const cy = this.app.canvas.height / 2
    this.container.pivot.set(cx, cy)
    this.container.position.set(cx, cy)
    this.container.scale.set(this.scale)
  }

  private drawCrosshair(): void {
    if (!this.app) return
    const w = this.app.canvas.width
    const h = this.app.canvas.height
    const cx = w / 2
    const cy = h / 2

    this.crosshair.clear()
    this.crosshair.moveTo(cx, 0).lineTo(cx, h)
    this.crosshair.stroke({ width: 1, color: 0x555555 })
    this.crosshair.moveTo(0, cy).lineTo(w, cy)
    this.crosshair.stroke({ width: 1, color: 0x555555 })
  }

  private onWheel(e: WheelEvent): void {
    e.preventDefault()
    const delta = e.deltaY > 0 ? -0.25 : 0.25
    this.scale = Math.max(1, Math.min(16, this.scale + delta))
    this.applyScale()
  }

  private updateHighlight(g: Graphics, sprite: Sprite, selected: boolean): void {
    g.clear()
    if (!selected) return
    g.rect(sprite.x - 1, sprite.y - 1, sprite.width + 2, sprite.height + 2)
    g.stroke({ width: 1, color: 0x00ff00 })
  }

  private clear(): void {
    for (const ls of this.layerSprites) {
      this.container.removeChild(ls.sprite)
      this.container.removeChild(ls.highlight)
      ls.sprite.destroy()
      ls.highlight.destroy()
    }
    this.layerSprites = []
  }

  private onPointerDown(e: PointerEvent): void {
    const rect = (e.target as HTMLCanvasElement).getBoundingClientRect()
    const canvasX = e.clientX - rect.left
    const canvasY = e.clientY - rect.top

    const local = this.container.toLocal({ x: canvasX, y: canvasY })
    const px = local.x
    const py = local.y

    for (let i = this.layerSprites.length - 1; i >= 0; i--) {
      const ls = this.layerSprites[i]!
      if (!ls.sprite.visible) continue
      const s = ls.sprite
      if (px >= s.x && px <= s.x + s.width && py >= s.y && py <= s.y + s.height) {
        this.onLayerClickCallback?.(ls.layerIndex)
        this.isDragging = true
        this.dragLayerIndex = ls.layerIndex
        this.dragStartX = e.clientX
        this.dragStartY = e.clientY
        return
      }
    }
  }

  private onPointerMove(e: PointerEvent): void {
    if (!this.isDragging || this.dragLayerIndex < 0) return

    const dx = e.clientX - this.dragStartX
    const dy = e.clientY - this.dragStartY

    const sdx = Math.round(dx / this.scale)
    const sdy = Math.round(dy / this.scale)
    if (sdx !== 0 || sdy !== 0) {
      this.onDragMoveCallback?.(this.dragLayerIndex, sdx, sdy)
      this.dragStartX += sdx * this.scale
      this.dragStartY += sdy * this.scale
    }
  }

  private onPointerUp(): void {
    this.isDragging = false
    this.dragLayerIndex = -1
  }

  destroy(): void {
    this.clear()
    this.app?.destroy(true)
    this.app = null
    this.spritesheet = null
  }
}
