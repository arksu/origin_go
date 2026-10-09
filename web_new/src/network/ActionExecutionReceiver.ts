import type { proto } from './proto/packets'
import { proto as packets } from './proto/packets.js'
import { compareUint64, decodeNonzeroUint64 } from '../types/networkIdentity'
import { getDirectionSector, type DirectionSector } from '../game/hud/directionAim'

interface ActionExecutionPresentation {
  showAttackSector(angle: number, sector: DirectionSector): void
  clearAttackSector(): void
}

const phases = new Set(['selecting', 'approaching', 'executing', 'cooldown_wait'])

/** Owns execution identity independently of character models and animation bindings. */
export class ActionExecutionReceiver {
  private epoch = 0
  private generation = '0'
  private actionId = ''
  private facingAngle: number | undefined
  private shownGeneration = '0'
  private runningSector = false
  private idle = true

  constructor(private readonly presentation: ActionExecutionPresentation) {}

  reset(epoch = 0): void {
    this.epoch = Number.isInteger(epoch) && epoch > 0 && epoch <= 4294967295 ? epoch : 0
    this.generation = this.shownGeneration = '0'
    this.actionId = ''
    this.facingAngle = undefined
    this.runningSector = false
    this.idle = true
    this.presentation.clearAttackSector()
  }

  /** Validate before the caller changes action UI or its cooldown snapshot. */
  acceptState(message: proto.IS2C_ActionStateChanged, definitions: ReadonlyMap<string, proto.IActionDefinition>): boolean {
    if (!this.epoch || message.streamEpoch !== this.epoch) return false
    const actionId = message.actionId ?? '', phase = message.phase ?? ''
    const facingAngle = message.facingAngle ?? undefined
    if (phase === 'idle') {
      if (String(message.actionGeneration ?? 0) !== '0' || actionId !== '' || facingAngle !== undefined) return false
      if (this.runningSector) this.presentation.clearAttackSector()
      this.runningSector = false
      this.idle = true
      return true
    }
    const generation = decodeNonzeroUint64(message.actionGeneration)
    if (!phases.has(phase) || typeof actionId !== 'string' || !actionId || generation === null) return false
    const order = compareUint64(generation, this.generation)
    if (order < 0 || (order === 0 && this.idle)) return false
    if (order === 0 && (actionId !== this.actionId ||
        (facingAngle !== undefined && this.facingAngle !== undefined && facingAngle !== this.facingAngle))) return false
    const definition = definitions.get(actionId)
    const directional = definition?.targetKind === 'direction'
    const sector = phase === 'executing' && directional ? getDirectionSector(definition) : null
    if (phase === 'executing' && directional) {
      if (!sector || typeof facingAngle !== 'number' || !Number.isFinite(facingAngle) || facingAngle < 0 ||
          facingAngle > Math.fround(2 * Math.PI)) return false
      if (generation === this.shownGeneration && !this.runningSector) return false
    } else if (facingAngle !== undefined) return false

    // Nothing before this point mutates receiver state or presentation.
    if (order > 0) {
      if (!sector) this.presentation.clearAttackSector()
      this.runningSector = false
      this.facingAngle = undefined
    }
    this.generation = generation
    this.actionId = actionId
    this.idle = false
    if (facingAngle !== undefined) this.facingAngle = facingAngle
    if (sector && generation !== this.shownGeneration) {
      this.shownGeneration = generation
      this.runningSector = true
      this.presentation.showAttackSector(facingAngle!, sector)
    }
    return true
  }

  /** Critical server FIFO pairs this terminal packet with the current execution. */
  finish(message: proto.IS2C_CyclicActionFinished): void {
    if (!this.epoch || !this.runningSector || message.actionId !== this.actionId) return
    if (message.result === packets.CyclicActionFinishResult.CYCLIC_ACTION_FINISH_RESULT_CANCELED) {
      this.presentation.clearAttackSector()
    } else if (message.result !== packets.CyclicActionFinishResult.CYCLIC_ACTION_FINISH_RESULT_COMPLETED) return
    this.runningSector = false
  }
}
