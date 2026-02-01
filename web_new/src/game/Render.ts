import { Application, Container } from 'pixi.js'
import { DebugOverlay } from './DebugOverlay'
import { ChunkManager } from './ChunkManager'
import { ObjectManager } from './ObjectManager'
import { moveController } from './MoveController'
import { InputController } from './InputController'
import { cameraController } from './CameraController'
import { playerCommandController } from './PlayerCommandController'
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
  private inputController: InputController

  private lastClickScreen: ScreenPoint = { x: 0, y: 0 }
  private lastClickWorld: ScreenPoint = { x: 0, y: 0 }

  private onClickCallback: ((screen: ScreenPoint) => void) | null = null

  private canvas: HTMLCanvasElement | null = null
  private keyDownHandler: ((e: KeyboardEvent) => void) | null = null

  constructor() {
    this.app = new Application()
    this.mapContainer = new Container()
    this.objectsContainer = new Container()
    this.uiContainer = new Container()
    this.debugOverlay = new DebugOverlay()
    this.chunkManager = new ChunkManager()
    this.objectManager = new ObjectManager()
    this.inputController = new InputController()
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

    this.chunkManager.setObjectsContainer(this.objectsContainer)
    await this.chunkManager.init()
    this.mapContainer.addChild(this.chunkManager.getContainer())
    this.objectsContainer.addChild(this.objectManager.getContainer())

    this.app.stage.addChild(this.mapContainer)
    this.app.stage.addChild(this.objectsContainer)
    this.app.stage.addChild(this.uiContainer)
    this.uiContainer.addChild(this.debugOverlay.getContainer())

    this.debugOverlay.setVisible(config.DEBUG)

    this.setupInputController()
    this.setupKeyboardEvents()

    this.app.ticker.add(this.update.bind(this))
  }

  private setupInputController(): void {
    if (!this.canvas) return

    this.inputController.init(this.canvas)

    this.inputController.onClick((event) => {
      this.lastClickScreen = { x: event.screenX, y: event.screenY }
      this.lastClickWorld = this.screenToWorld(event.screenX, event.screenY)
      this.onClickCallback?.(this.lastClickScreen)

      if (event.button === 0) {
        const clickedEntity = this.objectManager.getEntityAtScreen(
          event.screenX,
          event.screenY,
          this.screenToWorld.bind(this)
        )

        if (clickedEntity !== null) {
          playerCommandController.sendMoveToEntity(clickedEntity, true, event.modifiers)
        } else {
          playerCommandController.sendMoveTo(
            this.lastClickWorld.x,
            this.lastClickWorld.y,
            event.modifiers
          )
        }
      }
    })

    this.inputController.onDragStart((button) => {
      if (button === 1) {
        cameraController.startPan()
      }
    })

    this.inputController.onDragMove((event) => {
      if (event.button === 1) {
        cameraController.pan(event.deltaX, event.deltaY)
      }
    })

    this.inputController.onDragEnd((button) => {
      if (button === 1) {
        cameraController.endPan()
      }
    })

    this.inputController.onWheel((event) => {
      cameraController.adjustZoom(event.deltaY > 0 ? 1 : -1)
    })
  }

  private setupKeyboardEvents(): void {
    this.keyDownHandler = (e: KeyboardEvent) => {
      if (e.key === '`') {
        this.debugOverlay.toggle()
      }
    }

    window.addEventListener('keydown', this.keyDownHandler)
  }

  private lastUpdateTime: number = 0

  private update(): void {
    const now = performance.now()
    const dt = this.lastUpdateTime > 0 ? now - this.lastUpdateTime : 16.67 // Default 60 FPS
    this.lastUpdateTime = now

    // Log frame time for debugging
    if (config.DEBUG_MOVEMENT && (dt > 20 || dt < 4)) { // Log unusual frame times
      console.log(`[Render] Frame time: ${dt.toFixed(2)}ms (${(1000 / dt).toFixed(1)} FPS)`)
    }

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
    }
  }

  private updateCamera(): void {
    // Update camera controller (handles follow logic)
    const camState = cameraController.update()

    // Convert world coordinates to screen coordinates for camera positioning
    const screenPos = coordGame2Screen(camState.x, camState.y)

    this.mapContainer.x = -screenPos.x * camState.zoom + this.app.screen.width / 2
    this.mapContainer.y = -screenPos.y * camState.zoom + this.app.screen.height / 2
    this.mapContainer.scale.set(camState.zoom)

    this.objectsContainer.x = this.mapContainer.x
    this.objectsContainer.y = this.mapContainer.y
    this.objectsContainer.scale.set(camState.zoom)
  }

  private updateDebugOverlay(): void {
    if (!this.debugOverlay.isVisible()) return

    const timeSyncMetrics = timeSync.getDebugMetrics()
    const moveMetrics = moveController.getGlobalDebugMetrics()
    const camState = cameraController.getState()

    const info: DebugInfo = {
      fps: this.app.ticker.FPS,
      cameraX: camState.x,
      cameraY: camState.y,
      zoom: camState.zoom,
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
    const camState = cameraController.getState()
    const cameraScreenPos = coordGame2Screen(camState.x, camState.y)

    const relativeScreenX = (screenX - this.app.screen.width / 2) / camState.zoom + cameraScreenPos.x
    const relativeScreenY = (screenY - this.app.screen.height / 2) / camState.zoom + cameraScreenPos.y

    return coordScreen2Game(relativeScreenX, relativeScreenY)
  }

  worldToScreen(worldX: number, worldY: number): ScreenPoint {
    const camState = cameraController.getState()
    const screenPos = coordGame2Screen(worldX, worldY)
    const cameraScreenPos = coordGame2Screen(camState.x, camState.y)

    const screenX = (screenPos.x - cameraScreenPos.x) * camState.zoom + this.app.screen.width / 2
    const screenY = (screenPos.y - cameraScreenPos.y) * camState.zoom + this.app.screen.height / 2
    return { x: screenX, y: screenY }
  }

  setCamera(x: number, y: number): void {
    cameraController.setPosition(x, y)
  }

  setZoom(zoom: number): void {
    cameraController.setZoom(zoom)
  }

  getZoom(): number {
    return cameraController.getZoom()
  }

  getCameraPosition(): ScreenPoint {
    return cameraController.getPosition()
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
    cameraController.setTargetEntity(entityId)
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

    const camState = cameraController.getState()
    const info: DebugInfo = {
      fps: this.app.ticker.FPS,
      cameraX: camState.x,
      cameraY: camState.y,
      zoom: camState.zoom,
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

    this.inputController.destroy()

    if (this.keyDownHandler) {
      window.removeEventListener('keydown', this.keyDownHandler)
    }

    this.chunkManager.destroy()
    this.objectManager.destroy()
    this.debugOverlay.destroy()
    cameraController.reset()
    this.app.destroy(true, { children: true, texture: true })

    this.canvas = null
    this.keyDownHandler = null
    this.onClickCallback = null
  }
}
