import { EQUIPMENT_SLOT_BY_ID, type EquipmentSlot } from './equipmentSlots.ts'

export interface ActionAnimationSource { kind: 'context' | 'menu' | 'craft' | 'build'; namespace?: string; id: string }
export interface ActionAnimationEquipment { slot: EquipmentSlot; visual_key: string }
export interface ActionAnimationVariant { clip: string; equipment: ActionAnimationEquipment[] }
export interface ActionAnimationFrame { width: number; height: number; origin_x: number; origin_y: number }
export type ActionAnimationEligibility = 'stationary' | 'not_carrying' | 'not_knocked_out'
export interface ActionAnimationSoundCue { id: string; phase: number; sound_key: string; source: 'actor' | 'target' }
export interface ActionAnimationDefinition {
  key: string
  actor: string
  variants: ActionAnimationVariant[]
  eligibility: ActionAnimationEligibility[]
  facing: 'preserve' | 'target'
  blend_ms: number
  frame: ActionAnimationFrame
  unbind_equipment_slots?: EquipmentSlot[]
  preview?: { label: string; duration_ms: number; equipment: ActionAnimationEquipment[] }
  sound_cues?: ActionAnimationSoundCue[]
}
export interface ActionAnimationBinding extends ActionAnimationDefinition { source: ActionAnimationSource }

const NAME = /^[a-zA-Z0-9_][a-zA-Z0-9_.:-]{0,127}$/
const VISUAL = /^[a-zA-Z0-9_-]{1,128}$/
const SLOTS = new Set<string>(Object.values(EQUIPMENT_SLOT_BY_ID))
const ELIGIBILITY = new Set<string>(['stationary', 'not_carrying', 'not_knocked_out'])

