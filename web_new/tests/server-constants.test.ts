import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { ServerConstants, serverConstants } from '../src/network/ServerConstants'
import { GameConnection } from '../src/network/GameConnection'
import { gameCalendarSync } from '../src/network/GameCalendarSync'
import { timeSync } from '../src/network/TimeSync'
import { proto } from '../src/network/proto/packets.js'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { gameFacade } from '../src/game/GameFacade'
import { moveController } from '../src/game/MoveController'
import { Render } from '../src/game/Render'
import { TerrainManager } from '../src/game/terrain/TerrainManager'
import { hasWorldParams, getChunkSize, getCoordPerTile } from '../src/game/tiles/Tile'
import { coordGame2Screen } from '../src/game/utils/coordConvert'
import { useGameStore, type WorldParams } from '../src/stores/gameStore'
import { serverProfile } from './serverConstantsFixture'

class FakeSocket {
  static readonly OPEN = 1
  static sockets: FakeSocket[] = []
  readyState = 1
  binaryType = ''
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: (() => void) | null = null
  readonly packets: proto.ClientMessage[] = []
  constructor() { FakeSocket.sockets.push(this) }
  send(buffer: Uint8Array): void { this.packets.push(proto.ClientMessage.decode(buffer)) }
  close(): void { this.readyState = 3 }
  receive(packet: proto.IServerMessage): void {
    const wire = proto.ServerMessage.encode(proto.ServerMessage.fromObject(packet)).finish()
    this.onmessage?.({ data: wire } as unknown as MessageEvent)
  }
}

function connectionFixture(t: TestContext) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'WebSocket')
  Object.defineProperty(globalThis, 'WebSocket', { configurable: true, value: FakeSocket })
  FakeSocket.sockets = []
  const connection = new GameConnection()
  t.after(() => {
    connection.disconnect()
    if (previous) Object.defineProperty(globalThis, 'WebSocket', previous)
    else Reflect.deleteProperty(globalThis, 'WebSocket')
  })
  const connect = () => {
    connection.connect('token')
    const socket = FakeSocket.sockets.at(-1)!
    socket.onopen?.()
    socket.receive({ authResult: { success: true } })
    return socket
  }
  return { connection, connect }
}

function pong(runtimeSecondsTotal: number | string = 0): proto.IServerMessage {
  return proto.ServerMessage.fromObject({ pong: { clientTimeMs: Date.now(), serverTimeMs: Date.now(), runtimeSecondsTotal } })
}

test('constants validate ranges without fixed defaults, accept false, and remain immutable', () => {
  const holder = new ServerConstants()
  assert.equal(holder.getSnapshot(), null)
  assert.throws(() => holder.requireSnapshot())
  const alternate = { ...serverProfile, coordPerTile: 71, tickRate: 25, realSecondsPerGameDay: 100, hoursPerDay: 10,
    daysPerMonth: 3, monthsPerYear: 2, directionalMovementSupported: false }
  assert.equal(holder.accept(alternate), 'accepted')
  const snapshot = holder.requireSnapshot()
  assert.equal(snapshot.directionalMovementSupported, false)
  assert.equal(Object.isFrozen(snapshot), true)
  alternate.coordPerTile = 3
  assert.equal(snapshot.coordPerTile, 71)
  assert.equal(holder.accept({ ...snapshot }), 'duplicate')
  assert.strictEqual(holder.getSnapshot(), snapshot)
  assert.equal(holder.accept({ ...snapshot, tickRate: 26 }), 'changed')
  assert.strictEqual(holder.getSnapshot(), snapshot)
  holder.reset()
  for (const key of ['coordPerTile', 'chunkSize', 'tickRate', 'realSecondsPerGameDay', 'hoursPerDay', 'daysPerMonth', 'monthsPerYear']) {
    for (const invalid of [0, -1, 1.5, NaN, Infinity, 4294967296, undefined, '10']) assert.equal(holder.accept({ ...serverProfile, [key]: invalid }), 'invalid')
  }
  assert.equal(holder.accept({ ...serverProfile, directionalMovementSupported: undefined }), 'invalid')
  assert.equal(holder.accept(proto.S2C_ServerConstants.decode(proto.S2C_ServerConstants.encode({ ...serverProfile, directionalMovementSupported: false }).finish())), 'accepted')
})

