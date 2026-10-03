import type { proto } from '../network/proto/packets'
import { compareUint64, decodeGeneration, decodeUint64 } from './networkIdentity'

export interface CombatExecution {
  generation: string; revision: string; executionId: string; actionId: string; phase: string
  direction: { x: number; y: number }; range: number; angle: number
  startEvent: string; elapsedMs: number; durationMs: number; serverTimeMs: number; strikeAtMs: number; recoveryEndMs: number
}
export interface CombatTarget { generation: string; revision: string; hp: number; maxHp: number; depleted: boolean }
export interface CombatFeedback { event: string; timeMs: number; text: string }
export interface CombatEntity { generation: string; startEvent: string; execution?: CombatExecution; target?: CombatTarget; feedback?: CombatFeedback }
const finite = (value: number) => Number.isFinite(value)
const timestamp = (value: unknown): number => {
  const result = Number(String(value ?? 0))
  if (!Number.isSafeInteger(result)) throw new Error('Invalid combat timestamp')
  return result
}
export function formatCombatValue(value: number): string {
  if (!finite(value) || value < 0) throw new Error('Invalid combat value')
  return value > 0 && value < .1 ? '<0.1' : value.toFixed(1)
}
export function decodeCombatExecution(input: proto.ICombatExecutionState): CombatExecution {
  const state: CombatExecution = {
    generation: decodeGeneration(input.generation), revision: decodeUint64(input.revision), executionId: decodeUint64(input.executionId),
    startEvent: decodeUint64(input.startEventSequence), actionId: input.actionId ?? '', phase: input.phase ?? 'idle', direction: { x: input.lockedDirection?.x ?? 0, y: input.lockedDirection?.y ?? 0 },
    range: input.range ?? 0, angle: input.sectorAngleDegrees ?? 0, elapsedMs: input.elapsedMs ?? 0, durationMs: input.durationMs ?? 0,
    serverTimeMs: timestamp(input.serverTimeMs), strikeAtMs: timestamp(input.strikeAtMs), recoveryEndMs: timestamp(input.recoveryEndMs),
  }
  if (!['idle', 'windup', 'recovery'].includes(state.phase)) throw new Error('Invalid combat phase')
  if (state.phase !== 'idle' && (state.executionId === '0' || !state.actionId || ![state.range, state.angle, state.durationMs, state.elapsedMs, state.direction.x, state.direction.y].every(finite) || state.range <= 0 || state.angle <= 0 || state.angle > 180 || state.durationMs <= 0 || state.elapsedMs < 0 || state.elapsedMs > state.durationMs || Math.abs(Math.hypot(state.direction.x, state.direction.y) - 1) > 1e-7)) throw new Error('Invalid combat execution')
  return state
}
export function decodeCombatTarget(input: proto.ICombatTargetState): CombatTarget {
  const state = { generation: decodeGeneration(input.generation), revision: decodeUint64(input.revision), hp: input.hp ?? 0, maxHp: input.maxHp ?? 0, depleted: input.depleted === true }
  if (![state.hp, state.maxHp].every(finite) || state.hp < 0 || state.maxHp <= 0 || state.hp > state.maxHp || state.depleted !== (state.hp === 0)) throw new Error('Invalid combat target')
  return state
}

