import { Render } from './Render'
import type { ChunkEventIdentity } from '../network/ChunkStreamGuard'
import { CursorManager } from './CursorManager'
import { playerCommandController } from './PlayerCommandController'
import type { DebugInfo, ScreenPoint } from './types'
import type { ArmBuildGhostOptions } from './BuildGhostController'
import type { ArmLiftGhostOptions } from './LiftGhostController'
import type { EquippedVisual } from '../types/characterVisual'
import type { CharacterActionAnimationState } from '../types/actionAnimation'
import type { ObjectViewOptions } from './ObjectView'
import { DEFAULT_ACTOR_RENDER_SETTINGS, resolveActorRenderSettings, type ActorRenderSettings } from './actors/config'
import { config } from '@/config'
import type { MinimapPose } from './minimap/types'
import { localAudioController, worldAudioReceiver } from './audioRuntime'
import type { DamageNumberHit } from './hud/damageNumbers'

export class GameFacade {
  private render: Render | null = null
  private cursorManager = new CursorManager()
  private initialized: boolean = false
  private actorRenderSettings: Readonly<ActorRenderSettings> = DEFAULT_ACTOR_RENDER_SETTINGS
  private renderDebugEnabled = config.DEBUG

  async init(canvas: HTMLCanvasElement): Promise<void> {
    if (this.initialized) {
      this.destroy()
    }

    this.render = new Render(this.actorRenderSettings)
    this.render.setDebugOverlayVisible(this.renderDebugEnabled)
    await this.render.init(canvas)
    this.cursorManager.attach(canvas, this.render.getApp().renderer.events)
    this.initialized = true
  }

  destroy(): void {
    worldAudioReceiver.reset()
    localAudioController.reset()
    this.cursorManager.detach()
    if (this.render) {
      this.render.destroy()
      this.render = null
    }
    this.initialized = false
  }

  isInitialized(): boolean {
    return this.initialized
  }

  setActionCursor(cursorId: string): void {
    this.cursorManager.set(cursorId)
  }

  /** Can be set before init; the future Settings UI will call this method. */
  setActorRenderSettings(settings: Partial<ActorRenderSettings>): void {
    this.actorRenderSettings = resolveActorRenderSettings({ ...this.actorRenderSettings, ...settings })
    this.render?.setActorRenderSettings(this.actorRenderSettings)
  }

  getActorRenderSettings(): Readonly<ActorRenderSettings> {
    return this.actorRenderSettings
  }

  setRenderDebugEnabled(enabled: boolean): void {
    this.renderDebugEnabled = enabled
    this.render?.setDebugOverlayVisible(enabled)
  }

  onPlayerClick(callback: (event: { screenX: number; screenY: number; worldX: number; worldY: number; button: number }) => boolean | void): void {
    this.render?.onPointerClick((event) => {
      return callback({
        screenX: event.screen.x,
        screenY: event.screen.y,
        worldX: event.world.x,
        worldY: event.world.y,
        button: event.button,
      })
    })
  }

  setCamera(x: number, y: number): void {
    this.render?.setCamera(x, y)
  }

  setZoom(zoom: number): void {
    this.render?.setZoom(zoom)
  }

  getZoom(): number {
    return this.render?.getZoom() ?? 1
  }

  getCameraPosition(): ScreenPoint {
    return this.render?.getCameraPosition() ?? { x: 0, y: 0 }
  }

  screenToWorld(screenX: number, screenY: number): ScreenPoint {
    return this.render?.screenToWorld(screenX, screenY) ?? { x: 0, y: 0 }
  }

  worldToScreen(worldX: number, worldY: number): ScreenPoint {
    return this.render?.worldToScreen(worldX, worldY) ?? { x: 0, y: 0 }
  }

  getObjectOverheadScreenPosition(entityId: number): ScreenPoint | null {
    return this.render?.getObjectOverheadScreenPosition(entityId) ?? null
  }

  armBuildGhost(options: ArmBuildGhostOptions): void {
    this.render?.armBuildGhost(options)
  }

  cancelBuildGhost(): void {
    this.render?.cancelBuildGhost()
  }

  isBuildGhostActive(): boolean {
    return this.render?.isBuildGhostActive() ?? false
  }

  getBuildGhostWorldPosition(): ScreenPoint | null {
    return this.render?.getBuildGhostWorldPosition() ?? null
  }

  armLiftGhost(options: ArmLiftGhostOptions): void {
    this.render?.armLiftGhost(options)
  }

  cancelLiftGhost(): void {
    this.render?.cancelLiftGhost()
  }

  isLiftGhostActive(): boolean {
    return this.render?.isLiftGhostActive() ?? false
  }

  getLiftGhostWorldPosition(): ScreenPoint | null {
    return this.render?.getLiftGhostWorldPosition() ?? null
  }

  updateDebugStats(objectsCount: number, chunksLoaded: number): void {
    this.render?.updateDebugStats(objectsCount, chunksLoaded)
  }

  resetWorld(): void {
    worldAudioReceiver.reset()
    localAudioController.reset()
    this.render?.resetWorld()
  }

