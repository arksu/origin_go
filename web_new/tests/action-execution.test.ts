import { setWorldParams } from './serverConstantsFixture'
import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { util } from 'protobufjs/minimal'
import { ActionExecutionReceiver } from '../src/network/ActionExecutionReceiver'
import { proto } from '../src/network/proto/packets.js'
import { gameFacade } from '../src/game/GameFacade'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { registerMessageHandlers, resetAttackResultStream } from '../src/network/handlers'
import { useGameStore } from '../src/stores/gameStore'
import { useActionCooldownStore } from '../src/stores/actionCooldownStore'
import type { DirectionSector } from '../src/game/hud/directionAim'

const completed = proto.CyclicActionFinishResult.CYCLIC_ACTION_FINISH_RESULT_COMPLETED
const canceled = proto.CyclicActionFinishResult.CYCLIC_ACTION_FINISH_RESULT_CANCELED
const definitions = new Map<string, proto.IActionDefinition>([
  ['strike', { id: 'strike', targetKind: 'direction', sector: { range: 18, sectorAngle: Math.PI / 2 } }],
  ['sweep', { id: 'sweep', targetKind: 'direction', sector: { range: 18, sectorAngle: Math.PI / 2 } }],
  ['dig', { id: 'dig', targetKind: 'tile' }],
])

function state(overrides: Record<string, unknown> = {}): proto.IS2C_ActionStateChanged {
  return { streamEpoch: 7, actionGeneration: 1, actionId: 'strike', phase: 'executing', facingAngle: 0,
    serverTimeMs: 1000, ...overrides } as proto.IS2C_ActionStateChanged
}

function idle(): proto.IS2C_ActionStateChanged {
  return state({ actionGeneration: 0, actionId: '', phase: 'idle', facingAngle: undefined })
}

function fixture() {
  const shows: { angle: number; sector: DirectionSector }[] = []
  let clears = 0
  const receiver = new ActionExecutionReceiver({
    showAttackSector: (angle, sector) => { shows.push({ angle, sector }) },
    clearAttackSector: () => { clears++ },
  })
  receiver.reset(7)
  return { receiver, shows, clears: () => clears }
}

test('execution confirmation uses catalog geometry and explicit zero independently of animations', () => {
  const { receiver, shows } = fixture()
  assert.equal(receiver.acceptState(state(), definitions), true)
  assert.deepEqual(shows, [{ angle: 0, sector: { range: 18, angle: Math.PI / 2 } }])
  assert.equal(receiver.acceptState(state(), definitions), true)
  assert.equal(shows.length, 1, 'repeated states must not restart the preview deadline')
  assert.equal(receiver.acceptState(state({ actionGeneration: 2, actionId: 'sweep', facingAngle: 1.25 }), definitions), true)
  assert.deepEqual(shows[1], { angle: 1.25, sector: { range: 18, angle: Math.PI / 2 } })
})

test('execution receiver orders full uint64 generations without losing precision through protobuf', () => {
  const { receiver, shows } = fixture()
  for (const [generation, accepted] of [
    ['9007199254740992', true], ['9007199254740993', true],
    ['9007199254740992', false], ['18446744073709551615', true],
  ] as const) {
    const encoded = proto.S2C_ActionStateChanged.encode(proto.S2C_ActionStateChanged.fromObject(state({ actionGeneration: generation }))).finish()
    const decoded = proto.S2C_ActionStateChanged.decode(encoded)
    assert.equal(String(decoded.actionGeneration), generation)
    assert.equal(Object.hasOwn(decoded, 'facingAngle'), true)
    assert.equal(decoded.facingAngle, 0)
    assert.equal(receiver.acceptState(decoded, definitions), accepted)
  }
  assert.equal(shows.length, 3)
})

