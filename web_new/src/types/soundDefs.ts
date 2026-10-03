export interface LocalAttenuation { near_distance?: number; near_gain: number; far_gain: number; shape: number }
export interface SoundProfile {
  key: string
  mode: 'world' | 'local'
  loudness: number
  volume: number
  files: string[]
  priority: number
  max_voices: number
  max_voices_per_source: number
  local_attenuation?: LocalAttenuation
  feedback_trigger?: string
}
export interface LocomotionContact { id: string; phase: number; sound_key: string }
export interface LocomotionAudioBinding {
  actor: string
  clip: string
  contacts: LocomotionContact[]
  cycle_distance_tiles?: number
}

const KEY = /^[a-zA-Z0-9_-]{1,128}$/
function record(value: unknown, label: string, fields: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${label}: expected object`)
  for (const key of Object.keys(value)) if (!fields.includes(key)) throw new Error(`${label}: unknown field ${key}`)
  return value as Record<string, unknown>
}
function text(value: unknown, label: string, pattern = KEY): string {
  if (typeof value !== 'string' || !pattern.test(value)) throw new Error(`${label}: invalid name`)
  return value
}
function numeric(value: unknown, label: string, minimum: number, maximum = Infinity): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < minimum || value > maximum) throw new Error(`${label}: invalid number`)
  return value
}
function integer(value: unknown, label: string, minimum: number): number {
  const result = numeric(value, label, minimum, Number.MAX_SAFE_INTEGER)
  if (!Number.isSafeInteger(result)) throw new Error(`${label}: expected integer`)
  return result
}
function array(value: unknown, label: string): unknown[] {
  if (!Array.isArray(value)) throw new Error(`${label}: expected array`)
  return value
}
export function parseSoundFile(value: unknown, label: string): SoundProfile[] {
  const file = record(value, label, ['v', 'sounds'])
  if (file.v !== 1) throw new Error(`${label}: unsupported version`)
  const keys = new Set<string>(), triggers = new Set<string>()
  return array(file.sounds, `${label}.sounds`).map((raw, index) => {
    const field = `${label}.sounds[${index}]`
    const item = record(raw, field, ['key', 'mode', 'loudness', 'volume', 'files', 'priority', 'max_voices', 'max_voices_per_source', 'local_attenuation', 'feedback_trigger'])
    const key = text(item.key, `${field}.key`)
    if (keys.has(key)) throw new Error(`${field}: duplicate sound key ${key}`)
    keys.add(key)
    if (item.mode !== 'world' && item.mode !== 'local') throw new Error(`${field}.mode: expected world or local`)
    const files = array(item.files, `${field}.files`).map((path, fileIndex) => {
      if (typeof path !== 'string' || !/^sound\/[a-zA-Z0-9_/-]+\.(mp3|wav|ogg)$/.test(path) || path.includes('//') || path.split('/').some(part => part === '.' || part === '..')) throw new Error(`${field}.files[${fileIndex}]: invalid sound asset path`)
      return path
    })
    if (!files.length || new Set(files).size !== files.length) throw new Error(`${field}.files: empty or duplicate samples`)
    const profile: SoundProfile = { key, mode: item.mode, loudness: numeric(item.loudness, `${field}.loudness`, Number.MIN_VALUE), volume: numeric(item.volume, `${field}.volume`, 0, 1), files,
      priority: integer(item.priority, `${field}.priority`, 0), max_voices: integer(item.max_voices, `${field}.max_voices`, 1), max_voices_per_source: integer(item.max_voices_per_source, `${field}.max_voices_per_source`, 1) }
    if (profile.priority > 255) throw new Error(`${field}.priority: must be within [0,255]`)
    if (profile.max_voices > 128) throw new Error(`${field}.max_voices: must not exceed 128`)
    if (profile.max_voices_per_source > profile.max_voices) throw new Error(`${field}: per-source voices exceed profile voices`)
    if (item.local_attenuation !== undefined) {
      const curve = record(item.local_attenuation, `${field}.local_attenuation`, ['near_distance', 'near_gain', 'far_gain', 'shape'])
      profile.local_attenuation = { near_gain: numeric(curve.near_gain, `${field}.near_gain`, 0, 1), far_gain: numeric(curve.far_gain, `${field}.far_gain`, 0, 1), shape: numeric(curve.shape, `${field}.shape`, Number.MIN_VALUE) }
      if (curve.near_distance !== undefined) {
        profile.local_attenuation.near_distance = numeric(curve.near_distance, `${field}.near_distance`, 0)
        if (profile.local_attenuation.near_distance >= profile.loudness) throw new Error(`${field}: near distance must be less than loudness`)
      }
      if (profile.local_attenuation.far_gain > profile.local_attenuation.near_gain) throw new Error(`${field}: far gain exceeds near gain`)
    }
    if (profile.mode === 'local' ? !profile.local_attenuation : profile.local_attenuation !== undefined) throw new Error(`${field}: attenuation does not match sound mode`)
    if (item.feedback_trigger !== undefined) {
      if (profile.mode !== 'local') throw new Error(`${field}: world sound cannot use local feedback trigger`)
      profile.feedback_trigger = text(item.feedback_trigger, `${field}.feedback_trigger`)
      if (triggers.has(profile.feedback_trigger)) throw new Error(`${field}: duplicate feedback trigger`)
      triggers.add(profile.feedback_trigger)
    }
    return profile
  })
}
export const parseSoundProjection = parseSoundFile

export function parseLocomotionAudioFile(value: unknown, label: string): LocomotionAudioBinding[] {
  const file = record(value, label, ['v', 'bindings'])
  if (file.v !== 1) throw new Error(`${label}: unsupported version`)
  const selectors = new Set<string>()
  return array(file.bindings, `${label}.bindings`).map((raw, index) => {
    const field = `${label}.bindings[${index}]`, item = record(raw, field, ['actor', 'clip', 'contacts', 'cycle_distance_tiles'])
    const binding: LocomotionAudioBinding = { actor: text(item.actor, `${field}.actor`, /^character\/[a-zA-Z0-9_-]{1,128}$/), clip: text(item.clip, `${field}.clip`, /^[a-zA-Z0-9_.-]{1,128}$/), contacts: [] }
    const selector = `${binding.actor}/${binding.clip}`
    if (selectors.has(selector)) throw new Error(`${field}: duplicate locomotion selector`)
    selectors.add(selector)
    let previousPhase = 0
    const ids = new Set<string>()
    binding.contacts = array(item.contacts, `${field}.contacts`).map((rawContact, contactIndex) => {
      const contactField = `${field}.contacts[${contactIndex}]`, contact = record(rawContact, contactField, ['id', 'phase', 'sound_key'])
      const id = text(contact.id, `${contactField}.id`), phase = numeric(contact.phase, `${contactField}.phase`, Number.MIN_VALUE)
      if (ids.has(id) || phase <= previousPhase || phase >= 1) throw new Error(`${contactField}: duplicate ID or unordered phase outside (0,1)`)
      ids.add(id); previousPhase = phase
      return { id, phase, sound_key: text(contact.sound_key, `${contactField}.sound_key`) }
    })
    if (!binding.contacts.length) throw new Error(`${field}: contacts must not be empty`)
    if (item.cycle_distance_tiles !== undefined) binding.cycle_distance_tiles = numeric(item.cycle_distance_tiles, `${field}.cycle_distance_tiles`, Number.MIN_VALUE)
    return binding
  })
}