test('first Pong initializes before world entry; transfers retain constants/calendar and tick rate', t => {
  setActivePinia(createPinia())
  const store = useGameStore()
  t.mock.method(gameFacade, 'resetWorld', () => {})
  t.mock.method(console, 'log', () => {})
  t.mock.method(console, 'error', () => {})
  registerMessageHandlers()
  const { connection, connect } = connectionFixture(t)
  connection.onStateChange(state => store.setConnectionState(state))
  connection.onMessage(message => messageDispatcher.dispatch(message))
  const socket = connect()
  assert.ok(socket.packets[0]?.auth)
  assert.ok(socket.packets[1]?.ping, 'immediate Ping is unchanged')
  assert.equal(serverConstants.isReady(), false)
  socket.receive({ pong: { clientTimeMs: Date.now(), serverTimeMs: Date.now() } })
  assert.equal(timeSync.isInitialized(), true, 'legacy Pong preserves wall synchronization')
  assert.equal(gameCalendarSync.getCalendar(), null)
  socket.receive({ serverConstants: { ...serverProfile, tickRate: 25, directionalMovementSupported: false } })
  socket.receive(pong(364686))
  assert.equal(gameCalendarSync.getCalendar()?.day, 13)
  assert.equal(store.worldParams, null)
  const snapshot = serverConstants.getSnapshot()
  socket.receive({ playerEnterWorld: { entityId: 42, streamEpoch: 7 } })
  assert.equal((store.worldParams as WorldParams | null)?.coordPerTile, serverProfile.coordPerTile)
  assert.equal((store.worldParams as WorldParams | null)?.directionalMovementSupported, false)
  assert.equal(moveController.getTickRate(), 25)
  socket.receive({ playerLeaveWorld: {} })
  assert.strictEqual(serverConstants.getSnapshot(), snapshot)
  assert.equal(gameCalendarSync.getCalendar()?.day, 13)
  socket.receive({ playerEnterWorld: { entityId: 42, streamEpoch: 8 } })
  assert.equal((store.worldParams as WorldParams | null)?.streamEpoch, 8)
  assert.equal((store.worldParams as WorldParams | null)?.directionalMovementSupported, false)
  assert.equal(moveController.getTickRate(), 25)
  assert.strictEqual(serverConstants.getSnapshot(), snapshot)
  t.after(() => store.reset())
})

test('Pong keeps the server calendar snapshot independent of wall synchronization', t => {
  t.mock.timers.enable({ apis: ['Date'], now: 10000 })
  const { connect } = connectionFixture(t)
  const socket = connect()
  socket.receive({ serverConstants: serverProfile })
  socket.receive({ pong: { clientTimeMs: 8000, serverTimeMs: 9000, runtimeSecondsTotal: 28799 } })
  assert.equal(timeSync.isInitialized(), true)
  assert.equal(timeSync.getLastRttMs(), 2000)
  assert.equal(timeSync.estimateServerNowMs(), 10000)
  const snapshot = gameCalendarSync.getCalendar()
  assert.deepEqual([snapshot?.day, snapshot?.hour, snapshot?.minute, snapshot?.second], [1, 23, 59, 57])
  t.mock.timers.tick(60000)
  assert.equal(timeSync.estimateServerNowMs(), 70000, 'wall synchronization continues using its existing local clock')
  assert.strictEqual(gameCalendarSync.getCalendar(), snapshot, 'calendar changes only when a valid runtime Pong arrives')
  socket.receive({ pong: { clientTimeMs: 70000, serverTimeMs: 70000, runtimeSecondsTotal: 28800 } })
  assert.equal(gameCalendarSync.getCalendar()?.day, 2)
  assert.equal(gameCalendarSync.getDayPhase(), 0)
})

test('missing, malformed and changed constants use connection errors', t => {
  const { connection, connect } = connectionFixture(t)
  const delivered: proto.ServerMessage[] = []
  connection.onMessage(message => delivered.push(message))
  let socket = connect()
  socket.receive({ playerEnterWorld: { entityId: 1, streamEpoch: 1 } })
  assert.equal(connection.getState(), 'error')
  assert.equal(socket.readyState, 3)
  assert.equal(delivered.length, 0)
  socket = connect()
  socket.receive({ serverConstants: { ...serverProfile, chunkSize: 0 } })
  assert.equal(connection.getState(), 'error')
  assert.equal(serverConstants.getSnapshot(), null)
  socket = connect()
  socket.receive({ serverConstants: serverProfile })
  socket.receive(pong(1))
  const snapshot = serverConstants.getSnapshot()
  socket.receive({ serverConstants: { ...serverProfile } })
  assert.strictEqual(serverConstants.getSnapshot(), snapshot)
  assert.notEqual(gameCalendarSync.getCalendar(), null)
  socket.receive({ serverConstants: { ...serverProfile, tickRate: 20 } })
  assert.equal(connection.getState(), 'error')
  assert.equal(serverConstants.getSnapshot(), null)
  assert.equal(gameCalendarSync.getCalendar(), null)
})