// Plain state only. All combat identities remain decimal strings, including IDs
// above JavaScript's exact integer range.
export class CombatStateCache {
  epoch = 0
  entities = new Map<string, CombatEntity>()
  cooldowns = new Map<string, number>()
  ownerGeneration = ''
  ownerRevision = '0'
  requestRevision = '0'
  reset(epoch = 0): void { this.epoch = epoch; this.entities.clear(); this.cooldowns.clear(); this.ownerGeneration = ''; this.ownerRevision = '0'; this.requestRevision = '0' }
  request(): { streamEpoch: number; requestRevision: string } {
    if (!this.epoch || this.requestRevision === '18446744073709551615') throw new Error('Combat request stream unavailable')
    this.requestRevision = (BigInt(this.requestRevision) + BigInt(1)).toString()
    return { streamEpoch: this.epoch, requestRevision: this.requestRevision }
  }
  spawn(id: unknown, generation: string, execution?: proto.ICombatExecutionState | null, target?: proto.ICombatTargetState | null): void {
    const key = decodeUint64(id)
    const current = this.entities.get(key)
    if (current?.generation !== generation) this.entities.set(key, { generation: decodeGeneration(generation), startEvent: '0' })
    if (execution) this.execution(key, execution, this.epoch)
    if (target) this.target(key, target, this.epoch)
  }
  execution(id: unknown, input: proto.ICombatExecutionState, epoch: number): boolean {
    if (epoch !== this.epoch) return false
    const state = decodeCombatExecution(input), entity = this.entities.get(decodeUint64(id))
    if (!entity || state.generation !== entity.generation) return false
    const current = entity.execution
    if (current && (compareUint64(state.revision, current.revision) < 0 || state.revision === current.revision && state.serverTimeMs <= current.serverTimeMs)) return false
    if (current && state.revision === current.revision && (state.executionId !== current.executionId || state.phase !== current.phase || state.actionId !== current.actionId || state.elapsedMs < current.elapsedMs || state.direction.x !== current.direction.x || state.direction.y !== current.direction.y || state.durationMs !== current.durationMs || state.range !== current.range || state.angle !== current.angle || state.startEvent !== current.startEvent)) return false
    if (compareUint64(state.startEvent, entity.startEvent) > 0) {
      entity.startEvent = state.startEvent
      entity.feedback = undefined
    }
    entity.execution = state
    return true
  }
  target(id: unknown, input: proto.ICombatTargetState, epoch: number): boolean {
    if (epoch !== this.epoch) return false
    const state = decodeCombatTarget(input), entity = this.entities.get(decodeUint64(id))
    if (!entity || state.generation !== entity.generation || entity.target && compareUint64(state.revision, entity.target.revision) <= 0) return false
    entity.target = state
    return true
  }
  owner(input: proto.IS2C_CombatOwnerState): boolean {
    if (input.streamEpoch !== this.epoch) return false
    const generation = decodeGeneration(input.generation), revision = decodeUint64(input.revision)
    if (this.ownerGeneration && this.ownerGeneration !== generation || this.ownerGeneration === generation && compareUint64(revision, this.ownerRevision) <= 0) return false
    this.ownerGeneration = generation; this.ownerRevision = revision
    this.cooldowns.clear()
    for (const cooldown of input.cooldowns ?? []) if (cooldown.actionId) this.cooldowns.set(cooldown.actionId, timestamp(cooldown.readyAtMs))
    return true
  }
  result(input: proto.IS2C_CombatResult): boolean {
    if (input.streamEpoch !== this.epoch) return false
    const entity = this.entities.get(decodeUint64(input.entityId)), event = decodeUint64(input.eventSequence)
    if (!entity || entity.generation !== input.generation || entity.feedback && compareUint64(event, entity.feedback.event) <= 0) return false
    if (compareUint64(event, entity.startEvent) <= 0) return false
    const timeMs = timestamp(input.serverTimeMs)
    for (const hit of input.hits ?? []) {
      if (!hit.target) continue
      this.target(hit.targetId, hit.target, this.epoch)
      const target = this.entities.get(decodeUint64(hit.targetId))
      const hitEvent = decodeUint64(hit.eventSequence)
      if (target && target.generation === hit.target.generation && target.target?.revision === decodeUint64(hit.target.revision) && (!target.feedback || compareUint64(hitEvent, target.feedback.event) > 0)) target.feedback = { event: hitEvent, timeMs, text: '−' + formatCombatValue(hit.damage ?? 0) }
    }
    entity.feedback = { event, timeMs, text: input.hit ? 'Hit' : 'Miss' }
    return true
  }
}

export function combatAttempt(state: proto.IS2C_ActionStateChanged, action: proto.IActionDefinition | undefined, supported: boolean, cache: CombatStateCache): Record<string, unknown> | undefined {
  if (!supported || state.phase !== 'selecting' || action?.targetKind !== 'direction' || !action.combat) return undefined
  return { ...cache.request(), actionId: state.actionId, selectionGeneration: decodeUint64(state.selectionGeneration) }
}
