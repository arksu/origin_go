import { Howl } from 'howler'
import { loadActorCatalog } from '../src/game/actors/ActorAssetCatalog'
import { SoundManager, type SoundSample } from '../src/game/SoundManager'
import { LocalAudioController, localDistanceGain, type LocalAudioSnapshot } from '../src/game/LocalAudioController'
import { WorldAudioReceiver } from '../src/game/WorldAudioReceiver'
import { proto } from '../src/network/proto/packets'
import { verifyRenderedVariants } from './audio-browser-render'
import { validateFootstepSoundProfiles } from '../src/game/footstepConfig'
import {
  TILE_BROADLEAF_FOREST, TILE_CLAY, TILE_CONIFEROUS_FOREST,
  TILE_DIRT, TILE_MOUNTAIN, TILE_PLOWED, TILE_SHALLOW_WATER,
} from '../src/game/tiles/tileIds'

const results = document.querySelector<HTMLPreElement>('#results')!
const run = document.querySelector<HTMLButtonElement>('#run')!
const fixtureInput = document.querySelector<HTMLInputElement>('#server-fixture')!
const renderVariants = document.querySelector<HTMLInputElement>('#render-variants')!
const attenuationOnly = document.querySelector<HTMLInputElement>('#attenuation-only')!
const failures: string[] = []
window.addEventListener('error', event => failures.push(event.message))
window.addEventListener('unhandledrejection', event => failures.push(String(event.reason)))
function assert(condition: unknown, message: string): asserts condition { if (!condition) throw new Error(message) }
function pause(ms: number): Promise<void> { return new Promise(resolve => setTimeout(resolve, ms)) }
function report(value: unknown): void { results.textContent = JSON.stringify(value, null, 2) }

interface ServerFixture {
  v: 1
  enter_world_base64: string
  emissions: Array<{ tick: number; sound_key: string; distance_gain: number; packet_base64: string }>
  expected: { effects: number; chop_count: number; fall_count: number; target_alive: boolean; outside_visibility: boolean }
}
function decodePacket(encoded: string): proto.ServerMessage {
  assert(typeof encoded === 'string' && encoded.length <= 1048576 && /^[a-zA-Z0-9+/]+=*$/.test(encoded), 'Invalid fixture packet encoding')
  return proto.ServerMessage.decode(Uint8Array.from(atob(encoded), character => character.charCodeAt(0)))
}
async function readServerFixture(): Promise<ServerFixture | undefined> {
  const file = fixtureInput.files?.[0]
  if (!file) return undefined
  assert(file.size <= 1048576, 'Server fixture is too large')
  const value = JSON.parse(await file.text()) as ServerFixture
  assert(value?.v === 1 && Array.isArray(value.emissions) && value.emissions.length === 3, 'Invalid server fixture envelope')
  assert(value.expected?.chop_count === 2 && value.expected.fall_count === 1 && value.expected.effects === 2 &&
    value.expected.target_alive === false && value.expected.outside_visibility === true, 'Fixture did not complete the required server scenario')
  decodePacket(value.enter_world_base64)
  for (const emission of value.emissions) assert(Number.isSafeInteger(emission.tick) && emission.tick > 0 &&
    typeof emission.sound_key === 'string' && Number.isFinite(emission.distance_gain) && emission.distance_gain >= 0 && emission.distance_gain <= 1,
    'Invalid server fixture emission')
  return value
}

run.addEventListener('click', () => { void verify().catch(error => { report({ status: 'FAILED', error: String(error), failures }); run.disabled = false }) })

