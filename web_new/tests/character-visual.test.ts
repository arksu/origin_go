/// <reference types="node" />
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { AnimationClip, Bone, BoxGeometry, Group, Mesh, MeshStandardMaterial, ShaderMaterial, Vector3, VectorKeyframeTrack } from 'three'
import type { GLTF } from 'three/addons/loaders/GLTFLoader.js'
import { proto } from '../src/network/proto/packets.js'
import { decodeCharacterVisual } from '../src/types/characterVisual'
import { useGameStore } from '../src/stores/gameStore'
import { ActorInstance } from '../src/game/actors/ActorInstance'
import { ActorArmLayers } from '../src/game/actors/ActorArmLayers'
import { ActorSockets } from '../src/game/actors/ActorSockets'
import { ACTOR_RENDER, COMMONER_MODEL, DEFAULT_ACTOR_RENDER_SETTINGS, resolveActorRenderSettings } from '../src/game/actors/config'
import { actorYawForScreenAngle } from '../src/game/actors/facing'
import { ACTOR_RENDER_MODE_STORAGE_KEY, loadActorRenderMode, persistActorRenderMode } from '../src/composables/useActorRenderSettings'
import { RENDER_DEBUG_STORAGE_KEY, loadRenderDebugEnabled, persistRenderDebugEnabled } from '../src/composables/useRenderDebugSettings'
import type { EquipmentDefinition } from '../src/game/actors/equipment'

function visual(revision: string, generation = '0:4294967297') {
  return decodeCharacterVisual(proto.CharacterVisualState.fromObject({ generation, revision,
    equipment: [{ slot: proto.EquipSlot.EQUIP_SLOT_RIGHT_HAND, visualKey: 'stone_axe' }],
  }))
}

test('public snapshot round-trips uint64 and rejects malformed slots, keys and revisions', () => {
  const state = proto.CharacterVisualState.fromObject({ generation: '0:4294967297', revision: '18446744073709551615',
    equipment: [{ slot: 6, visualKey: 'stone_axe' }, { slot: 7, visualKey: 'stone_axe' }],
  })
  const decoded = decodeCharacterVisual(proto.CharacterVisualState.decode(proto.CharacterVisualState.encode(state).finish()))
  assert.equal(decoded.revision, '18446744073709551615')
  assert.deepEqual(decoded.equipment.map((item) => item.slot), ['left_hand', 'right_hand'])
  assert.throws(() => decodeCharacterVisual({ ...state, revision: Number.MAX_SAFE_INTEGER + 1 }))
  assert.throws(() => decodeCharacterVisual({ ...state, generation: 'invalid' }))
  assert.throws(() => decodeCharacterVisual({ ...state, equipment: [{ slot: 0, visualKey: 'axe' }] }))
  assert.throws(() => decodeCharacterVisual({ ...state, equipment: [{ slot: 7, visualKey: '../../axe' }] }))
  assert.throws(() => decodeCharacterVisual({ ...state, equipment: [{ slot: 7, visualKey: 'axe' }, { slot: 7, visualKey: 'sword' }] }))
})

test('actor render settings expose validated local and remote update rates', () => {
  assert.deepEqual(DEFAULT_ACTOR_RENDER_SETTINGS, {
    mode: 'hybrid3d',
    localAnimationFps: 60,
    remoteAnimationFps: 20,
    turnDurationMs: 500,
    renderStationaryChangesImmediately: true,
  })
  assert.deepEqual(resolveActorRenderSettings({ remoteAnimationFps: 12 }), {
    mode: 'hybrid3d',
    localAnimationFps: 60,
    remoteAnimationFps: 12,
    turnDurationMs: 500,
    renderStationaryChangesImmediately: true,
  })
  assert.throws(() => resolveActorRenderSettings({ localAnimationFps: 0 }))
  assert.throws(() => resolveActorRenderSettings({ mode: 'sprite8' as 'hybrid3d' }))
  assert.throws(() => resolveActorRenderSettings({ turnDurationMs: 0 }))
  assert.throws(() => resolveActorRenderSettings({ renderStationaryChangesImmediately: 'yes' as unknown as boolean }))
})

