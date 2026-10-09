import { Application, Container, TextureStyle, WebGLRenderer } from 'pixi.js'
import { DebugOverlay, setObjectManager } from './DebugOverlay'
import { ChunkManager } from './ChunkManager'
import { ObjectManager } from './ObjectManager'
import { moveController } from './MoveController'
import { InputController, Modifiers } from './InputController'
import { cameraController } from './CameraController'
import { playerCommandController } from './PlayerCommandController'
import { KeyboardMovementController } from './KeyboardMovementController'
import { gameConnection } from '@/network/GameConnection'
import { coordGame2Screen, coordScreen2Game } from './utils/coordConvert'
import { BuildGhostController, type ArmBuildGhostOptions } from './BuildGhostController'
import { LiftGhostController, type ArmLiftGhostOptions } from './LiftGhostController'
import { CombatSectorPreview } from './CombatSectorPreview'
import type { DirectionSector } from './hud/directionAim'
import { ChatBalloonManager } from './ChatBalloonManager'
import { NicknameManager } from './NicknameManager'
import { DamageNumberManager } from './DamageNumberManager'
import type { DamageNumberHit } from './hud/damageNumbers'
import { MoveMarkerManager } from './MoveMarkerManager'
import { timeSync } from '@/network/TimeSync'
import { useGameStore } from '@/stores/gameStore'
import { proto } from '@/network/proto/packets.js'
import { DROP_ITEM_TYPE_ID, MAX_FPS } from '@/constants/render'
import { cullingController } from './culling'
import { cacheMetrics } from './cache'
import { terrainManager } from './terrain'
import { fxManager } from './fx/FxManager'
import type { DebugInfo, ScreenPoint } from './types'
import { clearAlphaMaskCache } from './PixelHitTest'
import { ActorRenderer } from './actors/ActorRenderer'
import { DEFAULT_ACTOR_RENDER_SETTINGS, resolveActorRenderSettings, type ActorRenderSettings } from './actors/config'
import type { EquippedVisual } from '../types/characterVisual'
import type { LyingPresentationMode } from './actors/lyingPresentation'
import type { CharacterActionAnimationState } from '../types/actionAnimation'
import type { ObjectViewOptions } from './ObjectView'
import type { ChunkEventIdentity } from '../network/ChunkStreamGuard'
import { MinimapRenderer } from './minimap/MinimapRenderer'
import type { MinimapPose } from './minimap/types'
import { getChunkSize, getCoordPerTile } from './tiles/Tile'

const CARRIED_OBJECT_OFFSET_PX = 56

export class Render {
  private app: Application
  private mapContainer: Container
  private objectsContainer: Container
  private uiContainer: Container
  private debugOverlay: DebugOverlay
  private chunkManager: ChunkManager
  private objectManager: ObjectManager
  private inputController: InputController
  private keyboardMovement: KeyboardMovementController
  private buildGhostController: BuildGhostController
  private liftGhostController: LiftGhostController
  private combatSectorPreview: CombatSectorPreview
  private chatBalloonManager: ChatBalloonManager
  private nicknameManager: NicknameManager
  private damageNumberManager: DamageNumberManager
  private moveMarkerManager: MoveMarkerManager | null = null
  private actorRenderer: ActorRenderer | null = null
  private actorRenderSettings: Readonly<ActorRenderSettings>
  private renderErrorNotice: HTMLElement | null = null
  private minimapRenderer: MinimapRenderer | null = null
  private minimapCanvas: HTMLCanvasElement | null = null
  private playerEntityId: number | null = null

  private lastClickScreen: ScreenPoint = { x: 0, y: 0 }
  private lastClickWorld: ScreenPoint = { x: 0, y: 0 }

  private onClickCallback: ((event: { screen: ScreenPoint; world: ScreenPoint; button: number }) => boolean | void) | null = null

