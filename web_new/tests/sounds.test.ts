import test from 'node:test'
import assert from 'node:assert/strict'
import { createPinia, setActivePinia } from 'pinia'
import { proto } from '../src/network/proto/packets'
import { parseSoundFile, parseLocomotionAudioFile, type SoundProfile } from '../src/types/soundDefs'
import type { ActionAnimationDefinition } from '../src/types/actionAnimationDefs'
import type { CharacterActionAnimationState } from '../src/types/actionAnimation'
import { SoundManager, type SoundSample } from '../src/game/SoundManager'
import { WorldAudioReceiver } from '../src/game/WorldAudioReceiver'
import { LocalAudioController, localDistanceGain, type LocalAudioSnapshot } from '../src/game/LocalAudioController'
import { soundManager } from '../src/game/SoundManager'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { useGameStore } from '../src/stores/gameStore'
import { gameFacade } from '../src/game/GameFacade'
import { worldAudioReceiver } from '../src/game/audioRuntime'
import { timeSync } from '../src/network/TimeSync'
import authoredSounds from '../../data/sounds/actions.json'

const world: SoundProfile = { key: 'chop', mode: 'world', loudness: 1000, volume: .9, files: ['sound/chop/chop_01.mp3'], priority: 10, max_voices: 8, max_voices_per_source: 2 }
const authoredFootstep = parseSoundFile(authoredSounds, 'data/sounds/actions.json').find(profile => profile.key === 'footstep')!
const local: SoundProfile = { ...authoredFootstep, volume: .6, files: ['sound/footstep/step.wav'], priority: 1, max_voices: 8, max_voices_per_source: 2 }
const feedback: SoundProfile = { ...local, key: 'exp_gain', volume: .4, feedback_trigger: 'exp_gain' }
const registry = { chop: world, footstep: local, exp_gain: feedback }

class FakeSample implements SoundSample {
  status = 'loaded'
  ids = 0
  readonly volumes = new Map<number, number>()
  readonly listeners: Array<{ event: string; callback: (...args: unknown[]) => void; id?: number }> = []
  state(): string { return this.status }
  load(): void { this.status = 'loading' }
  play(): number { assert.equal(this.status, 'loaded'); return ++this.ids }
  stop(id?: number): void { this.emit('stop', id) }
  volume(volume: number, id: number): void { this.volumes.set(id, volume) }
  once(event: string, callback: (...args: unknown[]) => void, id?: number): void { this.listeners.push({ event, callback, id }) }
  off(event: string, callback: (...args: unknown[]) => void, id?: number): void {
    for (let index = this.listeners.length - 1; index >= 0; index--) if (this.listeners[index]?.event === event && this.listeners[index]?.callback === callback && this.listeners[index]?.id === id) this.listeners.splice(index, 1)
  }
  emit(event: string, id?: number): void {
    for (const listener of [...this.listeners]) if (listener.event === event && (listener.id === undefined || listener.id === id)) {
      this.off(event, listener.callback, listener.id); listener.callback(id)
    }
  }
}
class NativeLockedSample extends FakeSample {
  locked = false
  stops = 0
  override play(): number { this.locked = true; return super.play() }
  override volume(volume: number, id: number): void { if (!this.locked) super.volume(volume, id) }
  override stop(id?: number): void { this.stops++; if (!this.locked) super.stop(id) }
  override emit(event: string, id?: number): void { if (event === 'play') this.locked = false; super.emit(event, id) }
}
function playbackFixture(status = 'loaded', maxVoices = 32) {
  let now = 1000
  const settings = { enabled: true, masterVolume: .8, sfxVolume: .5 }
  const samples: FakeSample[] = []
  const manager = new SoundManager({ createSample: () => { const sample = new FakeSample(); sample.status = status; samples.push(sample); return sample }, settings: () => settings, serverNow: () => now, maxVoices })
  manager.configure(registry); manager.setStreamEpoch(7)
  return { manager, samples, settings, setNow: (value: number) => { now = value } }
}

