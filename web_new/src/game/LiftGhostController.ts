import { Container } from 'pixi.js'
import { ResourceLoader } from './ResourceLoader'
import { BuildGhostView } from './BuildGhostView'
import type { ScreenPoint } from './types'

export interface ArmLiftGhostOptions {
  entityId: number
  resourcePath?: string | null
}

export class LiftGhostController {
  private parentContainer: Container
  private ghostView: BuildGhostView | null = null
  private carriedEntityId: number | null = null
  private activeResourcePath = ''
  private currentWorldPos: ScreenPoint | null = null

  constructor(parentContainer: Container) {
    this.parentContainer = parentContainer
  }

  arm(options: ArmLiftGhostOptions): void {
    const entityId = Math.trunc(options.entityId || 0)
    if (!Number.isFinite(entityId) || entityId <= 0) {
      this.cancel()
      return
    }

    const resourcePath = this.resolveResourcePath(options)
    const canReuse = (
      this.ghostView !== null &&
      this.carriedEntityId === entityId &&
      this.activeResourcePath === resourcePath
    )

    this.carriedEntityId = entityId
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
    this.carriedEntityId = null
    this.activeResourcePath = ''
    this.currentWorldPos = null
  }

  destroy(): void {
    this.cancel()
  }

  isActive(): boolean {
    return this.ghostView !== null && this.carriedEntityId !== null
  }

  getCarriedEntityId(): number | null {
    return this.carriedEntityId
  }

  getCurrentWorldPosition(): ScreenPoint | null {
    if (!this.currentWorldPos) return null
    return { x: this.currentWorldPos.x, y: this.currentWorldPos.y }
  }

  update(
    pointerScreen: ScreenPoint | null,
    screenToWorld: (screenX: number, screenY: number) => ScreenPoint,
  ): void {
    if (!this.ghostView || !pointerScreen) {
      return
    }
    const nextWorld = screenToWorld(pointerScreen.x, pointerScreen.y)
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

  private resolveResourcePath(options: ArmLiftGhostOptions): string {
    const preferred = (options.resourcePath || '').trim()
    if (preferred && ResourceLoader.getResourceDef(preferred)) {
      return preferred
    }
    return preferred
  }
}