  attachMinimap(canvas: HTMLCanvasElement): void {
    this.render?.attachMinimap(canvas)
  }

  detachMinimap(canvas: HTMLCanvasElement): void {
    this.render?.detachMinimap(canvas)
  }

  setMinimapZoom(zoom: number): void {
    this.render?.setMinimapZoom(zoom)
  }

  clearMinimap(): void {
    this.render?.clearMinimap()
  }

  getMinimapPlayerPose(): MinimapPose | null {
    return this.render?.getMinimapPlayerPose() ?? null
  }

  setKeyboardMovementEnabled(enabled: boolean): void {
    this.render?.setKeyboardMovementEnabled(this.initialized && enabled)
  }

  releaseKeyboardMovement(): void {
    this.render?.releaseKeyboardMovement()
  }

  setWorldParams(coordPerTile: number, chunkSize: number): void {
    this.render?.setWorldParams(coordPerTile, chunkSize)
  }

  setPlayerEntityId(entityId: number | null): void {
    this.render?.setPlayerEntityId(entityId)
    if (entityId !== null) {
      playerCommandController.setPlayerId(entityId)
    }
  }

  async setCharacterEquipment(entityId: number, items: readonly EquippedVisual[]): Promise<void> {
    if (!this.render) throw new Error('Game renderer is not initialized')
    await this.render.setCharacterEquipment(entityId, items)
  }

  loadChunk(x: number, y: number, tiles: Uint8Array, version: number, identity: ChunkEventIdentity): void {
    this.render?.loadChunk(x, y, tiles, version, identity)
  }

  unloadChunk(x: number, y: number, identity: ChunkEventIdentity): void {
    this.render?.unloadChunk(x, y, identity)
  }

  setActionAnimation(entityId: number, state: CharacterActionAnimationState | null): void {
    this.render?.setActionAnimation(entityId, state)
  }

  spawnObject(options: ObjectViewOptions): void {
    this.render?.spawnObject(options)
  }

  despawnObject(entityId: number): void {
    this.render?.despawnObject(entityId)
  }

  showDamageNumbers(hits: readonly DamageNumberHit[]): void {
    this.render?.showDamageNumbers(hits)
  }

  clearDamageNumbers(): void {
    this.render?.clearDamageNumbers()
  }

  updateObjectPosition(entityId: number, x: number, y: number): void {
    this.render?.updateObjectPosition(entityId, x, y)
  }

  setObjectHeading(entityId: number, heading: number): void {
    this.render?.setObjectHeading(entityId, heading)
  }

  setObjectKnockedOutPose(entityId: number, knockedOut: boolean): void {
    this.render?.setObjectKnockedOutPose(entityId, knockedOut)
  }

  setObjectCarryVisualRelation(objectId: number, carrierId: number | null): void {
    this.render?.setObjectCarryVisualRelation(objectId, carrierId)
  }

  setObjectNickname(entityId: number, name: string, nameColor: number): void {
    this.render?.setObjectNickname(entityId, name, nameColor)
  }

  clearObjectCarryVisualRelation(objectId: number): void {
    this.render?.clearObjectCarryVisualRelation(objectId)
  }

  getObjectCarryVisualCarrierId(objectId: number): number | null {
    return this.render?.getObjectCarryVisualCarrierId(objectId) ?? null
  }

  playFx(entityId: number, fxKey: string): void {
    this.render?.playFx(entityId, fxKey)
  }

  showChatBalloon(entityId: number, text: string): void {
    this.render?.showChatBalloon(entityId, text)
  }

  showMoveTargetMarker(worldX: number, worldY: number): void {
    this.render?.showMoveTargetMarker(worldX, worldY)
  }

  endMoveTargetMarker(): void {
    this.render?.endMoveTargetMarker()
  }

  hideMoveTargetMarker(): void {
    this.render?.hideMoveTargetMarker()
  }

  toggleDebugOverlay(): void {
    this.render?.toggleDebugOverlay()
  }

  getDebugInfo(): DebugInfo {
    if (!this.render) {
      return {
        fps: 0,
        cameraX: 0,
        cameraY: 0,
        zoom: 1,
        viewportWidth: 0,
        viewportHeight: 0,
        lastClickScreenX: 0,
        lastClickScreenY: 0,
        lastClickWorldX: 0,
        lastClickWorldY: 0,
        objectsCount: 0,
        chunksLoaded: 0,
      }
    }

    const cam = this.render.getCameraPosition()
    return {
      fps: this.render.getApp().ticker.FPS,
      cameraX: cam.x,
      cameraY: cam.y,
      zoom: this.render.getZoom(),
      viewportWidth: Math.round(this.render.getApp().screen.width),
      viewportHeight: Math.round(this.render.getApp().screen.height),
      lastClickScreenX: 0,
      lastClickScreenY: 0,
      lastClickWorldX: 0,
      lastClickWorldY: 0,
      objectsCount: 0,
      chunksLoaded: 0,
    }
  }
}

export const gameFacade = new GameFacade()