test('actor render mode preference persists only supported values', () => {
  const values = new Map<string, string>()
  const storage = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
  }

  assert.equal(loadActorRenderMode(storage), 'hybrid3d')
  values.set(ACTOR_RENDER_MODE_STORAGE_KEY, 'baked8')
  assert.equal(loadActorRenderMode(storage), 'baked8')
  values.set(ACTOR_RENDER_MODE_STORAGE_KEY, 'unsupported')
  assert.equal(loadActorRenderMode(storage), 'hybrid3d')
  persistActorRenderMode('hybrid3d', storage)
  assert.equal(values.get(ACTOR_RENDER_MODE_STORAGE_KEY), 'hybrid3d')
  assert.equal(loadActorRenderMode(null), 'hybrid3d')
  assert.doesNotThrow(() => persistActorRenderMode('baked8', null))
})

test('render debug preference persists only strict boolean values', () => {
  const values = new Map<string, string>()
  const storage = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
  }

  assert.equal(loadRenderDebugEnabled(storage), false)
  values.set(RENDER_DEBUG_STORAGE_KEY, 'true')
  assert.equal(loadRenderDebugEnabled(storage), true)
  values.set(RENDER_DEBUG_STORAGE_KEY, 'false')
  assert.equal(loadRenderDebugEnabled(storage, true), false)
  values.set(RENDER_DEBUG_STORAGE_KEY, 'enabled')
  assert.equal(loadRenderDebugEnabled(storage, true), true)
  persistRenderDebugEnabled(true, storage)
  assert.equal(values.get(RENDER_DEBUG_STORAGE_KEY), 'true')
  assert.equal(loadRenderDebugEnabled(null), false)
  assert.doesNotThrow(() => persistRenderDebugEnabled(false, null))
})

test('store accepts only a newer revision of the current incarnation and cannot resurrect despawned actors', () => {
  setActivePinia(createPinia())
  const store = useGameStore()
  const original = visual('9007199254740992')
  assert.equal(store.updateCharacterVisual(101, original), false)
  const entity = { entityId: 101, typeId: 1, resourcePath: 'player', position: { x: 10, y: 20 }, size: { x: 4, y: 4 }, characterVisual: original }
  store.spawnEntity(entity)
  const canonical = store.entities.get(101)
  assert.equal(store.updateCharacterVisual(101, original), false)
  assert.equal(store.updateCharacterVisual(101, visual('9007199254740991')), false)
  assert.equal(store.updateCharacterVisual(101, visual('9007199254740993')), true)
  assert.equal(store.entities.get(101), canonical, 'equipment must not replace the entity or movement state')
  assert.equal(canonical!.characterVisual!.revision, '9007199254740993')
  assert.equal(store.updateCharacterVisual(101, visual('9007199254740994', '1:4294967297')), false)
  store.despawnEntity(101)
  assert.equal(store.updateCharacterVisual(101, visual('9007199254740994')), false)
  store.spawnEntity({ ...entity, characterVisual: visual('1', '0:8589934593') })
  assert.equal(store.updateCharacterVisual(101, visual('9007199254740995')), false)
  store.setPlayerLeaveWorld()
  assert.equal(store.updateCharacterVisual(101, visual('2', '0:8589934593')), false)
})

// Synthetic test fixtures only: production meshes and animation files are not
// modified or generated by phase 1.
function fixtureRig() {
  const scene = new Group()
  const bones: Record<string, Bone> = {}
  function bone(name: string, parent: Group | Bone) {
    const joint = new Bone()
    joint.name = name
    bones[name] = joint
    parent.add(joint)
    return joint
  }
  const pelvis = bone('pelvis', scene)
  bone('leg', pelvis)
  for (const side of ['l', 'r']) {
    const clavicle = bone(`clavicle${side}`, pelvis)
    const forearm = bone(`forearm${side}`, clavicle)
    bone(`hand${side}`, forearm)
  }
  const clip = (name: string, factor: number) => new AnimationClip(name, 1, Object.keys(bones).map((name, index) =>
    new VectorKeyframeTrack(`${name}.position`, [0, 1], [factor * (index + 1), 0, 0, factor * (index + 2), 0, 0])))
  const animations = [clip('idle', 0), clip('walk', 1), clip('carry_idle', 2), clip('carry_walk', 3), clip('hold', 10)]
  return { scene, animations, bones }
}

function gltf(scene: Group, animations: AnimationClip[] = []): GLTF {
  return { scene, scenes: [scene], animations, cameras: [], asset: { version: '2.0' }, userData: {}, parser: {} } as unknown as GLTF
}

