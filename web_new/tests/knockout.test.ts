import { setWorldParams } from './serverConstantsFixture'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useGameStore } from '../src/stores/gameStore'
import { useActionCooldownStore } from '../src/stores/actionCooldownStore'
import { gameFacade } from '../src/game/GameFacade'
import { proto } from '../src/network/proto/packets.js'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { registerMessageHandlers } from '../src/network/handlers'
import { sendStandUp, sendStartCraftOne, sendStartBuild, gameConnection } from '../src/network'
import { soundManager } from '../src/game/SoundManager'
import { timeSync } from '../src/network/TimeSync'
import { GameConnection } from '../src/network/GameConnection'

function dispatch(packet: proto.IServerMessage) {
  messageDispatcher.dispatch(proto.ServerMessage.decode(proto.ServerMessage.encode(proto.ServerMessage.create(packet)).finish()))
}

test('authorization immediately sends existing Ping and a new connection resets its clock', t => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'WebSocket')
  const sockets: TestSocket[] = []
  class TestSocket {
    static OPEN = 1
    readyState = 1
    binaryType = ''
    onopen: (() => void) | null = null
    onmessage: ((event: { data: ArrayBuffer }) => void) | null = null
    onclose = null
    onerror = null
    packets: proto.ClientMessage[] = []
    constructor(_url: string) { sockets.push(this) }
    send(bytes: Uint8Array) { this.packets.push(proto.ClientMessage.decode(bytes)) }
    close() { this.readyState = 3 }
    receive(packet: proto.IServerMessage) {
      const encoded = proto.ServerMessage.encode(packet).finish()
      this.onmessage?.({ data: Uint8Array.from(encoded).buffer })
    }
  }
  Object.defineProperty(globalThis, 'WebSocket', { configurable: true, value: TestSocket })
  const connection = new GameConnection()
  t.after(() => {
    connection.disconnect(); timeSync.reset()
    if (previous) Object.defineProperty(globalThis, 'WebSocket', previous)
    else Reflect.deleteProperty(globalThis, 'WebSocket')
  })
  connection.connect('test-token')
  sockets[0]!.onopen?.()
  assert.ok(sockets[0]!.packets[0]!.auth)
  assert.equal(sockets[0]!.packets.length, 1)
  sockets[0]!.receive({ authResult: { success: true } })
  assert.ok(sockets[0]!.packets[1]!.ping, 'Ping must not wait for the interval')
  assert.equal(sockets[0]!.packets[1]!.sequence, 2)
  sockets[0]!.receive({ pong: { clientTimeMs: Date.now(), serverTimeMs: Date.now() } })
  assert.equal(timeSync.isInitialized(), true)
  connection.connect('next-token')
  assert.equal(timeSync.isInitialized(), false)
  assert.equal(sockets[0]!.readyState, 3)
})

test('owner snapshots control the window atomically, reject old epochs and do not set the rendered pose', t => {
  setActivePinia(createPinia())
  const game = useGameStore()
  game.setConnectionState('connected')
  t.mock.method(gameFacade, 'resetWorld', () => {})
  const poses = t.mock.method(gameFacade, 'setObjectKnockedOutPose', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  setWorldParams(12, 4)
  registerMessageHandlers()
  dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 7} })
  const snapshot = { streamEpoch: 7, shp: 0, hhp: 20, mhp: 25, isKnockedOut: true, isLying: true, koUntilMs: 61000, canStandUp: false }
  dispatch({ playerStats: snapshot })
  assert.equal(game.playerStats.isKnockedOut, true)
  assert.equal(game.playerStats.koUntilMs, 61000)
  assert.equal(poses.mock.callCount(), 0)
  dispatch({ playerStats: { ...snapshot, streamEpoch: 6, isLying: false, isKnockedOut: false } })
  assert.equal(game.playerStats.isLying, true)
  dispatch({ playerStats: { ...snapshot, shp: 1, koUntilMs: 0, isKnockedOut: false, canStandUp: true } })
  const send = t.mock.method(gameConnection, 'send', () => {})
  sendStandUp()
  const command = proto.ClientMessage.decode(proto.ClientMessage.encode(proto.ClientMessage.create(send.mock.calls[0]!.arguments[0])).finish())
  assert.equal(command.playerAction?.standUp?.streamEpoch, 7)
  assert.equal(game.playerStats.isLying, true, 'click must wait for a snapshot')
  dispatch({ playerStats: { ...snapshot, shp: 1, koUntilMs: 0, isKnockedOut: false, canStandUp: false } })
  sendStandUp()
  assert.equal(send.mock.callCount(), 1, 'stun must disable the button')
  dispatch({ playerStats: { ...snapshot, shp: 1, koUntilMs: 0, isKnockedOut: false, isLying: false } })
  assert.equal(game.playerStats.isLying, false)
  dispatch({ playerStats: snapshot })
  assert.equal(game.playerStats.isKnockedOut, true, 'repeat KO restores the timer')
  game.setPlayerLeaveWorld()
  dispatch({ playerStats: snapshot })
  assert.equal(game.playerStats.isLying, false)
})

