import { setWorldParams } from './serverConstantsFixture'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { BitmapText, Container } from 'pixi.js'
import { DamageNumberManager } from '../src/game/DamageNumberManager'
import {
  CACHE_CAPACITY, CACHE_TTL_MS, CAPACITY, DamageNumbersPresentation,
  FADE_START_MS, LANE_OFFSET_PX, LIFETIME_MS, PER_TARGET, POP_DURATION_MS, RISE_PX,
  formatDamageNumber, safeDamageTargetId,
  type DamageNumberHit,
} from '../src/game/hud/damageNumbers'
import { AttackResultReceiver } from '../src/network/AttackResultReceiver'
import { proto } from '../src/network/proto/packets.js'
import { createPinia, setActivePinia } from 'pinia'
import { gameFacade } from '../src/game/GameFacade'
import { useGameStore } from '../src/stores/gameStore'
import { registerMessageHandlers, resetAttackResultStream } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'

function near(actual: number, expected: number): void {
  assert.ok(Math.abs(actual - expected) < 1e-9, `${actual} != ${expected}`)
}

function active(presentation: DamageNumbersPresentation) {
  return presentation.entries.filter(entry => entry.active)
}

function fixture() {
  const parent = new Container()
  const manager = new DamageNumberManager(parent)
  const positions = new Map<number, { x: number; y: number; top: number }>()
  const objects = {
    getObject(id: number) {
      const position = positions.get(id)
      if (!position) return undefined
      return { getContainer: () => ({
        x: position.x, y: position.y,
        getLocalBounds: () => ({ top: position.top }),
      }) as unknown as Container }
    },
  }
  return { manager, parent, positions, objects }
}

test('damage formatting preserves zero, fractional values and compact extremes', () => {
  for (const [value, expected] of [
    [0, '0'], [-0, '0'], [3.6, '3.6'], [81 / 13, '6.2'], [30, '30'],
    [21.4, '21.4'], [0.1, '0.1'], [0.49, '0.5'], [0.001, '<0.1'],
    [Number.MIN_VALUE, '<0.1'], [1_000_000, '1.0e+6'],
    [1_234_567, '1.2e+6'], [Number.MAX_VALUE, '1.8e+308'],
  ] as const) assert.equal(formatDamageNumber(value), expected)
  for (const value of [-1, NaN, Infinity, -Infinity]) {
    assert.equal(formatDamageNumber(value), null)
  }
})

test('render identity conversion accepts only canonical exactly represented IDs', () => {
  for (const id of ['1', '42', String(Number.MAX_SAFE_INTEGER)]) {
    assert.equal(safeDamageTargetId(id), Number(id))
  }
  for (const id of [
    '', '0', '-1', '01', ' 1', '1 ', '+1', '1.0', '1e2', '0x1',
    '9007199254740992', '9007199254740993', '18446744073709551615',
    '18446744073709551616',
  ]) assert.equal(safeDamageTargetId(id), null, id)
})

test('presentation animation uses elapsed time, constant screen size and a captured anchor', () => {
  const presentation = new DamageNumbersPresentation()
  assert.equal(presentation.emit(1, 3.6, 100, 200, 1000), true)
  const entry = active(presentation)[0]!
  presentation.update(1000, 1)
  near(entry.scale, 1.2)
  near(entry.alpha, 1)
  near(entry.x, 100 - LANE_OFFSET_PX)
  near(entry.y, 200)
  presentation.update(1000 + POP_DURATION_MS / 2, 1)
  near(entry.scale, 1.1)
  presentation.update(1000 + POP_DURATION_MS, 1)
  near(entry.scale, 1)
  presentation.update(1450, 1)
  near(entry.y, 200 - RISE_PX * 0.75)
  const screenOffsetX = entry.x - entry.anchorX
  const screenOffsetY = entry.y - entry.anchorY
  presentation.update(1450, 2)
  near((entry.x - entry.anchorX) * 2, screenOffsetX)
  near((entry.y - entry.anchorY) * 2, screenOffsetY)
  near(entry.scale * 2, 1)
  assert.equal(entry.anchorX, 100)
  assert.equal(entry.anchorY, 200)
  presentation.update(1000 + FADE_START_MS, 1)
  near(entry.alpha, 1)
  presentation.update(1750, 1)
  near(entry.alpha, 0.5)
  presentation.update(1000 + LIFETIME_MS - 1, 1)
  assert.equal(entry.active, true)
  presentation.update(1000 + LIFETIME_MS, 1)
  assert.equal(entry.active, false)
  assert.equal(presentation.activeCount, 0)
})