test('invalid or stale confirmation never consumes identity or creates a partial preview', () => {
  const malformedLong = new util.Long(1, 0, true)
  malformedLong.low = NaN
  for (const invalid of [
    { streamEpoch: 8 }, { actionGeneration: 0 }, { actionGeneration: -1 },
    { actionGeneration: Number.MAX_SAFE_INTEGER + 1 }, { actionGeneration: '18446744073709551616' },
    { actionGeneration: '01' }, { actionGeneration: { toString: () => '1' } }, { actionGeneration: malformedLong },
    { facingAngle: undefined }, { facingAngle: NaN }, { facingAngle: Infinity }, { facingAngle: -0.1 },
    { facingAngle: Math.fround(2 * Math.PI) + 0.01 }, { actionId: '' }, { phase: 'invalid' },
  ]) {
    const { receiver, shows, clears } = fixture()
    const before = clears()
    assert.equal(receiver.acceptState(state(invalid), definitions), false, JSON.stringify(invalid))
    assert.equal(shows.length, 0)
    assert.equal(clears(), before)
    assert.equal(receiver.acceptState(state(), definitions), true)
    assert.equal(shows.length, 1)
  }
  const { receiver, shows, clears } = fixture()
  assert.equal(receiver.acceptState(state({ actionGeneration: 2 }), definitions), true)
  const clearCount = clears()
  for (const invalid of [
    state({ actionGeneration: 1 }), state({ actionGeneration: 2, actionId: 'sweep' }),
    state({ actionGeneration: 2, facingAngle: 0.5 }), idleWithGeneration(2),
  ]) assert.equal(receiver.acceptState(invalid, definitions), false)
  assert.equal(shows.length, 1)
  assert.equal(clears(), clearCount)
})

function idleWithGeneration(generation: number): proto.IS2C_ActionStateChanged {
  return { ...idle(), actionGeneration: generation }
}

test('execution geometry must be present and valid in the catalog before accepting a directional start', () => {
  for (const sector of [null, {}, { range: 0, sectorAngle: 1 }, { range: 18, sectorAngle: NaN }, { range: 18, sectorAngle: 7 }]) {
    const { receiver, shows } = fixture()
    const catalog = new Map([['strike', { id: 'strike', targetKind: 'direction', sector }]])
    assert.equal(receiver.acceptState(state(), catalog), false)
    assert.equal(shows.length, 0)
    assert.equal(receiver.acceptState(state(), definitions), true)
  }
  const { receiver } = fixture()
  assert.equal(receiver.acceptState(state(), new Map()), false)
  assert.equal(receiver.acceptState(state({ facingAngle: Math.fround(2 * Math.PI) }), definitions), true)
})

test('completion preserves preview TTL while cancellation and unpaired idle clear immediately', () => {
  for (const result of [completed, canceled]) {
    const { receiver, shows, clears } = fixture()
    receiver.acceptState(state(), definitions)
    const before = clears()
    receiver.finish({ actionId: 'sweep', result })
    assert.equal(clears(), before, 'a terminal for another action must not affect the preview')
    receiver.finish({ actionId: 'strike', result: proto.CyclicActionFinishResult.CYCLIC_ACTION_FINISH_RESULT_UNSPECIFIED })
    assert.equal(clears(), before)
    receiver.finish({ actionId: 'strike', result })
    assert.equal(clears(), before + (result === canceled ? 1 : 0))
    receiver.finish({ actionId: 'strike', result })
    assert.equal(clears(), before + (result === canceled ? 1 : 0), 'duplicate terminals do nothing')
    assert.equal(receiver.acceptState(state(), definitions), false, 'a terminal generation cannot restart the sector')
    assert.equal(receiver.acceptState(idle(), definitions), true)
    assert.equal(clears(), before + (result === canceled ? 1 : 0), 'idle must preserve a completed preview')
    assert.equal(receiver.acceptState(state(), definitions), false, 'idle preserves the generation watermark')
    assert.equal(receiver.acceptState(state({ actionGeneration: 2 }), definitions), true)
    assert.equal(shows.length, 2)
  }
  const { receiver, clears } = fixture()
  receiver.acceptState(state(), definitions)
  const before = clears()
  assert.equal(receiver.acceptState(idle(), definitions), true)
  assert.equal(clears(), before + 1, 'missing terminal must fail closed')
})

test('noncombat phase transitions still update action UI without starting a sector', () => {
  const { receiver, shows } = fixture()
  for (const phase of ['selecting', 'approaching', 'executing', 'cooldown_wait', 'executing', 'selecting']) {
    assert.equal(receiver.acceptState(state({ actionId: 'dig', phase, facingAngle: undefined }), definitions), true)
  }
  assert.equal(shows.length, 0)
  assert.equal(receiver.acceptState(state({ actionId: 'dig' }), definitions), false, 'noncombat state must not smuggle a facing angle')
})

test('reset clears the sector and resets the generation watermark only for a valid active epoch', () => {
  const { receiver, shows, clears } = fixture()
  receiver.acceptState(state({ actionGeneration: 999 }), definitions)
  const before = clears()
  receiver.reset(8)
  assert.equal(clears(), before + 1)
  assert.equal(receiver.acceptState(state({ actionGeneration: 1000 }), definitions), false)
  assert.equal(receiver.acceptState(state({ streamEpoch: 8 }), definitions), true)
  assert.equal(shows.length, 2)
  for (const epoch of [NaN, Infinity, -1, 0, 0.5, 4294967296]) {
    receiver.reset(epoch)
    assert.equal(receiver.acceptState(state({ streamEpoch: epoch }), definitions), false)
  }
})