// Presence is protocol state: protobuf's prototype zero is not an authored gain.
test('protobuf retains gain presence, existing tags, batches and entry audio parameters', () => {
  const legacy = proto.S2C_Sound.decode(proto.S2C_Sound.encode({ soundKey: 'chop', x: 1, y: 2, maxHearDistance: 200 }).finish())
  assert.equal(Object.hasOwn(legacy, 'distanceGain'), false)
  assert.equal(legacy.maxHearDistance, 200)
  const zero = proto.S2C_Sound.decode(proto.S2C_Sound.encode({ soundKey: 'chop', distanceGain: 0 }).finish())
  assert.equal(Object.hasOwn(zero, 'distanceGain'), true); assert.equal(zero.distanceGain, 0)
  const batch = proto.ServerMessage.decode(proto.ServerMessage.encode({ soundBatch: { sounds: [zero], streamEpoch: 7, serverTimeMs: 1000 } }).finish()).soundBatch!
  assert.equal(batch.streamEpoch, 7); assert.equal(String(batch.serverTimeMs), '1000'); assert.equal(Object.hasOwn(batch.sounds![0]!, 'distanceGain'), true)
  const entry = proto.S2C_PlayerEnterWorld.decode(proto.S2C_PlayerEnterWorld.encode({ audio: { hearing: 1, freshnessMs: 500 } }).finish())
  assert.equal(entry.audio?.hearing, 1); assert.equal(entry.audio?.freshnessMs, 500)
})

test('strict catalogs validate modes, numerical bounds, names, unknown fields and references shape', () => {
  assert.deepEqual(parseSoundFile({ v: 1, sounds: Object.values(registry) }, 'test'), Object.values(registry))
  for (const patch of [{ mode: 'loud' }, { loudness: 0 }, { loudness: NaN }, { volume: 1.1 }, { files: ['sound/../bad.wav'] }, { priority: 256 }, { max_voices: 129 }, { max_voices_per_source: 9 }, { ignored: true }]) {
    assert.throws(() => parseSoundFile({ v: 1, sounds: [{ ...world, ...patch }] }, 'bad'))
  }
  assert.throws(() => parseSoundFile({ v: 1, sounds: [world, world] }, 'duplicate'))
  assert.throws(() => parseSoundFile({ v: 1, sounds: [{ ...local, local_attenuation: { near_gain: .8, far_gain: .9, shape: 4 } }] }, 'curve'))
  assert.throws(() => parseSoundFile({ v: 1, sounds: [{ ...local, local_attenuation: { near_gain: .9, far_gain: .8, shape: 0 } }] }, 'curve'))
  const binding = { actor: 'character/male_commoner', clip: 'walk', cycle_distance_tiles: 1.6, contacts: [{ id: 'right', phase: .4, sound_key: 'footstep' }, { id: 'left', phase: .9, sound_key: 'footstep' }] }
  assert.equal(parseLocomotionAudioFile({ v: 1, bindings: [binding] }, 'gait')[0]?.cycle_distance_tiles, 1.6)
  for (const contacts of [[{ id: 'edge', phase: 1, sound_key: 'footstep' }], [{ id: 'a', phase: .8, sound_key: 'footstep' }, { id: 'b', phase: .2, sound_key: 'footstep' }], [{ id: 'a', phase: .2, sound_key: 'footstep' }, { id: 'a', phase: .8, sound_key: 'footstep' }]]) assert.throws(() => parseLocomotionAudioFile({ v: 1, bindings: [{ ...binding, contacts }] }, 'bad gait'))
})