test('animation result is independent of frame count and expires after a background pause', () => {
  const sparse = new DamageNumbersPresentation()
  const frequent = new DamageNumbersPresentation()
  sparse.emit(1, 30, -10, -20, 0)
  frequent.emit(1, 30, -10, -20, 0)
  for (let now = 0; now < 750; now += 16) frequent.update(now, 1.5)
  sparse.update(750, 1.5)
  frequent.update(750, 1.5)
  for (const property of ['x', 'y', 'scale', 'alpha'] as const) {
    near(sparse.entries[0]![property], frequent.entries[0]![property])
  }
  sparse.update(60_000, 1)
  assert.equal(sparse.activeCount, 0)
})

test('repeat hits use separate alternating trajectories and replace the oldest target label', () => {
  const presentation = new DamageNumbersPresentation()
  for (let index = 0; index < 3; index++) presentation.emit(1, index + 1, 100, 200, 0)
  presentation.update(450, 1)
  const offsets = active(presentation).map(entry => Math.sign(entry.x - entry.anchorX))
  assert.deepEqual(offsets, [-1, 0, 1])
  presentation.emit(1, 4, 100, 200, 1)
  presentation.emit(2, 50, 0, 0, 1)
  presentation.emit(1, 5, 100, 200, 2)
  assert.equal(active(presentation).filter(entry => entry.targetId === 1).length, PER_TARGET)
  assert.deepEqual(active(presentation).filter(entry => entry.targetId === 1).map(entry => entry.text).sort(), ['2', '3', '4', '5'])
  assert.equal(active(presentation).find(entry => entry.targetId === 2)?.text, '50')
})

test('four simultaneous labels occupy separate positions even at their initial frame', () => {
  const presentation = new DamageNumbersPresentation()
  for (let index = 0; index < PER_TARGET; index++) presentation.emit(1, index, 0, 0, 0)
  presentation.update(0, 1)
  const labels = active(presentation)
  assert.equal(new Set(labels.map(entry => `${entry.x}:${entry.y}`)).size, PER_TARGET)
  assert.ok(Math.abs(labels[0]!.x - labels[1]!.x) >= 32)
  assert.ok(Math.abs(labels[0]!.y - labels[3]!.y) >= 28)
})

test('global capacity is bounded and expired or cleared records are reused', () => {
  const presentation = new DamageNumbersPresentation()
  const records = [...presentation.entries]
  for (let id = 1; id <= CAPACITY; id++) assert.equal(presentation.emit(id, id, 0, 0, 0), true)
  assert.equal(presentation.activeCount, CAPACITY)
  presentation.emit(CAPACITY + 1, 0.6, 0, 0, 1)
  assert.equal(presentation.activeCount, CAPACITY)
  assert.equal(active(presentation).some(entry => entry.targetId === 1), false)
  assert.equal(active(presentation).some(entry => entry.targetId === CAPACITY + 1), true)
  presentation.emit(10000, 1, 0, 0, LIFETIME_MS + 1)
  assert.equal(presentation.activeCount, 1)
  presentation.clear()
  assert.equal(presentation.activeCount, 0)
  for (let index = 0; index < CAPACITY; index++) assert.equal(presentation.entries[index], records[index])
  presentation.emit(1, 10, 0, 0, 2000)
  assert.equal(presentation.activeCount, 1)
})

test('equal-time replacement follows insertion order after a low-index slot is reused', () => {
  const presentation = new DamageNumbersPresentation()
  for (let id = 1; id <= CAPACITY; id++) presentation.emit(id, 1, 0, 0, 0)
  presentation.emit(CAPACITY + 1, 1, 0, 0, 0)
  presentation.emit(CAPACITY + 2, 1, 0, 0, 0)
  const targets = new Set(active(presentation).map(entry => entry.targetId))
  assert.equal(targets.size, CAPACITY)
  assert.equal(targets.has(1), false)
  assert.equal(targets.has(2), false)
  assert.equal(targets.has(CAPACITY + 1), true)
  assert.equal(targets.has(CAPACITY + 2), true)
})