function dispatch(packet: proto.IServerMessage): void {
  const encoded = proto.ServerMessage.encode(proto.ServerMessage.fromObject(packet)).finish()
  messageDispatcher.dispatch(proto.ServerMessage.decode(encoded))
}

function setupHandlers(t: TestContext) {
  setActivePinia(createPinia())
  const store = useGameStore(), cooldowns = useActionCooldownStore()
  const shows = t.mock.method(gameFacade, 'showAttackSector', () => {})
  const clears = t.mock.method(gameFacade, 'clearAttackSector', () => {})
  t.mock.method(gameFacade, 'spawnObject', () => {})
  t.mock.method(console, 'log', () => {})
  t.mock.method(console, 'error', () => {})
  setWorldParams(32, 128)
  registerMessageHandlers()
  const enter = (epoch = 7) => {
    dispatch({ playerEnterWorld: { entityId: 42, streamEpoch: epoch} })
    dispatch({ actionList: { actions: [...definitions.values()] } })
  }
  enter()
  t.after(() => { resetAttackResultStream(); store.reset() })
  return { store, cooldowns, shows, clears, enter }
}

test('network handler validates epoch and full execution before changing store or cooldowns', t => {
  const { store, cooldowns, shows } = setupHandlers(t)
  const valid = state({ actionGeneration: '9007199254740993', cooldowns: [{ actionId: 'strike', startedAtMs: 1000, expiresAtMs: 2000 }] })
  dispatch({ actionStateChanged: valid })
  assert.equal(store.gameActionState.actionId, 'strike')
  assert.equal(String(store.gameActionState.actionGeneration), '9007199254740993')
  assert.equal(cooldowns.cooldowns.get('strike')?.expiresAtMs, 2000)
  assert.equal(shows.mock.callCount(), 1)
  const beforeState = store.gameActionState
  for (const invalid of [
    { ...valid, streamEpoch: 6, cooldowns: [] },
    { ...valid, actionGeneration: '9007199254740992', cooldowns: [] },
    { ...valid, facingAngle: 0.5, cooldowns: [] },
    { ...valid, actionGeneration: '9007199254740994', facingAngle: NaN, cooldowns: [] },
  ]) dispatch({ actionStateChanged: state(invalid) })
  assert.equal(store.gameActionState, beforeState)
  assert.equal(cooldowns.cooldowns.get('strike')?.expiresAtMs, 2000)
  assert.equal(shows.mock.callCount(), 1)
  dispatch({ actionStateChanged: valid })
  assert.equal(shows.mock.callCount(), 1, 'same-generation refresh must not restart display')
})

test('network terminal FIFO preserves successful preview and clears cancellation and lifecycle resets', t => {
  const { shows, clears, enter } = setupHandlers(t)
  dispatch({ actionStateChanged: state() })
  const before = clears.mock.callCount()
  dispatch({ cyclicActionFinished: { actionId: 'strike', result: completed } })
  dispatch({ actionStateChanged: idle() })
  assert.equal(clears.mock.callCount(), before)
  dispatch({ actionStateChanged: state({ actionGeneration: 2 }) })
  dispatch({ cyclicActionFinished: { actionId: 'strike', result: canceled } })
  assert.equal(clears.mock.callCount(), before + 1)
  dispatch({ actionStateChanged: idle() })
  assert.equal(clears.mock.callCount(), before + 1)
  dispatch({ actionStateChanged: state({ actionGeneration: 3 }) })
  dispatch({ objectDespawn: { entityId: 42, streamEpoch: 7 } })
  assert.equal(clears.mock.callCount(), before + 2)
  enter(8)
  const afterEnter = clears.mock.callCount()
  dispatch({ actionStateChanged: state({ actionGeneration: 4 }) })
  assert.equal(shows.mock.callCount(), 3, 'old-world messages do nothing')
  dispatch({ actionStateChanged: state({ streamEpoch: 8 }) })
  assert.equal(shows.mock.callCount(), 4, 'a fresh world permits generation restart')
  dispatch({ playerLeaveWorld: {} })
  assert.ok(clears.mock.callCount() > afterEnter)
  resetAttackResultStream()
  dispatch({ actionStateChanged: state({ streamEpoch: 8, actionGeneration: 2 }) })
  assert.equal(shows.mock.callCount(), 4, 'disconnect blocks stale notifications')
})
