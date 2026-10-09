import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { AnimationClip, Bone, Group, VectorKeyframeTrack } from 'three'
import { Container, DOMAdapter, Graphics, Sprite, Texture } from 'pixi.js'
import fixture from '../../tests/fixtures/action_animations/bindings.json'
import { parseActionAnimationFile } from '../src/types/actionAnimationDefs'
import { decodeActionAnimation, actionAnimationPhase, acceptActionAnimation } from '../src/types/actionAnimation'
import { ActionAnimationPlayer, type ActionPresentationContext } from '../src/game/actors/ActionAnimationPlayer'
import { ActorActionLayers } from '../src/game/actors/ActorActionLayers'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { GameFacade, gameFacade } from '../src/game/GameFacade'
import { ObjectView } from '../src/game/ObjectView'
import { ResourceLoader } from '../src/game/ResourceLoader'
import type { ActorRenderer } from '../src/game/actors/ActorRenderer'
import { ACTOR_RENDER } from '../src/game/actors/config'
import { NicknameManager } from '../src/game/NicknameManager'
import { cameraController } from '../src/game/CameraController'
import { ObjectManager } from '../src/game/ObjectManager'
import { Render } from '../src/game/Render'
import { NICKNAME_Y_OFFSET_PX } from '../src/constants/nickname'
import { moveController, type RenderPosition } from '../src/game/MoveController'
import { screenFacingAngle, screenFacingAngleFromDisplacement } from '../src/game/actors/facing'

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

function headingFixture(t: TestContext) {
  const adapter = DOMAdapter.get()
  DOMAdapter.set({ ...adapter, createCanvas: () => ({ getContext: () => null }) as unknown as HTMLCanvasElement })
  t.mock.method(ResourceLoader, 'getResourceDef', () => ({ actor3d: true, layers: [] }))
  const player = new ActionAnimationPlayer(baseFrame, message => { throw new Error(message) })
  player.configure(catalog, first.actor)
  let resolveReady!: () => void
  let direction = 3, baseFacing = screenFacingAngle(direction)
  const actor = {
    ready: new Promise<void>(resolve => { resolveReady = resolve }),
    get direction() { return direction },
    set direction(value: number) { direction = value; baseFacing = screenFacingAngle(value) },
    setFacingAngle: (angle: number) => { baseFacing = angle },
    setKnockedOutPose: () => {},
    setActionAnimation: (input: Parameters<ActionAnimationPlayer['setInput']>[0]) => player.setInput(input),
    prepareActionAnimation: (now: number) => player.update(context, now),
    get outputFrame() { return player.frame },
    setEquipment: async () => {},
  }
  const renderer = { create: () => ({ actor, sprite: new Sprite(Texture.EMPTY), immersionPx: 0 }), release: () => {} }
  const parent = new Container(), manager = new ObjectManager()
  manager.setParentContainer(parent)
  manager.setActorRenderer(renderer as unknown as ActorRenderer)
  const render = Object.assign(Object.create(Render.prototype), { objectManager: manager }) as Render
  let positions = new Map<number, RenderPosition>()
  t.mock.method(moveController, 'update', () => positions)
  t.after(() => { manager.destroy(); parent.destroy(); DOMAdapter.set(adapter) })
  return {
    actor, manager, render,
    resolveReady,
    spawn(heading: number) {
      manager.spawnObject({ entityId: 17, typeId: 1, resourcePath: 'player', position: { x: 50, y: 50, heading }, size: { x: 4, y: 4 } })
      return manager.getObject(17)!
    },
    update(heading: number, moving = false) {
      const position = manager.getObject(17)!.getPosition()
      positions = new Map([[17, { x: position.x + (moving ? 10 : 0), y: position.y, heading,
        isMoving: moving, moveMode: 1, direction: 3, distanceMoved: moving ? 10 : 0 }]])
      ;(render as unknown as { updateMovement(): void }).updateMovement()
    },
    get baseFacing() { return baseFacing },
    get displayedFacing() { return player.facingAngle ?? baseFacing },
  }
}

function assertWorldFacing(screenAngle: number, heading: number, message: string): void {
  const expected = screenFacingAngleFromDisplacement(Math.cos(heading), Math.sin(heading))!
  const difference = Math.atan2(Math.sin(screenAngle - expected), Math.cos(screenAngle - expected))
  assert.ok(Math.abs(difference) < 1e-7, message)
}

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

test('fixed world facing survives the wire, including zero and float-rounded full turns', () => {
  for (const facingAngle of [0, Math.fround(Math.PI / 3), Math.fround(2 * Math.PI)]) {
    const value = wire({ targetPosition: null, facingAngle })
    const received = proto.CharacterActionAnimationState.decode(proto.CharacterActionAnimationState.encode(value).finish())
    const decoded = decodeActionAnimation(received)
    assert.equal(decoded.facingAngle, facingAngle)
    assert.equal(decoded.targetPosition, undefined)
  }
  assert.equal(state({ targetPosition: null }).facingAngle, undefined)
  for (const facingAngle of [-.1, NaN, Infinity, -Infinity, Math.fround(2 * Math.PI) + .000001]) {
    assert.throws(() => decodeActionAnimation({ ...wire({ targetPosition: null }), facingAngle }), /facing angle/)
  }
  assert.throws(() => decodeActionAnimation(wire({ facingAngle: 0 })), /facing angle/, 'fixed angle and target position are mutually exclusive')
})

