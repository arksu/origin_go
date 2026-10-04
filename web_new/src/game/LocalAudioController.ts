import type { CharacterActionAnimationState } from '../types/actionAnimation'
import { actionAnimationPhase } from '../types/actionAnimation'
import type { ActionAnimationDefinition, ActionAnimationSoundCue } from '../types/actionAnimationDefs'
import type { LocomotionAudioBinding, SoundProfile } from '../types/soundDefs'
import type { PlaybackOptions } from './SoundManager'
import { AUDIO_PLAYBACK } from './audioConfig'
import { locomotionSoundKey } from './footstepConfig'

export interface LocalAudioSnapshot {
  entityId: number
  actor: string
  position: { x: number; y: number }
  tileType?: number
  ready: boolean
  moving: boolean
  clip: string
  distanceTiles: number
  discontinuity: boolean
  action: CharacterActionAnimationState | null
  actionReady?: boolean
}
interface LocalPlayback { profile(key: string): SoundProfile | undefined; play(key: string, gain: number, options?: PlaybackOptions): boolean }
interface LocomotionState { selector: string; cycle: number; time: number; active: boolean; audible: Set<string> }
interface ActionState { identity: string; phase: number; time: number; ready: boolean; consumed: Set<string>; audible: Set<string> }

export function localDistanceGain(profile: SoundProfile, hearing: number, distance: number, ownSource: boolean): number {
  const radius = profile.loudness * hearing
  if (profile.mode !== 'local' || !Number.isFinite(radius) || radius <= 0 || !Number.isFinite(distance) || distance < 0 || distance >= radius) return 0
  if (ownSource) return 1
  const curve = profile.local_attenuation
  if (!curve) return 0
  const nearDistance = curve.near_distance ?? 0
  if (nearDistance >= radius) return 0
  if (distance <= nearDistance) return curve.near_gain
  // Hearing changes the outer radius, while the near zone stays in absolute world units.
  const fadeProgress = (distance - nearDistance) / (radius - nearDistance)
  const normalized = Math.log1p(curve.shape * fadeProgress) / Math.log1p(curve.shape)
  return curve.near_gain - (curve.near_gain - curve.far_gain) * normalized
}

export class LocalAudioController {
  private bindings = new Map<string, LocomotionAudioBinding>()
  private actionCues = new Map<string, readonly ActionAnimationSoundCue[]>()
  private readonly locomotion = new Map<number, LocomotionState>()
  private readonly actions = new Map<number, ActionState>()
  private ownerId: number | null = null
  private hearing = 1
  private listener: { x: number; y: number } | null = null
  constructor(private readonly playback: LocalPlayback) {}

  configure(bindings: readonly LocomotionAudioBinding[], definitions: Readonly<Record<string, ActionAnimationDefinition>>): void {
    this.reset()
    this.bindings = new Map(bindings.map(binding => [`${binding.actor}/${binding.clip}`, binding]))
    this.actionCues = new Map(Object.values(definitions).map(definition => [definition.key, (definition.sound_cues ?? []).filter(cue => playbackMode(this.playback, cue.sound_key) === 'local')]))
  }
  setListener(ownerId: number | null, hearing: number): void {
    if (!Number.isFinite(hearing) || hearing <= 0) throw new Error('Invalid local hearing')
    this.ownerId = ownerId; this.hearing = hearing
  }
  setListenerPosition(position: { x: number; y: number } | null): void { this.listener = position }
  remove(entityId: number): void { this.locomotion.delete(entityId); this.actions.delete(entityId) }
  reset(): void { this.locomotion.clear(); this.actions.clear(); this.listener = null }

  update(snapshot: LocalAudioSnapshot, nowMs: number, serverNowMs: number): void {
    this.updateLocomotion(snapshot, nowMs)
    this.updateAction(snapshot, nowMs, serverNowMs)
  }
  private gain(key: string, entityId: number, position: { x: number; y: number }): number {
    const profile = this.playback.profile(key)
    if (!profile || !this.listener || this.ownerId === null) return 0
    return localDistanceGain(profile, this.hearing, Math.hypot(position.x - this.listener.x, position.y - this.listener.y), entityId === this.ownerId)
  }
  private updateLocomotion(snapshot: LocalAudioSnapshot, nowMs: number): void {
    const selector = `${snapshot.actor}/${snapshot.clip}`, binding = this.bindings.get(selector)
    if (!binding?.cycle_distance_tiles) { this.locomotion.delete(snapshot.entityId); return }
    const cycle = snapshot.distanceTiles / binding.cycle_distance_tiles
    const active = snapshot.ready && snapshot.moving && this.listener !== null
    const previous = this.locomotion.get(snapshot.entityId)
    const rebase = !previous || !active || !previous.active || previous.selector !== selector || snapshot.discontinuity ||
      nowMs - previous.time > AUDIO_PLAYBACK.maxPresentationGapMs || cycle < previous.cycle || cycle - previous.cycle >= 1
    const audible = previous?.audible ?? new Set<string>()
    for (const contact of binding.contacts) {
      const soundKey = locomotionSoundKey(contact.sound_key, snapshot.tileType)
      const gain = this.gain(soundKey, snapshot.entityId, snapshot.position), wasAudible = audible.has(contact.id)
      if (gain > 0) audible.add(contact.id); else audible.delete(contact.id)
      // Entering the radius starts at the current gait, never at an old contact.
      if (rebase || !wasAudible || gain <= 0 || !previous) continue
      const nextContact = Math.floor(previous.cycle - contact.phase) + 1 + contact.phase
      if (nextContact <= cycle) this.playback.play(soundKey, gain, { sourceId: snapshot.entityId })
    }
    this.locomotion.set(snapshot.entityId, { selector, cycle, time: nowMs, active, audible })
  }
  private updateAction(snapshot: LocalAudioSnapshot, nowMs: number, serverNowMs: number): void {
    const action = snapshot.action
    if (!action?.animationKey) { this.actions.delete(snapshot.entityId); return }
    const cues = this.actionCues.get(action.animationKey)
    if (!cues?.length) return
    const phase = actionAnimationPhase(action, serverNowMs), identity = `${action.generation}/${action.revision}/${action.animationKey}`
    const previous = this.actions.get(snapshot.entityId)
    const sameCycle = previous?.identity === identity
    const state: ActionState = sameCycle ? previous : { identity, phase, time: nowMs, ready: snapshot.ready, consumed: new Set(), audible: new Set() }
    const ready = snapshot.actionReady ?? snapshot.ready
    const rebase = !sameCycle || !ready || !state.ready || snapshot.discontinuity || nowMs - state.time > AUDIO_PLAYBACK.maxPresentationGapMs
    for (const cue of cues) {
      const source = cue.source === 'actor' ? snapshot.position : action.targetPosition
      const gain = source ? this.gain(cue.sound_key, snapshot.entityId, source) : 0, wasAudible = state.audible.has(cue.id)
      if (gain > 0) state.audible.add(cue.id); else state.audible.delete(cue.id)
      if (state.consumed.has(cue.id) || phase < cue.phase) continue
      // The consumed set survives backward timing corrections within this cycle.
      state.consumed.add(cue.id)
      if (!rebase && wasAudible && state.phase < cue.phase && gain > 0) this.playback.play(cue.sound_key, gain, { sourceId: snapshot.entityId })
    }
    state.phase = phase; state.time = nowMs; state.ready = ready
    this.actions.set(snapshot.entityId, state)
  }
}
function playbackMode(playback: LocalPlayback, key: string): SoundProfile['mode'] | undefined { return playback.profile(key)?.mode }
