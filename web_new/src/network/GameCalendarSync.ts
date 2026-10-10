import { timeSync } from './TimeSync'
import type { ServerConstantsSnapshot } from './ServerConstants'

const MAX_RUNTIME = (1n << 63n) - 1n
const MILLISECONDS_PER_SECOND = 1000n
const SECONDS_PER_MINUTE = 60n
const SECONDS_PER_HOUR = 3600n

export interface GameCalendar {
  readonly year: bigint
  readonly month: number
  readonly day: number
  readonly hour: number
  readonly minute: number
  readonly second: number
  readonly dayIndex: bigint
  readonly monthIndex: bigint
  readonly yearIndex: bigint
  readonly dayPhase: number
}

function parseRuntime(value: unknown): bigint | null {
  try {
    let decimal: string
    if (typeof value === 'number') {
      if (!Number.isSafeInteger(value)) return null
      decimal = String(value)
    } else if (typeof value === 'bigint' || typeof value === 'string') {
      decimal = String(value)
    } else if (value && typeof value === 'object' && 'toString' in value) {
      decimal = String(value)
    } else return null
    if (!/^(0|[1-9]\d*)$/.test(decimal)) return null
    const runtime = BigInt(decimal)
    return runtime <= MAX_RUNTIME ? runtime : null
  } catch { return null }
}

function parseWallTimestamp(value: unknown): number | null {
  // Existing wire timestamps are milliseconds; unlike runtime they fit exactly in JS.
  const runtime = parseRuntime(value)
  if (runtime === null || runtime > BigInt(Number.MAX_SAFE_INTEGER)) return null
  return Number(runtime)
}

/** Pure exact conversion, using a validated received server profile. */
export function gameCalendarFromRuntimeSeconds(runtimeValue: unknown, constants: ServerConstantsSnapshot): GameCalendar | null {
  const runtime = parseRuntime(runtimeValue)
  return runtime === null ? null : calendarFromRuntimeMilliseconds(runtime * MILLISECONDS_PER_SECOND, constants)
}

function calendarFromRuntimeMilliseconds(runtimeMs: bigint, constants: ServerConstantsSnapshot): GameCalendar {
  const dayMs = BigInt(constants.realSecondsPerGameDay) * MILLISECONDS_PER_SECOND
  const dayIndex = runtimeMs / dayMs
  const withinDayMs = runtimeMs % dayMs
  const monthIndex = dayIndex / BigInt(constants.daysPerMonth)
  const yearIndex = monthIndex / BigInt(constants.monthsPerYear)
  const gameSecondOfDay = withinDayMs * BigInt(constants.hoursPerDay) * SECONDS_PER_HOUR / dayMs
  return {
    year: yearIndex + 1n,
    month: Number(monthIndex % BigInt(constants.monthsPerYear)) + 1,
    day: Number(dayIndex % BigInt(constants.daysPerMonth)) + 1,
    hour: Number(gameSecondOfDay / SECONDS_PER_HOUR),
    minute: Number(gameSecondOfDay / SECONDS_PER_MINUTE % SECONDS_PER_MINUTE),
    second: Number(gameSecondOfDay % SECONDS_PER_MINUTE),
    dayIndex, monthIndex, yearIndex,
    dayPhase: Number(withinDayMs) / Number(dayMs),
  }
}

/** On-demand estimate. The server runtime remains the gameplay authority. */
export class GameCalendarSync {
  private constants: ServerConstantsSnapshot | null = null
  private anchor: { runtime: bigint; wallMs: number; correctedRuntimeMs: bigint; monotonicMs: number } | null = null

  constructor(
    private readonly monotonicNow: () => number = () => performance.now(),
    private readonly estimateServerNowMs: () => number = () => timeSync.estimateServerNowMs(),
  ) {}

  configure(constants: ServerConstantsSnapshot): void { this.constants = constants }

  acceptSample(runtimeValue: unknown, wallValue: unknown): boolean {
    const runtime = parseRuntime(runtimeValue)
    const wallMs = parseWallTimestamp(wallValue)
    if (runtime === null || wallMs === null) return false
    if (this.anchor && (runtime < this.anchor.runtime ||
      (runtime === this.anchor.runtime && wallMs === this.anchor.wallMs))) return false
    const monotonicMs = this.monotonicNow()
    const estimatedServerMs = this.estimateServerNowMs()
    if (!Number.isFinite(monotonicMs) || !Number.isFinite(estimatedServerMs)) return false
    const deliveryAge = Math.max(0, estimatedServerMs - wallMs)
    if (!Number.isSafeInteger(Math.floor(deliveryAge))) return false
    this.anchor = { runtime, wallMs, correctedRuntimeMs: runtime * MILLISECONDS_PER_SECOND + BigInt(Math.floor(deliveryAge)), monotonicMs }
    return true
  }

  getCalendar(): GameCalendar | null {
    if (!this.constants || !this.anchor) return null
    const elapsed = Math.max(0, this.monotonicNow() - this.anchor.monotonicMs)
    if (!Number.isFinite(elapsed) || !Number.isSafeInteger(Math.floor(elapsed))) return null
    const runtimeMs = this.anchor.correctedRuntimeMs + BigInt(Math.floor(elapsed))
    return calendarFromRuntimeMilliseconds(runtimeMs, this.constants)
  }

  getDayPhase(): number | null { return this.getCalendar()?.dayPhase ?? null }
  reset(): void { this.constants = null; this.anchor = null }
}

export const gameCalendarSync = new GameCalendarSync()