  private canvas: HTMLCanvasElement | null = null
  private lastPointerScreen: ScreenPoint | null = null
  private lastHoverCheckScreen: ScreenPoint | null = null
  private lastHoverCamX = Number.NaN
  private lastHoverCamY = Number.NaN
  private lastHoverZoom = Number.NaN

  constructor(actorRenderSettings: Partial<ActorRenderSettings> = {}) {
    this.actorRenderSettings = resolveActorRenderSettings({ ...DEFAULT_ACTOR_RENDER_SETTINGS, ...actorRenderSettings })
    this.app = new Application()
    this.mapContainer = new Container()
    this.objectsContainer = new Container()
    this.uiContainer = new Container()
    this.debugOverlay = new DebugOverlay()
    this.chunkManager = new ChunkManager()
    this.objectManager = new ObjectManager()
    this.objectManager.setParentContainer(this.objectsContainer)
    this.inputController = new InputController()
    this.keyboardMovement = new KeyboardMovementController(
      (x, y, revision, epoch) => playerCommandController.sendMoveDirection(x, y, revision, epoch),
      () => {
        const game = useGameStore()
        return this.canvas !== null && gameConnection.getState() === 'connected' && game.worldBootstrapState === 'ready' &&
          !game.playerStats.isKnockedOut && !game.playerStats.isLying
      },
      () => this.inputController.suppressMovementKeys(),
    )
    this.buildGhostController = new BuildGhostController(this.objectsContainer)
    this.liftGhostController = new LiftGhostController(this.objectsContainer)
    this.combatSectorPreview = new CombatSectorPreview(this.objectsContainer)
    this.nicknameManager = new NicknameManager(this.objectsContainer)
    this.chatBalloonManager = new ChatBalloonManager(this.objectsContainer, this.nicknameManager)
    this.damageNumberManager = new DamageNumberManager(this.objectsContainer)
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
      preference: 'webgl',
    })

    this.actorRenderer = new ActorRenderer(this.app.renderer as WebGLRenderer, this.actorRenderSettings)
    this.damageNumberManager.installFont()
    this.objectManager.setActorRenderer(this.actorRenderer)
    await this.objectManager.initShallowWater((x, y) => this.chunkManager.getTileTypeAtWorld(x, y))

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

    this.moveMarkerManager = new MoveMarkerManager(this.objectsContainer)

    setObjectManager(this.objectManager)
    this.debugOverlay.setVisible(this.debugOverlay.isVisible())

    this.setupInputController()

    this.app.ticker.add(this.update.bind(this))
  }

  setActorRenderSettings(settings: Partial<ActorRenderSettings>): void {
    this.actorRenderSettings = resolveActorRenderSettings({ ...this.actorRenderSettings, ...settings })
    this.actorRenderer?.setSettings(this.actorRenderSettings)
  }

  getActorRenderSettings(): Readonly<ActorRenderSettings> {
    return this.actorRenderSettings
  }

  private setupInputController(): void {
    if (!this.canvas) return

    this.inputController.init(this.canvas)
    this.inputController.onDirection((x, y) => this.keyboardMovement.setDirection(x, y))

    this.inputController.onClick((event) => {
      this.lastClickScreen = { x: event.screenX, y: event.screenY }
      this.lastPointerScreen = { x: event.screenX, y: event.screenY }
      this.lastClickWorld = this.screenToWorld(event.screenX, event.screenY)

      if (event.button === 2) {
        this.handleSecondaryMapClick(event.screenX, event.screenY, event.modifiers)
        return
      }

      const gameStore = useGameStore()
      if (event.button === 0) {
        this.releaseKeyboardMovement()
        gameStore.closeContextMenu()

        // Dropped items precede build/lift callbacks so mouse and touch use the same pickup route.
        if (this.trySendDroppedItemMapClick(event.screenX, event.screenY, event.modifiers)) {
          return
        }
      }

      if (event.button === 0 && this.buildGhostController.isActive()) {
        this.updateBuildGhostAtScreen(event.screenX, event.screenY, event.modifiers)
      } else if (event.button === 0 && this.liftGhostController.isActive()) {
        this.updateLiftGhostAtScreen(event.screenX, event.screenY)
      }
      const consumed = this.onClickCallback?.({
        screen: this.lastClickScreen,
        world: this.lastClickWorld,
        button: event.button,
      }) === true
      if (consumed) {
        return
      }

      if (event.button === 0) {
        const hand = gameStore.handState
        const handInv = gameStore.handInventoryState

        const actionState = gameStore.gameActionState
        const action = gameStore.gameActions.find(entry => entry.id === actionState.actionId)
        const activeTargetAction = ['selecting', 'approaching', 'executing'].includes(actionState.phase ?? '')
          && (action?.targetKind === 'tile' || action?.targetKind === 'object')
        // Authoritative targeting stays active while its visual cursor is hidden.
        if (hand?.item && handInv?.ref && handInv.revision != null && !activeTargetAction) {
          playerCommandController.sendDropToWorld(
            handInv.ref,
            Number(handInv.revision),
            hand.item.itemId!,
            gameStore.allocOpId(),
          )
        } else {
          playerCommandController.sendMapClick(
            this.lastClickWorld.x,
            this.lastClickWorld.y,
            this.objectManager.getEntityAtScreen(event.screenX, event.screenY, this.screenToWorld.bind(this))?.entityId ?? 0,
            event.modifiers
          )
        }
      }
    })

    this.inputController.onLongPress((event) => {
      this.handleSecondaryMapClick(event.screenX, event.screenY, event.modifiers)
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

    this.inputController.onPinchMove((event) => {
      // Continuous pinch zoom: distance growth zooms in, shrink zooms out.
      cameraController.setZoom(cameraController.getZoom() * event.scaleFactor)
    })

    this.inputController.onPointerMove((screenX, screenY) => {
      this.lastPointerScreen = { x: screenX, y: screenY }
    })
  }

  private handleSecondaryMapClick(screenX: number, screenY: number, modifiers: number): void {
    this.releaseKeyboardMovement()
    this.lastClickScreen = { x: screenX, y: screenY }
    this.lastPointerScreen = { x: screenX, y: screenY }
    this.lastClickWorld = this.screenToWorld(screenX, screenY)

    const consumed = this.onClickCallback?.({
      screen: this.lastClickScreen,
      world: this.lastClickWorld,
      button: 2,
    }) === true
    if (consumed) {
      return
    }

    const gameStore = useGameStore()
    gameStore.updateMousePos(screenX, screenY)

    const clickedEntity = this.objectManager.getEntityAtScreen(
      screenX,
      screenY,
      this.screenToWorld.bind(this),
    )
    gameStore.closeContextMenu()
    playerCommandController.sendMapClick(
      this.lastClickWorld.x,
      this.lastClickWorld.y,
      clickedEntity?.entityId ?? 0,
      modifiers,
      proto.MapClickButton.MAP_CLICK_BUTTON_SECONDARY,
    )
  }

  private trySendDroppedItemMapClick(screenX: number, screenY: number, modifiers: number): boolean {
    const clickedEntity = this.objectManager.getEntityAtScreen(
      screenX,
      screenY,
      this.screenToWorld.bind(this),
    )
    if (clickedEntity?.typeId !== DROP_ITEM_TYPE_ID) {
      return false
    }

    playerCommandController.sendMapClick(this.lastClickWorld.x, this.lastClickWorld.y, clickedEntity.entityId, modifiers)
    return true
  }

  private updateCombatSector(now: number): void {
    const view = this.playerEntityId === null ? undefined : this.objectManager.getObject(this.playerEntityId)
    this.combatSectorPreview.update(view?.getContainer() ?? null, cameraController.getZoom(), now)
  }

  private update(): void {
    const now = performance.now()
    this.updateMovement()
    this.updateCamera()
    this.updateCombatSector(now)
    this.updateBuildGhost()
    this.updateLiftGhost()
    this.updateChunkBuilds()
    this.updateMinimap()
    this.objectManager.update(now, timeSync.estimateServerNowMs())
    this.updateCulling()
    this.objectManager.syncActiveCarryVisuals(CARRIED_OBJECT_OFFSET_PX)
    this.nicknameManager.update(this.objectManager)
    this.chatBalloonManager.update(this.objectManager)
    this.damageNumberManager.update(now, cameraController.getZoom())
    this.moveMarkerManager?.update()
    try {
      this.actorRenderer?.render(now)
    } catch (error) {
      console.error('[Render] Character rendering failed', error)
      this.app.stop()
      const notice = document.createElement('div')
      this.renderErrorNotice = notice
      notice.setAttribute('role', 'alert')
      notice.textContent = 'Unable to render the character. Reload the page.'
      notice.style.cssText = 'position:fixed;inset:40% 10% auto;padding:24px;background:#281f1b;color:#fff;z-index:10000;text-align:center'
      this.app.canvas.parentElement?.append(notice)
      return
    }
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

    if (!camChanged && !pointerChanged && !this.actorRenderer?.hasUpdatedPoses) {
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

  private updateBuildGhost(): void {
    if (!this.buildGhostController.isActive()) {
      return
    }

    this.buildGhostController.update(
      this.lastPointerScreen,
      this.screenToWorld.bind(this),
      (this.inputController.getModifiers() & Modifiers.SHIFT) === 0,
    )
  }

  private updateBuildGhostAtScreen(screenX: number, screenY: number, modifiers: number): void {
    if (!this.buildGhostController.isActive()) {
      return
    }

    this.buildGhostController.update(
      { x: screenX, y: screenY },
      this.screenToWorld.bind(this),
      (modifiers & Modifiers.SHIFT) === 0,
    )
  }

  private updateLiftGhost(): void {
    if (!this.liftGhostController.isActive()) {
      return
    }
    this.liftGhostController.update(
      this.lastPointerScreen,
      this.screenToWorld.bind(this),
    )
  }

  private updateLiftGhostAtScreen(screenX: number, screenY: number): void {
    if (!this.liftGhostController.isActive()) {
      return
    }
    this.liftGhostController.update(
      { x: screenX, y: screenY },
      this.screenToWorld.bind(this),
    )
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
        renderPos.isMoving, renderPos.direction, renderPos.distanceMoved, renderPos.stopProgress, renderPos.moveMode, renderPos.heading,
      )
    }
  }

  private updateMinimap(): void {
    this.minimapRenderer?.render({
      player: this.getMinimapPlayerPose(),
      coordPerTile: getCoordPerTile(),
      chunkSize: getChunkSize(),
      getChunk: (x, y) => this.chunkManager.getMinimapChunk(x, y),
    })
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

  getObjectOverheadScreenPosition(entityId: number): ScreenPoint | null {
    const objectView = this.objectManager.getObject(entityId)
    if (!objectView) return null
    const container = objectView.getContainer()
    if (!container.visible) return null
    const point = this.objectsContainer.toGlobal({ x: container.x, y: container.y + objectView.getOverheadAnchorY() })
    return { x: point.x, y: point.y }
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

  attachMinimap(canvas: HTMLCanvasElement): void {
    if (this.minimapCanvas === canvas) return
    this.minimapRenderer?.destroy()
    this.minimapRenderer = new MinimapRenderer(canvas)
    this.minimapCanvas = canvas
  }

  detachMinimap(canvas: HTMLCanvasElement): void {
    if (this.minimapCanvas !== canvas) return
    this.minimapRenderer?.destroy()
    this.minimapRenderer = null
    this.minimapCanvas = null
  }

  setMinimapZoom(zoom: number): void {
    this.minimapRenderer?.setZoom(zoom)
  }

  clearMinimap(): void {
    this.minimapRenderer?.clear()
  }

  getMinimapPlayerPose(): MinimapPose | null {
    return this.playerEntityId === null ? null : moveController.getVisualPosition(this.playerEntityId)
  }

  setPlayerEntityId(entityId: number | null): void {
    if (entityId !== this.playerEntityId) this.clearAttackSector()
    this.playerEntityId = entityId
    cameraController.setTargetEntity(entityId)
    this.objectManager.setPlayerEntityId(entityId)
  }

  async setCharacterEquipment(entityId: number, items: readonly EquippedVisual[]): Promise<void> {
    const view = this.objectManager.getObject(entityId)
    if (!view) throw new Error(`Cannot equip missing character ${entityId}`)
    await view.setActorEquipment(items)
  }

  loadChunk(x: number, y: number, tiles: Uint8Array, version: number, identity: ChunkEventIdentity): void {
    this.chunkManager.loadChunk(x, y, tiles, version, identity)
  }

  unloadChunk(x: number, y: number, identity: ChunkEventIdentity): void {
    this.chunkManager.unloadChunk(x, y, identity)
  }

  setActionAnimation(entityId: number, state: CharacterActionAnimationState | null): void {
    this.objectManager.setActionAnimation(entityId, state)
  }

  spawnObject(options: ObjectViewOptions): void {
    this.damageNumberManager.forgetSpawn(options.entityId)
    this.objectManager.spawnObject(options)
  }

  despawnObject(entityId: number): void {
    if (entityId === this.playerEntityId) {
      this.moveMarkerManager?.clear()
      this.clearAttackSector()
    }
    this.damageNumberManager.rememberDespawn(entityId, this.objectManager, performance.now())
    this.objectManager.despawnObject(entityId)
    this.nicknameManager.remove(entityId)
  }

  showDamageNumbers(hits: readonly DamageNumberHit[]): void {
    this.damageNumberManager.show(hits, this.objectManager, performance.now(), cameraController.getZoom())
  }

  clearDamageNumbers(): void {
    this.damageNumberManager.clear()
  }

  showAttackSector(angle: number, sector: DirectionSector): void {
    const view = this.playerEntityId === null ? undefined : this.objectManager.getObject(this.playerEntityId)
    if (!view) {
      this.clearAttackSector()
      return
    }
    this.combatSectorPreview.show(view.getContainer(), angle, sector, cameraController.getZoom(), performance.now())
  }

  clearAttackSector(): void {
    this.combatSectorPreview.clear()
  }

  updateObjectPosition(entityId: number, x: number, y: number): void {
    this.objectManager.updateObjectPosition(entityId, x, y)
  }

  setObjectHeading(entityId: number, heading: number): void {
    this.objectManager.getObject(entityId)?.setHeading(heading)
  }

  setObjectKnockedOutPose(entityId: number, knockedOut: boolean, mode: LyingPresentationMode = 'snapshot', nowMs = performance.now()): void {
    this.objectManager.setKnockedOutPose(entityId, knockedOut, mode, nowMs)
  }

  setObjectCarryVisualRelation(objectId: number, carrierId: number | null): void {
    this.objectManager.setCarryVisualRelation(objectId, carrierId)
  }

  clearObjectCarryVisualRelation(objectId: number): void {
    this.objectManager.clearCarryVisualRelation(objectId)
  }

  getObjectCarryVisualCarrierId(objectId: number): number | null {
    return this.objectManager.getCarryVisualCarrierId(objectId)
  }

  playFx(entityId: number, fxKey: string): void {
    const objectView = this.objectManager.getObject(entityId)
    if (!objectView) return

    const pos = objectView.getPosition()
    const screenPos = coordGame2Screen(pos.x, pos.y)

    fxManager.playFx({
      container: this.objectsContainer,
      x: screenPos.x,
      y: screenPos.y - 60, // Above the character
      durationMs: 1500,
      fxKey: fxKey,
    })
  }

  showChatBalloon(entityId: number, text: string): void {
    if (!this.objectManager.getObject(entityId)) return
    this.chatBalloonManager.show(entityId, text)
  }

  showMoveTargetMarker(worldX: number, worldY: number): void {
    this.moveMarkerManager?.show(worldX, worldY)
  }

  endMoveTargetMarker(): void {
    this.moveMarkerManager?.endTarget()
  }

  hideMoveTargetMarker(): void {
    this.moveMarkerManager?.hide()
  }

  setObjectNickname(entityId: number, name: string, nameColor: number): void {
    if (!this.objectManager.getObject(entityId)) return
    this.nicknameManager.show(entityId, name, nameColor)
  }

  onPointerClick(callback: (event: { screen: ScreenPoint; world: ScreenPoint; button: number }) => boolean | void): void {
    this.onClickCallback = callback
  }

  armBuildGhost(options: ArmBuildGhostOptions): void {
    this.buildGhostController.arm(options)
    this.updateBuildGhost()
  }

  cancelBuildGhost(): void {
    this.buildGhostController.cancel()
  }

  isBuildGhostActive(): boolean {
    return this.buildGhostController.isActive()
  }

  getBuildGhostWorldPosition(): ScreenPoint | null {
    return this.buildGhostController.getCurrentWorldPosition()
  }

  armLiftGhost(options: ArmLiftGhostOptions): void {
    this.liftGhostController.arm(options)
    this.updateLiftGhost()
  }

  cancelLiftGhost(): void {
    this.liftGhostController.cancel()
  }

  isLiftGhostActive(): boolean {
    return this.liftGhostController.isActive()
  }

  getLiftGhostWorldPosition(): ScreenPoint | null {
    return this.liftGhostController.getCurrentWorldPosition()
  }

  toggleDebugOverlay(): void {
    this.debugOverlay.toggle()
  }

  setDebugOverlayVisible(visible: boolean): void {
    this.debugOverlay.setVisible(visible)
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
    this.clearAttackSector()
    this.playerEntityId = null
    this.clearMinimap()
    this.keyboardMovement.reset()
    this.inputController.setKeyboardMovementEnabled(false)
    this.buildGhostController.cancel()
    this.liftGhostController.cancel()
    this.chatBalloonManager.clear()
    this.damageNumberManager.clear()
    this.nicknameManager.clear()
    this.moveMarkerManager?.clear()
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
    this.combatSectorPreview.destroy()
    if (this.minimapCanvas) this.detachMinimap(this.minimapCanvas)
    this.playerEntityId = null
    this.keyboardMovement.destroy()
    this.renderErrorNotice?.remove()
    this.renderErrorNotice = null
    this.app.ticker.stop()

    this.inputController.destroy()
    this.buildGhostController.destroy()
    this.liftGhostController.destroy()
    this.chatBalloonManager.destroy()
    this.damageNumberManager.destroy()
    this.nicknameManager.destroy()
    this.moveMarkerManager?.destroy()

    this.chunkManager.destroy()
    this.objectManager.destroy()
    this.actorRenderer?.destroy()
    this.actorRenderer = null
    this.debugOverlay.destroy()
    cameraController.reset()
    this.app.destroy(true, { children: true, texture: true })
    clearAlphaMaskCache()

    this.canvas = null
    this.onClickCallback = null
    this.lastPointerScreen = null
    this.lastHoverCheckScreen = null
    this.lastHoverCamX = Number.NaN
    this.lastHoverCamY = Number.NaN
    this.lastHoverZoom = Number.NaN
  }

  setKeyboardMovementEnabled(enabled: boolean): void {
    const game = useGameStore()
    const params = game.worldParams
    const accepted = this.keyboardMovement.configure(params?.streamEpoch ?? 0, params?.directionalMovementSupported === true,
      enabled && !game.playerStats.isKnockedOut && !game.playerStats.isLying)
    this.inputController.setKeyboardMovementEnabled(accepted)
  }

  releaseKeyboardMovement(): void {
    this.keyboardMovement.release()
    this.inputController.suppressMovementKeys()
  }
}
