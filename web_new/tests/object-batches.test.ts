import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { gameFacade } from '../src/game/GameFacade'
import { moveController } from '../src/game/MoveController'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { timeSync } from '../src/network/TimeSync'
import { LOCOMOTION_STOP_MS } from '../src/game/movementTiming'

function dispatch(packet: proto.IServerMessage): void {
  const encoded = proto.ServerMessage.encode(proto.ServerMessage.create(packet)).finish()
  messageDispatcher.dispatch(proto.ServerMessage.decode(encoded))
}

function spawn(entityId: number, overrides: proto.IS2C_ObjectSpawn = {}): proto.IS2C_ObjectSpawn {
  return { entityId, typeId: 1, resourcePath: 'player', streamEpoch: 1, name: `Player ${entityId}`,
    position: { position: { x: entityId, y: -entityId, heading: 1.25 }, size: { x: 8, y: 9 } },
    characterVisual: { generation: '0:4294967297', revision: 1, equipment: [] },
    actionAnimation: { generation: '0:4294967297', revision: 2, animationKey: 'chop', totalTicks: 20,
      elapsedTicks: 4, tickDurationMs: 100, serverTimeMs: 10000, targetPosition: { x: 40, y: -20 } },
    ...overrides }
}

function move(entityId: number, overrides: proto.IS2C_ObjectMove = {}): proto.IS2C_ObjectMove {
  return { entityId, serverTimeMs: 10000, moveSeq: 5,
    movement: { position: { x: 50, y: -20, heading: 1.25 }, velocity: { x: 2, y: -3 },
      moveMode: proto.MovementMode.MOVE_MODE_RUN, isMoving: true, targetPosition: { x: 90, y: -80 } },
    ...overrides }
}

function setup(t: TestContext) {
  setActivePinia(createPinia())
  const store = useGameStore()
  const carriers = new Map<number, number>()
  t.after(() => { store.reset(); moveController.clear() })
  const spawns = t.mock.method(gameFacade, 'spawnObject', () => {})
  const names = t.mock.method(gameFacade, 'setObjectNickname', () => {})
  t.mock.method(gameFacade, 'setCharacterEquipment', async () => {})
  t.mock.method(gameFacade, 'resetWorld', () => { carriers.clear() })
  t.mock.method(gameFacade, 'getObjectCarryVisualCarrierId', (entityId: number) => carriers.get(entityId) ?? null)
  const carryUpdates = t.mock.method(gameFacade, 'setObjectCarryVisualRelation', (entityId: number, carrierId: number | null) => {
    if (carrierId == null) carriers.delete(entityId)
    else carriers.set(entityId, carrierId)
  })
  const targets = t.mock.method(gameFacade, 'showMoveTargetMarker', () => {})
  const endTargets = t.mock.method(gameFacade, 'endMoveTargetMarker', () => {})
  const clearTargets = t.mock.method(gameFacade, 'hideMoveTargetMarker', () => {})
  const errors = t.mock.method(console, 'error', () => {})
  t.mock.method(console, 'log', () => {})
  registerMessageHandlers()
  const begin = () => dispatch({ playerEnterWorld: { entityId: 17, streamEpoch: 1, coordPerTile: 12, chunkSize: 4, tickRate: 10 } })
  begin()
  return { store, carriers, spawns, names, carryUpdates, targets, endTargets, clearTargets, errors, begin }
}

test('directional batches end the local target and settle both local and remote players at the resolved stop', t => {
  t.mock.timers.enable({ apis: ['Date'], now: 10000 })
  const { targets, endTargets, clearTargets, errors } = setup(t)
  t.mock.method(timeSync, 'estimateServerNowMs', (now: number) => now)
  t.mock.method(timeSync, 'getInterpolationDelayMs', () => 100)
  dispatch({ objectSpawnBatch: { spawns: [spawn(17), spawn(18)] } })
  dispatch({ objectMove: move(17) })
  assert.equal(targets.mock.callCount(), 1)
  const send = (sequence: number, moving: boolean) => dispatch({ objectMoveBatch: { moves: [17, 18].map(entityId => move(entityId, {
    moveSeq: sequence, serverTimeMs: Date.now(), movement: {
      position: { x: 60, y: -20, heading: 0 }, velocity: { x: moving ? 32 : 0, y: 0 },
      moveMode: proto.MovementMode.MOVE_MODE_WALK, isMoving: moving,
    },
  })) } })
  send(6, true)
  assert.equal(endTargets.mock.callCount(), 1)
  assert.equal(clearTargets.mock.callCount(), 0, 'direction lets the existing ring finish fading')
  moveController.update()
  t.mock.timers.tick(100)
  send(7, false)
  for (let frame = 0; frame < (100 + LOCOMOTION_STOP_MS) / 10 + 2; frame++) {
    t.mock.timers.tick(10)
    moveController.update()
  }
  for (const entityId of [17, 18]) {
    const position = moveController.getRenderPosition(entityId)!
    assert.equal(position.x, 60)
    assert.equal(position.y, -20)
    assert.equal(position.isMoving, false)
    assert.equal(position.distanceMoved, 0)
    assert.equal(moveController.getEntityDebugMetrics(entityId)!.lastMoveSeq, 7)
  }
  t.mock.timers.tick(5000)
  const later = moveController.update()
  assert.equal(later.get(17)!.x, 60)
  assert.equal(later.get(18)!.x, 60)
  assert.equal(targets.mock.callCount(), 1, 'direction must not create a point marker')
  assert.equal(errors.mock.callCount(), 0)
})

