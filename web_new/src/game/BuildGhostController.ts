import { Container } from 'pixi.js'
import { getCoordPerTile } from './tiles/Tile'
import { ResourceLoader } from './ResourceLoader'
import { BuildGhostView } from './BuildGhostView'
import type { ScreenPoint } from './types'

export interface ArmBuildGhostOptions {
  buildKey: string
  objectKey?: string | null
  objectResourcePath?: string | null
}

export class BuildGhostController {
  private parentContainer: Container
  private ghostView: BuildGhostView | null = null
  private armedBuildKey = ''
  private activeResourcePath = ''
  private currentWorldPos: ScreenPoint | null = null

  constructor(parentContainer: Container) {
    this.parentContainer = parentContainer
  }

  arm(options: ArmBuildGhostOptions): void {
    const buildKey = (options.buildKey || '').trim()
    if (!buildKey) {
      this.cancel()
      return
    }

    const resourcePath = this.resolveResourcePath(options)
    const canReuse = (
      this.ghostView !== null &&
      this.armedBuildKey === buildKey &&
      this.activeResourcePath === resourcePath
    )

    this.armedBuildKey = buildKey
    this.activeResourcePath = resourcePath
    this.currentWorldPos = null

    if (canReuse) {
      return
    }

    this.replaceGhost(resourcePath)
  }

  cancel(): void {
    if (this.ghostView) {
      this.parentContainer.removeChild(this.ghostView.getContainer())
      this.ghostView.destroy()
      this.ghostView = null
    }
    this.armedBuildKey = ''
    this.activeResourcePath = ''
    this.currentWorldPos = null
  }

  destroy(): void {
    this.cancel()
  }

  isActive(): boolean {
    return this.ghostView !== null && this.armedBuildKey !== ''
  }

  getArmedBuildKey(): string {
    return this.armedBuildKey
  }

  getCurrentWorldPosition(): ScreenPoint | null {
    if (!this.currentWorldPos) return null
    return { x: this.currentWorldPos.x, y: this.currentWorldPos.y }
  }

  update(
    pointerScreen: ScreenPoint | null,
    screenToWorld: (screenX: number, screenY: number) => ScreenPoint,
    shouldSnapToTileCenter: boolean,
  ): void {
    if (!this.ghostView || !pointerScreen) {
      return
    }

    const rawWorld = screenToWorld(pointerScreen.x, pointerScreen.y)
    const nextWorld = shouldSnapToTileCenter ? this.snapToTileCenter(rawWorld) : rawWorld

    this.currentWorldPos = nextWorld
    this.ghostView.updatePosition(nextWorld.x, nextWorld.y)
  }

  private replaceGhost(resourcePath: string): void {
    if (this.ghostView) {
      this.parentContainer.removeChild(this.ghostView.getContainer())
      this.ghostView.destroy()
      this.ghostView = null
    }

    this.ghostView = new BuildGhostView(resourcePath)
    this.parentContainer.addChild(this.ghostView.getContainer())
  }

  private resolveResourcePath(options: ArmBuildGhostOptions): string {
    const preferred = (options.objectResourcePath || '').trim()
    if (preferred && ResourceLoader.getResourceDef(preferred)) {
      return preferred
    }

    const objectKey = (options.objectKey || '').trim()
    if (objectKey && ResourceLoader.getResourceDef(objectKey)) {
      return objectKey
    }

    return preferred || objectKey
  }

  private snapToTileCenter(world: ScreenPoint): ScreenPoint {
    const coordPerTile = getCoordPerTile()
    if (!Number.isFinite(coordPerTile) || coordPerTile <= 0) {
      return world
    }

    return {
      x: Math.round(world.x / coordPerTile) * coordPerTile,
      y: Math.round(world.y / coordPerTile) * coordPerTile,
    }
  }
}
