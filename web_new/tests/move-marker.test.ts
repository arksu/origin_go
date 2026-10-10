import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { Container, Graphics } from 'pixi.js'
import { createPinia, setActivePinia } from 'pinia'
import { MoveMarkerManager } from '../src/game/MoveMarkerManager'
import { Render } from '../src/game/Render'
import { gameFacade } from '../src/game/GameFacade'
import { moveController } from '../src/game/MoveController'
import { playerCommandController } from '../src/game/PlayerCommandController'
import { soundManager } from '../src/game/SoundManager'
import { gameConnection } from '../src/network/GameConnection'
import { registerMessageHandlers } from '../src/network/handlers'
import { initNetwork } from '../src/network'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { coordGame2Screen } from '../src/game/utils/coordConvert'
import { setWorldParams } from './serverConstantsFixture'
import { TERRAIN_BASE_Z_INDEX } from '../src/constants/terrain'

function markerFixture(t: TestContext) {
  let now = 1000
  const parent = new Container()
  const marker = new MoveMarkerManager(parent, () => now)
  const ring = parent.children[0] as Graphics
  setWorldParams(32, 128)
  t.after(() => { marker.destroy(); parent.destroy(); setWorldParams(32, 128) })
  return { parent, marker, ring, advance(ms: number) { now += ms; marker.update() } }
}

test('one ring grows from 16x8 to 48x24 and fades completely after 700 ms', t => {
  const { parent, marker, ring, advance } = markerFixture(t)
  assert.equal(ring.visible, false)
  marker.show(64, 32)
  const position = coordGame2Screen(64, 32)
  assert.equal(ring.x, position.x)
  assert.equal(ring.y, position.y)
  assert.equal(ring.zIndex, TERRAIN_BASE_Z_INDEX + position.y)
  assert.equal(ring.eventMode, 'none')
  assert.equal(ring.visible, true)
  assert.equal(ring.alpha, 0.9)
  assert.equal(ring.scale.x * 48, 16)
  assert.equal(ring.scale.y * 24, 8)
  const geometry = ring.context
  let previousAlpha = ring.alpha, previousScale = ring.scale.x
  for (let frame = 0; frame < 7; frame++) {
    advance(100)
    assert.ok(ring.alpha < previousAlpha)
    assert.ok(ring.scale.x > previousScale)
    previousAlpha = ring.alpha
    previousScale = ring.scale.x
  }
  assert.equal(ring.scale.x * 48, 48)
  assert.equal(ring.scale.y * 24, 24)
  assert.equal(ring.alpha, 0)
  assert.equal(ring.visible, false)
  advance(5000)
  assert.equal(ring.visible, false)
  assert.equal(parent.children.length, 1)
  assert.equal(ring.context, geometry, 'animation reuses its geometry')
})

test('same-target updates never restart the ring, including after it has faded', t => {
  const { marker, ring, advance } = markerFixture(t)
  marker.show(0, 0)
  advance(350)
  const alpha = ring.alpha, scale = ring.scale.x
  marker.show(0, 0)
  assert.equal(ring.alpha, alpha)
  assert.equal(ring.scale.x, scale)
  advance(350)
  marker.show(0, 0)
  assert.equal(ring.visible, false)
  advance(700)
  assert.equal(ring.alpha, 0)
})

test('a new target replaces the ring; ending the route preserves the tail and allows the same target again', t => {
  const { parent, marker, ring, advance } = markerFixture(t)
  marker.show(64, 32)
  advance(350)
  marker.show(-32, 64)
  assert.equal(parent.children.length, 1)
  assert.equal(parent.children[0], ring)
  assert.equal(ring.alpha, 0.9)
  assert.equal(ring.scale.x, 1 / 3)
  assert.equal(ring.x, coordGame2Screen(-32, 64).x)
  assert.equal(ring.y, coordGame2Screen(-32, 64).y)
  advance(350)
  marker.endTarget()
  assert.equal(ring.visible, true)
  assert.ok(Math.abs(ring.alpha - 0.45) < 1e-10)
  advance(350)
  assert.equal(ring.visible, false)
  marker.show(-32, 64)
  assert.equal(ring.visible, true)
  assert.equal(ring.alpha, 0.9)
})

test('world-space positioning follows camera pan and zoom without moving the target', t => {
  const { parent, marker, ring } = markerFixture(t)
  marker.show(64, -32)
  const position = coordGame2Screen(64, -32)
  for (const zoom of [0.5, 1, 3]) {
    parent.position.set(150, -100)
    parent.scale.set(zoom)
    const screen = ring.toGlobal({ x: 0, y: 0 })
    assert.equal(screen.x, position.x * zoom + 150)
    assert.equal(screen.y, position.y * zoom - 100)
  }
})

test('hiding and world reset discard both the ring and its remembered target', t => {
  const { marker, ring, advance } = markerFixture(t)
  for (const reset of [() => marker.hide(), () => marker.clear()]) {
    marker.show(0, 0)
    advance(100)
    reset()
    assert.equal(ring.visible, false)
    assert.equal(ring.alpha, 0)
    marker.show(0, 0)
    assert.equal(ring.visible, true)
    assert.equal(ring.alpha, 0.9)
  }
})

test('destroy removes the reusable ring from its parent', () => {
  const parent = new Container()
  const marker = new MoveMarkerManager(parent)
  const ring = parent.children[0]!
  marker.show(0, 0)
  marker.destroy()
  assert.equal(ring.destroyed, true)
  assert.equal(parent.children.length, 0)
  parent.destroy()
})

function dispatch(packet: proto.IServerMessage): void {
  messageDispatcher.dispatch(proto.ServerMessage.decode(proto.ServerMessage.encode(packet).finish()))
}

