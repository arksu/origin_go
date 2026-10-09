import { Container, Graphics } from 'pixi.js'
import { directionSectorPoints, type DirectionSector } from './hud/directionAim'
import { coordGame2Screen } from './utils/coordConvert'
import type { ScreenPoint } from './types'

export const COMBAT_SECTOR_DURATION_MS = 1000

/** One confirmed attack sector in the same projected space as object containers. */
export class CombatSectorPreview {
  private readonly shape = new Graphics()
  private readonly points: number[] = []
  private expiresAt = 0
  private zoom = 1

  constructor(parent: Container) {
    this.shape.eventMode = 'none'
    this.shape.zIndex = 1900000000
    this.shape.visible = false
    parent.addChild(this.shape)
  }

  show(origin: Readonly<ScreenPoint>, direction: number, sector: DirectionSector, zoom: number, now: number): void {
    if (!Number.isFinite(direction) || !Number.isFinite(sector.range) || sector.range <= 0 ||
        !Number.isFinite(sector.angle) || sector.angle <= 0 || sector.angle > 2 * Math.PI ||
        !Number.isFinite(origin.x) || !Number.isFinite(origin.y) ||
        !Number.isFinite(zoom) || zoom <= 0 || !Number.isFinite(now)) {
      this.clear()
      return
    }
    this.points.length = 0
    // Project around a local origin once. Movement changes only the container position.
    for (const point of directionSectorPoints({ x: 0, y: 0 }, direction, sector)) {
      const projected = coordGame2Screen(point.x, point.y)
      this.points.push(projected.x, projected.y)
    }
    this.expiresAt = now + COMBAT_SECTOR_DURATION_MS
    this.shape.position.set(origin.x, origin.y)
    this.shape.visible = true
    this.draw(zoom)
  }

  update(origin: Readonly<ScreenPoint> | null, zoom: number, now: number): void {
    if (!this.shape.visible) return
    if (!origin || now >= this.expiresAt) {
      this.clear()
      return
    }
    this.shape.position.set(origin.x, origin.y)
    if (zoom !== this.zoom) this.draw(zoom)
  }

  clear(): void {
    this.shape.visible = false
    this.expiresAt = 0
  }

  destroy(): void {
    this.shape.destroy()
    this.points.length = 0
  }

  private draw(zoom: number): void {
    this.zoom = zoom
    this.shape.clear().poly(this.points)
      .fill({ color: 0x74e3ab, alpha: 0.16 })
      .stroke({ color: 0x74e3ab, alpha: 0.9, width: 1.5 / zoom })
  }
}