test('invalid presentation input creates no partial label', () => {
  const presentation = new DamageNumbersPresentation()
  for (const damage of [-1, NaN, Infinity, -Infinity]) assert.equal(presentation.emit(1, damage, 0, 0, 0), false)
  for (const coordinate of [NaN, Infinity, -Infinity]) {
    assert.equal(presentation.emit(1, 1, coordinate, 0, 0), false)
    assert.equal(presentation.emit(1, 1, 0, coordinate, 0), false)
  }
  assert.equal(presentation.activeCount, 0)
})

test('actual network handler presents only fully accepted events and clears the stream', context => {
  setActivePinia(createPinia())
  const store = useGameStore()
  const f = fixture()
  context.after(() => { store.reset(); f.manager.destroy(); f.parent.destroy() })
  context.mock.method(console, 'error', () => {})
  context.mock.method(gameFacade, 'resetWorld', () => f.manager.clear())
  const clears = context.mock.method(gameFacade, 'clearDamageNumbers', () => f.manager.clear())
  const shows = context.mock.method(gameFacade, 'showDamageNumbers', (hits: readonly DamageNumberHit[]) => f.manager.show(hits, f.objects, 1000, 1))
  setWorldParams(32, 128)
  registerMessageHandlers()
  const dispatch = (packet: proto.IServerMessage) => messageDispatcher.dispatch(proto.ServerMessage.create(packet))
  dispatch({ playerEnterWorld: { entityId: 21, streamEpoch: 7} })
  f.positions.set(31, { x: 100, y: 200, top: -30 })
  const healthBefore = { ...store.playerStats }
  const send = (eventId: string, hits: unknown[], epoch = 7) => dispatch(proto.ServerMessage.fromObject({ attackResult: {
    eventId, streamEpoch: epoch, attackerId: 21, hits,
  } }))
  send('1', [{ targetId: '31', damage: 3.6 }])
  assert.equal(shows.mock.callCount(), 1)
  assert.equal(f.manager.presentation.activeCount, 1)
  send('1', [{ targetId: '31', damage: 3.6 }])
  send('2', [{ targetId: '31', damage: 0 }], 6)
  send('2', [{ targetId: '31', damage: 0.6 }, { targetId: '41', damage: -1 }])
  assert.equal(shows.mock.callCount(), 1, 'invalid final hit must not show the first hit')
  send('2', [{ targetId: '31', damage: 0 }, { targetId: '18446744073709551615', damage: 10 }])
  assert.equal(shows.mock.callCount(), 2)
  assert.deepEqual(shows.mock.calls[1]!.arguments[0], [
    { targetId: '31', damage: 0 }, { targetId: '18446744073709551615', damage: 10 },
  ])
  assert.equal(f.manager.presentation.activeCount, 2, 'unknown exact uint64 must not alias another target')
  assert.deepEqual(store.playerStats, healthBefore)
  assert.equal(store.entities.size, 0, 'combat notifications must not create entities')
  send('3', [])
  assert.equal(f.manager.presentation.activeCount, 2, 'miss adds no label')
  const clearCount = clears.mock.callCount()
  resetAttackResultStream()
  assert.equal(clears.mock.callCount(), clearCount + 1)
  assert.equal(f.manager.presentation.activeCount, 0)
  send('4', [{ targetId: '31', damage: 3.6 }])
  assert.equal(f.manager.presentation.activeCount, 0)
  dispatch({ playerEnterWorld: { entityId: 21, streamEpoch: 8} })
  send('1', [{ targetId: '31', damage: 0.6 }], 8)
  assert.equal(f.manager.presentation.activeCount, 1)
  dispatch({ playerLeaveWorld: {} })
  assert.equal(f.manager.presentation.activeCount, 0)
})

