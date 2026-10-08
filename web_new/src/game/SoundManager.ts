import { Howl, type HowlOptions } from 'howler'
import { useAudioSettingsStore } from '@/stores/audioSettingsStore'
import type { SoundProfile } from '../types/soundDefs'
import { timeSync } from '@/network/TimeSync'
import { loadActorCatalog, type ActorCatalog } from './actors/ActorAssetCatalog'
import { AUDIO_PLAYBACK } from './audioConfig'

export interface SoundSample {
  state(): string
  load(): unknown
  play(): number
  stop(id?: number): unknown
  volume(value: number, id: number): unknown
  once(event: string, callback: (...args: unknown[]) => void, id?: number): unknown
  off(event: string, callback: (...args: unknown[]) => void, id?: number): unknown
}
export interface PlaybackOptions {
  sourceId?: string | number
  // Local presentation can tune a tile independently of its shared sound profile.
  localVolume?: number
  streamEpoch?: number
  deadlineServerMs?: number
  // Local contacts are consumed when samples are unavailable, never queued.
  waitForLoad?: boolean
}
interface Voice {
  token: number
  profile: SoundProfile
  source: string
  sample: SoundSample
  options: PlaybackOptions
  session: number
  gain: number
  id?: number
  loaded?: (...args: unknown[]) => void
  failed?: (...args: unknown[]) => void
  finished?: (...args: unknown[]) => void
  started?: (...args: unknown[]) => void
  nativeStarted?: boolean
}
interface AudioSettings { enabled: boolean; masterVolume: number; sfxVolume: number }
export interface SoundManagerDependencies {
  createSample?: (options: HowlOptions) => SoundSample
  settings?: () => AudioSettings
  serverNow?: () => number
  maxVoices?: number
  diagnostics?: boolean
}

export class SoundManager {
  private profiles: Readonly<Record<string, SoundProfile>> = {}
  private readonly samples = new Map<string, SoundSample>()
  private readonly fileCursors = new Map<string, number>()
  private readonly voices = new Map<number, Voice>()
  private readonly missingKeys = new Set<string>()
  private readonly feedbackKeys = new Map<string, string>()
  private catalogPromise?: Promise<ActorCatalog>
  private token = 0
  private session = 0
  private streamEpoch = 0
  private readonly createSample: (options: HowlOptions) => SoundSample
  private readonly settings: () => AudioSettings
  private readonly serverNow: () => number
  private readonly maxVoices: number
  private readonly diagnostics: boolean
  readonly metrics = { played: 0, invalid: 0, stale: 0, unloadedLocal: 0, voiceLimit: 0, failed: 0 }

  constructor(dependencies: SoundManagerDependencies = {}) {
    this.createSample = dependencies.createSample ?? (options => new Howl(options) as unknown as SoundSample)
    this.settings = dependencies.settings ?? (() => useAudioSettingsStore())
    this.serverNow = dependencies.serverNow ?? (() => timeSync.estimateServerNowMs())
    this.maxVoices = dependencies.maxVoices ?? AUDIO_PLAYBACK.maxVoices
    this.diagnostics = dependencies.diagnostics ?? AUDIO_PLAYBACK.diagnostics
    if (!Number.isSafeInteger(this.maxVoices) || this.maxVoices < 1) throw new Error('Invalid global audio voice budget')
  }

  initialize(): Promise<ActorCatalog> {
    return this.catalogPromise ??= loadActorCatalog().then(catalog => {
      this.configure(catalog.sounds ?? {})
      return catalog
    })
  }

  configure(profiles: Readonly<Record<string, SoundProfile>>): void {
    this.reset()
    this.profiles = profiles
    this.feedbackKeys.clear()
    for (const profile of Object.values(profiles)) {
      if (profile.feedback_trigger) this.feedbackKeys.set(profile.feedback_trigger, profile.key)
      // Prepare metadata and start fetching before action/contact markers arrive.
      for (const file of profile.files) this.sample(profile.key, file)
    }
  }

  setStreamEpoch(epoch: number): void {
    this.reset()
    this.streamEpoch = epoch
  }

  reset(): void {
    this.session++
    for (const voice of [...this.voices.values()]) {
      if (voice.id !== undefined) voice.sample.stop(voice.id)
      this.release(voice)
    }
  }

  profile(key: string): SoundProfile | undefined { return Object.hasOwn(this.profiles, key) ? this.profiles[key] : undefined }
  get activeVoices(): number { return this.voices.size }
  isLoaded(key: string): boolean {
    const profile = this.profile(key)
    return !!profile && profile.files.some(file => this.samples.get(`${key}:${file}`)?.state() === 'loaded')
  }

  playFeedback(trigger: string, ownerId: number): void {
    const key = this.feedbackKeys.get(trigger)
    if (key) this.play(key, 1, { sourceId: ownerId })
  }