function prop() {
  const scene = new Group()
  const mesh = new Mesh(new BoxGeometry(.1, .2, .1), new MeshStandardMaterial())
  mesh.name = 'test_prop'
  scene.add(mesh)
  return gltf(scene)
}

class FixtureCache {
  readonly assets = new Map<string, GLTF>()
  readonly pending = new Map<string, Promise<GLTF>>()
  readonly references = new Map<string, number>()
  readonly calls: string[] = []
  async acquire(url: string) {
    this.calls.push(url)
    const asset = await (this.pending.get(url) ?? Promise.resolve(this.assets.get(url)))
    if (!asset) throw new Error(`Fixture asset missing: ${url}`)
    this.references.set(url, (this.references.get(url) ?? 0) + 1)
    let released = false
    return { asset, release: () => {
      if (released) return
      released = true
      this.references.set(url, this.references.get(url)! - 1)
    } }
  }
  get liveReferences() { return [...this.references.values()].reduce((total, count) => total + count, 0) }
}

const axeURL = '/assets/game/equipment/test_axe.glb'
const shieldURL = '/assets/game/equipment/test_shield.glb'
const catalog: Record<string, EquipmentDefinition> = {
  stone_axe: { kind: 'rigid', url: axeURL, bindings: {
    right_hand: { socket: 'grip_r', armMotion: { kind: 'layered', idlePose: 'hold', walkPose: 'hold' }, transform: { position: [.1, 0, 0], scale: .5 } },
    left_hand: { socket: 'grip_l', armMotion: { kind: 'layered', idlePose: 'hold' } },
  } },
  shield: { kind: 'rigid', url: shieldURL, bindings: { left_hand: { socket: 'forearm_l', armMotion: { kind: 'layered', idlePose: 'hold' } } } },
  deferred: { kind: 'deferred' },
}

async function fixtureActor() {
  const cache = new FixtureCache()
  const rig = fixtureRig()
  cache.assets.set(COMMONER_MODEL, gltf(rig.scene, rig.animations))
  cache.assets.set(axeURL, prop())
  cache.assets.set(shieldURL, prop())
  const actor = new ActorInstance(cache, catalog)
  await actor.ready
  return { actor, cache }
}

test('distance-driven walk samples the 3D clip continuously', async () => {
  const { actor } = await fixtureActor()
  actor.walking = true
  actor.distanceTiles = ACTOR_RENDER.cycleDistanceTiles * .10
  actor.updatePose(10)
  const first = actor.root.getObjectByName('pelvis')!.position.x
  actor.distanceTiles = ACTOR_RENDER.cycleDistanceTiles * .11
  actor.updatePose(20)
  const second = actor.root.getObjectByName('pelvis')!.position.x
  assert.notEqual(second, first, 'a sub-eighth stride movement must update the skeletal pose')
  actor.destroy()
})

test('skeletal weights start in 500ms, stop in 300ms and reverse continuously', async () => {
  const { actor } = await fixtureActor()
  try {
    const pelvis = actor.root.getObjectByName('pelvis')!
    const now = Math.ceil(performance.now())
    actor.distanceTiles = ACTOR_RENDER.cycleDistanceTiles * .5
    actor.walking = true
    actor.updatePose(now)
    assert.equal(pelvis.position.x, 0, 'start must retain idle at zero weight')
    actor.updatePose(now + 250)
    assert.equal(pelvis.position.x, .75)
    actor.updatePose(now + 500)
    assert.equal(pelvis.position.x, 1.5)
    actor.walking = false
    actor.updatePose(now + 600)
    assert.equal(pelvis.position.x, 1.5, 'stop must retain the outgoing walk phase')
    actor.updatePose(now + 750)
    assert.equal(pelvis.position.x, .75)
    actor.distanceTiles = 0 // ObjectView resets accumulated distance on stop.
    actor.walking = true
    actor.updatePose(now + 750)
    assert.equal(pelvis.position.x, .75, 'reversal must preserve the current weights')
    actor.updatePose(now + 1000)
    assert.equal(pelvis.position.x, 1.125)
    actor.updatePose(now + 1250)
    assert.equal(pelvis.position.x, 1.5)
    actor.walking = false
    actor.updatePose(now + 1400)
    actor.updatePose(now + 1700)
    assert.equal(pelvis.position.x, 0)
    assert.equal(actor.updatePose(now + 1701), false, 'settled idle must remain cacheable')
  } finally { actor.destroy() }
})