test('world batches apply gain once, preserve zero and reject malformed siblings independently', () => {
  const played: Array<{ key: string; gain: number }> = []
  const playback = { profile: (key: string) => registry[key as keyof typeof registry], play: (key: string, gain: number) => { played.push({ key, gain }); return true }, setStreamEpoch: () => {} }
  const receiver = new WorldAudioReceiver(playback, () => 1000, () => ({ x: 1000, y: 0 }))
  receiver.configure(7, { hearing: 1, freshnessMs: 500 })
  receiver.batch({ streamEpoch: 7, serverTimeMs: 900, sounds: [
    { soundKey: 'chop', x: 0, y: 0, distanceGain: 0 },
    { soundKey: 'chop', x: Infinity, distanceGain: .5 },
    { soundKey: 'chop', distanceGain: 2 },
    { soundKey: 'footstep', distanceGain: 1 },
    { soundKey: 'chop' },
    { soundKey: 'chop', distanceGain: .5 },
  ] })
  assert.deepEqual(played, [{ key: 'chop', gain: 0 }, { key: 'chop', gain: .5 }])
  assert.equal(receiver.metrics.invalid, 3); assert.equal(receiver.metrics.localPacket, 1)
  receiver.batch({ streamEpoch: 6, serverTimeMs: 1000, sounds: [{ soundKey: 'chop', distanceGain: 1 }] })
  receiver.batch({ streamEpoch: 7, serverTimeMs: 499, sounds: [{ soundKey: 'chop', distanceGain: 1 }] })
  assert.equal(played.length, 2); assert.equal(receiver.metrics.wrongStream, 1); assert.equal(receiver.metrics.stale, 1)
})

test('legacy single world fallback keeps old radius curve; local packets cannot duplicate feedback', () => {
  const played: number[] = []
  const receiver = new WorldAudioReceiver({ profile: key => registry[key as keyof typeof registry], play: (_key, gain) => { played.push(gain); return true }, setStreamEpoch: () => {} }, () => 1000, () => ({ x: 100, y: 0 }))
  receiver.configure(7)
  receiver.legacy(proto.S2C_Sound.decode(proto.S2C_Sound.encode({ soundKey: 'chop', x: 0, y: 0, maxHearDistance: 200 }).finish()))
  receiver.legacy({ soundKey: 'chop', x: 0, y: 0, maxHearDistance: 200, distanceGain: .25 })
  receiver.legacy({ soundKey: 'exp_gain', maxHearDistance: 80 })
  assert.deepEqual(played, [.5, .25]); assert.equal(receiver.metrics.localPacket, 1)
})

test('same sample simultaneous voices retain independent ID gains and bound voice admission', () => {
  const { manager, samples } = playbackFixture('loaded', 2)
  assert.equal(manager.play('chop', 1, { sourceId: 1 }), true)
  assert.equal(manager.play('chop', .5, { sourceId: 2 }), true)
  assert.equal(manager.play('chop', .2, { sourceId: 3 }), false)
  assert.equal(samples[0]?.volumes.get(1), .8 * .5 * .9)
  assert.equal(samples[0]?.volumes.get(2), .8 * .5 * .9 * .5)
  samples[0]!.emit('end', 1)
  assert.equal(manager.activeVoices, 1); assert.equal(manager.play('chop', .2, { sourceId: 3 }), true)
  manager.reset(); assert.equal(manager.activeVoices, 0); assert.equal(samples[0]!.listeners.length, 0)
})

test('per-source voice cap and mute are honored independently of server selection', () => {
  const { manager, settings } = playbackFixture()
  manager.play('chop', 1, { sourceId: 'tree' }); manager.play('chop', 1, { sourceId: 'tree' })
  assert.equal(manager.play('chop', 1, { sourceId: 'tree' }), false)
  assert.equal(manager.play('chop', 1, { sourceId: 'other' }), true)
  settings.enabled = false
  assert.equal(manager.play('footstep'), false)
})

