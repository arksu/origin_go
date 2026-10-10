import { shallowRef } from 'vue'
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

/** Reactive calendar snapshot from the last accepted server runtime sample. */
export class GameCalendarSync {
  private constants: ServerConstantsSnapshot | null = null
  private lastSample: { runtime: bigint; wallMs: number } | null = null
  private readonly snapshot = shallowRef<GameCalendar | null>(null)

  configure(constants: ServerConstantsSnapshot): void {
    this.constants = constants
    this.updateSnapshot()
  }

  acceptSample(runtimeValue: unknown, wallValue: unknown): boolean {
    const runtime = parseRuntime(runtimeValue)
    const wallMs = parseWallTimestamp(wallValue)
    if (runtime === null || wallMs === null) return false
    if (this.lastSample && (runtime < this.lastSample.runtime ||
      (runtime === this.lastSample.runtime && wallMs === this.lastSample.wallMs))) return false
    this.lastSample = { runtime, wallMs }
    this.updateSnapshot()
    return true
  }

  getCalendar(): GameCalendar | null { return this.snapshot.value }
  getDayPhase(): number | null { return this.snapshot.value?.dayPhase ?? null }

  reset(): void {
    this.constants = null
    this.lastSample = null
    this.snapshot.value = null
  }

  private updateSnapshot(): void {
    if (this.constants && this.lastSample) {
      this.snapshot.value = Object.freeze(calendarFromRuntimeMilliseconds(
        this.lastSample.runtime * MILLISECONDS_PER_SECOND, this.constants,
      ))
    }
  }
}

export const gameCalendarSync = new GameCalendarSync()