function networkFixture(t: TestContext) {
  const fixture = markerFixture(t)
  setActivePinia(createPinia())
  const store = useGameStore()
  t.mock.method(console, 'log', () => {})
  t.mock.method(console, 'warn', () => {})
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  t.mock.method(gameFacade, 'showMoveTargetMarker', (x: number, y: number) => fixture.marker.show(x, y))
  t.mock.method(gameFacade, 'endMoveTargetMarker', () => fixture.marker.endTarget())
  t.mock.method(gameFacade, 'hideMoveTargetMarker', () => fixture.marker.hide())
  t.mock.method(gameFacade, 'resetWorld', () => fixture.marker.clear())
  t.mock.method(gameFacade, 'getObjectCarryVisualCarrierId', () => null)
  t.mock.method(gameFacade, 'setObjectCarryVisualRelation', () => {})
  t.mock.method(gameConnection, 'send', () => {})
  setWorldParams(32, 128)
  registerMessageHandlers()
  dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 1} })
  t.after(() => { store.reset(); moveController.clear() })
  const send = (sequence: number, movement: proto.IEntityMovement, batched = false, entityId = 17, isTeleport = false) => {
    const entry = { entityId, moveSeq: sequence, serverTimeMs: 10000 + sequence * 100, movement, isTeleport }
    dispatch(batched ? { objectMoveBatch: { moves: [entry] } } : { objectMove: entry })
  }
  return { ...fixture, send }
}

const moving = (x = 64, y = 32): proto.IEntityMovement => ({
  position: { x: 10, y: 20 }, velocity: { x: 3, y: 2 }, isMoving: true, targetPosition: { x, y },
})

for (const batched of [false, true]) {
  test(`${batched ? 'batched' : 'single'} movement starts rings only from accepted local movement with a destination`, t => {
    const { marker, ring, send, advance } = networkFixture(t)
    playerCommandController.sendMapClick(64, 32, 0, 0)
    assert.equal(ring.visible, false, 'sending a click is not confirmation')
    send(1, moving(), batched, 18)
    assert.equal(ring.visible, false, 'remote movement cannot mark the local target')
    send(1, { ...moving(), isMoving: false }, batched)
    assert.equal(ring.visible, false, 'a target without movement is not confirmation')
    send(2, { position: { x: 10, y: 20 }, isMoving: true }, batched)
    assert.equal(ring.visible, false, 'directional movement has no target')
    send(3, moving(), batched)
    assert.equal(ring.visible, true)
    advance(350)
    const alpha = ring.alpha, scale = ring.scale.x, x = ring.x, y = ring.y
    send(3, moving(-64, -32), batched)
    send(2, { isMoving: false }, batched)
    assert.equal(ring.alpha, alpha)
    assert.equal(ring.scale.x, scale)
    assert.equal(ring.x, x)
    assert.equal(ring.y, y)
    marker.show(64, 32)
    assert.equal(ring.alpha, alpha, 'a stale stop cannot clear the remembered target')
    send(4, moving(), batched)
    assert.equal(ring.alpha, alpha, 'regular updates do not restart the ring')
    advance(350)
    send(5, moving(), batched)
    assert.equal(ring.visible, false, 'regular updates do not revive an expired ring')
    send(6, moving(-64, -32), batched)
    assert.equal(ring.visible, true)
    assert.equal(ring.alpha, 0.9)
    advance(350)
    send(7, { ...moving(-64, -32), isMoving: false }, batched)
    assert.equal(ring.visible, true, 'arrival preserves the fade even with a remaining target')
    advance(350)
    assert.equal(ring.visible, false)
    send(8, moving(-64, -32), batched)
    assert.equal(ring.visible, true, 'a fresh route can mark the same point again')
    advance(350)
    send(9, { position: { x: 10, y: 20 }, isMoving: true }, batched)
    assert.equal(ring.visible, true, 'WASD handoff preserves the existing tail')
    advance(350)
    assert.equal(ring.visible, false)
    send(10, moving(), batched)
    send(0, moving(-64, -32), batched, 17, true)
    assert.equal(ring.visible, false, 'teleport clears the ring despite a destination')
    send(1, moving(), batched)
    assert.equal(ring.visible, true)
    dispatch({ playerLeaveWorld: {} })
    assert.equal(ring.visible, false, 'world leave immediately clears the ring')
    dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 2} })
    send(1, moving(), batched)
    assert.equal(ring.visible, true, 'world re-entry clears target memory')
    dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 3} })
    assert.equal(ring.visible, false, 'new entry clears a ring without a preceding leave')
  })
}

test('despawning the local player immediately clears its ring', t => {
  const { marker, ring, advance } = markerFixture(t)
  const render = Object.create(Render.prototype) as Render
  Object.assign(render, {
    playerEntityId: 17, moveMarkerManager: marker,
    objectManager: { despawnObject() {} }, nicknameManager: { remove() {} },
    damageNumberManager: { rememberDespawn() {} }, combatSectorPreview: { clear() {} },
  })
  marker.show(64, 32)
  advance(350)
  render.despawnObject(18)
  assert.equal(ring.visible, true)
  render.despawnObject(17)
  assert.equal(ring.visible, false)
})

test('disconnect and connection errors immediately clear the ring and its target memory', t => {
  const { ring, send } = networkFixture(t)
  let changeState: Parameters<typeof gameConnection.onStateChange>[0] = () => { throw new Error('state handler missing') }
  t.mock.method(gameConnection, 'onStateChange', (handler: typeof changeState) => { changeState = handler })
  t.mock.method(gameConnection, 'onMessage', () => {})
  initNetwork()
  for (const state of ['disconnected', 'error'] as const) {
    dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 1} })
    send(1, moving())
    assert.equal(ring.visible, true)
    changeState(state)
    assert.equal(ring.visible, false)
  }
})