  play(key: string, gain = 1, options: PlaybackOptions = {}): boolean {
    const profile = this.profile(key)
    if (!profile || !Number.isFinite(gain) || gain < 0 || gain > 1 ||
      (options.localVolume !== undefined && (profile.mode !== 'local' || !Number.isFinite(options.localVolume) || options.localVolume < 0 || options.localVolume > 1))) {
      this.metrics.invalid++
      if (!profile && !this.missingKeys.has(key)) {
        this.missingKeys.add(key)
        console.warn(`[SoundManager] Unknown sound profile: ${key}`)
      }
      return false
    }
    if (gain === 0 || options.localVolume === 0 || !this.settings().enabled) return false
    for (const voice of this.voices.values()) {
      if (!voice.nativeStarted && !this.current(voice)) { this.metrics.stale++; this.release(voice) }
    }
    const source = String(options.sourceId ?? 'unspecified')
    let profileVoices = 0, sourceVoices = 0
    for (const voice of this.voices.values()) {
      if (voice.profile.key === key) {
        profileVoices++
        if (voice.source === source) sourceVoices++
      }
    }
    if (this.voices.size >= this.maxVoices || profileVoices >= profile.max_voices || sourceVoices >= profile.max_voices_per_source) {
      this.metrics.voiceLimit++
      return false
    }
    const cursor = this.fileCursors.get(key) ?? 0
    const file = profile.files[cursor % profile.files.length]!
    this.fileCursors.set(key, cursor + 1)
    const sample = this.sample(key, file)
    // A loaded HTML5 sample can still wait for its native play Promise; quiet cues must not catch up later.
    const playbackOptions = profile.mode === 'local' && options.deadlineServerMs === undefined
      ? { ...options, deadlineServerMs: this.serverNow() + AUDIO_PLAYBACK.maxPresentationGapMs }
      : options
    const voice: Voice = { token: ++this.token, profile, source, sample, options: playbackOptions, session: this.session, gain }
    if (!this.current(voice)) { this.metrics.stale++; return false }
    if (sample.state() !== 'loaded' && !options.waitForLoad) { this.metrics.unloadedLocal++; return false }
    this.voices.set(voice.token, voice)
    if (sample.state() === 'loaded') return this.start(voice)
    voice.loaded = () => this.start(voice)
    voice.failed = () => { this.metrics.failed++; this.release(voice) }
    sample.once('load', voice.loaded)
    sample.once('loaderror', voice.failed)
    if (sample.state() === 'unloaded') {
      try { sample.load() }
      catch (error) { this.metrics.failed++; this.release(voice); console.warn(`[SoundManager] Loading failed: ${key}`, error); return false }
    }
    return true
  }

  private current(voice: Voice): boolean {
    return voice.session === this.session &&
      (voice.options.streamEpoch === undefined || voice.options.streamEpoch === this.streamEpoch) &&
      (voice.options.deadlineServerMs === undefined || this.serverNow() <= voice.options.deadlineServerMs)
  }

  private start(voice: Voice): boolean {
    if (!this.voices.has(voice.token)) return false
    if (!this.current(voice)) { this.metrics.stale++; this.release(voice); return false }
    const volume = this.playbackVolume(voice)
    if (volume <= 0) { this.release(voice); return false }
    this.clearLoadCallbacks(voice)
    try {
      // Only loaded samples reach play(), so Howler cannot replay stale queued events.
      const id = voice.sample.play()
      if (!Number.isSafeInteger(id) || id <= 0) throw new Error('Audio backend returned an invalid playback ID')
      voice.id = id
      const started = () => {
        voice.nativeStarted = true
        const currentVolume = this.playbackVolume(voice)
        if (!this.voices.has(voice.token) || !this.current(voice) || currentVolume <= 0) {
          if (this.voices.has(voice.token) && !this.current(voice)) this.metrics.stale++
          this.release(voice)
          voice.sample.stop(id)
          return
        }
        // HTML5 play() may hold Howler's lock until its native Promise resolves.
        // Reapply by ID after that lock clears instead of trusting its volume queue.
        voice.sample.volume(currentVolume, id)
      }
      voice.started = started
      voice.sample.once('play', started, id)
      voice.sample.volume(volume, id)
      const finished = () => { voice.nativeStarted = true; this.release(voice) }
      voice.finished = finished
      voice.sample.once('end', finished, id)
      voice.sample.once('stop', finished, id)
      voice.sample.once('playerror', finished, id)
      this.metrics.played++
      if (this.diagnostics) console.debug('[SoundManager] playing', { key: voice.profile.key, id, gain: voice.gain, source: voice.source })
      return true
    } catch (error) {
      this.metrics.failed++
      this.release(voice)
      console.warn(`[SoundManager] Playback failed: ${voice.profile.key}`, error)
      return false
    }
  }

  private playbackVolume(voice: Voice): number {
    const settings = this.settings()
    const volume = settings.masterVolume * settings.sfxVolume * (voice.options.localVolume ?? voice.profile.volume) * voice.gain
    return settings.enabled && Number.isFinite(volume) ? Math.max(0, Math.min(1, volume)) : 0
  }

  private clearLoadCallbacks(voice: Voice): void {
    if (voice.loaded) voice.sample.off('load', voice.loaded)
    if (voice.failed) voice.sample.off('loaderror', voice.failed)
    voice.loaded = undefined; voice.failed = undefined
  }
  private release(voice: Voice): void {
    this.clearLoadCallbacks(voice)
    // Keep the one-shot native-start guard through a reset if play() is still pending.
    if (voice.started && voice.nativeStarted && voice.id !== undefined) voice.sample.off('play', voice.started, voice.id)
    if (voice.finished && voice.id !== undefined) for (const event of ['end', 'stop', 'playerror']) voice.sample.off(event, voice.finished, voice.id)
    this.voices.delete(voice.token)
  }
  private sample(key: string, file: string): SoundSample {
    const identity = `${key}:${file}`
    let sample = this.samples.get(identity)
    if (!sample) {
      sample = this.createSample({ src: [`/assets/game/${file}`], preload: true, html5: true, volume: 0,
        onloaderror: (_id, error) => { console.warn(`[SoundManager] Cannot load ${file}`, error) } })
      this.samples.set(identity, sample)
    }
    return sample
  }
}
export const soundManager = new SoundManager()
