import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { AnimationClip, Bone, Group, VectorKeyframeTrack } from 'three'
import { DOMAdapter, Sprite, Texture } from 'pixi.js'
import fixture from '../../tests/fixtures/action_animations/bindings.json'
import { parseActionAnimationFile } from '../src/types/actionAnimationDefs'
import { decodeActionAnimation, actionAnimationPhase, acceptActionAnimation } from '../src/types/actionAnimation'
import { ActionAnimationPlayer, type ActionPresentationContext } from '../src/game/actors/ActionAnimationPlayer'
import { ActorActionLayers } from '../src/game/actors/ActorActionLayers'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { gameFacade } from '../src/game/GameFacade'
import { ObjectView } from '../src/game/ObjectView'
import { ResourceLoader } from '../src/game/ResourceLoader'
import type { ActorRenderer } from '../src/game/actors/ActorRenderer'
import { ACTOR_RENDER } from '../src/game/actors/config'

const definitions = parseActionAnimationFile(fixture, 'shared fixture')
const first = definitions[0]!, second = definitions[1]!
const catalog = Object.fromEntries(definitions.map(binding => [binding.key, binding]))
const baseFrame = { width: 128, height: 128, origin_x: 64, origin_y: 116 }
const context: ActionPresentationContext = { stationary: true, carrying: false, knockedOut: false, equipmentReady: true,
  equipment: first.variants.flatMap(variant => variant.equipment.map(item => ({ slot: item.slot, visualKey: item.visual_key }))) }
type WireInput = Omit<proto.ICharacterActionAnimationState, 'revision'> & { revision?: proto.ICharacterActionAnimationState['revision'] | string }
function wire(overrides: WireInput = {}): proto.ICharacterActionAnimationState {
  return proto.CharacterActionAnimationState.fromObject({ generation: '0:4294967297', revision: '9007199254740993', animationKey: first.key,
    totalTicks: 20, elapsedTicks: 5, tickDurationMs: 100, serverTimeMs: 10000, targetPosition: { x: 12, y: -20 }, ...overrides })
}
function state(overrides: WireInput = {}) { return decodeActionAnimation(wire(overrides)) }

test('generic update and spawn preserve uint64, opaque keys, fractional periods and idle', () => {
  for (const animationKey of [first.key, 'future/binding:any', '']) {
    const value = wire({ animationKey, revision: '18446744073709551615', tickDurationMs: 1000 / 60 })
    const packet = proto.ServerMessage.fromObject({ characterActionAnimation: { entityId: 17, streamEpoch: 3, state: value } })
    const received = proto.ServerMessage.decode(proto.ServerMessage.encode(packet).finish()).characterActionAnimation!
    assert.equal(received.streamEpoch, 3)
    assert.equal(decodeActionAnimation(received.state!).revision, '18446744073709551615')
    assert.equal(decodeActionAnimation(received.state!).animationKey, animationKey)
    const spawn = proto.S2C_ObjectSpawn.fromObject({ entityId: 17, actionAnimation: value })
    assert.deepEqual(decodeActionAnimation(proto.S2C_ObjectSpawn.decode(proto.S2C_ObjectSpawn.encode(spawn).finish()).actionAnimation!), decodeActionAnimation(value))
  }
})

test('decoder rejects malformed active timing, identity and coordinates', () => {
  const valid = wire()
  const invalid: proto.ICharacterActionAnimationState[] = [
    { totalTicks: 0 }, { totalTicks: 1.5 }, { totalTicks: 4294967296 }, { elapsedTicks: -1 }, { elapsedTicks: 21 },
    { tickDurationMs: 0 }, { tickDurationMs: NaN }, { tickDurationMs: Infinity }, { tickDurationMs: Number.MAX_SAFE_INTEGER },
    { revision: 0 }, { revision: Number.MAX_SAFE_INTEGER + 1 }, { generation: 'bad' },
    { serverTimeMs: -1 }, { serverTimeMs: Number.MAX_SAFE_INTEGER + 1 },
    { targetPosition: { x: .5 } }, { targetPosition: { x: 2147483648 } }, { targetPosition: { y: NaN } }, { targetPosition: { heading: Infinity } },
    { animationKey: 'bad key' }, { animationKey: 'a'.repeat(129) },
  ]
  for (const override of invalid) assert.throws(() => decodeActionAnimation({ ...valid, ...override }), JSON.stringify(override))
})

