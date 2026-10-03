import type { proto } from '../network/proto/packets'
import type { SoundProfile } from '../types/soundDefs'
import type { PlaybackOptions } from './SoundManager'
import { smoothstepDistanceAttenuation } from './soundAttenuation'
import { AUDIO_PLAYBACK } from './audioConfig'

interface WorldAudioPlayback {
  profile(key: string): SoundProfile | undefined
  play(key: string, gain: number, options: PlaybackOptions): boolean
  setStreamEpoch(epoch: number): void
}
export class WorldAudioReceiver {
  private epoch = 0
  private freshnessMs: number = AUDIO_PLAYBACK.legacyFreshnessMs
  hearing = 1
  readonly metrics = { stale: 0, wrongStream: 0, invalid: 0, localPacket: 0 }
  constructor(private readonly playback: WorldAudioPlayback, private readonly serverNow: () => number, private readonly listenerPosition: () => { x: number; y: number }) {}

  configure(epoch: number, parameters?: proto.IS2C_AudioParameters | null): void {
    const hearing = parameters?.hearing ?? 1, freshnessMs = parameters?.freshnessMs ?? AUDIO_PLAYBACK.legacyFreshnessMs
    if (!Number.isSafeInteger(epoch) || epoch < 0 || !Number.isFinite(hearing) || hearing <= 0 || !Number.isSafeInteger(freshnessMs) || freshnessMs <= 0) throw new Error('Invalid world audio parameters')
    this.epoch = epoch; this.hearing = hearing; this.freshnessMs = freshnessMs
    this.playback.setStreamEpoch(epoch)
  }
  reset(): void { this.epoch = 0; this.playback.setStreamEpoch(0) }

  batch(message: proto.IS2C_SoundBatch): void {
    if (!this.epoch || message.streamEpoch !== this.epoch) { this.metrics.wrongStream++; return }
    const anchor = Number(String(message.serverTimeMs ?? 0)), now = this.serverNow()
    if (!Number.isSafeInteger(anchor) || anchor <= 0 || !Number.isFinite(now)) { this.metrics.invalid++; return }
    if (now - anchor > this.freshnessMs || anchor - now > this.freshnessMs) { this.metrics.stale++; return }
    for (const entry of message.sounds ?? []) this.entry(entry, { streamEpoch: this.epoch, deadlineServerMs: anchor + this.freshnessMs, waitForLoad: true }, true)
  }

  legacy(message: proto.IS2C_Sound): void {
    if (!this.epoch) return
    this.entry(message, { streamEpoch: this.epoch, deadlineServerMs: this.serverNow() + this.freshnessMs, waitForLoad: true }, false)
  }

  private entry(message: proto.IS2C_Sound, options: PlaybackOptions, requireGain: boolean): void {
    const key = message.soundKey, x = message.x ?? 0, y = message.y ?? 0
    if (typeof key !== 'string' || !/^[a-zA-Z0-9_-]{1,128}$/.test(key) || typeof x !== 'number' || typeof y !== 'number' || !Number.isFinite(x) || !Number.isFinite(y)) { this.metrics.invalid++; return }
    const profile = this.playback.profile(key)
    if (!profile) { this.metrics.invalid++; return }
    if (profile.mode !== 'world') { this.metrics.localPacket++; return }
    let gain: number
    if (Object.hasOwn(message, 'distanceGain') && message.distanceGain != null) {
      gain = message.distanceGain
      if (!Number.isFinite(gain) || gain < 0 || gain > 1) { this.metrics.invalid++; return }
    } else {
      if (requireGain) { this.metrics.invalid++; return }
      const radius = message.maxHearDistance ?? 0, listener = this.listenerPosition()
      if (!Number.isFinite(radius) || radius <= 0 || !Number.isFinite(listener.x) || !Number.isFinite(listener.y)) { this.metrics.invalid++; return }
      gain = smoothstepDistanceAttenuation(Math.hypot(listener.x - x, listener.y - y), radius)
    }
    this.playback.play(key, gain, { ...options, sourceId: `${x},${y}` })
  }
}
