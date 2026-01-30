import { Application, Container } from 'pixi.js'
import { DebugOverlay } from './DebugOverlay'
import { ChunkManager } from './ChunkManager'
import { config } from '@/config'
import type { DebugInfo, ScreenPoint } from './types'

export class Render {
  private app: Application
  private mapContainer: Container
  private objectsContainer: Container
  private uiContainer: Container
  private debugOverlay: DebugOverlay
  private chunkManager: ChunkManager

  private cameraX: number = 0
  private cameraY: number = 0
  private zoom: number = 1

  private lastClickScreen: ScreenPoint = { x: 0, y: 0 }
  private lastClickWorld: ScreenPoint = { x: 0, y: 0 }

  private onClickCallback: ((screen: ScreenPoint) => void) | null = null

  private canvas: HTMLCanvasElement | null = null
  private pointerDownHandler: ((e: PointerEvent) => void) | null = null
  private keyDownHandler: ((e: KeyboardEvent) => void) | null = null

  constructor() {
    this.app = new Application()
    this.mapContainer = new Container()
    this.objectsContainer = new Container()
    this.uiContainer = new Container()
    this.debugOverlay = new DebugOverlay()
    this.chunkManager = new ChunkManager()
  }

  async init(canvas: HTMLCanvasElement): Promise<void> {
    this.canvas = canvas
    const resolution = Math.min(window.devicePixelRatio, 2)

    await this.app.init({
      canvas,
      resolution,
      autoDensity: true,
      resizeTo: window,
      background: '#1a1a2e',
      antialias: true,
    })

    this.mapContainer.sortableChildren = true
    this.objectsContainer.sortableChildren = true
    this.uiContainer.sortableChildren = true

    await this.chunkManager.init()
    this.mapContainer.addChild(this.chunkManager.getContainer())

    this.app.stage.addChild(this.mapContainer)
    this.app.stage.addChild(this.objectsContainer)
    this.app.stage.addChild(this.uiContainer)
    this.uiContainer.addChild(this.debugOverlay.getContainer())

    this.debugOverlay.setVisible(config.DEBUG)

    this.setupPointerEvents()
    this.setupKeyboardEvents()

    this.app.ticker.add(this.update.bind(this))
  }

  private setupPointerEvents(): void {
    if (!this.canvas) return

    this.pointerDownHandler = (e: PointerEvent) => {
      this.lastClickScreen = { x: e.clientX, y: e.clientY }
      this.lastClickWorld = this.screenToWorld(e.clientX, e.clientY)
      this.onClickCallback?.(this.lastClickScreen)
    }

    this.canvas.addEventListener('pointerdown', this.pointerDownHandler)
  }

  private setupKeyboardEvents(): void {
    this.keyDownHandler = (e: KeyboardEvent) => {
      if (e.key === '`') {
        this.debugOverlay.toggle()
      }
    }

    window.addEventListener('keydown', this.keyDownHandler)
  }

  private update(): void {
    this.updateCamera()
    this.updateDebugOverlay()
  }

  private updateCamera(): void {
    this.mapContainer.x = -this.cameraX * this.zoom + this.app.screen.width / 2
    this.mapContainer.y = -this.cameraY * this.zoom + this.app.screen.height / 2
    this.mapContainer.scale.set(this.zoom)

    this.objectsContainer.x = this.mapContainer.x
    this.objectsContainer.y = this.mapContainer.y
    this.objectsContainer.scale.set(this.zoom)
  }

  private updateDebugOverlay(): void {
    if (!this.debugOverlay.isVisible()) return

    const info: DebugInfo = {
      fps: this.app.ticker.FPS,
      cameraX: this.cameraX,
      cameraY: this.cameraY,
      zoom: this.zoom,
      viewportWidth: Math.round(this.app.screen.width),
      viewportHeight: Math.round(this.app.screen.height),
      lastClickScreenX: this.lastClickScreen.x,
      lastClickScreenY: this.lastClickScreen.y,
      lastClickWorldX: this.lastClickWorld.x,
      lastClickWorldY: this.lastClickWorld.y,
      objectsCount: 0,
      chunksLoaded: this.chunkManager.getLoadedChunksCount(),
    }

    this.debugOverlay.update(info)
  }

  screenToWorld(screenX: number, screenY: number): ScreenPoint {
    const worldX = (screenX - this.app.screen.width / 2) / this.zoom + this.cameraX
    const worldY = (screenY - this.app.screen.height / 2) / this.zoom + this.cameraY
    return { x: worldX, y: worldY }
  }

  worldToScreen(worldX: number, worldY: number): ScreenPoint {
    const screenX = (worldX - this.cameraX) * this.zoom + this.app.screen.width / 2
    const screenY = (worldY - this.cameraY) * this.zoom + this.app.screen.height / 2
    return { x: screenX, y: screenY }
  }

  setCamera(x: number, y: number): void {
    this.cameraX = x
    this.cameraY = y
  }

  setZoom(zoom: number): void {
    this.zoom = Math.max(0.25, Math.min(4, zoom))
  }

  getZoom(): number {
    return this.zoom
  }

  getCameraPosition(): ScreenPoint {
    return { x: this.cameraX, y: this.cameraY }
  }

  getMapContainer(): Container {
    return this.mapContainer
  }

  getObjectsContainer(): Container {
    return this.objectsContainer
  }

  getApp(): Application {
    return this.app
  }

  getChunkManager(): ChunkManager {
    return this.chunkManager
  }

  setWorldParams(coordPerTile: number, chunkSize: number): void {
    this.chunkManager.setWorldParams(coordPerTile, chunkSize)
  }

  loadChunk(x: number, y: number, tiles: Uint8Array): void {
    this.chunkManager.loadChunk(x, y, tiles)
  }

  unloadChunk(x: number, y: number): void {
    this.chunkManager.unloadChunk(x, y)
  }

  onPointerClick(callback: (screen: ScreenPoint) => void): void {
    this.onClickCallback = callback
  }

  updateDebugStats(objectsCount: number, chunksLoaded: number): void {
    if (!this.debugOverlay.isVisible()) return

    const info: DebugInfo = {
      fps: this.app.ticker.FPS,
      cameraX: this.cameraX,
      cameraY: this.cameraY,
      zoom: this.zoom,
      viewportWidth: Math.round(this.app.screen.width),
      viewportHeight: Math.round(this.app.screen.height),
      lastClickScreenX: this.lastClickScreen.x,
      lastClickScreenY: this.lastClickScreen.y,
      lastClickWorldX: this.lastClickWorld.x,
      lastClickWorldY: this.lastClickWorld.y,
      objectsCount,
      chunksLoaded,
    }

    this.debugOverlay.update(info)
  }

  destroy(): void {
    this.app.ticker.stop()

    if (this.canvas && this.pointerDownHandler) {
      this.canvas.removeEventListener('pointerdown', this.pointerDownHandler)
    }

    if (this.keyDownHandler) {
      window.removeEventListener('keydown', this.keyDownHandler)
    }

    this.chunkManager.destroy()
    this.debugOverlay.destroy()
    this.app.destroy(true, { children: true, texture: true })

    this.canvas = null
    this.pointerDownHandler = null
    this.keyDownHandler = null
    this.onClickCallback = null
  }
}
