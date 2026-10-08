import { Container, Graphics } from 'pixi.js'
import { TERRAIN_BASE_Z_INDEX } from '@/constants/terrain'
import {
  MOVE_MARKER_COLOR,
  MOVE_MARKER_DURATION_MS,
  MOVE_MARKER_MAX_ALPHA,
  MOVE_MARKER_RADIUS_X,
  MOVE_MARKER_RADIUS_Y,
  MOVE_MARKER_START_SCALE,
  MOVE_MARKER_STROKE_WIDTH,
} from '@/constants/moveMarker'
import { coordGame2Screen } from './utils/coordConvert'

function nowMs(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

/** One reusable ground ring, triggered only by accepted server movement. */
export class MoveMarkerManager {
  private readonly ring = new Graphics()
  private shownAtMs = 0
  private hasTarget = false
  private lastWorldX = 0
  private lastWorldY = 0

  constructor(parent: Container, private readonly now: () => number = nowMs) {
    this.ring.ellipse(0, 0, MOVE_MARKER_RADIUS_X, MOVE_MARKER_RADIUS_Y)
      .stroke({ color: MOVE_MARKER_COLOR, width: MOVE_MARKER_STROKE_WIDTH })
    this.ring.eventMode = 'none'
    this.ring.visible = false
    parent.addChild(this.ring)
  }

  show(worldX: number, worldY: number): void {
    // Remember the confirmed target after expiry, so tick updates cannot replay it.
    if (this.hasTarget && this.lastWorldX === worldX && this.lastWorldY === worldY) return

    const screenPos = coordGame2Screen(worldX, worldY)
    this.ring.position.set(screenPos.x, screenPos.y)
    this.ring.zIndex = TERRAIN_BASE_Z_INDEX + screenPos.y
    this.ring.scale.set(MOVE_MARKER_START_SCALE)
    this.ring.alpha = MOVE_MARKER_MAX_ALPHA
    this.ring.visible = true
    this.shownAtMs = this.now()
    this.hasTarget = true
    this.lastWorldX = worldX
    this.lastWorldY = worldY
  }

  update(): void {
    if (!this.ring.visible) return

    const progress = Math.min(1, Math.max(0, (this.now() - this.shownAtMs) / MOVE_MARKER_DURATION_MS))
    const fade = progress * progress * (3 - 2 * progress)
    this.ring.scale.set(MOVE_MARKER_START_SCALE + (1 - MOVE_MARKER_START_SCALE) * progress)
    this.ring.alpha = MOVE_MARKER_MAX_ALPHA * (1 - fade)
    if (progress === 1) this.ring.visible = false
  }

  /** End the route while allowing its existing ring to finish fading. */
  endTarget(): void {
    this.hasTarget = false
  }

  hide(): void {
    this.endTarget()
    this.ring.visible = false
    this.ring.alpha = 0
  }

  clear(): void {
    this.hide()
  }

  destroy(): void {
    this.clear()
    this.ring.destroy()
  }
}
