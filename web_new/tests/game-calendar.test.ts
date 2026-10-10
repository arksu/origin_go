import assert from 'node:assert/strict'
import { test } from 'node:test'
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
  let monotonic = 0, wall = 1000
  const sync = new GameCalendarSync(() => monotonic, () => wall)
  sync.configure(serverProfile)
  return { sync, monotonic(value: number) { monotonic = value }, wall(value: number) { wall = value } }
}

test('optional runtime distinguishes zero, remains unsynchronized without constants, and ignores invalid samples', () => {
  const f = fixture()
  const absent = proto.S2C_Pong.decode(proto.S2C_Pong.encode({ serverTimeMs: 1000 }).finish())
  assert.equal(f.sync.acceptSample(absent.runtimeSecondsTotal, absent.serverTimeMs), false)
  assert.equal(f.sync.getCalendar(), null)
  const zero = proto.S2C_Pong.decode(proto.S2C_Pong.encode({ serverTimeMs: 1000, runtimeSecondsTotal: 0 }).finish())
  assert.equal(f.sync.acceptSample(zero.runtimeSecondsTotal, zero.serverTimeMs), true)
  assert.deepEqual(f.sync.getCalendar(), gameCalendarFromRuntimeSeconds(0, serverProfile))
  const before = f.sync.getCalendar()
  for (const runtime of [null, undefined, '-1', 'abc', Number.MAX_SAFE_INTEGER + 1]) assert.equal(f.sync.acceptSample(runtime, 2000), false)
  for (const wall of [null, undefined, -1, NaN, Infinity, 1.5, '9007199254740992']) assert.equal(f.sync.acceptSample(1, wall), false)
  assert.deepEqual(f.sync.getCalendar(), before)
  const noConstants = new GameCalendarSync(() => 0, () => 1000)
  noConstants.acceptSample(0, 1000)
  assert.equal(noConstants.getCalendar(), null)
  assert.equal(noConstants.getDayPhase(), null)
  noConstants.configure(serverProfile)
  assert.equal(noConstants.getCalendar()?.year, 1n)
})

test('delivery age and monotonic local time cross midnight at subsecond precision', () => {
  const f = fixture()
  f.wall(1250)
  assert.equal(f.sync.acceptSample(28799, 1000), true)
  assert.equal(f.sync.getCalendar()?.second, 57)
  f.monotonic(500)
  assert.equal(f.sync.getCalendar()?.second, 59)
  f.monotonic(750)
  assert.equal(f.sync.getCalendar()?.day, 2)
  assert.equal(f.sync.getDayPhase(), 0)
  f.wall(-1000000) // Local/wall estimator changes cannot alter an existing monotonic anchor.
  f.monotonic(1000)
  assert.equal(f.sync.getCalendar()?.day, 2)
  assert.ok(f.sync.getDayPhase()! > 0)
})

test('authoritative samples compare to the last sample and may correct local extrapolation', () => {
  const f = fixture()
  assert.equal(f.sync.acceptSample(100, 1000), true)
  f.monotonic(900)
  assert.equal(f.sync.getCalendar()?.second, 2)
  assert.equal(f.sync.acceptSample(100, 1000), false, 'identical pair is a duplicate')
  assert.equal(f.sync.acceptSample(99, 2000), false)
  assert.equal(f.sync.acceptSample(100, 1100), true, 'fresh equal-second response can reanchor behind extrapolation')
  assert.equal(f.sync.getCalendar()?.second, 0)
  f.monotonic(1900)
  f.wall(500)
  assert.equal(f.sync.acceptSample(100, 500), true, 'response wall clock is not a monotonic sequence')
  assert.equal(f.sync.getCalendar()?.second, 0)
  assert.equal(f.sync.acceptSample(101, 600), true)
  assert.equal(f.sync.getCalendar()?.second, 3)
  f.sync.reset()
  assert.equal(f.sync.getCalendar(), null)
  f.sync.configure(serverProfile)
  assert.equal(f.sync.acceptSample(0, 500), true, 'new connection accepts restored lower runtime')
})