function object(value: unknown, label: string, fields: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${label}: expected object`)
  for (const [key, item] of Object.entries(value)) {
    if (!fields.includes(key)) throw new Error(`${label}.${key}: unknown field`)
    if (item === null) throw new Error(`${label}.${key}: must not be null`)
  }
  return value as Record<string, unknown>
}
function string(value: unknown, label: string, pattern = NAME): string {
  if (typeof value !== 'string' || !pattern.test(value)) throw new Error(`${label}: invalid reference`)
  return value
}
function number(value: unknown, label: string, minimum: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < minimum) throw new Error(`${label}: invalid number`)
  return value
}
function array(value: unknown, label: string): unknown[] {
  if (!Array.isArray(value)) throw new Error(`${label}: expected array`)
  return value
}
function equipment(value: unknown, label: string): ActionAnimationEquipment[] {
  const slots = new Set<string>()
  return array(value, label).map((raw, index) => {
    const field = `${label}[${index}]`, item = object(raw, field, ['slot', 'visual_key'])
    const slot = string(item.slot, `${field}.slot`)
    if (!SLOTS.has(slot) || slots.has(slot)) throw new Error(`${field}.slot: unknown or duplicate slot`)
    slots.add(slot)
    return { slot: slot as EquipmentSlot, visual_key: string(item.visual_key, `${field}.visual_key`, VISUAL) }
  })
}

function parseBinding(value: unknown, label: string, withSource: boolean): ActionAnimationDefinition | ActionAnimationBinding {
  const item = object(value, label, ['key', 'actor', 'variants', 'eligibility', 'facing', 'blend_ms', 'frame', 'preview', 'unbind_equipment_slots', 'sound_cues', ...(withSource ? ['source'] : [])])
  const variants = array(item.variants, `${label}.variants`).map((raw, index) => {
    const field = `${label}.variants[${index}]`, variant = object(raw, field, ['clip', 'equipment'])
    return { clip: string(variant.clip, `${field}.clip`), equipment: equipment(variant.equipment, `${field}.equipment`) }
  })
  if (!variants.length) throw new Error(`${label}.variants: must not be empty`)
  const eligibility = array(item.eligibility, `${label}.eligibility`)
  if (eligibility.some(predicate => typeof predicate !== 'string' || !ELIGIBILITY.has(predicate)) || new Set(eligibility).size !== eligibility.length) throw new Error(`${label}.eligibility: unknown or duplicate predicate`)
  if (item.facing !== 'preserve' && item.facing !== 'target') throw new Error(`${label}.facing: expected preserve or target`)
  const rawFrame = object(item.frame, `${label}.frame`, ['width', 'height', 'origin_x', 'origin_y'])
  const frame = {
    width: number(rawFrame.width, `${label}.frame.width`, 1), height: number(rawFrame.height, `${label}.frame.height`, 1),
    origin_x: number(rawFrame.origin_x ?? 0, `${label}.frame.origin_x`, 0), origin_y: number(rawFrame.origin_y ?? 0, `${label}.frame.origin_y`, 0),
  }
  if (!Object.values(frame).every(Number.isInteger) || frame.width > 1024 || frame.height > 1024 || frame.origin_x > frame.width || frame.origin_y > frame.height) throw new Error(`${label}.frame: invalid bounds or origin`)
  const result: ActionAnimationDefinition = {
    key: string(item.key, `${label}.key`), actor: string(item.actor, `${label}.actor`, /^character\/[a-zA-Z0-9_-]{1,128}$/),
    variants, eligibility: eligibility as ActionAnimationEligibility[], facing: item.facing,
    blend_ms: number(item.blend_ms ?? 0, `${label}.blend_ms`, 0), frame,
  }
  if (item.sound_cues !== undefined) {
    const cueIDs = new Set<string>()
    let previousPhase = 0
    result.sound_cues = array(item.sound_cues, `${label}.sound_cues`).map((raw, index) => {
      const field = `${label}.sound_cues[${index}]`, cue = object(raw, field, ['id', 'phase', 'sound_key', 'source'])
      const id = string(cue.id, `${field}.id`), phase = number(cue.phase, `${field}.phase`, Number.MIN_VALUE)
      if (cueIDs.has(id) || phase <= previousPhase || phase > 1) throw new Error(`${field}: duplicate ID or phase not increasing within (0,1]`)
      if (cue.source !== 'actor' && cue.source !== 'target') throw new Error(`${field}.source: expected actor or target`)
      cueIDs.add(id); previousPhase = phase
      return { id, phase, sound_key: string(cue.sound_key, `${field}.sound_key`), source: cue.source }
    })
  }
  if (item.unbind_equipment_slots !== undefined) {
    const slots = array(item.unbind_equipment_slots, `${label}.unbind_equipment_slots`)
    if (slots.some(slot => typeof slot !== 'string' || !SLOTS.has(slot)) || new Set(slots).size !== slots.length) throw new Error(`${label}.unbind_equipment_slots: unknown or duplicate slot`)
    result.unbind_equipment_slots = slots as EquipmentSlot[]
  }
  if (item.preview !== undefined) {
    const preview = object(item.preview, `${label}.preview`, ['label', 'duration_ms', 'equipment'])
    if (typeof preview.label !== 'string' || !preview.label.trim() || new TextEncoder().encode(preview.label).length > 256) throw new Error(`${label}.preview.label: invalid label`)
    const duration = number(preview.duration_ms, `${label}.preview.duration_ms`, Number.MIN_VALUE)
    result.preview = { label: preview.label, duration_ms: duration, equipment: equipment(preview.equipment, `${label}.preview.equipment`) }
  }
  if (!withSource) return result
  const source = object(item.source, `${label}.source`, ['kind', 'namespace', 'id'])
  const kind = typeof source.kind === 'string' ? source.kind.trim() : ''
  if (!['context', 'menu', 'craft', 'build'].includes(kind)) throw new Error(`${label}.source.kind: unsupported source`)
  if (kind === 'craft' && result.sound_cues?.some(cue => cue.source === 'target')) throw new Error(`${label}.sound_cues: craft execution has no guaranteed target source`)
  const id = string(typeof source.id === 'string' ? source.id.trim() : source.id, `${label}.source.id`)
  const namespace = source.namespace === undefined ? '' : typeof source.namespace === 'string' ? source.namespace.trim() : source.namespace
  if (namespace !== '') string(namespace, `${label}.source.namespace`)
  return { ...result, source: { kind: kind as ActionAnimationSource['kind'], namespace: namespace as string, id } }
}

export function validateActionSoundReferences(definitions: readonly ActionAnimationDefinition[], sounds: ReadonlyMap<string, { mode: string }>, label: string): void {
  for (const binding of definitions) for (const cue of binding.sound_cues ?? []) {
    if (!sounds.has(cue.sound_key)) throw new Error(`${label}: binding ${binding.key} cue ${cue.id}: missing sound ${cue.sound_key}`)
  }
}

export function validateAnimationUniqueness(bindings: readonly (ActionAnimationDefinition | ActionAnimationBinding)[], label: string): void {
  const keys = new Set<string>(), sources = new Set<string>()
  for (const binding of bindings) {
    if (keys.has(binding.key)) throw new Error(`${label}: duplicate key ${binding.key}`)
    keys.add(binding.key)
    if ('source' in binding) {
      const selector = JSON.stringify([binding.source.kind, binding.source.namespace ?? '', binding.source.id])
      if (sources.has(selector)) throw new Error(`${label}: duplicate source selector for ${binding.key}`)
      sources.add(selector)
    }
  }
}

function parseFile(value: unknown, label: string, withSource: boolean): (ActionAnimationDefinition | ActionAnimationBinding)[] {
  const file = object(value, label, ['v', 'bindings'])
  if (file.v !== 1) throw new Error(`${label}.v: unsupported version`)
  const bindings = array(file.bindings, `${label}.bindings`).map((binding, index) => parseBinding(binding, `${label}.bindings[${index}]`, withSource))
  validateAnimationUniqueness(bindings, label)
  return bindings
}
export function parseActionAnimationFile(value: unknown, label: string): ActionAnimationBinding[] {
  return parseFile(value, label, true) as ActionAnimationBinding[]
}
export function parseActionAnimationProjection(value: unknown, label: string): ActionAnimationDefinition[] {
  return parseFile(value, label, false)
}

export interface AnimationAssetManifest {
  kind: string
  rigHash: string | null
  clips: Record<string, { rigHash: string; duration: number }>
  bindings: Partial<Record<EquipmentSlot, unknown>>
  equipmentSlots?: EquipmentSlot[]
}
export function validateActionAnimationAssets(definitions: readonly ActionAnimationDefinition[], manifests: Readonly<Record<string, AnimationAssetManifest>>, label: string): void {
  for (const binding of definitions) {
    const field = `${label}: binding ${binding.key}`, actor = manifests[binding.actor]
    if (!actor || actor.kind !== 'character' || !actor.rigHash) throw new Error(`${field}: missing actor ${binding.actor}`)
    for (const variant of binding.variants) {
      const clip = actor.clips[variant.clip]
      if (!clip || clip.rigHash !== actor.rigHash || !Number.isFinite(clip.duration) || clip.duration <= 0) throw new Error(`${field}: absent or incompatible clip ${variant.clip}`)
    }
    for (const required of [...binding.variants.flatMap(variant => variant.equipment), ...(binding.preview?.equipment ?? [])]) {
      const asset = manifests[`equipment/${required.visual_key}`]
      if (!asset || asset.kind !== 'equipment' || (asset.rigHash === null ? !asset.bindings[required.slot] : asset.rigHash !== actor.rigHash || !asset.equipmentSlots?.includes(required.slot))) {
        throw new Error(`${field}: incompatible equipment ${required.visual_key} in ${required.slot}`)
      }
    }
  }
}