test('fixed facing is immutable within a revision but can change in a successor', () => {
  const current = state({ targetPosition: null, facingAngle: 0 })
  const progress = state({ targetPosition: null, facingAngle: 0, serverTimeMs: 10100, elapsedTicks: 6 })
  assert.equal(acceptActionAnimation(current, progress, current.generation), true)
  for (const facingAngle of [1, undefined]) {
    assert.throws(() => acceptActionAnimation(current, state({ targetPosition: null, facingAngle, serverTimeMs: 10100 }), current.generation), /Contradictory/)
  }
  const successor = state({ targetPosition: null, facingAngle: 1, revision: '9007199254740994', serverTimeMs: 10100 })
  assert.equal(acceptActionAnimation(current, successor, current.generation), true)
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
    setEquipment: async () => {}, setFacingAngle: () => {}, setKnockedOutPose: () => {},
  }
  const renderer = { create: () => ({ actor, sprite: new Sprite(Texture.EMPTY), immersionPx: 0 }), release: () => {} }
  const view = new ObjectView({ entityId: 17, typeId: 1, resourcePath: 'player', position: { x: 0, y: 0 }, size: { x: 4, y: 4 }, actionAnimation: state() }, renderer as unknown as ActorRenderer)
  const parent = new Container()
  parent.addChild(view.getContainer())
  const nicknames = new NicknameManager(parent)
  nicknames.show(17, 'Player', 0)
  const manager = { getObject: () => view } as unknown as ObjectManager
  cameraController.setZoom(1)
  t.after(() => { nicknames.destroy(); cameraController.reset() })
  nicknames.update(manager)
  const nickname = parent.children[1]!
  const nicknameY = -ACTOR_RENDER.anchorY + NICKNAME_Y_OFFSET_PX
  assert.equal(nickname.y, nicknameY)
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
  view.getContainer().children[0]!.position.set(-second.frame.origin_x, -second.frame.origin_y)
  const overlay = new Graphics().rect(-40, -400, 80, 20).fill(0xffffff)
  view.getContainer().addChild(overlay)
  nicknames.update(manager)
  assert.equal(nickname.y, nicknameY, 'action-frame expansion and child overlays must not move the nickname')
  view.setActionAnimation(null)
  view.updateActionAnimation(1000, 11000)
  view.updatePosition(12, 24)
  cameraController.setZoom(.5)
  nicknames.update(manager)
  assert.equal(nickname.y, view.getContainer().y + nicknameY, 'movement follows the entity with a fixed vertical offset')
  assert.equal(nickname.scale.y, 2)
  view.getContainer().visible = false
  nicknames.update(manager)
  assert.equal(nickname.visible, false)
  view.destroy()
  const count = inputPhases.length
  view.setActionAnimation(state())
  view.updateActionAnimation(1000, 11000)
  assert.equal(inputPhases.length, count, 'destroyed view cannot consume a late result')
  assert.equal(ACTOR_RENDER.cellSize, baseFrame.width)
})

test('ObjectView projects fixed world facing and keeps it independent of actor movement', t => {
  DOMAdapter.set({ ...DOMAdapter.get(), createCanvas: () => ({ getContext: () => null }) as unknown as HTMLCanvasElement })
  t.mock.method(ResourceLoader, 'getResourceDef', () => ({ actor3d: true, layers: [] }))
  const player = new ActionAnimationPlayer(baseFrame, message => { throw new Error(message) })
  player.configure(catalog, first.actor)
  const inputs: Parameters<ActionAnimationPlayer['setInput']>[0][] = []
  const actor = {
    ready: Promise.resolve(), setEquipment: async () => {}, setFacingAngle: () => {}, setKnockedOutPose: () => {},
    setActionAnimation: (input: Parameters<ActionAnimationPlayer['setInput']>[0]) => { inputs.push(input); player.setInput(input) },
    prepareActionAnimation: (now: number) => player.update(context, now), get outputFrame() { return player.frame },
  }
  const renderer = { create: () => ({ actor, sprite: new Sprite(Texture.EMPTY), immersionPx: 0 }), release: () => {} }
  const view = new ObjectView({ entityId: 17, typeId: 1, resourcePath: 'player', position: { x: 50, y: 50 }, size: { x: 4, y: 4 } }, renderer as unknown as ActorRenderer)
  t.after(() => view.destroy())
  for (const [angle, projected] of [[0, Math.atan2(.5, 1)], [Math.PI / 4, Math.PI / 2], [Math.PI / 2, Math.atan2(.5, -1)], [Math.PI, Math.atan2(-.5, -1)], [Math.fround(2 * Math.PI), Math.atan2(.5, 1)]] as const) {
    view.setActionAnimation(state({ targetPosition: null, facingAngle: Math.fround(angle) }))
    view.updateActionAnimation(0, 10000)
    view.updateActionAnimation(120, 10000)
    assert.ok(Math.abs(inputs.at(-1)!.facingAngle! - projected) < 1e-6)
    assert.equal(player.samples[0]!.clip, first.variants[0]!.clip, 'target-facing binding accepts fixed-direction input')
    const facing = inputs.at(-1)!.facingAngle
    view.updatePosition(200, 400)
    view.updateActionAnimation(121, 10000)
    assert.equal(inputs.at(-1)!.facingAngle, facing, 'fixed direction does not turn toward a synthetic world point')
  }
  view.updatePosition(50, 50)
  view.setActionAnimation(state({ targetPosition: { x: 100, y: 100 }, facingAngle: null }))
  view.updateActionAnimation(122, 10000)
  assert.equal(inputs.at(-1)!.facingAngle, Math.PI / 2, 'object-target facing retains its existing projection')
})