test('position stop progress drives skeletal weights without another idle tail', async () => {
  const { actor } = await fixtureActor()
  try {
    const now = Math.ceil(performance.now())
    const pelvis = actor.root.getObjectByName('pelvis')!
    actor.walking = true
    actor.updatePose(now)
    actor.updatePose(now + 500)
    actor.stopProgress = 0
    actor.updatePose(now + 600)
    assert.equal(pelvis.position.x, 1)
    actor.stopProgress = .5
    actor.updatePose(now + 750)
    assert.equal(pelvis.position.x, .5)
    actor.stopProgress = 1
    actor.walking = false
    actor.updatePose(now + 900)
    assert.equal(pelvis.position.x, 0)
    assert.equal(actor.updatePose(now + 901), false)
  } finally { actor.destroy() }
})

test('baked8 starts immediately and holds its walk frame while position is still settling', async () => {
  const { actor } = await fixtureActor()
  try {
    const now = Math.ceil(performance.now())
    const settings = { ...DEFAULT_ACTOR_RENDER_SETTINGS, mode: 'baked8' as const }
    const pelvis = actor.root.getObjectByName('pelvis')!
    actor.walking = true
    actor.distanceTiles = ACTOR_RENDER.cycleDistanceTiles * .25
    actor.updatePose(now, settings)
    assert.notEqual(pelvis.position.x, 0, 'baked8 must not blend the start of the walk pose')

    actor.stopProgress = 0
    assert.equal(actor.updatePose(now + 100, settings), false, 'the initial stop must keep the existing baked frame')
    const heldPose = pelvis.position.x
    const heldRevision = actor.revision

    actor.stopProgress = .5
    assert.equal(actor.updatePose(now + 250, settings), false, 'intermediate stop progress must not blend a new pose')
    assert.equal(actor.revision, heldRevision)
    assert.equal(pelvis.position.x, heldPose)

    actor.walking = false
    actor.stopProgress = 1
    assert.equal(actor.updatePose(now + 400, settings), true, 'completed stop must produce the idle pose')
    assert.equal(pelvis.position.x, 0)
  } finally { actor.destroy() }
})

test('hybrid mode turns continuously while baked8 rounds Three.js output to eight angles', async () => {
  const { actor } = await fixtureActor()
  const angularDistance = (first: number, second: number) => Math.abs(Math.atan2(Math.sin(first - second), Math.cos(first - second)))
  actor.setFacingAngle(-Math.PI / 2)
  actor.updatePose(0, DEFAULT_ACTOR_RENDER_SETTINGS)
  actor.updatePose(250, DEFAULT_ACTOR_RENDER_SETTINGS)
  assert.ok(angularDistance(actor.root.rotation.y, actorYawForScreenAngle(0)) < 1e-6,
    'a 180-degree target must be halfway through its turn after 250ms')
  actor.setFacingAngle(Math.PI / 10)
  actor.updatePose(300, { ...DEFAULT_ACTOR_RENDER_SETTINGS, mode: 'baked8' })
  assert.ok(angularDistance(actor.root.rotation.y, actorYawForScreenAngle(0)) < 1e-6,
    'baked8 must round the current Three.js bake to the nearest eight-direction angle')
  actor.destroy()
})

test('sockets retain authored transforms and follow their hand or forearm bone', () => {
  const rig = fixtureRig()
  const authored = new Group()
  authored.name = 'grip_l'
  authored.position.set(1, 2, 3)
  rig.bones.handl!.add(authored)
  const sockets = new ActorSockets(rig.scene)
  assert.equal(sockets.get('grip_l'), authored)
  assert.equal(sockets.get('grip_r').parent, rig.bones.handr)
  assert.equal(sockets.get('forearm_l').parent, rig.bones.forearml)
  rig.bones.handl!.position.set(4, 5, 6)
  rig.scene.updateMatrixWorld(true)
  assert.deepEqual(authored.getWorldPosition(new Vector3()).toArray(), [5, 7, 9])
})

