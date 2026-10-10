import assert from 'node:assert/strict'
import { test } from 'node:test'
import { computed } from 'vue'
import { GameCalendarSync, gameCalendarFromRuntimeSeconds } from '../src/network/GameCalendarSync'
import { proto } from '../src/network/proto/packets.js'
import { serverProfile } from './serverConstantsFixture'

const date = (runtime: unknown) => {
  const value = gameCalendarFromRuntimeSeconds(runtime, serverProfile)!
  return [value.year, value.month, value.day, value.hour, value.minute, value.second]
}

test('pure calendar matches server boundary vectors and exact signed int64 age', () => {
  const vectors = [
    [0, [1n, 1, 1, 0, 0, 0]], [19, [1n, 1, 1, 0, 0, 57]], [20, [1n, 1, 1, 0, 1, 0]],
    [1199, [1n, 1, 1, 0, 59, 57]], [1200, [1n, 1, 1, 1, 0, 0]],
    [28799, [1n, 1, 1, 23, 59, 57]], [28800, [1n, 1, 2, 0, 0, 0]],
    [863999, [1n, 1, 30, 23, 59, 57]], [864000, [1n, 2, 1, 0, 0, 0]],
    [10367999, [1n, 12, 30, 23, 59, 57]], [10368000, [2n, 1, 1, 0, 0, 0]],
    [364686, [1n, 1, 13, 15, 54, 18]],
    ['9223372036854775807', [889599926395n, 3, 2, 22, 30, 21]],
  ] as const
  for (const [runtime, expected] of vectors) assert.deepEqual(date(runtime), expected)
  const oldWorld = gameCalendarFromRuntimeSeconds(364686, serverProfile)!
  assert.deepEqual([oldWorld.dayIndex, oldWorld.monthIndex, oldWorld.yearIndex], [12n, 0n, 0n])
  for (const invalid of [-1, 1.5, NaN, Infinity, 9007199254740992, '-1', '1.0', '01', '9223372036854775808', undefined, null, {}]) {
    assert.equal(gameCalendarFromRuntimeSeconds(invalid, serverProfile), null, String(invalid))
  }
  const wire = proto.S2C_Pong.fromObject({ runtimeSecondsTotal: '9223372036854775807' })
  assert.deepEqual(date(proto.S2C_Pong.decode(proto.S2C_Pong.encode(wire).finish()).runtimeSecondsTotal), vectors.at(-1)![1])
})

test('a different received calendar profile controls day duration, hours and calendar periods', () => {
  const profile = { ...serverProfile, realSecondsPerGameDay: 100, hoursPerDay: 10, daysPerMonth: 3, monthsPerYear: 2 }
  const value = gameCalendarFromRuntimeSeconds(725, profile)!
  assert.deepEqual([value.year, value.month, value.day, value.hour, value.minute, value.second], [2n, 1, 2, 2, 30, 0])
  assert.deepEqual([value.dayIndex, value.monthIndex, value.yearIndex, value.dayPhase], [7n, 2n, 1n, .25])
})

function fixture() {
  const sync = new GameCalendarSync()
  sync.configure(serverProfile)
  return sync
}

test('optional runtime distinguishes zero, remains unsynchronized without constants, and ignores invalid samples', () => {
  const sync = fixture()
  const absent = proto.S2C_Pong.decode(proto.S2C_Pong.encode({ serverTimeMs: 1000 }).finish())
  assert.equal(sync.acceptSample(absent.runtimeSecondsTotal, absent.serverTimeMs), false)
  assert.equal(sync.getCalendar(), null)
  const zero = proto.S2C_Pong.decode(proto.S2C_Pong.encode({ serverTimeMs: 1000, runtimeSecondsTotal: 0 }).finish())
  assert.equal(sync.acceptSample(zero.runtimeSecondsTotal, zero.serverTimeMs), true)
  assert.deepEqual(sync.getCalendar(), gameCalendarFromRuntimeSeconds(0, serverProfile))
  const before = sync.getCalendar()
  for (const runtime of [null, undefined, '-1', 'abc', Number.MAX_SAFE_INTEGER + 1]) assert.equal(sync.acceptSample(runtime, 2000), false)
  for (const wall of [null, undefined, -1, NaN, Infinity, 1.5, '9007199254740992']) assert.equal(sync.acceptSample(1, wall), false)
  assert.strictEqual(sync.getCalendar(), before)
  const noConstants = new GameCalendarSync()
  assert.equal(noConstants.acceptSample(0, 1000), true)
  assert.equal(noConstants.getCalendar(), null)
  assert.equal(noConstants.getDayPhase(), null)
  noConstants.configure(serverProfile)
  assert.deepEqual(noConstants.getCalendar(), gameCalendarFromRuntimeSeconds(0, serverProfile))
})