test('manager preallocates noninteractive views and copies live anchors only once', context => {
  const f = fixture()
  context.after(() => { f.manager.destroy(); f.parent.destroy() })
  const layer = f.manager.getContainer()
  assert.equal(layer.children.length, CAPACITY)
  assert.ok(layer.children.every(view => view instanceof BitmapText))
  assert.equal(layer.eventMode, 'none')
  const views = [...layer.children]
  f.positions.set(31, { x: 100, y: 200, top: -30 })
  f.manager.show([{ targetId: '31', damage: 81 / 13 }], f.objects, 1000, 1)
  const entry = active(f.manager.presentation)[0]!
  assert.equal(entry.text, '6.2')
  assert.equal(entry.anchorX, 100)
  assert.equal(entry.anchorY, 170)
  f.positions.set(31, { x: 900, y: 900, top: -100 })
  f.manager.update(1450, 2)
  assert.equal(entry.anchorX, 100)
  assert.equal(entry.anchorY, 170)
  const view = layer.children[f.manager.presentation.entries.indexOf(entry)]!
  near(view.x, entry.x)
  near(view.y, entry.y)
  near(view.alpha, entry.alpha)
  near(view.scale.x, entry.scale)
  f.parent.position.set(20, -30)
  f.parent.scale.set(2)
  // The inherited camera transform moves the captured anchor, while inverse
  // scale retains the font's screen size. Do not request text bounds in Node.
  near(f.parent.x + view.x * f.parent.scale.x, 20 + entry.x * 2)
  near(view.scale.x * f.parent.scale.x, 1)
  f.manager.clear()
  assert.equal(f.manager.presentation.activeCount, 0)
  assert.ok(layer.children.every(view => !view.visible))
  f.manager.show([{ targetId: '31', damage: 0 }], f.objects, 2000, 1)
  assert.equal(active(f.manager.presentation)[0]?.text, '0')
  for (let index = 0; index < CAPACITY; index++) assert.equal(layer.children[index], views[index])
})

test('unknown or inexact targets are skipped without creating entities', context => {
  const f = fixture()
  context.after(() => { f.manager.destroy(); f.parent.destroy() })
  f.positions.set(31, { x: 10, y: 20, top: -5 })
  f.positions.set(9007199254740992, { x: 999, y: 999, top: 0 })
  f.manager.show([
    { targetId: '31', damage: 3.6 }, { targetId: '41', damage: 1 },
    { targetId: '9007199254740993', damage: 2 },
    { targetId: '9007199254740992', damage: 3 },
    { targetId: '18446744073709551615', damage: 4 },
    { targetId: '01', damage: 1 },
  ], f.objects, 0, 1)
  assert.equal(f.manager.presentation.activeCount, 1)
  assert.equal(active(f.manager.presentation)[0]?.targetId, 31)
  assert.equal(f.positions.size, 2)
  f.manager.show([], f.objects, 1, 1)
  assert.equal(f.manager.presentation.activeCount, 1)
})

test('fatal hits use a copied despawn anchor until cache expiry', context => {
  const f = fixture()
  context.after(() => { f.manager.destroy(); f.parent.destroy() })
  const original = { x: 10, y: 20, top: -5 }
  f.positions.set(31, original)
  f.manager.rememberDespawn(31, f.objects, 100)
  f.positions.delete(31)
  original.x = 999
  original.y = 999
  original.top = -100
  f.manager.show([{ targetId: '31', damage: 30 }], f.objects, 101, 1)
  const entry = active(f.manager.presentation)[0]!
  assert.equal(entry.anchorX, 10)
  assert.equal(entry.anchorY, 15)
  f.manager.update(101 + LIFETIME_MS, 1)
  assert.equal(f.manager.presentation.activeCount, 0)
  f.manager.show([{ targetId: '31', damage: 1 }], f.objects, 100 + CACHE_TTL_MS - 1, 1)
  assert.equal(f.manager.presentation.activeCount, 1)
  f.manager.show([{ targetId: '31', damage: 2 }], f.objects, 100 + CACHE_TTL_MS, 1)
  assert.equal(f.manager.presentation.activeCount, 1, 'existing labels finish while the expired anchor rejects new hits')
  assert.equal(active(f.manager.presentation)[0]?.text, '1')
})

test('live anchors win over cache and new spawn invalidates the retired position', context => {
  const f = fixture()
  context.after(() => { f.manager.destroy(); f.parent.destroy() })
  f.positions.set(31, { x: 10, y: 20, top: -5 })
  f.manager.rememberDespawn(31, f.objects, 0)
  f.positions.set(31, { x: 100, y: 200, top: -30 })
  f.manager.show([{ targetId: '31', damage: 1 }], f.objects, 1, 1)
  assert.equal(active(f.manager.presentation)[0]?.anchorX, 100)
  f.manager.forgetSpawn(31)
  f.positions.delete(31)
  f.manager.show([{ targetId: '31', damage: 2 }], f.objects, 2, 1)
  assert.equal(f.manager.presentation.activeCount, 1, 'a new spawn must not reuse the previous incarnation cache')
  f.manager.update(LIFETIME_MS + 1, 1)
  assert.equal(f.manager.presentation.activeCount, 0)
})

