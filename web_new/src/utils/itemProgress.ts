import type { proto } from '@/network/proto/packets.js'
import { parseNonNegativeInt64 } from './protoNumbers'

export type ItemProgress =
  | { durationMs: bigint; paused: false; completionMs: bigint }
  | { durationMs: bigint; paused: true; remainingMs: bigint }

export interface ItemProgressPresentation {
  readonly paused: boolean
  readonly remainingMs: bigint
  readonly completedFraction: number
  readonly remainingText: string
}

export function parseItemProgress(progress: proto.IItemProgress | null | undefined): ItemProgress | null {
  if (!progress) return null
  const duration = parseNonNegativeInt64(progress.durationSeconds)
  if (duration === null || duration === 0n) return null
  // A zero oneof value is present and valid; missing or conflicting timing is not.
  const active = progress.completionRuntimeSeconds != null
  const paused = progress.pausedRemainingSeconds != null
  if (active === paused) return null
  if (paused) {
    const remaining = parseNonNegativeInt64(progress.pausedRemainingSeconds)
    if (remaining === null || remaining > duration) return null
    return { durationMs: duration * 1000n, paused: true, remainingMs: remaining * 1000n }
  }
  const completion = parseNonNegativeInt64(progress.completionRuntimeSeconds)
  return completion === null ? null : { durationMs: duration * 1000n, paused: false, completionMs: completion * 1000n }
}

export function formatRemainingSeconds(seconds: bigint): string {
  const parts: string[] = []
  for (const [unit, size] of [['d', 86400n], ['h', 3600n], ['m', 60n], ['s', 1n]] as const) {
    const count = seconds / size
    if (count > 0n) parts.push(`${count}${unit}`)
    seconds %= size
  }
  return parts.join(' ') || '0s'
}

export function presentItemProgress(progress: ItemProgress | null, runtimeMs: bigint | null): ItemProgressPresentation | null {
  if (!progress || (!progress.paused && runtimeMs === null)) return null
  const rawRemainingMs = progress.paused ? progress.remainingMs : progress.completionMs - runtimeMs!
  const remainingMs = rawRemainingMs < 0n ? 0n : rawRemainingMs > progress.durationMs ? progress.durationMs : rawRemainingMs
  // Convert only a bounded ratio; never convert absolute runtime/deadline int64s.
  const completedFraction = Number((progress.durationMs - remainingMs) * 1000000n / progress.durationMs) / 1000000
  return {
    paused: progress.paused, remainingMs, completedFraction,
    remainingText: formatRemainingSeconds((remainingMs + 999n) / 1000n),
  }
}