test('independent masks survive repeated locomotion writes, exclude torso/legs and fade out', () => {
  const rig = fixtureRig()
  const layers = new ActorArmLayers(rig.scene, rig.animations, 100)
  layers.setPose('right', 'hold', 0, 0)
  layers.apply(100)
  const right = rig.bones.handr!.position.x
  assert.ok(right > 0)
  for (let frame = 0; frame < 5; frame++) {
    for (const bone of Object.values(rig.bones)) bone.position.x = frame + 1
    layers.apply(200 + frame)
    assert.equal(rig.bones.handr!.position.x, right, 'constant held pose must be applied every frame')
    assert.equal(rig.bones.handl!.position.x, frame + 1)
    assert.equal(rig.bones.pelvis!.position.x, frame + 1)
    assert.equal(rig.bones.leg!.position.x, frame + 1)
  }
  layers.setPose('left', 'hold', .5, 300)
  layers.apply(400)
  assert.ok(rig.bones.handl!.position.x > 5)
  assert.equal(rig.bones.handr!.position.x, right)
  layers.setPose('right', null, 0, 400)
  rig.bones.handr!.position.x = 2
  layers.apply(450)
  assert.equal(rig.bones.handr!.position.x, (right + 2) / 2)
  rig.bones.handr!.position.x = 3
  layers.apply(500)
  assert.equal(rig.bones.handr!.position.x, 3)
  assert.equal(layers.transitioning, false)
  assert.throws(() => layers.setPose('left', 'missing'))
  assert.throws(() => layers.setPose('left', 'hold', -1))
  layers.destroy()
})

test('rigid equipment supports duplicate assets in separate slots, shield socket, carry priority and cleanup', async () => {
  const { actor, cache } = await fixtureActor()
  let sourceMaterialDisposals = 0
  const sourceMesh = cache.assets.get(axeURL)!.scene.getObjectByName('test_prop') as Mesh
  ;(sourceMesh.material as MeshStandardMaterial).addEventListener('dispose', () => sourceMaterialDisposals++)
  try {
    await actor.setEquipment([{ slot: 'left_hand', visualKey: 'stone_axe' }, { slot: 'right_hand', visualKey: 'stone_axe' }])
    const left = actor.root.getObjectByName('grip_l')!
    const right = actor.root.getObjectByName('grip_r')!
    assert.equal(left.children.length, 1)
    assert.equal(right.children.length, 1)
    assert.notEqual(left.children[0], right.children[0])
    assert.equal(cache.references.get(axeURL), 2)
    const mesh = right.getObjectByName('test_prop') as Mesh
    assert.equal(mesh.geometry, sourceMesh.geometry, 'immutable geometry is shared')
    assert.notEqual(mesh.material, sourceMesh.material, 'actor owns only its pixel material')
    assert.equal((mesh.material as ShaderMaterial).defines.ACTOR_RIGID, 1)
    assert.equal(right.children[0]!.scale.x, .5)
    let disposed = 0
    ;(mesh.material as ShaderMaterial).addEventListener('dispose', () => disposed++)
    actor.carrying = true
    actor.updatePose(performance.now() + 500)
    assert.equal(left.children[0]!.visible, false)
    assert.equal(right.children[0]!.visible, false)
    actor.carrying = false
    actor.updatePose(performance.now() + 600)
    assert.equal(right.children[0]!.visible, true)
    await actor.setEquipment([{ slot: 'left_hand', visualKey: 'shield' }, { slot: 'right_hand', visualKey: 'stone_axe' }])
    assert.equal(left.children.length, 0)
    assert.equal(actor.root.getObjectByName('forearm_l')!.children.length, 1)
    assert.equal(disposed, 1)
    assert.equal(cache.references.get(axeURL), 1)
    await actor.setEquipment([{ slot: 'right_hand', visualKey: 'future_sword' }])
    assert.equal(right.children.length, 0)
    assert.equal(cache.liveReferences, 1)
    assert.deepEqual(actor.equippedVisuals, [{ slot: 'right_hand', visualKey: 'future_sword' }])
    const calls = cache.calls.length
    await actor.setEquipment([{ slot: 'left_hand', visualKey: 'deferred' }])
    assert.equal(cache.calls.length, calls, 'no nonexistent phase-2 asset request')
  } finally { actor.destroy() }
  assert.equal(cache.liveReferences, 0)
  assert.equal(sourceMaterialDisposals, 0, 'shared cache resources must not be disposed by actors')
})