test('HTML5 native play locks do not lose per-ID volume, and reset guards a delayed native start', () => {
  const sample = new NativeLockedSample()
  const manager = new SoundManager({ createSample: () => sample, settings: () => ({ enabled: true, masterVolume: .5, sfxVolume: 1 }), serverNow: () => 1000 })
  manager.configure({ chop: world }); manager.setStreamEpoch(7)
  manager.play('chop', .5, { sourceId: 1 })
  assert.equal(sample.volumes.has(1), false)
  sample.emit('play', 1)
  assert.equal(sample.volumes.get(1), .5 * .9 * .5)
  sample.emit('end', 1)
  manager.play('chop', 1, { streamEpoch: 7, deadlineServerMs: 1200, sourceId: 2 })
  manager.setStreamEpoch(8)
  const stopsBeforeNative = sample.stops
  sample.emit('play', 2)
  assert.equal(sample.stops, stopsBeforeNative + 1)
  assert.equal(sample.volumes.has(2), false)
  assert.equal(manager.activeVoices, 0)
})

test('local voices expire before a delayed native start without requiring caller deadlines', () => {
  let now = 1000
  const sample = new NativeLockedSample()
  const manager = new SoundManager({ createSample: () => sample, settings: () => ({ enabled: true, masterVolume: 1, sfxVolume: 1 }), serverNow: () => now })
  manager.configure({ footstep: local })
  assert.equal(manager.play('footstep', 1, { sourceId: 1 }), true)
  now = 1251
  sample.emit('play', 1)
  assert.equal(sample.stops, 1)
  assert.equal(sample.volumes.has(1), false)
  assert.equal(manager.activeVoices, 0)
  assert.equal(manager.metrics.stale, 1)

  assert.equal(manager.play('footstep', .5, { sourceId: 1, deadlineServerMs: 1500 }), true)
  now = 1499
  sample.emit('play', 2)
  assert.equal(sample.volumes.get(2), local.volume * .5)
  assert.equal(manager.activeVoices, 1)
})

test('expired pending native voices free capacity while their stale-start stop guard survives', () => {
  let now = 1000
  const sample = new NativeLockedSample()
  const manager = new SoundManager({ createSample: () => sample, settings: () => ({ enabled: true, masterVolume: 1, sfxVolume: 1 }), serverNow: () => now, maxVoices: 1 })
  manager.configure({ footstep: local })
  assert.equal(manager.play('footstep', 1, { sourceId: 1 }), true)
  now = 1251
  assert.equal(manager.play('footstep', .5, { sourceId: 1 }), true)
  assert.equal(manager.activeVoices, 1)
  assert.equal(manager.metrics.stale, 1)
  sample.emit('play', 1)
  assert.equal(sample.stops, 1)
  assert.equal(sample.volumes.has(1), false)
  assert.equal(manager.activeVoices, 1)
  sample.emit('play', 2)
  assert.equal(sample.volumes.get(2), local.volume * .5)
  sample.emit('end', 2)
  assert.equal(manager.activeVoices, 0)
})

test('sample loading cannot start an expired, reset or old-stream voice; local sounds never queue', () => {
  const fixture = playbackFixture('loading')
  assert.equal(fixture.manager.play('footstep', 1, { sourceId: 1 }), false)
  assert.equal(fixture.manager.activeVoices, 0)
  fixture.manager.play('chop', 1, { streamEpoch: 7, deadlineServerMs: 1100, waitForLoad: true })
  fixture.setNow(1101); fixture.samples[0]!.status = 'loaded'; fixture.samples[0]!.emit('load')
  assert.equal(fixture.samples[0]!.ids, 0); assert.equal(fixture.manager.metrics.stale, 1)
  fixture.samples[0]!.status = 'loading'; fixture.setNow(1000)
  fixture.manager.play('chop', 1, { streamEpoch: 7, deadlineServerMs: 1100, waitForLoad: true })
  fixture.manager.setStreamEpoch(8); fixture.samples[0]!.status = 'loaded'; fixture.samples[0]!.emit('load')
  assert.equal(fixture.samples[0]!.ids, 0); assert.equal(fixture.manager.activeVoices, 0)
  assert.equal(fixture.manager.play('chop', 1, { streamEpoch: 7, deadlineServerMs: 1100, waitForLoad: true }), false)
})