test('local clocks never advance a snapshot; midnight arrives only with a new Pong', t => {
  let wall = 10000, monotonic = 0
  t.mock.method(Date, 'now', () => wall)
  t.mock.method(performance, 'now', () => monotonic)
  const sync = fixture()
  assert.equal(sync.acceptSample(28799, 1000), true)
  const before = sync.getCalendar()
  assert.deepEqual(before, gameCalendarFromRuntimeSeconds(28799, serverProfile))
  assert.equal(Object.isFrozen(before), true)
  wall += 1000000
  monotonic += 1000000
  assert.strictEqual(sync.getCalendar(), before)
  assert.equal(sync.getDayPhase(), 28799 / 28800)
  wall = -1000000
  monotonic = -1000000
  assert.strictEqual(sync.getCalendar(), before)
  assert.equal(sync.acceptSample(28800, 1001), true)
  assert.equal(sync.getCalendar()?.day, 2)
  assert.equal(sync.getDayPhase(), 0)
})

test('accepted samples and reset update reactive computed consumers', () => {
  const sync = new GameCalendarSync()
  const calendar = computed(() => sync.getCalendar())
  const dayPhase = computed(() => sync.getDayPhase())
  const currentCalendar = () => calendar.value
  assert.equal(currentCalendar(), null)
  assert.equal(dayPhase.value, null)
  assert.equal(sync.acceptSample(28799, 1000), true)
  assert.equal(currentCalendar(), null, 'a sample waits for constants')
  sync.configure(serverProfile)
  assert.equal(currentCalendar()?.day, 1)
  assert.equal(dayPhase.value, 28799 / 28800)
  assert.equal(sync.acceptSample(28800, 1001), true)
  assert.equal(currentCalendar()?.day, 2)
  assert.equal(dayPhase.value, 0)
  sync.reset()
  assert.equal(currentCalendar(), null)
  assert.equal(dayPhase.value, null)
})

test('duplicate and lower samples preserve snapshots; fresh same-second Pongs and reconnects are accepted', () => {
  const sync = fixture()
  assert.equal(sync.acceptSample(100, 1000), true)
  const first = sync.getCalendar()
  assert.equal(sync.acceptSample(100, 1000), false, 'identical pair is a duplicate')
  assert.equal(sync.acceptSample(99, 2000), false)
  assert.strictEqual(sync.getCalendar(), first)
  assert.equal(sync.acceptSample(100, 1100), true, 'fresh equal-second response is accepted')
  const fresh = sync.getCalendar()
  assert.notStrictEqual(fresh, first)
  assert.deepEqual(fresh, first)
  assert.equal(sync.acceptSample(100, 500), true, 'response wall clock is not a monotonic sequence')
  assert.deepEqual(sync.getCalendar(), first)
  assert.equal(sync.acceptSample(101, 600), true)
  assert.equal(sync.getCalendar()?.second, 3)
  const maximum = proto.S2C_Pong.fromObject({ runtimeSecondsTotal: '9223372036854775807', serverTimeMs: 700 })
  const decoded = proto.S2C_Pong.decode(proto.S2C_Pong.encode(maximum).finish())
  assert.equal(sync.acceptSample(decoded.runtimeSecondsTotal, decoded.serverTimeMs), true)
  assert.deepEqual(sync.getCalendar(), gameCalendarFromRuntimeSeconds('9223372036854775807', serverProfile))
  sync.reset()
  assert.equal(sync.getCalendar(), null)
  sync.configure(serverProfile)
  assert.equal(sync.acceptSample(0, 500), true, 'new connection accepts restored lower runtime')
  assert.deepEqual(sync.getCalendar(), gameCalendarFromRuntimeSeconds(0, serverProfile))
})