test('despawn cache evicts oldest records and clear removes all retired anchors', context => {
  const f = fixture()
  context.after(() => { f.manager.destroy(); f.parent.destroy() })
  for (let id = 1; id <= CACHE_CAPACITY + 1; id++) {
    f.positions.set(id, { x: id, y: id, top: 0 })
    f.manager.rememberDespawn(id, f.objects, id / 10)
    f.positions.delete(id)
  }
  f.manager.show([
    { targetId: '1', damage: 1 }, { targetId: '2', damage: 2 },
    { targetId: String(CACHE_CAPACITY + 1), damage: 3 },
  ], f.objects, 200, 1)
  assert.deepEqual(active(f.manager.presentation).map(entry => entry.targetId).sort((a, b) => a - b), [2, CACHE_CAPACITY + 1])
  f.manager.clear()
  f.manager.show([{ targetId: '2', damage: 4 }], f.objects, 201, 1)
  assert.equal(f.manager.presentation.activeCount, 0)
})

test('repeated emissions, expiration and reset reuse the same view pool', context => {
  const f = fixture()
  context.after(() => { f.manager.destroy(); f.parent.destroy() })
  const layer = f.manager.getContainer()
  const views = [...layer.children]
  const hits = Array.from({ length: CAPACITY }, (_, index) => ({ targetId: String(index + 1), damage: 3.6 }))
  for (let id = 1; id <= CAPACITY; id++) f.positions.set(id, { x: id, y: id, top: -10 })
  for (let cycle = 0; cycle < 10; cycle++) {
    f.manager.show(hits, f.objects, cycle * 1000, 1)
    assert.equal(f.manager.presentation.activeCount, CAPACITY)
    f.manager.update(cycle * 1000 + LIFETIME_MS, 1)
    assert.equal(f.manager.presentation.activeCount, 0)
    if (cycle % 2 === 0) f.manager.clear()
  }
  assert.equal(layer.children.length, CAPACITY)
  for (let index = 0; index < CAPACITY; index++) assert.equal(layer.children[index], views[index])
})

test('destroy detaches and destroys every pooled view', () => {
  const f = fixture()
  const layer = f.manager.getContainer()
  const views = [...layer.children]
  f.positions.set(31, { x: 10, y: 20, top: 0 })
  f.manager.show([{ targetId: '31', damage: 1 }], f.objects, 0, 1)
  f.manager.destroy()
  assert.equal(f.parent.children.length, 0)
  assert.equal(layer.destroyed, true)
  assert.ok(views.every(view => view.destroyed))
  f.parent.destroy()
})

test('receiver validation precedes all presentation and duplicates cannot display twice', context => {
  const f = fixture()
  context.after(() => { f.manager.destroy(); f.parent.destroy() })
  f.positions.set(31, { x: 10, y: 20, top: -5 })
  f.positions.set(41, { x: 20, y: 30, top: -5 })
  const receiver = new AttackResultReceiver()
  receiver.reset(7)
  function receive(eventId: string, hits: unknown[], epoch = 7): boolean {
    const message = proto.S2C_AttackResult.fromObject({ eventId, attackerId: '21', streamEpoch: epoch, hits })
    if (!receiver.accept(message)) return false
    f.manager.show((message.hits ?? []).map(hit => ({ targetId: String(hit.targetId), damage: hit.damage ?? 0 })), f.objects, 0, 1)
    return true
  }
  assert.equal(receive('1', [{ targetId: '31', damage: 3.6 }, { targetId: '41', damage: -1 }]), false)
  assert.equal(f.manager.presentation.activeCount, 0)
  assert.equal(receive('1', [{ targetId: '31', damage: 3.6 }, { targetId: '41', damage: 0 }]), true)
  assert.equal(f.manager.presentation.activeCount, 2)
  assert.equal(receive('1', [{ targetId: '31', damage: 3.6 }]), false)
  assert.equal(receive('2', [{ targetId: '31', damage: 3.6 }], 6), false)
  assert.equal(f.manager.presentation.activeCount, 2)
  assert.equal(receive('2', []), true)
  assert.equal(f.manager.presentation.activeCount, 2)
  receiver.reset(8)
  f.manager.clear()
  assert.equal(receive('1', [{ targetId: '31', damage: 1 }], 8), true)
  assert.equal(f.manager.presentation.activeCount, 1)
})