test('fresh delayed sample rechecks mute and volume at actual playback time', () => {
  const fixture = playbackFixture('loading')
  fixture.manager.play('chop', .5, { streamEpoch: 7, deadlineServerMs: 1100, waitForLoad: true })
  fixture.settings.masterVolume = .4
  fixture.samples[0]!.status = 'loaded'; fixture.samples[0]!.emit('load')
  assert.equal(fixture.samples[0]!.volumes.get(1), .4 * .5 * .9 * .5)
})

test('expired loading reservations free bounded capacity before subsequent audio admission', () => {
  const fixture = playbackFixture('loading', 1)
  fixture.manager.play('chop', 1, { streamEpoch: 7, deadlineServerMs: 1100, waitForLoad: true })
  fixture.setNow(1101)
  assert.equal(fixture.manager.play('chop', .5, { streamEpoch: 7, deadlineServerMs: 1200, waitForLoad: true }), true)
  assert.equal(fixture.manager.activeVoices, 1)
  fixture.samples[0]!.status = 'loaded'; fixture.samples[0]!.emit('load')
  assert.equal(fixture.samples[0]!.ids, 1)
  assert.equal(fixture.samples[0]!.volumes.get(1), .8 * .5 * .9 * .5)
})

test('authored footsteps stay at 90% through 16 world units, then fade continuously to silence', () => {
  assert.equal(localDistanceGain(local, 1, 0, true), 1)
  for (const distance of [0, 1, 8, 15.999, 16]) assert.equal(localDistanceGain(local, 1, distance, false), .9)
  assert.ok(Math.abs(localDistanceGain(local, 1, 16 + 1e-6, false) - .9) < 1e-7, 'No jump at the near-zone boundary')
  assert.ok(Math.abs(localDistanceGain(local, 1, 68, false) - .285654424963) < 1e-9)
  let previousGain = .9
  for (let distance = 17; distance < 120; distance++) {
    const gain = localDistanceGain(local, 1, distance, false)
    assert.ok(gain > 0 && gain < previousGain, `Footsteps must become quieter at distance ${distance}`)
    previousGain = gain
  }
  assert.ok(localDistanceGain(local, 1, 120 - 1e-6, false) < 1e-8, 'No audible jump at the radius boundary')
  assert.equal(localDistanceGain(local, 1, 120, false), 0)
  assert.equal(localDistanceGain(local, 1, 121, false), 0)
  assert.equal(localDistanceGain(local, 1.5, 16, false), .9)
  assert.ok(localDistanceGain(local, 1.5, 24, false) < .9, 'Hearing must not scale the absolute near distance')
  assert.equal(localDistanceGain(local, 1.5, 98, false), localDistanceGain(local, 1, 68, false))
  assert.equal(localDistanceGain(local, .1, 1, false), 0, 'A collapsed fade interval must not produce a gain')
})

function localFixture() {
  const played: Array<{ key: string; gain: number }> = []
  const playback = { profile: (key: string) => registry[key as keyof typeof registry], play: (key: string, gain: number) => { played.push({ key, gain }); return true } }
  const controller = new LocalAudioController(playback)
  const binding = { actor: 'character/male_commoner', clip: 'walk', cycle_distance_tiles: 1, contacts: [{ id: 'right', phase: .4, sound_key: 'footstep' }, { id: 'left', phase: .9, sound_key: 'footstep' }] }
  const definition: ActionAnimationDefinition = { key: 'test_action', actor: binding.actor, variants: [], eligibility: [], facing: 'preserve', blend_ms: 0, frame: { width: 128, height: 128, origin_x: 0, origin_y: 0 }, sound_cues: [{ id: 'local_contact', phase: .6, sound_key: 'footstep', source: 'actor' }, { id: 'world_contact', phase: .8, sound_key: 'chop', source: 'target' }] }
  controller.configure([binding, { ...binding, clip: 'carry_walk' }], { test_action: definition })
  controller.setListener(1, 1); controller.setListenerPosition({ x: 0, y: 0 })
  const snapshot: LocalAudioSnapshot = { entityId: 1, actor: binding.actor, position: { x: 0, y: 0 }, ready: true, moving: true, clip: 'walk', distanceTiles: 0, discontinuity: false, action: null }
  return { controller, played, snapshot }
}

