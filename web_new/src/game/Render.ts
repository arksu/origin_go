import { Application, Container, TextureStyle } from 'pixi.js'
import { DebugOverlay, setObjectManager } from './DebugOverlay'
import { ChunkManager } from './ChunkManager'
import { ObjectManager } from './ObjectManager'
import { moveController } from './MoveController'
import { InputController } from './InputController'
import { cameraController } from './CameraController'
import { playerCommandController } from './PlayerCommandController'
import { coordGame2Screen, coordScreen2Game } from './utils/coordConvert'
import { timeSync } from '@/network/TimeSync'
import { useGameStore } from '@/stores/gameStore'
import { config } from '@/config'
import { MAX_FPS } from '@/constants/render'
import { cullingController } from './culling'
import { cacheMetrics } from './cache'
import { terrainManager } from './terrain'
import type { DebugInfo, ScreenPoint } from './types'
import { clearAlphaMaskCache } from './PixelHitTest'

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
  private lastPointerScreen: ScreenPoint | null = null
  private lastHoverCheckScreen: ScreenPoint | null = null
  private lastHoverCamX = Number.NaN
  private lastHoverCamY = Number.NaN
  private lastHoverZoom = Number.NaN

  constructor() {
    this.app = new Application()
    this.mapContainer = new Container()
    this.objectsContainer = new Container()
    this.uiContainer = new Container()
    this.debugOverlay = new DebugOverlay()
    this.chunkManager = new ChunkManager()
    this.objectManager = new ObjectManager()
    this.objectManager.setParentContainer(this.objectsContainer)
    this.inputController = new InputController()
  }

  async init(canvas: HTMLCanvasElement): Promise<void> {
    this.canvas = canvas
    const resolution = Math.min(window.devicePixelRatio, 2)
    TextureStyle.defaultOptions.scaleMode = 'nearest'

    await this.app.init({
      canvas,
      resolution,
      autoDensity: true,
      resizeTo: window,
      background: '#353e67ff',
      antialias: false,
    })

    // Limit maximum FPS to reduce system load
    this.app.ticker.maxFPS = MAX_FPS

    this.mapContainer.sortableChildren = true
    this.objectsContainer.sortableChildren = true
    this.uiContainer.sortableChildren = true

    this.chunkManager.setObjectsContainer(this.objectsContainer)
    await this.chunkManager.init()
    this.mapContainer.addChild(this.chunkManager.getContainer())
    this.app.stage.addChild(this.mapContainer)
    this.app.stage.addChild(this.objectsContainer)
    this.app.stage.addChild(this.uiContainer)
    this.uiContainer.addChild(this.debugOverlay.getContainer())

    // Set ObjectManager reference for DebugOverlay to control bounds
    setObjectManager(this.objectManager)
    // Set debug overlay visibility (this will also set bounds visibility)
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

      const gameStore = useGameStore()

      if (event.button === 2) {
        // RMB is the single entry point for context interactions.
        const clickedEntity = this.objectManager.getEntityAtScreen(
          event.screenX,
          event.screenY,
          this.screenToWorld.bind(this)
        )
        if (clickedEntity !== null) {
          gameStore.closeContextMenu()
          playerCommandController.sendInteract(clickedEntity.entityId)
        }
        return
      }

      if (event.button === 0) {
        gameStore.closeContextMenu()
        const hand = gameStore.handState
        const handInv = gameStore.handInventoryState

        if (hand?.item && handInv?.ref && handInv.revision != null) {
          playerCommandController.sendDropToWorld(
            handInv.ref,
            Number(handInv.revision),
            hand.item.itemId!,
            gameStore.allocOpId(),
          )
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

    this.inputController.onPointerMove((screenX, screenY) => {
      this.lastPointerScreen = { x: screenX, y: screenY }
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

  private update(): void {
    this.updateMovement()
    this.updateCamera()
    this.updateChunkBuilds()
    this.updateCulling()
    this.objectManager.update()
    this.updateHoverHighlight()

    this.updateDebugOverlay()
  }

  private updateHoverHighlight(): void {
    if (!this.lastPointerScreen) {
      this.objectManager.clearHover()
      return
    }

    if (
      this.lastPointerScreen.x < 0 ||
      this.lastPointerScreen.y < 0 ||
      this.lastPointerScreen.x > this.app.screen.width ||
      this.lastPointerScreen.y > this.app.screen.height
    ) {
      this.objectManager.clearHover()
      this.lastHoverCheckScreen = null
      return
    }

    const camState = cameraController.getState()
    const camChanged = (
      this.lastHoverCamX !== camState.x ||
      this.lastHoverCamY !== camState.y ||
      this.lastHoverZoom !== camState.zoom
    )
    const pointerChanged = (
      !this.lastHoverCheckScreen ||
      this.lastHoverCheckScreen.x !== this.lastPointerScreen.x ||
      this.lastHoverCheckScreen.y !== this.lastPointerScreen.y
    )

    if (!camChanged && !pointerChanged) {
      return
    }

    this.objectManager.updateHover(
      this.lastPointerScreen.x,
      this.lastPointerScreen.y,
      this.screenToWorld.bind(this)
    )

    this.lastHoverCheckScreen = { x: this.lastPointerScreen.x, y: this.lastPointerScreen.y }
    this.lastHoverCamX = camState.x
    this.lastHoverCamY = camState.y
    this.lastHoverZoom = camState.zoom
  }

  private updateChunkBuilds(): void {
    // Update camera position for chunk priority calculation
    const camState = cameraController.getState()
    this.chunkManager.setCameraPosition(camState.x, camState.y)

    // Update terrain manager camera position for visibility radius
    terrainManager.setCameraPosition(camState.x, camState.y)

    // Process pending chunk builds within frame budget
    this.chunkManager.update()

    // Process pending terrain subchunk builds within frame budget
    terrainManager.update()
  }

  private updateMovement(): void {
    // Get interpolated positions from MoveController
    const positions = moveController.update()

    // Update visual positions and movement state for all tracked entities
    for (const [entityId, renderPos] of positions) {
      this.objectManager.updateObjectPosition(
        entityId, renderPos.x, renderPos.y,
        renderPos.isMoving, renderPos.direction,
      )
    }
  }

  private updateCamera(): void {
    // Update camera controller (handles follow logic)
    const camState = cameraController.update()

    // Convert world coordinates to screen coordinates for camera positioning
    const screenPos = coordGame2Screen(camState.x, camState.y)

    const rawX = -screenPos.x * camState.zoom + this.app.screen.width / 2
    const rawY = -screenPos.y * camState.zoom + this.app.screen.height / 2

    // Round to whole pixels to prevent subpixel drift:
    // fractional container offsets cause each child sprite to round
    // to different pixels at different camera positions, creating
    // visible per-object "swimming".
    this.mapContainer.x = Math.round(rawX)
    this.mapContainer.y = Math.round(rawY)
    this.mapContainer.scale.set(camState.zoom)

    this.objectsContainer.x = this.mapContainer.x
    this.objectsContainer.y = this.mapContainer.y
    this.objectsContainer.scale.set(camState.zoom)
  }

  private updateCulling(): void {
    cullingController.update(this.app, this.mapContainer, this.objectsContainer)
  }

  private updateDebugOverlay(): void {
    if (!this.debugOverlay.isVisible()) return

    const timeSyncMetrics = timeSync.getDebugMetrics()
    const moveMetrics = moveController.getGlobalDebugMetrics()
    const camState = cameraController.getState()

    const cullingMetrics = cullingController.getMetrics()
    const cacheMetricsData = cacheMetrics.getMetrics()
    const terrainMetricsData = terrainManager.getMetrics()

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
      // Culling metrics
      subchunksTotal: cullingMetrics.subchunksTotal,
      subchunksVisible: cullingMetrics.subchunksVisible,
      subchunksCulled: cullingMetrics.subchunksCulled,
      terrainTotal: terrainMetricsData.spritesActive,
      terrainVisible: terrainMetricsData.spritesActive,
      terrainCulled: 0,
      objectsVisibleCulling: cullingMetrics.objectsVisible,
      objectsCulled: cullingMetrics.objectsCulled,
      cullingTimeMs: cullingMetrics.cullingTimeMs,
      // Cache metrics
      cacheEntries: cacheMetricsData.entries,
      cacheHitRate: cacheMetricsData.hitRate,
      cacheBytesKb: cacheMetricsData.bytesTotal / 1024,
      buildQueueLength: cacheMetricsData.buildQueueLength,
      buildAvgMs: cacheMetricsData.cpuBuildMsAvg,
      // Terrain metrics
      terrainSpritesActive: terrainMetricsData.spritesActive,
      terrainSpritesPooled: terrainMetricsData.spritesPooled,
      terrainSubchunksQueued: terrainMetricsData.subchunksQueued,
      terrainBuildMsAvg: terrainMetricsData.buildMsAvg,
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

  loadChunk(x: number, y: number, tiles: Uint8Array, version: number = 0): void {
    this.chunkManager.loadChunk(x, y, tiles, version)
  }

  unloadChunk(x: number, y: number): void {
    this.chunkManager.unloadChunk(x, y)
  }

  spawnObject(options: { entityId: number; typeId: number; resourcePath: string; position: { x: number; y: number }; size: { x: number; y: number } }): void {
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

  toggleDebugOverlay(): void {
    this.debugOverlay.toggle()
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

  /**
   * Reset world state without tearing down PIXI app.
   * This keeps canvas/input alive between reconnect attempts.
   */
  resetWorld(): void {
    this.objectManager.clear()
    this.chunkManager.clear()
    terrainManager.resetWorld()
    cullingController.clear()
    cameraController.setTargetEntity(null)
    this.lastHoverCheckScreen = null
    this.lastHoverCamX = Number.NaN
    this.lastHoverCamY = Number.NaN
    this.lastHoverZoom = Number.NaN
    clearAlphaMaskCache()
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
    clearAlphaMaskCache()

    this.canvas = null
    this.keyDownHandler = null
    this.onClickCallback = null
    this.lastPointerScreen = null
    this.lastHoverCheckScreen = null
    this.lastHoverCamX = Number.NaN
    this.lastHoverCamY = Number.NaN
    this.lastHoverZoom = Number.NaN
  }
}
