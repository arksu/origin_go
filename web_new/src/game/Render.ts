import { Application, Container } from 'pixi.js'
import { DebugOverlay } from './DebugOverlay'
import { ChunkManager } from './ChunkManager'
import { ObjectManager } from './ObjectManager'
import { moveController } from './MoveController'
import { coordGame2Screen, coordScreen2Game } from './utils/coordConvert'
import { timeSync } from '@/network/TimeSync'
import { config } from '@/config'
import type { DebugInfo, ScreenPoint } from './types'

export class Render {
  private app: Application
  private mapContainer: Container
  private objectsContainer: Container
  private uiContainer: Container
  private debugOverlay: DebugOverlay
  private chunkManager: ChunkManager
  private objectManager: ObjectManager

  private cameraX: number = 0
  private cameraY: number = 0
  private zoom: number = 1

  private lastClickScreen: ScreenPoint = { x: 0, y: 0 }
  private lastClickWorld: ScreenPoint = { x: 0, y: 0 }

  private onClickCallback: ((screen: ScreenPoint) => void) | null = null

  private canvas: HTMLCanvasElement | null = null
  private pointerDownHandler: ((e: PointerEvent) => void) | null = null
  private keyDownHandler: ((e: KeyboardEvent) => void) | null = null

  private playerEntityId: number | null = null

  constructor() {
    this.app = new Application()
    this.mapContainer = new Container()
    this.objectsContainer = new Container()
    this.uiContainer = new Container()
    this.debugOverlay = new DebugOverlay()
    this.chunkManager = new ChunkManager()
    this.objectManager = new ObjectManager()
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
    this.objectsContainer.addChild(this.objectManager.getContainer())

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
    this.updateMovement()
    this.objectManager.update()
    this.updateCamera()
    this.updateDebugOverlay()
  }

  private updateMovement(): void {
    // Get interpolated positions from MoveController
    const positions = moveController.update()

    // Update visual positions for all tracked entities
    // ObjectView handles game->screen conversion internally
    for (const [entityId, renderPos] of positions) {
      this.objectManager.updateObjectPosition(entityId, renderPos.x, renderPos.y)

      // Update camera to follow player (cameraX/Y should be world coordinates)
      if (this.playerEntityId !== null && entityId === this.playerEntityId) {
        this.cameraX = renderPos.x
        this.cameraY = renderPos.y
      }
    }
  }

  private updateCamera(): void {
    // Convert world coordinates to screen coordinates for camera positioning
    const screenPos = coordGame2Screen(this.cameraX, this.cameraY)

    this.mapContainer.x = -screenPos.x * this.zoom + this.app.screen.width / 2
    this.mapContainer.y = -screenPos.y * this.zoom + this.app.screen.height / 2
    this.mapContainer.scale.set(this.zoom)

    this.objectsContainer.x = this.mapContainer.x
    this.objectsContainer.y = this.mapContainer.y
    this.objectsContainer.scale.set(this.zoom)
  }

  private updateDebugOverlay(): void {
    if (!this.debugOverlay.isVisible()) return

    const timeSyncMetrics = timeSync.getDebugMetrics()
    const moveMetrics = moveController.getGlobalDebugMetrics()

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
      objectsCount: this.objectManager.getObjectCount(),
      chunksLoaded: this.chunkManager.getLoadedChunksCount(),
      // Movement metrics
      rttMs: timeSyncMetrics.rttMs,
      jitterMs: timeSyncMetrics.jitterMs,
      timeOffsetMs: timeSyncMetrics.offsetMs,
      interpolationDelayMs: timeSyncMetrics.interpolationDelayMs,
      moveEntityCount: moveMetrics.entityCount,
      totalSnapCount: moveMetrics.totalSnapCount,
      totalIgnoredOutOfOrder: moveMetrics.totalIgnoredOutOfOrder,
      totalBufferUnderrun: moveMetrics.totalBufferUnderrun,
    }

    this.debugOverlay.update(info)
  }

  screenToWorld(screenX: number, screenY: number): ScreenPoint {
    // Convert screen coordinates to world coordinates using isometric projection
    const cameraScreenPos = coordGame2Screen(this.cameraX, this.cameraY)

    // Convert screen coordinates to game coordinates relative to camera
    const relativeScreenX = (screenX - this.app.screen.width / 2) / this.zoom + cameraScreenPos.x
    const relativeScreenY = (screenY - this.app.screen.height / 2) / this.zoom + cameraScreenPos.y

    // Convert screen coordinates to world coordinates
    return coordScreen2Game(relativeScreenX, relativeScreenY)
  }

  worldToScreen(worldX: number, worldY: number): ScreenPoint {
    // Convert world coordinates to screen coordinates using isometric projection
    const screenPos = coordGame2Screen(worldX, worldY)
    const cameraScreenPos = coordGame2Screen(this.cameraX, this.cameraY)

    const screenX = (screenPos.x - cameraScreenPos.x) * this.zoom + this.app.screen.width / 2
    const screenY = (screenPos.y - cameraScreenPos.y) * this.zoom + this.app.screen.height / 2
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

  setPlayerEntityId(entityId: number | null): void {
    this.playerEntityId = entityId
  }

  loadChunk(x: number, y: number, tiles: Uint8Array): void {
    this.chunkManager.loadChunk(x, y, tiles)
  }

  unloadChunk(x: number, y: number): void {
    this.chunkManager.unloadChunk(x, y)
  }

  spawnObject(options: { entityId: number; objectType: number; resourcePath: string; position: { x: number; y: number }; size: { x: number; y: number } }): void {
    this.objectManager.spawnObject(options)
  }

  despawnObject(entityId: number): void {
    this.objectManager.despawnObject(entityId)
  }

  updateObjectPosition(entityId: number, x: number, y: number): void {
    this.objectManager.updateObjectPosition(entityId, x, y)
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
    this.objectManager.destroy()
    this.debugOverlay.destroy()
    this.app.destroy(true, { children: true, texture: true })

    this.canvas = null
    this.pointerDownHandler = null
    this.keyDownHandler = null
    this.onClickCallback = null
  }
}
