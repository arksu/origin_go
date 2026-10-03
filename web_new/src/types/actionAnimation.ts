import type { proto } from '../network/proto/packets'
import { compareUint64, decodeGeneration, decodeUint64 } from './networkIdentity'

export interface CharacterActionAnimationState {
  readonly generation: string
  readonly revision: string
  readonly animationKey: string
  readonly totalTicks: number
  readonly elapsedTicks: number
  readonly tickDurationMs: number
  readonly serverTimeMs: number
  readonly durationMs?: number
  readonly elapsedMs?: number
  readonly executionId?: string
  readonly lockedDirection?: { readonly x: number; readonly y: number }
  readonly targetPosition?: { readonly x: number; readonly y: number }
}

export function decodeActionAnimation(input: proto.ICharacterActionAnimationState): CharacterActionAnimationState {
  const generation = decodeGeneration(input.generation), revision = decodeUint64(input.revision)
  const timestamp = String(input.serverTimeMs ?? 0), serverTimeMs = Number(timestamp)
  if (!/^(0|[1-9]\d*)$/.test(timestamp) || !Number.isSafeInteger(serverTimeMs)) throw new Error('Invalid action animation timestamp')
  const animationKey = input.animationKey ?? ''
  if (typeof animationKey !== 'string' || animationKey.length > 128 || /[\x00-\x20\x7f]/.test(animationKey)) throw new Error('Invalid action animation key')
  let targetPosition: CharacterActionAnimationState['targetPosition']
  if (input.targetPosition != null) {
    const { x = 0, y = 0, heading = 0 } = input.targetPosition
    if (typeof x !== 'number' || typeof y !== 'number' || !Number.isInteger(x) || !Number.isInteger(y) || x < -2147483648 || x > 2147483647 || y < -2147483648 || y > 2147483647 || typeof heading !== 'number' || !Number.isFinite(heading)) throw new Error('Invalid action animation target position')
    targetPosition = { x, y }
  }
  if (animationKey && (input.durationMs ?? 0) > 0) {
    const durationMs = input.durationMs!, elapsedMs = input.elapsedMs ?? 0
    const direction = input.lockedDirection, executionId = decodeUint64(input.executionId)
    if (revision === '0' || executionId === '0' || !Number.isFinite(durationMs) || durationMs > Number.MAX_SAFE_INTEGER || !Number.isFinite(elapsedMs) || elapsedMs < 0 || elapsedMs > durationMs || !direction || !Number.isFinite(direction.x) || !Number.isFinite(direction.y) || Math.abs(Math.hypot(direction.x!, direction.y!) - 1) > 1e-7) throw new Error('Invalid combat animation timing or direction')
    return { generation, revision, animationKey, totalTicks: 0, elapsedTicks: 0, tickDurationMs: 0, serverTimeMs, durationMs, elapsedMs, executionId, lockedDirection: { x: direction.x!, y: direction.y! } }
  }
  const totalTicks = animationKey ? input.totalTicks ?? 0 : 0
  const elapsedTicks = animationKey ? input.elapsedTicks ?? 0 : 0
  const tickDurationMs = animationKey ? input.tickDurationMs ?? 0 : 0
  if (animationKey && (revision === '0' || !Number.isInteger(totalTicks) || totalTicks < 1 || totalTicks > 4294967295 || !Number.isInteger(elapsedTicks) || elapsedTicks < 0 || elapsedTicks > totalTicks || !Number.isFinite(tickDurationMs) || tickDurationMs <= 0 || !Number.isFinite(totalTicks * tickDurationMs) || totalTicks * tickDurationMs > Number.MAX_SAFE_INTEGER)) throw new Error('Invalid action animation timing')
  return { generation, revision, animationKey, totalTicks, elapsedTicks, tickDurationMs, serverTimeMs, ...(targetPosition ? { targetPosition } : {}) }
}

export function acceptActionAnimation(current: CharacterActionAnimationState | undefined, incoming: CharacterActionAnimationState, generation: string | undefined): boolean {
  if (incoming.generation !== generation) return false
  if (!current) return true
  const order = compareUint64(incoming.revision, current.revision)
  if (order !== 0) return order > 0
  if (incoming.animationKey !== current.animationKey || incoming.totalTicks !== current.totalTicks || incoming.tickDurationMs !== current.tickDurationMs || incoming.durationMs !== current.durationMs || incoming.executionId !== current.executionId || incoming.lockedDirection?.x !== current.lockedDirection?.x || incoming.lockedDirection?.y !== current.lockedDirection?.y) throw new Error('Contradictory action animation revision')
  if (incoming.serverTimeMs <= current.serverTimeMs) return false
  if (incoming.elapsedTicks < current.elapsedTicks || (incoming.elapsedMs ?? 0) < (current.elapsedMs ?? 0)) throw new Error('Action animation progress regressed within a revision')
  return true
}

export function actionAnimationPhase(state: CharacterActionAnimationState, estimatedServerNowMs: number): number {
  if (!Number.isFinite(estimatedServerNowMs)) throw new Error('Invalid estimated server time')
  if (!state.animationKey) return 0
  const elapsedMs = (state.elapsedMs ?? state.elapsedTicks * state.tickDurationMs) + estimatedServerNowMs - state.serverTimeMs
  return Math.max(0, Math.min(1, elapsedMs / (state.durationMs ?? state.totalTicks * state.tickDurationMs)))
}