test('phase uses authoritative clock at any render rate, corrects delay, and clamps the endpoint', () => {
  const sample = state({ elapsedTicks: 0 })
  for (const fps of [20, 30, 60, 144]) {
    for (let frame = 0; frame <= fps * 2; frame++) {
      const elapsed = frame * 1000 / fps
      assert.ok(Math.abs(actionAnimationPhase(sample, 10000 + elapsed) - elapsed / 2000) < 1e-12)
    }
  }
  assert.equal(actionAnimationPhase(state(), 10500), .5)
  assert.equal(actionAnimationPhase(sample, 9000), 0)
  assert.equal(actionAnimationPhase(sample, 99999), 1)
  assert.equal(actionAnimationPhase(sample, 11000), .5, 'clock correction reanchors without integrating frames')
  const model = new Group(), bone = new Bone(); bone.name = 'joint'; model.add(bone)
  for (const duration of [3, 1]) {
    const clip = new AnimationClip(first.variants[0]!.clip, duration, [new VectorKeyframeTrack('joint.position', [0, duration], [0, 0, 0, duration, 0, 0])])
    const layers = new ActorActionLayers(model, [clip], new Set([clip.name]))
    for (const elapsed of [0, 500, 1000, 2000, 5000]) {
      bone.position.set(0, 0, 0)
      layers.apply([{ clip: clip.name, weight: 1, phase: actionAnimationPhase(sample, 10000 + elapsed) }], false, 8)
      assert.ok(Math.abs(bone.position.x - Math.min(1, elapsed / 2000) * duration) < 1e-6)
    }
    layers.destroy()
  }
})

test('revisions, timestamps and independent appearance updates converge without a backlog', t => {
  setActivePinia(createPinia())
  const store = useGameStore()
  t.after(() => store.reset())
  t.mock.method(gameFacade, 'resetWorld', () => {})
  const spawns = t.mock.method(gameFacade, 'spawnObject', () => {})
  const updates = t.mock.method(gameFacade, 'setActionAnimation', () => {})
  t.mock.method(gameFacade, 'setCharacterEquipment', async () => {})
  const errors = t.mock.method(console, 'error', () => {})
  registerMessageHandlers()
  const dispatch = (packet: proto.IServerMessage) => messageDispatcher.dispatch(proto.ServerMessage.create(packet))
  const enter = (streamEpoch: number) => dispatch({ playerEnterWorld: { entityId: 1, streamEpoch, coordPerTile: 12, chunkSize: 4 } })
  const update = (value = wire(), streamEpoch = 1) => dispatch({ characterActionAnimation: { entityId: 17, streamEpoch, state: value } })
  const spawn = (value: proto.ICharacterActionAnimationState | undefined = wire()) => dispatch({ objectSpawn: { entityId: 17, typeId: 1, resourcePath: 'player', streamEpoch: 1,
    characterVisual: { generation: '0:4294967297', revision: 1 }, actionAnimation: value } })
  enter(1)
  update()
  assert.equal(updates.mock.callCount(), 0)
  spawn()
  const canonical = store.entities.get(17)!
  assert.equal(canonical.actionAnimation!.elapsedTicks, 5)
  for (const [value, epoch] of [[wire(), 2], [wire({ generation: '1:4294967297' }), 1], [wire({ revision: '2' }), 1], [wire({ serverTimeMs: 9999 }), 1]] as const) update(value, epoch)
  assert.equal(updates.mock.callCount(), 0)
  update(wire({ serverTimeMs: 10100, elapsedTicks: 6 }))
  assert.equal(canonical.actionAnimation!.elapsedTicks, 6)
  update(wire({ serverTimeMs: 10200, totalTicks: 21 }))
  assert.equal(errors.mock.callCount(), 1)
  assert.equal(canonical.actionAnimation!.totalTicks, 20)
  spawn(wire({ revision: '9007199254740994', animationKey: '' }))
  assert.equal(canonical.actionAnimation!.animationKey, '')
  assert.equal(spawns.mock.callCount(), 1, 'appearance refresh preserves the render object')
  spawn(wire({ revision: '9007199254740993' }))
  assert.equal(canonical.actionAnimation!.animationKey, '')
  dispatch({ objectSpawn: { entityId: 17, typeId: 1, resourcePath: 'player', streamEpoch: 1, characterVisual: { generation: '0:4294967297', revision: 1 } } })
  assert.equal(canonical.actionAnimation, undefined, 'legacy spawn clears retained state')
  dispatch({ objectDespawn: { entityId: 17, streamEpoch: 1 } })
  update()
  assert.equal(store.entities.has(17), false)
  spawn()
  enter(2)
  update()
  assert.equal(store.entities.size, 0, 'new world cannot resurrect the old incarnation')
  assert.equal(acceptActionAnimation(state(), state({ serverTimeMs: 9999 }), state().generation), false)
  assert.throws(() => acceptActionAnimation(state(), state({ elapsedTicks: 4, serverTimeMs: 10100 }), state().generation))
})