test('footsteps follow unwrapped interpolated distance without pose sampling or culling', () => {
  const { controller, snapshot, played } = localFixture()
  controller.update(snapshot, 0, 1000)
  for (const [distance, time] of [[.5, 100], [1, 200], [1.5, 300], [2, 400]]) { snapshot.distanceTiles = distance!; controller.update(snapshot, time!, 1000 + time!) }
  assert.deepEqual(played.map(event => event.key), ['footstep', 'footstep', 'footstep', 'footstep'])
  assert.ok(played.every(event => event.gain === 1))
})

test('other character contacts use current world distance and approach silence before leaving range', () => {
  const { controller, snapshot, played } = localFixture()
  snapshot.entityId = 2
  controller.update(snapshot, 0, 1000)
  for (const [index, distance] of [0, 8, 16, 68, 119, 120].entries()) {
    snapshot.position.x = distance
    snapshot.distanceTiles = (index + 1) * .5
    controller.update(snapshot, (index + 1) * 100, 1000 + (index + 1) * 100)
  }
  assert.equal(played.length, 5)
  assert.deepEqual(played.slice(0, 3).map(event => event.gain), [.9, .9, .9])
  assert.ok(Math.abs(played[3]!.gain - .285654424963) < 1e-9)
  assert.ok(played[4]!.gain < .005)
  for (let index = 3; index < played.length; index++) assert.ok(played[index]!.gain < played[index - 1]!.gain)
})

test('stops, snaps, loading, radius entry, clip changes, long gaps and reset rebase footsteps', () => {
  for (const transition of ['stop', 'snap', 'load', 'radius', 'clip', 'pause', 'reset']) {
    const { controller, snapshot, played } = localFixture()
    controller.update(snapshot, 0, 1000)
    snapshot.distanceTiles = .3; controller.update(snapshot, 100, 1100)
    if (transition === 'stop') { snapshot.moving = false; snapshot.distanceTiles = 0 }
    if (transition === 'snap') snapshot.discontinuity = true
    if (transition === 'load') snapshot.ready = false
    if (transition === 'radius') { snapshot.entityId = 2; snapshot.position.x = 130 }
    if (transition === 'clip') snapshot.clip = 'carry_walk'
    if (transition === 'reset') controller.reset()
    snapshot.distanceTiles = .8; controller.update(snapshot, transition === 'pause' ? 1000 : 200, 1200)
    assert.equal(played.length, 0, transition)
    snapshot.moving = true; snapshot.ready = true; snapshot.discontinuity = false; snapshot.position.x = 0
    controller.setListenerPosition({ x: 0, y: 0 })
    snapshot.distanceTiles = .95; controller.update(snapshot, transition === 'pause' ? 1100 : 300, 1300)
    if (['snap', 'clip', 'pause'].includes(transition)) assert.equal(played.length, 1, transition)
    else assert.equal(played.length, 0, transition)
    if (transition === 'stop') {
      snapshot.distanceTiles = 1.5; controller.update(snapshot, 400, 1400)
      assert.equal(played.length, 1, 'Restart plays only the next genuine contact')
      controller.update(snapshot, 500, 1500)
      assert.equal(played.length, 1, 'Stationary distance cannot replay the restarted contact')
    }
  }
})

function actionAt(revision = '1', elapsedTicks = 0): CharacterActionAnimationState {
  return { generation: '1', revision, animationKey: 'test_action', totalTicks: 10, elapsedTicks, tickDurationMs: 100, serverTimeMs: 1000, targetPosition: { x: 0, y: 0 } }
}
test('local action cues retain consumed markers across backward corrections and only re-arm a new cycle', () => {
  const { controller, snapshot, played } = localFixture()
  snapshot.moving = false; snapshot.action = actionAt()
  controller.update(snapshot, 0, 1500); controller.update(snapshot, 100, 1700)
  assert.equal(played.length, 1)
  controller.update(snapshot, 200, 1400); controller.update(snapshot, 300, 1900)
  assert.equal(played.length, 1)
  snapshot.action = actionAt('2'); controller.update(snapshot, 400, 1200); controller.update(snapshot, 500, 1700)
  assert.equal(played.length, 2); assert.ok(played.every(event => event.key === 'footstep'))
  snapshot.action = null; controller.update(snapshot, 600, 1900)
  snapshot.action = actionAt('3', 8); controller.update(snapshot, 700, 1000)
  controller.update(snapshot, 800, 1100); assert.equal(played.length, 2)
})

