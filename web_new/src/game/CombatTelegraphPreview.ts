import { Container, Graphics } from 'pixi.js'
import { combatTelegraphPhase, type CombatTelegraphState } from '../types/combatTelegraph'
import { projectSectorPoints } from './hud/projectedSector'

/** Server-confirmed warning for any object, independent of character animation assets. */
export class CombatTelegraphPreview {
  private readonly shape = new Graphics()
  private readonly points: number[] = []
  private zoom = Number.NaN

  constructor(parent: Container, private readonly state: CombatTelegraphState) {
    this.shape.eventMode = 'none'
    this.shape.zIndex = 1900000000
    this.shape.visible = false
    projectSectorPoints(this.points, state.facingAngle, state)
    parent.addChild(this.shape)
  }

  update(origin: Container | undefined, zoom: number, serverNowMs: number): boolean {
    const phase = combatTelegraphPhase(this.state, serverNowMs)
    if (!origin || origin.destroyed || phase >= 1) return false
    this.shape.position.copyFrom(origin.position)
    this.shape.visible = origin.visible
    this.shape.alpha = 1 - 0.65 * phase
    if (zoom !== this.zoom) {
      this.zoom = zoom
      this.shape.clear().poly(this.points)
        .fill({ color: 0xff873e, alpha: 0.3 })
        .stroke({ color: 0xff4c3e, alpha: 0.95, width: 1.5 / zoom })
    }
    return true
  }

  destroy(): void {
    this.shape.destroy()
    this.points.length = 0
  }
}