test('the same selector supports both defs, reverses preference, blends bounds, and retains phase across suppression', () => {
  for (const definition of definitions) {
    const player = new ActionAnimationPlayer(baseFrame, message => { throw new Error(message) })
    player.configure(catalog, definition.actor)
    player.setInput({ key: definition.key, phase: .25, facingAngle: 1 })
    player.update(context, 0); player.update(context, definition.blend_ms)
    assert.equal(player.samples[0]!.clip, definition.variants[0]!.clip)
    assert.equal(player.samples[0]!.weight, 1)
    assert.deepEqual(player.frame, definition.frame)
    player.setInput({ key: definition.key, phase: .75, facingAngle: 1 })
    player.update(context, 500)
    assert.equal(player.samples[0]!.weight, 1, 'duplicates/fresh samples never restart blending')
    for (const suppressed of [{ ...context, stationary: false }, { ...context, carrying: true }]) {
      player.update(suppressed, 600); player.update(suppressed, 600 + definition.blend_ms)
      assert.equal(player.samples.length, 0)
      player.update(context, 1000); player.update(context, 1000 + definition.blend_ms)
      assert.equal(player.samples[0]!.phase, .75)
    }
    player.setInput({ key: definition.key, phase: 0, facingAngle: 1 })
    player.update(context, 1400)
    assert.equal(player.samples[0]!.weight, 1, 'confirmed successor resets phase without fading again')
    player.setInput(null); player.update(context, 1500)
    assert.deepEqual(player.frame, definition.frame, 'outgoing pose retains bounds')
    player.update(context, 1500 + definition.blend_ms)
    assert.deepEqual(player.frame, baseFrame)
  }
  const player = new ActionAnimationPlayer(baseFrame)
  player.configure({ ...catalog, [first.key]: { ...first, variants: [...first.variants].reverse() } }, first.actor)
  player.setInput({ key: first.key, phase: .6, facingAngle: 1 })
  player.update(context, 0); player.update(context, 500)
  assert.equal(player.samples[0]!.clip, first.variants[1]!.clip)
  player.update({ ...context, equipmentReady: false }, 600); player.update({ ...context, equipmentReady: false }, 1000)
  assert.equal(player.samples.length, 0)
  player.update(context, 1100); player.update(context, 1300)
  assert.equal(player.samples[0]!.phase, .6)
  player.update({ ...context, knockedOut: true }, 1400)
  assert.equal(player.samples.length, 0)
})

test('unknown binding and missing def-required context fall back and report once', () => {
  const messages: string[] = [], player = new ActionAnimationPlayer(baseFrame, message => messages.push(message))
  player.configure(catalog, first.actor)
  for (const key of ['future/binding', first.key]) {
    player.setInput({ key, phase: .4 })
    for (let now = 0; now < 10; now++) player.update(context, now)
    assert.equal(player.samples.length, 0)
  }
  assert.equal(messages.length, 2)
})

