import { coordGame2Screen } from '../utils/coordConvert'
import { ACTOR_RENDER } from './config'

// Same indices as existing animation directions: NE, E, SE, S, SW, W, NW, N.
export function screenFacingAngle(direction: number): number {
  if (!Number.isInteger(direction) || direction < 0 || direction > 7) throw new Error(`Invalid facing direction: ${direction}`)
  return (direction - 1) * Math.PI / 4
}

export function facingFromDisplacement(dx: number, dy: number, previous: number): number {
  if (!Number.isFinite(dx) || !Number.isFinite(dy)) throw new Error('Non-finite character displacement')
  if (dx === 0 && dy === 0) return previous
  const screen = coordGame2Screen(dx, dy)
  const angle = Math.atan2(screen.y, screen.x)
  const difference = Math.atan2(Math.sin(angle - screenFacingAngle(previous)), Math.cos(angle - screenFacingAngle(previous)))
  // A small dead band prevents alternating frames at a sector boundary.
  if (Math.abs(difference) <= Math.PI / 8 + ACTOR_RENDER.facingHysteresis) return previous
  return ((Math.floor(angle / (Math.PI / 4) + .5) + 1) % 8 + 8) % 8
}

export function actorYawForFacing(direction: number): number {
  const angle = screenFacingAngle(direction)
  // Camera pitch compresses ground-plane depth by sin(elevation).
  return Math.atan2(Math.cos(angle), Math.sin(angle) / Math.sin(ACTOR_RENDER.cameraElevation))
}

/** Adjacent sectors must persist; deliberate sharp turns remain responsive. */
export class FacingStabilizer {
  private candidate = -1
  private candidateSince = 0
  private changedAt = -Infinity

  update(current: number, next: number, now: number, immediate = false): number {
    const steps = Math.min((next - current + 8) % 8, (current - next + 8) % 8)
    if (immediate || steps >= 2) {
      this.candidate = next
      this.candidateSince = now
      this.changedAt = now
      return next
    }
    if (next !== this.candidate) {
      this.candidate = next
      this.candidateSince = now
    }
    if (next === current) return current
    if (now - this.candidateSince < ACTOR_RENDER.facingConfirmMs || now - this.changedAt < ACTOR_RENDER.facingHoldMs) return current
    this.changedAt = now
    return next
  }
}
