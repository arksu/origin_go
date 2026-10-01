import { DIRECTION_INPUT_TTL_MS, DIRECTION_REFRESH_MS } from '@/constants/movement'
import { screenMovementDirection } from './utils/movementDirection'
import type { Coord } from './utils/Coord'

export type DirectionSender = (x: number, y: number, revision: number, epoch: number) => void

export class KeyboardMovementController {
  private epoch = 0
  private enabled = false
  private revision = 0
  private direction: Coord | null = null
  private timer: ReturnType<typeof setTimeout> | null = null
  private lastSentAt = 0

  constructor(
    private readonly send: DirectionSender,
    private readonly canSend: () => boolean,
    private readonly suppressKeys: () => void,
    private readonly now: () => number = () => performance.now(),
  ) {}

  configure(epoch: number, supported: boolean, enabled: boolean): boolean {
    if (epoch !== this.epoch) {
      this.reset()
      this.epoch = epoch
    }
    const nextEnabled = enabled && supported && epoch > 0
    if (!nextEnabled) this.release()
    this.enabled = nextEnabled
    return nextEnabled
  }

  setDirection(screenX: number, screenY: number): void {
    if (!this.enabled || !this.canSend()) {
      this.release(false)
      return
    }
    const direction = screenMovementDirection(screenX, screenY)
    if (direction.x === 0 && direction.y === 0) {
      this.release()
      return
    }
    if (direction.x === this.direction?.x && direction.y === this.direction?.y) return
    this.direction = direction
    this.advanceRevision()
    this.sendCurrent()
    this.scheduleRefresh()
  }

  release(sendStop = true): void {
    this.clearTimer()
    if (!this.direction) return
    this.direction = null
    this.advanceRevision()
    if (sendStop && this.canSend()) this.send(0, 0, this.revision, this.epoch)
  }

  reset(): void {
    this.release(false)
    this.enabled = false
    this.epoch = 0
    this.revision = 0
    this.suppressKeys()
  }

  destroy(): void {
    this.release()
    this.reset()
  }

  private advanceRevision(): void {
    this.revision = (this.revision + 1) >>> 0
    if (this.revision === 0) this.revision = 1
  }

  private sendCurrent(): void {
    if (!this.direction) return
    this.send(this.direction.x, this.direction.y, this.revision, this.epoch)
    this.lastSentAt = this.now()
  }

  private scheduleRefresh(): void {
    this.clearTimer()
    this.timer = setTimeout(() => {
      this.timer = null
      if (!this.enabled || !this.canSend() || this.now() - this.lastSentAt >= DIRECTION_INPUT_TTL_MS) {
        this.release()
        this.suppressKeys()
        return
      }
      this.sendCurrent()
      this.scheduleRefresh()
    }, DIRECTION_REFRESH_MS)
  }

  private clearTimer(): void {
    if (this.timer !== null) clearTimeout(this.timer)
    this.timer = null
  }
}