test('ObjectView forwards latest state after asynchronous readiness, cancellation, replacement and destruction', async t => {
  DOMAdapter.set({ ...DOMAdapter.get(), createCanvas: () => ({ getContext: () => null }) as unknown as HTMLCanvasElement })
  t.mock.method(ResourceLoader, 'getResourceDef', () => ({ actor3d: true, layers: [] }))
  let resolveReady!: () => void
  const player = new ActionAnimationPlayer(baseFrame)
  const inputPhases: number[] = []
  const actor = { ready: new Promise<void>(resolve => { resolveReady = resolve }),
    direction: 3, walking: false, carrying: false, knockedOut: false, hovered: false,
    setActionAnimation: (input: Parameters<ActionAnimationPlayer['setInput']>[0]) => { player.setInput(input); if (input) inputPhases.push(input.phase) },
    prepareActionAnimation: (now: number) => player.update(context, now), get outputFrame() { return player.frame },
    setEquipment: async () => {}, setFacingAngle: () => {},
  }
  const renderer = { create: () => ({ actor, sprite: new Sprite(Texture.EMPTY), immersionPx: 0 }), release: () => {} }
  const view = new ObjectView({ entityId: 17, typeId: 1, resourcePath: 'player', position: { x: 0, y: 0 }, size: { x: 4, y: 4 }, actionAnimation: state() }, renderer as unknown as ActorRenderer)
  view.setActionAnimation(null)
  resolveReady(); await actor.ready
  player.configure(catalog, first.actor)
  view.updateActionAnimation(0, 10500)
  assert.equal(player.samples.length, 0)
  view.setActionAnimation(state({ animationKey: second.key }))
  view.updateActionAnimation(100, 10500); view.updateActionAnimation(500, 10900)
  assert.equal(player.samples[0]!.clip, second.variants[0]!.clip)
  assert.equal(player.samples[0]!.phase, .7)
  const bounds = view.computeScreenBounds()
  assert.ok(bounds.minY <= -second.frame.origin_y, 'bounds expand before GPU rendering')
  view.destroy()
  const count = inputPhases.length
  view.setActionAnimation(state())
  view.updateActionAnimation(1000, 11000)
  assert.equal(inputPhases.length, count, 'destroyed view cannot consume a late result')
  assert.equal(ACTOR_RENDER.cellSize, baseFrame.width)
})


test('unbind slots follow displayed layers through entry, terminal holds, repeats and overlapping blends', () => {
  const player = new ActionAnimationPlayer(baseFrame)
  player.configure(catalog, first.actor)
  const slots = () => [...player.unboundEquipmentSlots].sort()
  const hands = [...first.unbind_equipment_slots!].sort()
  player.update(context, 0)
  assert.deepEqual(slots(), [])
  player.setInput({ key: first.key, phase: .4, facingAngle: 1 })
  player.update(context, 0)
  assert.equal(player.samples.length, 0, 'blend starts at zero weight')
  assert.deepEqual(slots(), hands, 'detach before the first contributing pose')
  player.update(context, 120)
  player.setInput({ key: first.key, phase: 1, facingAngle: 1 })
  player.update(context, 10000)
  assert.deepEqual(slots(), hands, 'the confirmed endpoint continues to own its slots')
  player.setInput({ key: first.key, phase: 0, facingAngle: 1 })
  player.update(context, 10001)
  assert.deepEqual(slots(), hands, 'successor cycles must not flash equipment')
  player.setInput({ key: second.key, phase: .5 })
  player.update(context, 11000)
  assert.deepEqual(slots(), [...hands, ...second.unbind_equipment_slots!].sort())
  player.update(context, 11000 + first.blend_ms)
  assert.deepEqual(slots(), second.unbind_equipment_slots)
  player.setInput(null)
  player.update(context, 12000)
  assert.deepEqual(slots(), second.unbind_equipment_slots, 'retain unbind during blend out')
  player.update(context, 12000 + second.blend_ms)
  assert.deepEqual(slots(), [])
})

test('unbind defaults, zero-duration blending, eligibility and unknown states preserve ordinary presentation', () => {
  const { unbind_equipment_slots: ignored, ...legacy } = second
  void ignored
  for (const definition of [legacy, { ...second, unbind_equipment_slots: [] }]) {
    const player = new ActionAnimationPlayer(baseFrame)
    player.configure({ [definition.key]: definition }, definition.actor)
    player.setInput({ key: definition.key, phase: .5 })
    player.update(context, 0); player.update(context, 1000)
    assert.equal(player.unboundEquipmentSlots.size, 0)
  }
  const player = new ActionAnimationPlayer(baseFrame, () => {})
  player.configure({ [first.key]: { ...first, blend_ms: 0 } }, first.actor)
  for (const suppressed of [{ ...context, stationary: false }, { ...context, carrying: true }, { ...context, knockedOut: true }]) {
    player.setInput({ key: first.key, phase: .6, facingAngle: 1 })
    player.update(suppressed, 0)
    assert.equal(player.unboundEquipmentSlots.size, 0)
    player.update(context, 1)
    assert.deepEqual([...player.unboundEquipmentSlots], first.unbind_equipment_slots)
    player.update(suppressed, 2)
    assert.equal(player.unboundEquipmentSlots.size, 0)
  }
  player.setInput({ key: 'unknown', phase: .5 })
  player.update(context, 3)
  assert.equal(player.unboundEquipmentSlots.size, 0)
  player.setInput(null)
  player.update(context, 4)
  assert.equal(player.unboundEquipmentSlots.size, 0)
})
