import { util } from 'protobufjs/minimal'
import type { proto } from '../network/proto/packets'
import { compareUint64, decodeGeneration, decodeNonzeroUint64 } from './networkIdentity'

export interface CombatTelegraphState {
  readonly generation: string
  readonly revision: string
  readonly executionGeneration: string
  readonly active: boolean
  readonly totalTicks: number
  readonly elapsedTicks: number
  readonly tickDurationMs: number
  readonly serverTimeMs: number
  readonly facingAngle: number
  readonly range: number
  readonly angle: number
}

function counter(value: unknown): string {
  if (value == null || value === 0 || value === '0' ||
      (typeof util.Long === 'function' && value instanceof util.Long && value.low === 0 && value.high === 0 && typeof value.unsigned === 'boolean')) return '0'
  const decoded = decodeNonzeroUint64(value)
  if (decoded === null) throw new Error('Invalid combat telegraph integer')
  return decoded
}

export function decodeCombatTelegraph(input: proto.ICombatTelegraphState): CombatTelegraphState {
  const generation = decodeGeneration(input.generation)
  const revision = counter(input.revision), executionGeneration = counter(input.executionGeneration)
  const serverTimeMs = Number(counter(input.serverTimeMs))
  const active = input.active ?? false
  const totalTicks = input.totalTicks ?? 0, elapsedTicks = input.elapsedTicks ?? 0, tickDurationMs = input.tickDurationMs ?? 0
  const facingAngle = input.facingAngle ?? 0, range = input.range ?? 0, angle = input.angle ?? 0
  if (typeof active !== 'boolean' || !Number.isSafeInteger(serverTimeMs) ||
      !Number.isInteger(totalTicks) || totalTicks < 0 || totalTicks > 4294967295 ||
      !Number.isInteger(elapsedTicks) || elapsedTicks < 0 || elapsedTicks > totalTicks ||
      !Number.isFinite(tickDurationMs) || tickDurationMs < 0 ||
      !Number.isFinite(facingAngle) || !Number.isFinite(range) || range < 0 ||
      !Number.isFinite(angle) || angle < 0 || angle > Math.fround(2 * Math.PI) ||
      !Number.isFinite(totalTicks * tickDurationMs) || totalTicks * tickDurationMs + serverTimeMs > Number.MAX_SAFE_INTEGER ||
      (active && (revision === '0' || executionGeneration === '0' || totalTicks === 0 || tickDurationMs === 0 || range === 0 || angle === 0))) {
    throw new Error('Invalid combat telegraph state')
  }
  return { generation, revision, executionGeneration, active, totalTicks, elapsedTicks, tickDurationMs, serverTimeMs, facingAngle, range, angle: Math.min(angle, 2 * Math.PI) }
}

/** Spawn establishes the incarnation; only strictly newer transitions can change it. */
export function acceptCombatTelegraph(current: CombatTelegraphState | undefined, incoming: CombatTelegraphState): boolean {
  if (!current || incoming.generation !== current.generation || compareUint64(incoming.revision, current.revision) <= 0) return false
  const executionOrder = compareUint64(incoming.executionGeneration, current.executionGeneration)
  return executionOrder >= 0 && (!incoming.active || executionOrder > 0)
}

export function combatTelegraphPhase(state: CombatTelegraphState, estimatedServerNowMs: number): number {
  if (!state.active || !Number.isFinite(estimatedServerNowMs)) return 1
  const elapsedMs = state.elapsedTicks * state.tickDurationMs + estimatedServerNowMs - state.serverTimeMs
  return Math.max(0, Math.min(1, elapsedMs / (state.totalTicks * state.tickDurationMs)))
}