async function verify(): Promise<void> {
  run.disabled = true
  const context = new AudioContext()
  await context.resume()
  report({ status: 'Loading published catalogs and decoding every audio sample' })
  const catalog = await loadActorCatalog()
  const serverFixture = attenuationOnly.checked ? undefined : await readServerFixture()
  const profiles = catalog.sounds!
  assert(['chop', 'tree_fall', 'exp_gain'].every(key => profiles[key]), 'Expected published world and feedback profiles')
  validateFootstepSoundProfiles(profiles)
  const chopBinding = catalog.actionAnimations.tree_chop!
  const handClips = chopBinding.variants.map(variant => variant.clip).sort()
  assert(handClips.join(',') === 'chop_l,chop_r' && chopBinding.sound_cues?.[0]?.phase === .4, 'Published hand variants lost the shared impact marker')
  const decoded: Array<{ file: string; seconds: number; channels: number; energy: number }> = []
  const checkedProfiles = attenuationOnly.checked ? { footstep: profiles.footstep! } : profiles
  for (const file of new Set(Object.values(checkedProfiles).flatMap(profile => profile.files))) {
    const response = await fetch(`/assets/game/${file}`)
    assert(response.ok, `Missing sample ${file}`)
    const buffer = await context.decodeAudioData(await response.arrayBuffer())
    let energy = 0
    for (const amplitude of buffer.getChannelData(0)) energy += amplitude * amplitude
    assert(buffer.duration > 0 && energy > 0, `Empty decoded sample ${file}`)
    decoded.push({ file, seconds: buffer.duration, channels: buffer.numberOfChannels, energy })
  }
  const playEvents: Array<{ file: string; id: number; volume: number; playing: boolean }> = []
  const samples: Howl[] = []
  const samplesByFile = new Map<string, Howl>()
  let serverNow = Date.now()
  const manager = new SoundManager({ settings: () => ({ enabled: true, masterVolume: .5, sfxVolume: 1 }), serverNow: () => serverNow,
    createSample: options => {
      const file = String(options.src?.[0])
      const howl = new Howl({ ...options, onplay: id => playEvents.push({ file, id, volume: Number(howl.volume(id)), playing: howl.playing(id) }),
        onplayerror: (_id, error) => failures.push(`Play error ${file}: ${String(error)}`),
        onloaderror: (_id, error) => failures.push(`Load error ${file}: ${String(error)}`) })
      samples.push(howl)
      samplesByFile.set(file, howl)
      return howl as unknown as SoundSample
    } })
  manager.configure(checkedProfiles); manager.setStreamEpoch(7)
  const expires = performance.now() + 10000
  while (samples.some(sample => sample.state() !== 'loaded') && performance.now() < expires) await pause(50)
  assert(samples.every(sample => sample.state() === 'loaded'), 'HTML5 Audio preload failed')
  const receiver = new WorldAudioReceiver(manager, () => serverNow, () => ({ x: 0, y: 0 }))
  receiver.configure(7, { hearing: 1, freshnessMs: 500 })
  let worldPlayback: (typeof playEvents)[number] | undefined
  const producerPlayback: Array<{ tick: number; key: string; serverTimeMs: number; gain: number; id: number; volume: number; playing: boolean }> = []
  if (!attenuationOnly.checked) {
    const worldPacket = proto.ServerMessage.decode(proto.ServerMessage.encode({ soundBatch: { streamEpoch: 7, serverTimeMs: serverNow,
      sounds: [{ soundKey: 'chop', x: 800, y: 0, maxHearDistance: 1000, distanceGain: .104 }] } }).finish())
    receiver.batch(worldPacket.soundBatch!)
    assert(manager.metrics.played === 1, 'Invisible source packet did not play')
    receiver.batch({ streamEpoch: 6, serverTimeMs: serverNow, sounds: [{ soundKey: 'chop', distanceGain: 1 }] })
    receiver.batch({ streamEpoch: 7, serverTimeMs: serverNow - 501, sounds: [{ soundKey: 'chop', distanceGain: 1 }] })
    receiver.legacy({ soundKey: 'exp_gain', x: 0, y: 0, maxHearDistance: 80 })
    assert(manager.metrics.played === 1, 'Stale/old-stream/local server sound was duplicated')
    await pause(250)
    worldPlayback = playEvents.find(event => event.file.includes('/chop/'))!
    assert(worldPlayback.playing, 'Invisible-source chop did not start native playback')
    receiver.batch(proto.S2C_SoundBatch.decode(proto.S2C_SoundBatch.encode({ streamEpoch: 7, serverTimeMs: serverNow,
      sounds: [{ soundKey: 'tree_fall', x: 800, y: 0, maxHearDistance: 1400, distanceGain: .3 }] }).finish()))
    await pause(100)
    assert(playEvents.some(event => event.file.includes('/tree_fall/') && event.playing), 'Decoded final fall feedback did not start')
    if (serverFixture) {
      const entry = decodePacket(serverFixture.enter_world_base64).playerEnterWorld
      assert(entry?.streamEpoch && entry.audio && entry.tickRate === 10, 'Server entry lacks stream/audio parameters')
      receiver.configure(entry.streamEpoch, entry.audio)
      for (const emission of serverFixture.emissions) {
        const batch = decodePacket(emission.packet_base64).soundBatch
        assert(batch?.sounds?.length === 1 && batch.streamEpoch === entry.streamEpoch, 'Server fixture batch has the wrong stream')
        const sound = batch.sounds[0]!
        assert(sound.soundKey === emission.sound_key && Object.hasOwn(sound, 'distanceGain') &&
          Math.abs(sound.distanceGain! - emission.distance_gain) < 1e-6, 'Captured server gain/key changed during decoding')
        serverNow = Number(batch.serverTimeMs)
        const before = playEvents.length
        receiver.batch(batch)
        await pause(100)
        const event = playEvents.slice(before).find(event => profiles[emission.sound_key]!.files.some(file => event.file.endsWith(file)))
        assert(event, `Captured ${emission.sound_key} packet did not start native playback`)
        const sample = samplesByFile.get(event.file)!, volume = Number(sample.volume(event.id)), playing = sample.playing(event.id)
        assert(playing && Math.abs(volume - .5 * profiles[emission.sound_key]!.volume * sound.distanceGain!) < 1e-6, 'Captured server packet lost its per-ID gain')
        producerPlayback.push({ tick: emission.tick, key: emission.sound_key, serverTimeMs: serverNow, gain: sound.distanceGain!, id: event.id, volume, playing })
        manager.reset()
      }
      assert(producerPlayback.filter(event => event.key === 'chop').length === 2 && producerPlayback.filter(event => event.key === 'tree_fall').length === 1, 'Captured server cycles were duplicated or lost')
    }
  }
  manager.reset()
  const controller = new LocalAudioController(manager)
  controller.configure(catalog.locomotionAudio!, catalog.actionAnimations)
  controller.setListener(1, 1); controller.setListenerPosition({ x: 0, y: 0 })
  const walk = catalog.locomotionAudio!.find(binding => binding.clip === 'walk')!
  const stride = walk.cycle_distance_tiles!
  const distances = attenuationOnly.checked ? [0, 0, 8, 68, 80, 159, 160] : [0, 0, 60, 119]
  const snapshots: LocalAudioSnapshot[] = distances.map((distance, index) => ({ entityId: index + 1, actor: walk.actor, position: { x: distance, y: 0 }, ready: true, moving: true, clip: 'walk', distanceTiles: 0, discontinuity: false, action: null }))
  const stepProfile = profiles.footstep!
  assert(stepProfile.local_attenuation?.near_gain === .9 && stepProfile.local_attenuation.far_gain === 0 && stepProfile.local_attenuation.near_distance === undefined,
    'Published footsteps must fade continuously from 90% to zero at the radius')
  const distanceSamples = distances.map((distance, index) => ({ distance, ownSource: index === 0,
    gain: localDistanceGain(stepProfile, 1, distance, index === 0) }))
  const audibleSamples = distanceSamples.filter(sample => sample.gain > 0)
  const contactsBefore = playEvents.length
  for (const snapshot of snapshots) controller.update(snapshot, 0, serverNow)
  for (const snapshot of snapshots) { snapshot.distanceTiles = stride * .5; controller.update(snapshot, 100, serverNow) }
  await pause(100)
  const ownNearFar = playEvents.slice(contactsBefore).filter(event => event.file.includes('/steps/')).map(event => ({ ...event,
    volume: Number(samplesByFile.get(event.file)!.volume(event.id)), playing: samplesByFile.get(event.file)!.playing(event.id) }))
  assert(ownNearFar.length === audibleSamples.length, 'Audible footsteps or silent radius boundary were incorrect')
  const expectedVolumes = audibleSamples.map(sample => .5 * stepProfile.volume * sample.gain)
  const sortedVolumes = ownNearFar.map(event => event.volume).sort((left, right) => left - right)
  expectedVolumes.sort((left, right) => left - right)
  sortedVolumes.forEach((volume, index) => assert(Math.abs(volume - expectedVolumes[index]!) < 1e-6, `Incorrect local per-ID gains: ${JSON.stringify(ownNearFar)}`))
  assert(ownNearFar.every(event => event.playing), 'Native step instance was not playing')
  const assignedNativeIds = new Set<number>()
  const attenuationSamples = distanceSamples.map(sample => {
    const expectedVolume = .5 * stepProfile.volume * sample.gain
    const native = sample.gain > 0 ? ownNearFar.find(event => !assignedNativeIds.has(event.id) && Math.abs(event.volume - expectedVolume) < 1e-6) : undefined
    assert(sample.gain === 0 || native, 'No native playback ID matched the expected distance volume')
    if (native) assignedNativeIds.add(native.id)
    return { ...sample, expectedVolume, nativeId: native?.id, nativeVolume: native?.volume ?? 0, playing: native?.playing ?? false }
  })
  assert(attenuationSamples[0]!.gain === 1 && attenuationSamples[1]!.gain === .9, 'Own/other distinction was lost')
  if (attenuationOnly.checked) {
    assert(Math.abs(attenuationSamples[2]!.gain - .7980455226965595) < 1e-9, 'Fade must begin immediately for the authored curve')
    assert(Math.abs(attenuationSamples[3]!.gain - .34457217715389343) < 1e-9, 'Mid-fade gain drifted')
    assert(Math.abs(attenuationSamples[4]!.gain - .28565442496) < 1e-10, 'Fade midpoint did not use the authored log curve')
    for (let index = 2; index < attenuationSamples.length - 1; index++) assert(attenuationSamples[index]!.gain < attenuationSamples[index - 1]!.gain, 'Footsteps must fade monotonically')
    assert(attenuationSamples.at(-1)!.gain === 0, 'Radius boundary was lost')
    const entering: LocalAudioSnapshot = { ...snapshots[1]!, entityId: 999, position: { x: 160, y: 0 }, distanceTiles: stride * .3 }
    const beforeEntry = playEvents.length, playedBeforeEntry = manager.metrics.played
    controller.update(entering, 0, serverNow)
    entering.position.x = 159; entering.distanceTiles = stride * .99
    controller.update(entering, 100, serverNow)
    assert(playEvents.length === beforeEntry && manager.metrics.played === playedBeforeEntry, 'Radius entry replayed an old contact')
    entering.distanceTiles = stride * 1.5; controller.update(entering, 200, serverNow)
    await pause(100)
    const entryPlayback = playEvents.slice(beforeEntry).find(event => event.file.includes('/steps/'))
    assert(entryPlayback && manager.metrics.played === playedBeforeEntry + 1, 'Next real contact after radius entry did not play')
    const nativeVolume = Number(samplesByFile.get(entryPlayback.file)!.volume(entryPlayback.id))
    const expectedVolume = .5 * stepProfile.volume * localDistanceGain(stepProfile, 1, 159, false)
    assert(Math.abs(nativeVolume - expectedVolume) < 1e-6, 'Radius re-entry lost the distance gain')
    manager.reset(); samples.forEach(sample => sample.unload()); await context.close()
    assert(failures.length === 0, 'Browser reported runtime/audio errors')
    report({ status: 'PASSED', mode: 'footstep-attenuation', browser: navigator.userAgent, decodedSampleCount: decoded.length,
      profile: { loudness: stepProfile.loudness, volume: stepProfile.volume, curve: stepProfile.local_attenuation },
      attenuationSamples, radiusReentry: { oldContactsReplayed: 0, genuineContactsPlayed: 1, nativeId: entryPlayback.id, nativeVolume }, failures,
      listening: 'Native playback IDs and volumes verified; subjective listening is not claimed.' })
    return
  }
  const beforeClip = manager.metrics.played
  for (const snapshot of snapshots) { snapshot.clip = 'carry_walk'; snapshot.distanceTiles = stride * .99; controller.update(snapshot, 200, serverNow) }
  assert(manager.metrics.played === beforeClip, 'Carry transition replayed missed contacts')
  for (const snapshot of snapshots) { snapshot.distanceTiles = stride * 1.5; controller.update(snapshot, 300, serverNow) }
  await pause(100)
  assert(manager.metrics.played === beforeClip + audibleSamples.length,
    `Carry-walk contacts did not follow manifest stride: before=${beforeClip}, expected=${audibleSamples.length}, metrics=${JSON.stringify(manager.metrics)}, active=${manager.activeVoices}, attenuation=${JSON.stringify(attenuationSamples)}`)
  const beforePause = manager.metrics.played
  for (const snapshot of snapshots) { snapshot.distanceTiles = stride * 2; controller.update(snapshot, 1000, serverNow) }
  assert(manager.metrics.played === beforePause, 'Presentation pause replayed missed contacts')
  manager.reset()
  const surfacePlayback: Array<{ tileType?: number; soundKey: string; file: string }> = []
  const surfaceCases: Array<[number | undefined, string]> = [
    [undefined, 'footstep'], [TILE_SHALLOW_WATER, 'footstep_shallow_water'], [TILE_MOUNTAIN, 'footstep_stone'],
    [TILE_DIRT, 'footstep_gravel'], [TILE_CLAY, 'footstep_gravel'], [TILE_PLOWED, 'footstep_gravel'],
    [TILE_CONIFEROUS_FOREST, 'footstep_forest_leaves'], [TILE_BROADLEAF_FOREST, 'footstep_forest_leaves'],
    [254, 'footstep'],
  ]
  controller.reset(); controller.setListenerPosition({ x: 0, y: 0 })
  const surfaceWalker: LocalAudioSnapshot = { ...snapshots[0]!, clip: 'walk', distanceTiles: 0 }
  controller.update(surfaceWalker, 0, serverNow)
  for (const [index, [tileType, soundKey]] of surfaceCases.entries()) {
    manager.reset()
    const before = playEvents.length
    surfaceWalker.tileType = tileType
    surfaceWalker.distanceTiles = stride * (index + 1) * .5
    controller.update(surfaceWalker, (index + 1) * 100, serverNow)
    await pause(50)
    const events = playEvents.slice(before)
    assert(events.length === 1 && profiles[soundKey]!.files.some(file => events[0]!.file.endsWith(file)),
      `Tile ${tileType ?? 'unknown'} did not play the selected ${soundKey} recording exactly once`)
    surfacePlayback.push({ tileType, soundKey, file: events[0]!.file })
  }
  manager.reset()
  // Exercise the exact same decoded sample twice with different independent IDs.
  manager.play('exp_gain', 1, { sourceId: 'own' }); manager.play('exp_gain', .83, { sourceId: 'other' })
  await pause(100)
  const feedbackVoices = playEvents.filter(event => event.file.endsWith('/exp_gain.mp3')).slice(-2).map(event => ({ ...event,
    volume: Number(samplesByFile.get(event.file)!.volume(event.id)), playing: samplesByFile.get(event.file)!.playing(event.id) }))
  assert(feedbackVoices.length === 2 && feedbackVoices[0]!.id !== feedbackVoices[1]!.id, 'Same-sample voices did not get independent IDs')
  const feedbackVolumes = feedbackVoices.map(event => event.volume).sort((left, right) => left - right)
  assert(Math.abs(feedbackVolumes[0]! - .166) < 1e-6 && Math.abs(feedbackVolumes[1]! - .2) < 1e-6, `Same sample voice gains interfered: ${JSON.stringify(feedbackVoices)}`)
  manager.reset()
  let socketRequests = 0, sentPackets = 0
  const originalWebSocket = globalThis.WebSocket
  const originalSend = originalWebSocket.prototype.send
  originalWebSocket.prototype.send = function () { sentPackets++; throw new Error('Local audio attempted sending a packet') }
  globalThis.WebSocket = class { constructor() { socketRequests++; throw new Error('Local audio attempted networking') } } as unknown as typeof WebSocket
  const started = performance.now(), metricsBeforeWorkload = { ...manager.metrics }
  try {
    controller.reset(); controller.setListener(1, 1); controller.setListenerPosition({ x: 0, y: 0 })
    for (let frame = 0; frame < 100; frame++) for (let entityId = 1; entityId <= 200; entityId++) {
      controller.update({ entityId, actor: walk.actor, position: { x: entityId % 60, y: 0 }, ready: true, moving: true, clip: 'walk', distanceTiles: stride * frame * .1, discontinuity: false, action: null }, frame * 16, serverNow)
    }
  } finally { globalThis.WebSocket = originalWebSocket; originalWebSocket.prototype.send = originalSend }
  const metrics = Object.fromEntries(Object.entries(manager.metrics).map(([key, value]) => [key, value - metricsBeforeWorkload[key as keyof typeof manager.metrics]]))
  const workload = { knownCharacters: 200, frames: 100, contactUpdates: 20000, milliseconds: performance.now() - started, metrics, activeVoices: manager.activeVoices, webSocketCreations: socketRequests, webSocketSendCalls: sentPackets, extraAudioOrMovementPackets: sentPackets }
  assert(socketRequests === 0 && sentPackets === 0 && manager.activeVoices <= 32, 'Local workload violated networking or voice budget')
  manager.reset()
  const renderedPreview = renderVariants.checked ? await verifyRenderedVariants(catalog, manager, serverNow) : undefined
  await pause(250)
  assert(failures.length === 0, 'Browser reported runtime/audio errors')
  samples.forEach(sample => sample.unload())
  await context.close()
  report({ status: 'PASSED', browser: navigator.userAgent, decodedSampleCount: decoded.length, decoded, ownNearFar, sameSampleIndependentVoices: feedbackVoices,
    world: { sourceX: 800, visibilityRadius: 600, sourceObjectsKnown: 0, authoritativeGain: .104, nativeVolume: worldPlayback?.volume, finalFallPlayed: true, streamDrops: receiver.metrics.wrongStream, staleDrops: receiver.metrics.stale, localPacketDrops: receiver.metrics.localPacket },
    chop: { handClips, sharedCue: chopBinding.sound_cues![0], poseRenderingChecked: false },
    capturedServer: serverFixture ? { expected: serverFixture.expected, sourceObjectsKnown: 0, playback: producerPlayback } : undefined,
    renderedPreview,
    localDistances: distances, attenuationSamples, surfacePlayback,
    locomotion: { strideTiles: stride, contacts: walk.contacts, walkAndCarryPlayed: true, transitionAndPauseCatchUp: false }, workload, failures,
    listening: 'Native browser playback and volumes verified. Subjective listening/tuning requires a human ear; no subjective claim is made.' })
}