test('reconnect accepts new constants and restored runtime; retired callbacks cannot install state', t => {
  setActivePinia(createPinia())
  const store = useGameStore()
  const { connection, connect } = connectionFixture(t)
  const first = connect()
  first.receive({ serverConstants: serverProfile })
  first.receive(pong(999999))
  store.setConnectionState('connected')
  store.setPlayerEnterWorld(1, 'old world', 7)
  assert.equal(store.isInGame, true)
  const retiredMessage = first.onmessage!
  const retiredClose = first.onclose!
  const second = connect()
  assert.equal(hasWorldParams(), false)
  assert.equal(store.isInGame, false, 'connection reset invalidates old world without a state callback')
  assert.equal(gameCalendarSync.getCalendar(), null)
  retiredMessage({ data: proto.ServerMessage.encode({ serverConstants: serverProfile }).finish() } as unknown as MessageEvent)
  retiredClose({ reason: 'late close' } as CloseEvent)
  assert.equal(serverConstants.getSnapshot(), null)
  assert.equal(connection.getState(), 'connected')
  const next = { ...serverProfile, coordPerTile: 24, chunkSize: 64, tickRate: 30, realSecondsPerGameDay: 100 }
  second.receive({ serverConstants: next })
  second.receive(pong(0))
  assert.equal(store.isInGame, false, 'fresh constants cannot reactivate the old world')
  assert.equal(gameCalendarSync.getCalendar()?.year, 1n)
  assert.equal(getCoordPerTile(), 24)
  assert.equal(getChunkSize(), 64)
  assert.deepEqual(coordGame2Screen(24, 0), { x: 32, y: 16 })
  assert.equal(moveController.getTickRate(), 30)
  retiredMessage({ data: proto.ServerMessage.encode(proto.ServerMessage.fromObject(pong(999999))).finish() } as unknown as MessageEvent)
  assert.equal(gameCalendarSync.getCalendar()?.year, 1n)
  connection.disconnect()
  second.receive({ serverConstants: next })
  assert.equal(hasWorldParams(), false)
})

test('auth failure, remote close, socket error and direct disconnect clear connection state', t => {
  const { connection, connect } = connectionFixture(t)
  for (const end of ['close', 'error', 'disconnect', 'auth-failure']) {
    const socket = connect()
    socket.receive({ serverConstants: serverProfile })
    socket.receive(pong(100))
    assert.equal(hasWorldParams(), true)
    if (end === 'close') socket.onclose?.({ reason: 'remote' } as CloseEvent)
    if (end === 'error') socket.onerror?.()
    if (end === 'disconnect') connection.disconnect()
    if (end === 'auth-failure') {
      connection.connect('bad-token')
      const unauthenticated = FakeSocket.sockets.at(-1)!
      unauthenticated.onopen?.()
      unauthenticated.receive({ serverConstants: serverProfile })
      assert.equal(hasWorldParams(), false)
      unauthenticated.receive({ authResult: { success: false, errorMessage: 'Denied' } })
    }
    assert.equal(hasWorldParams(), false)
    assert.equal(gameCalendarSync.getCalendar(), null)
    assert.equal(timeSync.isInitialized(), false)
  }
})

test('empty renderer and terrain calculations wait safely for constants', () => {
  serverConstants.reset()
  const render = Object.create(Render.prototype) as { update(): void }
  assert.doesNotThrow(() => render.update(), 'pre-auth frame must not touch world consumers')
  const terrain = new TerrainManager()
  assert.doesNotThrow(() => terrain.setCameraPosition(100, 100))
  assert.equal(hasWorldParams(), false)
  assert.throws(() => getCoordPerTile())
  serverConstants.accept(serverProfile)
  terrain.setCameraPosition(100, 100)
  const state = terrain as unknown as { cameraSubchunkX: number; cameraSubchunkY: number }
  assert.ok(Number.isFinite(state.cameraSubchunkX) && Number.isFinite(state.cameraSubchunkY))
  serverConstants.reset()
  assert.doesNotThrow(() => render.update(), 'reconnect also gates frames after reset')
})