test('local and observed poses share the visual gate for epoch, generation, exact revision and duplicates', t => {
  setActivePinia(createPinia())
  const game = useGameStore()
  game.setConnectionState('connected')
  t.mock.method(gameFacade, 'resetWorld', () => {})
  t.mock.method(gameFacade, 'spawnObject', () => {})
  t.mock.method(gameFacade, 'setCharacterEquipment', async () => {})
  const poses = t.mock.method(gameFacade, 'setObjectKnockedOutPose', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  setWorldParams(12, 4)
  registerMessageHandlers()
  dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 1} })
  const state = proto.CharacterVisualState.fromObject({ generation: '0:4294967297', revision: '9007199254740993', isLying: true })
  for (const entityId of [17, 18]) {
    dispatch({ objectSpawn: { entityId, typeId: 1, resourcePath: 'player', streamEpoch: 1, characterVisual: state } })
    assert.equal(game.entities.get(entityId)!.characterVisual!.isLying, true)
    const standing = proto.CharacterVisualState.fromObject({ ...state, revision: '9007199254740994', isLying: false })
    dispatch({ characterVisual: { entityId, streamEpoch: 1, state: standing } })
    const count = poses.mock.callCount()
    dispatch({ characterVisual: { entityId, streamEpoch: 1, state } })
    dispatch({ characterVisual: { entityId, streamEpoch: 1, state: standing } })
    dispatch({ characterVisual: { entityId, streamEpoch: 2, state: { ...standing, revision: 1, isLying: true } } })
    dispatch({ characterVisual: { entityId, streamEpoch: 1, state: { ...standing, generation: '1:4294967297', isLying: true } } })
    dispatch({ objectSpawn: { entityId, typeId: 1, resourcePath: 'player', streamEpoch: 1, characterVisual: state } })
    assert.equal(poses.mock.callCount(), count)
    assert.equal(game.entities.get(entityId)!.characterVisual!.isLying, false)
  }
  assert.deepEqual(poses.mock.calls.map(call => call.arguments.slice(0, 3)),
    [[17, true, 'snapshot'], [17, false, 'transition'], [18, true, 'snapshot'], [18, false, 'transition']])
  game.setPlayerLeaveWorld()
})

test('live lying uses one transition path and death fallback cannot be cleared by a queued standing visual', t => {
  setActivePinia(createPinia())
  const game = useGameStore()
  game.setConnectionState('connected')
  const resets = t.mock.method(gameFacade, 'resetWorld', () => {})
  t.mock.method(gameFacade, 'spawnObject', () => {})
  t.mock.method(gameFacade, 'setCharacterEquipment', async () => {})
  const poses = t.mock.method(gameFacade, 'setObjectKnockedOutPose', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  let nowMs = 1000
  t.mock.method(performance, 'now', () => nowMs)
  setWorldParams(12, 4)
  registerMessageHandlers()
  const visual = (revision: number, isLying: boolean) => ({ generation: '0:4294967297', revision, isLying })
  dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 1} })
  dispatch({ objectSpawn: { entityId: 17, typeId: 1, resourcePath: 'player', streamEpoch: 1, characterVisual: visual(1, false) } })
  nowMs = 1100
  dispatch({ characterVisual: { entityId: 17, streamEpoch: 1, state: visual(2, true) } })
  assert.deepEqual(poses.mock.calls.at(-1)!.arguments, [17, true, 'transition', 1100])
  nowMs = 1200
  dispatch({ deathDialog: { title: 'Death', message: 'You are dead' } })
  assert.equal(resets.mock.callCount(), 1, 'death keeps the world and renderer alive')
  dispatch({ characterVisual: { entityId: 17, streamEpoch: 1, state: visual(3, true) } })
  assert.equal(game.deathDialog?.title, 'Death')

  dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 2} })
  dispatch({ objectSpawn: { entityId: 17, typeId: 1, resourcePath: 'player', streamEpoch: 2, characterVisual: visual(1, false) } })
  dispatch({ deathDialog: { title: 'Death' } })
  const deathCall = poses.mock.callCount()
  dispatch({ characterVisual: { entityId: 17, streamEpoch: 2, state: visual(2, false) } })
  dispatch({ characterVisual: { entityId: 17, streamEpoch: 2, state: visual(3, true) } })
  assert.ok(poses.mock.calls.slice(deathCall).every(call => call.arguments[1] === true))
  assert.equal(resets.mock.callCount(), 2)
  assert.equal(game.connectionState, 'connected')
  game.setPlayerLeaveWorld()
})