test('single and batch protocol roundtrips preserve every entry field and uint64 values', () => {
  const spawnEntry = proto.S2C_ObjectSpawn.fromObject({ ...spawn(17), entityId: '18446744073709551615', carriedByEntityId: '9007199254740993',
    characterVisual: { generation: '0:4294967297', revision: '18446744073709551615', equipment: [{ slot: 7, visualKey: 'stone_axe' }] } })
  const moveEntry = proto.S2C_ObjectMove.fromObject({ ...move(17), entityId: '18446744073709551615', carriedByEntityId: '9007199254740993',
    moveSeq: 4294967295, isTeleport: true })
  for (const payload of [
    { objectSpawn: spawnEntry }, { objectSpawnBatch: { spawns: [spawnEntry, spawn(18, { streamEpoch: 2 })] } },
    { objectMove: moveEntry }, { objectMoveBatch: { moves: [moveEntry, move(18, { moveSeq: 0 })] } },
  ]) {
    const packet = proto.ServerMessage.fromObject(payload)
    const decoded = proto.ServerMessage.decode(proto.ServerMessage.encode(packet).finish())
    assert.deepEqual(decoded.toJSON(), packet.toJSON())
  }
})

test('spawn singles and batches have identical store, animation, name and carry effects in entry order', t => {
  const { store, carriers, spawns, names, errors, begin } = setup(t)
  const entries = [spawn(17), spawn(18, { carriedByEntityId: 17 })]
  const run = (batched: boolean) => {
    begin()
    const spawnOffset = spawns.mock.callCount(), nameOffset = names.mock.callCount()
    if (batched) dispatch({ objectSpawnBatch: { spawns: entries } })
    else for (const entry of entries) dispatch({ objectSpawn: entry })
    return {
      entities: JSON.parse(JSON.stringify([...store.entities.entries()])),
      carriers: [...carriers.entries()],
      movement: entries.map(entry => moveController.getEntityDebugMetrics(Number(entry.entityId))),
      spawns: spawns.mock.calls.slice(spawnOffset).map(call => call.arguments),
      names: names.mock.calls.slice(nameOffset).map(call => call.arguments),
    }
  }
  const singles = run(false), batch = run(true)
  assert.deepEqual(batch, singles)
  assert.deepEqual(batch.spawns.map(args => args[0]!.entityId), [17, 18])
  assert.equal(store.entities.get(17)!.actionAnimation!.elapsedTicks, 4)
  assert.equal(errors.mock.callCount(), 0)
})

test('a bad animation incarnation and stale spawn epoch do not prevent later entries or singles', t => {
  const { store, spawns, errors } = setup(t)
  dispatch({ objectSpawnBatch: { spawns: [
    spawn(90, { actionAnimation: { ...spawn(90).actionAnimation, generation: '1:4294967297' } }),
    spawn(91, { streamEpoch: 2 }), spawn(17), spawn(18),
  ] } })
  assert.deepEqual([...store.entities.keys()], [17, 18])
  assert.deepEqual(spawns.mock.calls.map(call => call.arguments[0]!.entityId), [17, 18])
  assert.equal(errors.mock.callCount(), 1)
  assert.match(String((errors.mock.calls[0]!.arguments[1] as { error: Error }).error), /incarnation mismatch/)
  dispatch({ objectSpawn: spawn(19) })
  dispatch({ objectDespawn: { entityId: 18, streamEpoch: 1 } })
  assert.deepEqual([...store.entities.keys()], [17, 19])
  dispatch({ objectSpawnBatch: {} })
  assert.equal(errors.mock.callCount(), 1)
})

