import { Render } from './Render'
import { playerCommandController } from './PlayerCommandController'
import type { DebugInfo, ScreenPoint } from './types'
import type { ArmBuildGhostOptions } from './BuildGhostController'

export class GameFacade {
  private render: Render | null = null
  private initialized: boolean = false

  async init(canvas: HTMLCanvasElement): Promise<void> {
    if (this.initialized) {
      this.destroy()
    }

    this.render = new Render()
    await this.render.init(canvas)
    this.initialized = true
  }

  destroy(): void {
    if (this.render) {
      this.render.destroy()
      this.render = null
    }
    this.initialized = false
  }

  isInitialized(): boolean {
    return this.initialized
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

  updateDebugStats(objectsCount: number, chunksLoaded: number): void {
    this.render?.updateDebugStats(objectsCount, chunksLoaded)
  }

  resetWorld(): void {
    this.render?.resetWorld()
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

  loadChunk(x: number, y: number, tiles: Uint8Array, version: number = 0): void {
    this.render?.loadChunk(x, y, tiles, version)
  }

  unloadChunk(x: number, y: number): void {
    this.render?.unloadChunk(x, y)
  }

  spawnObject(options: { entityId: number; typeId: number; resourcePath: string; position: { x: number; y: number }; size: { x: number; y: number } }): void {
    this.render?.spawnObject(options)
  }

  despawnObject(entityId: number): void {
    this.render?.despawnObject(entityId)
  }

  updateObjectPosition(entityId: number, x: number, y: number): void {
    this.render?.updateObjectPosition(entityId, x, y)
  }

  playFx(entityId: number, fxKey: string): void {
    this.render?.playFx(entityId, fxKey)
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