test('same-ID skeleton spawn clears corpse appearance and rejects late character packets', t => {
  setActivePinia(createPinia())
  const game = useGameStore()
  game.setConnectionState('connected')
  t.mock.method(gameFacade, 'resetWorld', () => {})
  const spawns = t.mock.method(gameFacade, 'spawnObject', () => {})
  const equipment = t.mock.method(gameFacade, 'setCharacterEquipment', async () => {})
  const poses = t.mock.method(gameFacade, 'setObjectKnockedOutPose', () => {})
  const animations = t.mock.method(gameFacade, 'setActionAnimation', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  setWorldParams(12, 4)
  registerMessageHandlers()
  dispatch({ playerEnterWorld: { entityId: 99, streamEpoch: 1} })
  const corpseVisual = { generation: '0:4294967297', revision: 5, isLying: true,
    equipment: [{ slot: proto.EquipSlot.EQUIP_SLOT_RIGHT_HAND, visualKey: 'stone_axe' }] }
  dispatch({ objectSpawn: { entityId: 17, typeId: 15, resourcePath: 'player', streamEpoch: 1, characterVisual: corpseVisual } })
  dispatch({ objectSpawn: { entityId: 17, typeId: 17, resourcePath: 'player_skeleton', streamEpoch: 1 } })
  assert.equal(spawns.mock.callCount(), 2)
  assert.equal(game.entities.get(17)!.resourcePath, 'player_skeleton')
  assert.equal(game.entities.get(17)!.characterVisual, undefined)
  assert.equal(game.entities.get(17)!.actionAnimation, undefined)
  const calls = [equipment.mock.callCount(), poses.mock.callCount(), animations.mock.callCount()]
  dispatch({ characterVisual: { entityId: 17, streamEpoch: 1, state: { ...corpseVisual, revision: 6 } } })
  dispatch({ characterActionAnimation: { entityId: 17, streamEpoch: 1, state: {
    generation: corpseVisual.generation, revision: 7, animationKey: '', elapsedTicks: 0,
    totalTicks: 0, tickDurationMs: 100, serverTimeMs: 1000, facingAngle: 0,
  } } })
  assert.deepEqual([equipment.mock.callCount(), poses.mock.callCount(), animations.mock.callCount()], calls)
  game.setPlayerLeaveWorld()
})

test('forbidden craft and build retire keyboard input and reach the server for an explicit refusal', t => {
  setActivePinia(createPinia())
  const game = useGameStore()
  game.setConnectionState('connected')
  game.setPlayerStats({ isKnockedOut: true, isLying: true })
  const releases = t.mock.method(gameFacade, 'releaseKeyboardMovement', () => {})
  const sends = t.mock.method(gameConnection, 'send', () => {})
  sendStartCraftOne('branch'); sendStartBuild('campfire', { x: 100, y: 100 })
  assert.equal(releases.mock.callCount(), 2)
  assert.equal(sends.mock.callCount(), 2)
  assert.ok(sends.mock.calls[0]!.arguments[0]!.startCraftOne)
  assert.ok(sends.mock.calls[1]!.arguments[0]!.buildStart)
})

test('existing action time anchors KO before Pong and resets with the world', t => {
  setActivePinia(createPinia()); timeSync.reset()
  t.mock.method(performance, 'now', () => 100)
  const clock = useActionCooldownStore()
  assert.equal(clock.serverNow(), 0)
  clock.setSnapshot({ serverTimeMs: 10000 })
  t.mock.method(performance, 'now', () => 59999)
  assert.equal(clock.serverNow(), 69899)
  t.mock.method(timeSync, 'isInitialized', () => true)
  t.mock.method(timeSync, 'estimateServerNowMs', () => 70000)
  assert.equal(clock.serverNow(), 70000)
  t.mock.method(timeSync, 'isInitialized', () => false)
  clock.reset(); assert.equal(clock.serverNow(), 0)
})