test('stationary spawn heading survives deferred readiness and later stationary server updates', async t => {
  const fixture = headingFixture(t)
  fixture.spawn(-Math.PI / 2)
  assertWorldFacing(fixture.baseFacing, -Math.PI / 2, 'spawn must apply its heading before any movement')
  fixture.update(Math.PI)
  fixture.resolveReady()
  await fixture.actor.ready
  assertWorldFacing(fixture.baseFacing, Math.PI, 'late readiness must preserve the latest stationary heading')
  fixture.update(0)
  assertWorldFacing(fixture.baseFacing, 0, 'an explicit zero heading must reach a stationary actor')
})

test('the render movement path uses server heading even when displayed displacement points elsewhere', async t => {
  const fixture = headingFixture(t)
  fixture.spawn(0)
  fixture.resolveReady()
  await fixture.actor.ready
  fixture.update(Math.PI / 2, true)
  assertWorldFacing(fixture.baseFacing, Math.PI / 2, 'world +x displacement must not replace world +y server heading')
  assert.deepEqual(fixture.manager.getObject(17)!.getPosition(), { x: 60, y: 50 })
  fixture.update(-Math.PI / 2)
  assertWorldFacing(fixture.baseFacing, -Math.PI / 2, 'the subsequent server stop must apply its heading')
})

test('target-facing action overrides stay local and return to the latest server heading after finish or cancellation', async t => {
  const fixture = headingFixture(t)
  const view = fixture.spawn(Math.PI / 2)
  fixture.resolveReady()
  await fixture.actor.ready
  for (const end of ['finished', 'canceled'] as const) {
    view.setActionAnimation(state({ targetPosition: null, facingAngle: 0 }))
    view.updateActionAnimation(0, 10000)
    assertWorldFacing(fixture.displayedFacing, 0, 'the directed action controls presentation while active')
    fixture.update(Math.PI)
    view.updateActionAnimation(120, 10120)
    assertWorldFacing(fixture.displayedFacing, 0, 'stationary server heading must not retarget the active action')
    assertWorldFacing(fixture.baseFacing, Math.PI, 'the action must still retain the new ordinary heading')
    if (end === 'finished') {
      view.setActionAnimation(state({ animationKey: '', revision: '9007199254740994' }))
    } else {
      view.setActionAnimation(null)
    }
    view.updateActionAnimation(121, 10121)
    assertWorldFacing(fixture.displayedFacing, Math.PI, `${end} action must resume the latest server heading`)
    fixture.update(Math.PI / 2)
  }
})

test('overhead screen coordinates follow rendered placement, zoom and pan and hide with the entity', () => {
  const parent = new Container()
  const container = new Container()
  parent.addChild(container)
  parent.position.set(320, 240)
  parent.scale.set(2)
  container.position.set(40, 60)
  let present = true
  const render: Render = Object.assign(Object.create(Render.prototype), {
    objectsContainer: parent,
    objectManager: { getObject: () => present ? { getContainer: () => container, getOverheadAnchorY: () => -116 } : undefined },
  })
  const facade = new GameFacade()
  assert.equal(facade.getObjectOverheadScreenPosition(17), null)
  Object.assign(facade, { render })
  assert.deepEqual(facade.getObjectOverheadScreenPosition(17), { x: 400, y: 128 })
  parent.position.set(100, 200)
  parent.scale.set(.5)
  assert.deepEqual(facade.getObjectOverheadScreenPosition(17), { x: 120, y: 172 })
  container.visible = false
  assert.equal(facade.getObjectOverheadScreenPosition(17), null)
  container.visible = true
  present = false
  assert.equal(facade.getObjectOverheadScreenPosition(17), null)
  parent.destroy({ children: true })
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
