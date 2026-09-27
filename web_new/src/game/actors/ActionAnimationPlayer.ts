import type { ActionAnimationDefinition, ActionAnimationFrame, ActionAnimationVariant } from '../../types/actionAnimationDefs'
import type { EquippedVisual } from '../../types/characterVisual'

export interface ActionAnimationInput { key: string; phase: number; facingAngle?: number }
export interface ActionPresentationContext {
  stationary: boolean
  carrying: boolean
  knockedOut: boolean
  equipment: readonly EquippedVisual[]
  equipmentReady: boolean
}
export interface ActionPoseSample { clip: string; phase: number; weight: number }
interface Layer {
  definition: ActionAnimationDefinition
  variant: ActionAnimationVariant
  phase: number
  weight: number
  from: number
  target: number
  startedMs: number
}

/** Presentation blending is independent of the caller's authoritative phase. */
export class ActionAnimationPlayer {
  private definitions: Readonly<Record<string, ActionAnimationDefinition>> = {}
  private actorId = ''
  private configured = false
  private input: ActionAnimationInput | null = null
  private readonly layers = new Map<string, Layer>()
  private readonly reported = new Set<string>()
  private currentFrame: ActionAnimationFrame
  private currentSamples: ActionPoseSample[] = []
  private currentFacing: number | undefined

  constructor(private readonly baseFrame: ActionAnimationFrame, private readonly report: (message: string) => void = console.error) {
    this.currentFrame = baseFrame
  }

  configure(definitions: Readonly<Record<string, ActionAnimationDefinition>>, actorId: string): void {
    this.definitions = definitions
    this.actorId = actorId
    this.configured = true
  }

  setInput(input: ActionAnimationInput | null): void {
    if (input && (!Number.isFinite(input.phase) || input.phase < 0 || input.phase > 1 || input.facingAngle !== undefined && !Number.isFinite(input.facingAngle))) throw new Error('Invalid action animation presentation input')
    this.input = input
  }

  private issue(message: string): void {
    if (!this.reported.has(message)) { this.reported.add(message); this.report(message) }
  }

  private selection(context: ActionPresentationContext): { definition: ActionAnimationDefinition; variant: ActionAnimationVariant; index: number } | null {
    if (!this.configured || !this.input || context.knockedOut) return null
    const definition = Object.hasOwn(this.definitions, this.input.key) ? this.definitions[this.input.key] : undefined
    if (!definition) { this.issue(`Unknown action animation binding: ${this.input.key}`); return null }
    if (definition.actor !== this.actorId) { this.issue(`Incompatible action animation actor: ${definition.key}`); return null }
    if (definition.facing === 'target' && this.input.facingAngle === undefined) { this.issue(`Action animation requires a facing target: ${definition.key}`); return null }
    for (const rule of definition.eligibility) {
      if (rule === 'stationary' && !context.stationary || rule === 'not_carrying' && context.carrying || rule === 'not_knocked_out' && context.knockedOut) return null
    }
    const index = definition.variants.findIndex(variant => variant.equipment.every(required => context.equipmentReady && context.equipment.some(item => item.slot === required.slot && item.visualKey === required.visual_key)))
    return index < 0 ? null : { definition, variant: definition.variants[index]!, index }
  }

  update(context: ActionPresentationContext, now: number): void {
    if (context.knockedOut) this.layers.clear()
    const selected = this.selection(context)
    const selectedKey = selected ? `${selected.definition.key}/${selected.index}` : null
    for (const [key, layer] of this.layers) {
      const progress = layer.definition.blend_ms === 0 ? 1 : Math.max(0, Math.min(1, (now - layer.startedMs) / layer.definition.blend_ms))
      layer.weight = layer.from + (layer.target - layer.from) * progress
      const target = key === selectedKey ? 1 : 0
      if (layer.target !== target) { layer.from = layer.weight; layer.target = target; layer.startedMs = now }
      if (layer.definition.blend_ms === 0) layer.weight = target
      if (target === 0 && layer.weight === 0) this.layers.delete(key)
    }
    if (selected && selectedKey && this.input) {
      let layer = this.layers.get(selectedKey)
      if (!layer) {
        layer = { ...selected, phase: this.input.phase, weight: selected.definition.blend_ms === 0 ? 1 : 0, from: 0, target: 1, startedMs: now }
        this.layers.set(selectedKey, layer)
      }
      layer.phase = this.input.phase
    }
    this.currentFacing = selected?.definition.facing === 'target' ? this.input?.facingAngle : undefined
    this.currentSamples = [...this.layers.values()].filter(layer => layer.weight > 0).map(layer => ({ clip: layer.variant.clip, phase: layer.phase, weight: layer.weight }))
    let left = -this.baseFrame.origin_x, top = -this.baseFrame.origin_y
    let right = this.baseFrame.width + left, bottom = this.baseFrame.height + top
    for (const layer of this.layers.values()) {
      const frame = layer.definition.frame
      left = Math.min(left, -frame.origin_x); top = Math.min(top, -frame.origin_y)
      right = Math.max(right, frame.width - frame.origin_x); bottom = Math.max(bottom, frame.height - frame.origin_y)
    }
    this.currentFrame = { width: right - left, height: bottom - top, origin_x: -left, origin_y: -top }
  }

  get frame(): ActionAnimationFrame { return this.currentFrame }
  get samples(): readonly ActionPoseSample[] { return this.currentSamples }
  get facingAngle(): number | undefined { return this.currentFacing }
}
