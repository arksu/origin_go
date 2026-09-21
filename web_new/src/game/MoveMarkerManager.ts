import { Container, Sprite, Texture } from 'pixi.js'
import { TERRAIN_BASE_Z_INDEX } from '@/constants/terrain'
import {
  MOVE_MARKER_BLINK_INTERVAL_MS,
  MOVE_MARKER_MAX_ALPHA,
  MOVE_MARKER_MIN_ALPHA,
} from '@/constants/moveMarker'
import { coordGame2Screen } from './utils/coordConvert'

function nowMs(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

/**
 * Renders the move-target marker for the local player. Entirely server-driven:
 * the network handler calls show() with the target coordinates from
 * S2C_ObjectMove.movement.targetPosition and hide() when a movement packet
 * carries no target (arrival, stop, teleport). The marker never decides
 * anything on its own.
 *
 * The sprite lives in objectsContainer, so camera panning and zoom follow for
 * free; only show() repositions it, while update() (called each frame from
 * Render) only animates transparency. World coordinates are raw server units,
 * the same convention ObjectView positions use for coordGame2Screen.
 */
export class MoveMarkerManager {
  private readonly parent: Container
  private readonly texture: Texture
  private sprite: Sprite | null = null
  private shownAtMs = 0
  private lastWorldX = 0
  private lastWorldY = 0

  constructor(parent: Container, texture: Texture) {
    this.parent = parent
    this.texture = texture
  }

  show(worldX: number, worldY: number): void {
    // While the player walks, movement packets re-show the same target at
    // server tick rate. Only a genuinely new target (or the first show) may
    // restart the blink phase, otherwise it would never advance.
    const isNewTarget = this.sprite === null
      || this.lastWorldX !== worldX
      || this.lastWorldY !== worldY

    if (!this.sprite) {
      this.sprite = new Sprite(this.texture)
      // Target reticle: centered on the point it marks.
      this.sprite.anchor.set(0.5, 0.5)
      this.sprite.eventMode = 'none'
      this.parent.addChild(this.sprite)
    }

    const screenPos = coordGame2Screen(worldX, worldY)
    this.sprite.position.set(screenPos.x, screenPos.y)
    // Same depth formula as world objects (see ObjectManager.sortByDepth), so
    // entities standing south of the target draw over the ground marker.
    this.sprite.zIndex = TERRAIN_BASE_Z_INDEX + screenPos.y

    if (isNewTarget) {
      this.sprite.alpha = MOVE_MARKER_MAX_ALPHA
      this.shownAtMs = nowMs()
    }

    this.lastWorldX = worldX
    this.lastWorldY = worldY
  }

  update(): void {
    if (!this.sprite) {
      return
    }
    // Smooth cosine pulse between MIN_ALPHA and MAX_ALPHA, one interval per
    // direction. The phase is anchored to the moment the target changed, so
    // every new target starts at max alpha.
    const pulse = 0.5 + 0.5 * Math.cos((Math.PI * (nowMs() - this.shownAtMs)) / MOVE_MARKER_BLINK_INTERVAL_MS)
    this.sprite.alpha = MOVE_MARKER_MIN_ALPHA + (MOVE_MARKER_MAX_ALPHA - MOVE_MARKER_MIN_ALPHA) * pulse
  }

  hide(): void {
    if (!this.sprite) {
      return
    }
    this.parent.removeChild(this.sprite)
    this.sprite.destroy()
    this.sprite = null
  }

  clear(): void {
    this.hide()
  }

  destroy(): void {
    this.hide()
  }
}