test('failed or superseded loads are atomic and do not leak or revive old equipment', async () => {
  const { actor, cache } = await fixtureActor()
  try {
    await actor.setEquipment([{ slot: 'right_hand', visualKey: 'stone_axe' }])
    const right = actor.root.getObjectByName('grip_r')!
    const original = right.children[0]
    await assert.rejects(actor.setEquipment([{ slot: 'right_hand', visualKey: 'shield' }]))
    assert.equal(right.children[0], original)
    cache.assets.delete(shieldURL)
    await assert.rejects(actor.setEquipment([{ slot: 'left_hand', visualKey: 'shield' }, { slot: 'right_hand', visualKey: 'stone_axe' }]))
    assert.equal(right.children[0], original)
    assert.equal(cache.liveReferences, 2)
    let finish!: (asset: GLTF) => void
    cache.pending.set(shieldURL, new Promise((resolve) => { finish = resolve }))
    const older = actor.setEquipment([{ slot: 'left_hand', visualKey: 'shield' }])
    await actor.setEquipment([])
    finish(prop())
    await older
    assert.deepEqual(actor.equippedVisuals, [])
    assert.equal(cache.liveReferences, 1)
    let finishDestroyed!: (asset: GLTF) => void
    cache.pending.set(shieldURL, new Promise((resolve) => { finishDestroyed = resolve }))
    const loading = actor.setEquipment([{ slot: 'left_hand', visualKey: 'shield' }])
    actor.destroy()
    finishDestroyed(prop())
    await loading
    assert.equal(cache.liveReferences, 0)
  } finally { actor.destroy() }
})

test('arm transitions advance while idle; carry keeps base pose and locomotion phase unchanged', async () => {
  const { actor } = await fixtureActor()
  try {
    const hand = actor.root.getObjectByName('handr')!
    actor.setArmPose('right', 'hold')
    const now = performance.now()
    assert.equal(actor.updatePose(now), true)
    const start = hand.position.x
    assert.equal(actor.updatePose(now + 200), true)
    assert.ok(hand.position.x > start)
    assert.equal(actor.updatePose(now + 201), false)
    const held = hand.position.x
    actor.walking = true
    actor.updatePose(now + 250)
    for (const distance of [.15, .4, .9]) {
      actor.distanceTiles = distance
      actor.updatePose(now + 300)
      assert.equal(hand.position.x, held)
      assert.notEqual(actor.root.getObjectByName('leg')!.position.x, 0)
    }
    const distance = actor.distanceTiles
    actor.carrying = true
    actor.updatePose(now + 400)
    assert.notEqual(hand.position.x, held)
    assert.equal(actor.distanceTiles, distance)
    actor.carrying = false
    actor.updatePose(now + 500)
    assert.equal(hand.position.x, held)
    actor.setArmPose('right', null)
    actor.walking = false
    actor.updatePose(now + 600)
    actor.updatePose(now + 1100)
    assert.equal(hand.position.x, 0)
  } finally { actor.destroy() }
})

test('equipment arm sampling follows distance without restarting the transition or touching the free arm', async () => {
  const { actor } = await fixtureActor()
  try {
    await actor.setEquipment([{ slot: 'right_hand', visualKey: 'stone_axe' }])
    const now = performance.now() + 200
    actor.updatePose(now)
    const right = actor.root.getObjectByName('handr')!
    const left = actor.root.getObjectByName('handl')!
    const heldIdle = right.position.x
    actor.walking = true
    const samples: number[] = []
    for (let phase = 0; phase < 8; phase++) {
      actor.distanceTiles = phase / 8 * ACTOR_RENDER.cycleDistanceTiles
      actor.updatePose(now + phase * 30)
      samples.push(right.position.x)
      assert.equal(right.position.x, heldIdle + phase / 8 * 10)
      assert.ok(left.position.x < heldIdle, 'free hand must use base locomotion')
    }
    assert.equal(new Set(samples).size, 8)
    actor.updatePose(now + 1000)
    assert.equal(actor.updatePose(now + 1001), false, 'after blending, wall clock alone must not advance a distance-driven arm')
    actor.walking = false
    actor.updatePose(now + 1100)
    assert.equal(right.position.x, heldIdle)
    await actor.setEquipment([])
    actor.updatePose(now + 1800)
    assert.equal(right.position.x, 0)
  } finally { actor.destroy() }
})
