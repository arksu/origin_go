/** Plain presentation data passed only after AttackResult validation. */
export interface DamageNumberHit {
  targetId: string
  damage: number
}

export const CAPACITY = 512
export const PER_TARGET = 4
export const LIFETIME_MS = 900
export const CACHE_CAPACITY = 1024
export const CACHE_TTL_MS = 2000
export const FONT_SIZE_PX = 22
export const FILL_COLOR = 0xff4040
export const STROKE_COLOR = 0x171010
export const STROKE_WIDTH_PX = 3
export const POP_DURATION_MS = 120
export const FADE_START_MS = 600
export const RISE_PX = 48
export const LATERAL_DRIFT_PX = 18
export const LANE_OFFSET_PX = 32
export const SECOND_ROW_OFFSET_PX = 28

const INITIAL_SCALE = 1.2
const CANONICAL_TARGET_ID = /^[1-9]\d{0,15}$/

/** Format authoritative damage for display without changing its numeric value. */
export function formatDamageNumber(damage: number): string | null {
  if (!Number.isFinite(damage) || damage < 0) return null
  if (damage === 0) return '0'
  if (damage < 0.1) return '<0.1'
  if (damage >= 1_000_000) return damage.toExponential(1)
  return damage.toFixed(1).replace(/\.0$/, '')
}

/** Render maps use numbers; never round a protocol uint64 into another target. */
export function safeDamageTargetId(targetId: string): number | null {
  if (typeof targetId !== 'string' || targetId.length > 16 || !CANONICAL_TARGET_ID.test(targetId)) return null
  const numericId = Number(targetId)
  return Number.isSafeInteger(numericId) && String(numericId) === targetId ? numericId : null
}

export interface DamageNumberEntry {
  active: boolean
  text: string
  targetId: number
  /** Captured position in the renderer's projected world coordinate system. */
  anchorX: number
  anchorY: number
  x: number
  y: number
  scale: number
  alpha: number
  startedAt: number
  /** Insertion order resolves equal timestamps independently of the slot index. */
  order: number
  lane: number
}

/** Bounded, renderer-independent animation state; update does not allocate. */
export class DamageNumbersPresentation {
  readonly entries: DamageNumberEntry[]
  private count = 0
  private nextOrder = 0

  constructor() {
    this.entries = Array.from({ length: CAPACITY }, () => ({
      active: false,
      text: '',
      targetId: 0,
      anchorX: 0,
      anchorY: 0,
      x: 0,
      y: 0,
      scale: 1,
      alpha: 0,
      startedAt: 0,
      order: 0,
      lane: 0,
    }))
  }

  get activeCount(): number {
    return this.count
  }

  emit(targetId: number, damage: number, anchorX: number, anchorY: number, now: number): boolean {
    if (!Number.isSafeInteger(targetId) || targetId <= 0 ||
        !Number.isFinite(anchorX) || !Number.isFinite(anchorY) ||
        !Number.isFinite(now) || now < 0) return false
    const text = formatDamageNumber(damage)
    if (text === null) return false

    let free: DamageNumberEntry | undefined
    let oldest: DamageNumberEntry | undefined
    let oldestTarget: DamageNumberEntry | undefined
    let newestTarget: DamageNumberEntry | undefined
    let targetCount = 0
    for (let index = 0; index < this.entries.length; index++) {
      const entry = this.entries[index]!
      if (entry.active && now - entry.startedAt >= LIFETIME_MS) this.deactivate(entry)
      if (!entry.active) {
        if (!free) free = entry
        continue
      }
      if (!oldest || entry.order < oldest.order) oldest = entry
      if (entry.targetId !== targetId) continue
      targetCount++
      if (!oldestTarget || entry.order < oldestTarget.order) oldestTarget = entry
      if (!newestTarget || entry.order > newestTarget.order) newestTarget = entry
    }

    const entry = targetCount >= PER_TARGET ? oldestTarget! : (free ?? oldest!)
    // A second row keeps simultaneous hits 1/4 apart while preserving left/up/right.
    const lane = newestTarget ? (newestTarget.lane + 1) % 6 : 0
    if (!entry.active) this.count++
    entry.active = true
    entry.text = text
    entry.targetId = targetId
    entry.anchorX = anchorX
    entry.anchorY = anchorY
    entry.x = anchorX + (lane % 3 - 1) * LANE_OFFSET_PX
    entry.y = anchorY - (lane >= 3 ? SECOND_ROW_OFFSET_PX : 0)
    entry.scale = INITIAL_SCALE
    entry.alpha = 1
    entry.startedAt = now
    entry.order = ++this.nextOrder
    entry.lane = lane
    return true
  }

  update(now: number, zoom: number): void {
    if (!Number.isFinite(now) || now < 0 || !Number.isFinite(zoom) || zoom <= 0) return
    if (this.count === 0) return
    const inverseZoom = 1 / zoom
    for (let index = 0; index < this.entries.length; index++) {
      const entry = this.entries[index]!
      if (!entry.active) continue
      const age = Math.max(0, now - entry.startedAt)
      if (age >= LIFETIME_MS) {
        this.deactivate(entry)
        continue
      }
      const progress = age / LIFETIME_MS
      const eased = progress * (2 - progress)
      entry.x = entry.anchorX + (entry.lane % 3 - 1) * (LANE_OFFSET_PX + LATERAL_DRIFT_PX * eased) * inverseZoom
      const rowOffset = entry.lane >= 3 ? SECOND_ROW_OFFSET_PX : 0
      entry.y = entry.anchorY - (RISE_PX * eased + rowOffset) * inverseZoom
      entry.scale = (1 + (INITIAL_SCALE - 1) * (1 - Math.min(1, age / POP_DURATION_MS))) * inverseZoom
      entry.alpha = age <= FADE_START_MS ? 1 : (LIFETIME_MS - age) / (LIFETIME_MS - FADE_START_MS)
    }
  }

  clear(): void {
    for (let index = 0; index < this.entries.length; index++) {
      const entry = this.entries[index]!
      entry.active = false
      entry.text = ''
      entry.targetId = 0
      entry.alpha = 0
    }
    this.count = 0
    this.nextOrder = 0
  }

  private deactivate(entry: DamageNumberEntry): void {
    entry.active = false
    entry.text = ''
    entry.targetId = 0
    entry.alpha = 0
    this.count--
  }
}