test('late action entry, loading and pause consume passed cues without catch-up', () => {
  for (const mode of ['late', 'loading', 'pause']) {
    const { controller, snapshot, played } = localFixture()
    snapshot.moving = false; snapshot.action = actionAt()
    snapshot.ready = mode !== 'loading'
    controller.update(snapshot, 0, mode === 'late' ? 1800 : 1300)
    controller.update(snapshot, mode === 'pause' ? 1000 : 100, 1800)
    snapshot.ready = true; controller.update(snapshot, mode === 'pause' ? 1100 : 200, 1900)
    assert.equal(played.length, 0, mode)
  }
})

test('local action suppression and locomotion stop/restart cannot re-arm a consumed action marker', () => {
  const { controller, snapshot, played } = localFixture()
  snapshot.moving = false; snapshot.action = actionAt()
  controller.update(snapshot, 0, 1500); controller.update(snapshot, 100, 1700)
  snapshot.actionReady = false; controller.update(snapshot, 200, 1400)
  snapshot.moving = true; controller.update(snapshot, 250, 1450)
  snapshot.actionReady = true; snapshot.moving = false; controller.update(snapshot, 300, 1900)
  assert.equal(played.length, 1)
})

test('discovery playback requires the feedback FX trigger, not an experience delta', t => {
  const { manager, samples } = playbackFixture()
  manager.playFeedback('craft_success', 1); assert.equal(manager.activeVoices, 0)
  manager.playFeedback('exp_gain', 1); assert.equal(manager.activeVoices, 1)
  assert.equal(samples[2]!.volumes.get(1), .8 * .5 * .4)
  setActivePinia(createPinia())
  const store = useGameStore()
  store.setPlayerEnterWorld(1, 'owner', 12, 128, 7)
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  const feedbackCalls = t.mock.method(soundManager, 'playFeedback', () => {})
  const fxCalls = t.mock.method(gameFacade, 'playFx', () => {})
  registerMessageHandlers()
  messageDispatcher.dispatch(proto.ServerMessage.create({ expGained: { lp: 100 } }))
  assert.equal(feedbackCalls.mock.callCount(), 0)
  messageDispatcher.dispatch(proto.ServerMessage.create({ fx: { fxKey: 'exp_gain' } }))
  assert.equal(feedbackCalls.mock.callCount(), 1); assert.equal(fxCalls.mock.callCount(), 1)
  assert.deepEqual(feedbackCalls.mock.calls[0]?.arguments, ['exp_gain', 1])
  t.after(() => store.reset())
})

test('the dispatcher routes decoded sound batches through the registered audio handler', t => {
  setActivePinia(createPinia())
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  t.mock.method(soundManager, 'profile', (key: string) => registry[key as keyof typeof registry])
  const played = t.mock.method(soundManager, 'play', () => true)
  t.mock.method(timeSync, 'estimateServerNowMs', () => 1000)
  registerMessageHandlers(); worldAudioReceiver.configure(7, { hearing: 1, freshnessMs: 500 })
  const packet = proto.ServerMessage.decode(proto.ServerMessage.encode({ soundBatch: { streamEpoch: 7, serverTimeMs: 900, sounds: [{ soundKey: 'chop', distanceGain: .5 }] } }).finish())
  messageDispatcher.dispatch(packet)
  assert.equal(played.mock.callCount(), 1)
  assert.equal(played.mock.calls[0]?.arguments[1], .5)
  worldAudioReceiver.reset()
})