for (const scenario of [
  { name: 'stale movement', carriedInitially: false, override: { moveSeq: 4, carriedByEntityId: 99 }, snaps: 0, ignored: 1 },
  { name: 'teleport', carriedInitially: false, override: { moveSeq: 0, isTeleport: true }, snaps: 1, ignored: 0 },
  { name: 'carry drop with default sequence', carriedInitially: true, override: { moveSeq: 0 }, snaps: 1, ignored: 0 },
] as const) {
  test(`single and batch ${scenario.name} retain movement, store and carry semantics`, t => {
    const { store, carriers, carryUpdates, targets, clearTargets, errors, begin } = setup(t)
    const run = (batched: boolean) => {
      begin()
      dispatch({ objectSpawnBatch: { spawns: [spawn(17), spawn(18)] } })
      dispatch({ objectMove: move(17, { carriedByEntityId: scenario.carriedInitially ? 99 : 0 }) })
      const entries = [move(17, { ...scenario.override, serverTimeMs: 10100,
        movement: { position: { x: 75, y: -40, heading: 2.5 }, velocity: { x: 3, y: -4 },
          moveMode: proto.MovementMode.MOVE_MODE_WALK, isMoving: true, targetPosition: { x: 100, y: -90 } } }), move(18)]
      const carryOffset = carryUpdates.mock.callCount(), targetOffset = targets.mock.callCount()
      const clearOffset = clearTargets.mock.callCount()
      if (batched) dispatch({ objectMoveBatch: { moves: entries } })
      else for (const entry of entries) dispatch({ objectMove: entry })
      return {
        entities: JSON.parse(JSON.stringify([...store.entities.entries()])),
        playerPosition: { ...store.playerPosition },
        carriers: [...carriers.entries()],
        metrics: moveController.getEntityDebugMetrics(17),
        carryUpdates: carryUpdates.mock.calls.slice(carryOffset).map(call => call.arguments),
        targets: targets.mock.calls.slice(targetOffset).map(call => call.arguments),
        clearTargets: clearTargets.mock.calls.slice(clearOffset).map(call => call.arguments),
      }
    }
    const singles = run(false), batch = run(true)
    assert.deepEqual(batch, singles)
    assert.equal(batch.metrics!.snapCount, scenario.snaps)
    assert.equal(batch.metrics!.ignoredOutOfOrder, scenario.ignored)
    assert.equal(batch.metrics!.lastMoveSeq, scenario.ignored ? 5 : 0)
    assert.equal(batch.targets.length, 0, 'stale moves and snaps must not start a target ring')
    assert.equal(batch.clearTargets.length, scenario.ignored ? 0 : 1)
    assert.equal(store.entities.get(17)!.movement!.position.x, 75, 'existing store update still follows rejected stale moves')
    assert.deepEqual(batch.carryUpdates.map(args => args[0]), [17, 18])
    if (scenario.name === 'stale movement') assert.equal(carriers.get(17), 99)
    else assert.equal(batch.metrics!.visualX, 75, 'teleport and drop snap immediately')
    dispatch({ objectMove: move(18, { moveSeq: 6 }) })
    assert.equal(moveController.getEntityDebugMetrics(18)!.lastMoveSeq, 6, 'single movement remains accepted after batch')
    assert.equal(errors.mock.callCount(), 0)
  })
}

test('a failing move entry leaves later moves and following messages usable', t => {
  const { store, errors, endTargets } = setup(t)
  dispatch({ objectSpawnBatch: { spawns: [spawn(17), spawn(18)] } })
  const onObjectMove = moveController.onObjectMove.bind(moveController)
  t.mock.method(moveController, 'onObjectMove', (...args: Parameters<typeof onObjectMove>) => {
    if (args[0] === 90) throw new Error('Cannot apply movement')
    return onObjectMove(...args)
  })
  dispatch({ objectMoveBatch: { moves: [move(90), move(17), move(18)] } })
  assert.equal(errors.mock.callCount(), 1)
  assert.equal(moveController.getEntityDebugMetrics(17)!.lastMoveSeq, 5)
  assert.equal(moveController.getEntityDebugMetrics(18)!.lastMoveSeq, 5)
  assert.equal(store.entities.get(18)!.movement!.position.x, 50)
  dispatch({ objectMove: move(17, { moveSeq: 6, movement: { position: { x: 75, y: -10 } } }) })
  assert.equal(endTargets.mock.callCount(), 1)
  assert.equal(store.entities.get(17)!.movement!.position.x, 75)
  dispatch({ objectMoveBatch: {} })
  assert.equal(errors.mock.callCount(), 1)
})
