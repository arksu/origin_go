import { Container, Graphics } from 'pixi.js'
import { directionSectorPoints, type DirectionSector } from './hud/directionAim'
import { coordGame2Screen } from './utils/coordConvert'
import type { ScreenPoint } from './types'

export class DirectionAimPreview {
  private readonly shape = new Graphics()

  constructor(parent: Container) {
    this.shape.eventMode = 'none'
    this.shape.zIndex = 1900000000
    parent.addChild(this.shape)
  }

  update(origin: ScreenPoint, direction: number, sector: DirectionSector, zoom: number): void {
    const points = directionSectorPoints(origin, direction, sector).flatMap(point => {
      const projected = coordGame2Screen(point.x, point.y)
      return [projected.x, projected.y]
    })
    this.shape.clear().poly(points)
      .fill({ color: 0x74e3ab, alpha: 0.16 })
      .stroke({ color: 0x74e3ab, alpha: 0.9, width: 1.5 / zoom })
  }

  clear(): void {
    this.shape.clear()
  }

  destroy(): void {
    this.shape.destroy()
  }
}
