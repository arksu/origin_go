import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { computed } from 'vue'
import { RuntimeTimeSync } from '../src/network/RuntimeTimeSync'
import { proto } from '../src/network/proto/packets.js'
import { parseNonNegativeInt64 } from '../src/utils/protoNumbers'
import { formatRemainingSeconds, parseItemProgress, presentItemProgress } from '../src/utils/itemProgress'

function fixture(t: TestContext) {
  let monotonic = 1000, wall = 10000, nextFrame = 0
  const frames = new Map<number, FrameRequestCallback>()
  t.mock.method(performance, 'now', () => monotonic)
  t.mock.method(Date, 'now', () => wall)
  t.mock.method(globalThis, 'requestAnimationFrame', (callback: FrameRequestCallback) => {
    frames.set(++nextFrame, callback)
    return nextFrame
  })
  t.mock.method(globalThis, 'cancelAnimationFrame', (id: number) => { frames.delete(id) })
  const sync = new RuntimeTimeSync()
  t.after(() => sync.reset())
  return {
    sync, frames,
    advance(ms: number, render = true) {
      monotonic += ms
      wall += ms
      if (render) {
        const callbacks = [...frames.values()]
        frames.clear()
        callbacks.forEach(callback => callback(monotonic))
      }
    },
    setWall(value: number) { wall = value },
  }
}

test('progress int64 parsing is exact and rejects malformed, unsafe and out-of-range values', () => {
  for (const value of [0, 3n, '9007199254740993', '9223372036854775807']) assert.equal(parseNonNegativeInt64(value), BigInt(value))
  for (const value of [null, undefined, {}, [1], true, -1, 1.5, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1, '-1', '01', '1.0', '9223372036854775808']) {
    assert.equal(parseNonNegativeInt64(value), null, String(value))
  }
  const huge = proto.ItemProgress.fromObject({ durationSeconds: '9223372036854775807', completionRuntimeSeconds: '9223372036854775807' })
  const decoded = proto.ItemProgress.decode(proto.ItemProgress.encode(huge).finish())
  assert.equal(parseItemProgress(decoded)?.durationMs, 9223372036854775807000n)
  for (const durationSeconds of [undefined, 0, -1, 1.5, Number.MAX_SAFE_INTEGER + 1, '9223372036854775808']) {
    assert.equal(parseItemProgress({ durationSeconds, completionRuntimeSeconds: 10 } as proto.IItemProgress), null)
  }
  for (const timing of [{}, { completionRuntimeSeconds: 5, pausedRemainingSeconds: 5 }, { completionRuntimeSeconds: -1 },
    { pausedRemainingSeconds: -1 }, { pausedRemainingSeconds: 11 }, { pausedRemainingSeconds: Number.MAX_SAFE_INTEGER + 1 }]) {
    assert.equal(parseItemProgress({ durationSeconds: 10, ...timing }), null)
  }
})

test('protobuf oneof preserves active and paused zero while absent timing is hidden', () => {
  for (const key of ['completionRuntimeSeconds', 'pausedRemainingSeconds'] as const) {
    const decoded = proto.ItemProgress.decode(proto.ItemProgress.encode({ durationSeconds: 10, [key]: 0 }).finish())
    assert.equal(decoded.timing, key)
    const parsed = parseItemProgress(decoded)
    assert.ok(parsed)
    assert.equal(presentItemProgress(parsed, 0n)?.remainingText, '0s')
    assert.equal(parsed.paused, key === 'pausedRemainingSeconds')
  }
  assert.equal(parseItemProgress(proto.ItemProgress.decode(proto.ItemProgress.encode({ durationSeconds: 10 }).finish())), null)
})

test('active presentation waits for sync, clamps before its fraction, and uses ceiling seconds', () => {
  const progress = parseItemProgress({ durationSeconds: 10, completionRuntimeSeconds: 20 })!
  assert.equal(presentItemProgress(progress, null), null)
  assert.deepEqual(presentItemProgress(progress, 5000n), { paused: false, remainingMs: 10000n, completedFraction: 0, remainingText: '10s' })
  assert.deepEqual(presentItemProgress(progress, 15001n), { paused: false, remainingMs: 4999n, completedFraction: .5001, remainingText: '5s' })
  assert.equal(presentItemProgress(progress, 19999n)?.remainingText, '1s')
  assert.deepEqual(presentItemProgress(progress, 25000n), { paused: false, remainingMs: 0n, completedFraction: 1, remainingText: '0s' })
  const huge = parseItemProgress(proto.ItemProgress.fromObject({ durationSeconds: 10, completionRuntimeSeconds: '9223372036854775807' }))!
  assert.equal(presentItemProgress(huge, 9223372036854775802000n)?.remainingText, '5s')
  assert.equal(presentItemProgress(huge, 9223372036854775802000n)?.completedFraction, .5)
})

test('paused remaining time is immediate, frozen, exact, and compact', () => {
  const paused = parseItemProgress({ durationSeconds: 6000, pausedRemainingSeconds: 5025 })!
  const expected = { paused: true, remainingMs: 5025000n, completedFraction: .1625, remainingText: '1h 23m 45s' }
  assert.deepEqual(presentItemProgress(paused, null), expected)
  assert.deepEqual(presentItemProgress(paused, 9223372036854775807000n), expected)
  assert.equal(formatRemainingSeconds(0n), '0s')
  assert.equal(formatRemainingSeconds(90061n), '1d 1h 1m 1s')
  assert.equal(formatRemainingSeconds(7200n), '2h')
  assert.equal(formatRemainingSeconds(9223372036854775807n), '106751991167300d 15h 30m 7s')
})

test('runtime ignores absent/invalid and duplicate Pongs, accepts explicit zero with exact int64 precision', t => {
  const { sync, frames } = fixture(t)
  assert.equal(sync.getRuntimeMs(), null)
  assert.equal(sync.estimateRuntimeMs(), null)
  const absent = proto.S2C_Pong.decode(proto.S2C_Pong.encode({ serverTimeMs: 10000 }).finish())
  assert.equal(sync.acceptSample(absent.runtimeSecondsTotal, absent.serverTimeMs, 0), false)
  for (const value of [undefined, null, -1, 1.5, NaN, Infinity, '9223372036854775808', Number.MAX_SAFE_INTEGER + 1]) {
    assert.equal(sync.acceptSample(value, 10000, 0), false)
    assert.equal(sync.acceptSample(0, value, 0), false)
  }
  for (const age of [-1, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1]) assert.equal(sync.acceptSample(0, 10000, age), false)
  const zero = proto.S2C_Pong.decode(proto.S2C_Pong.encode({ runtimeSecondsTotal: 0, serverTimeMs: 10000 }).finish())
  assert.equal(sync.acceptSample(zero.runtimeSecondsTotal, zero.serverTimeMs, 0), true)
  assert.equal(sync.getRuntimeMs(), 0n)
  assert.equal(sync.acceptSample(zero.runtimeSecondsTotal, zero.serverTimeMs, 0), false)
  const maximum = proto.S2C_Pong.fromObject({ runtimeSecondsTotal: '9223372036854775807', serverTimeMs: 10000 })
  assert.equal(sync.acceptSample(maximum.runtimeSecondsTotal, maximum.serverTimeMs, 0), true)
  assert.equal(sync.getRuntimeMs(), 9223372036854775807000n)
  assert.equal(frames.size, 0, 'samples alone never animate')
})

test('runtime compensates delivery age once then advances only monotonically across wall jumps and background suspension', t => {
  const { sync, advance, setWall } = fixture(t)
  const release = sync.subscribe()
  assert.equal(sync.acceptSample(100, 9750, 250), true)
  assert.equal(sync.getRuntimeMs(), 100250n)
  advance(800)
  assert.equal(sync.getRuntimeMs(), 101050n)
  assert.equal(sync.acceptSample(101, 10800, 0), true)
  assert.equal(sync.getRuntimeMs(), 101050n, 'whole-second resampling cannot rewind')
  setWall(-100000)
  advance(60000, false)
  assert.equal(sync.estimateRuntimeMs(), 161000n, 'background time catches up from the corrected anchor without frame counting or wall clock')
  advance(0)
  assert.equal(sync.getRuntimeMs(), 161000n)
  assert.equal(sync.acceptSample(100, 11000, 0), false, 'decreasing received runtime is ignored')
  assert.equal(sync.acceptSample(500, 11000, 0), true, 'administrator advance is accepted immediately with nonnegative delivery age')
  assert.equal(sync.getRuntimeMs(), 500000n)
  release()
})

test('runtime sheds an asymmetric RTT lead while item time and sector hold without rewinding', t => {
  const { sync, advance } = fixture(t)
  const release = sync.subscribe()
  const progress = parseItemProgress({ durationSeconds: 30, completionRuntimeSeconds: 130 })!
  const presentation = computed(() => presentItemProgress(progress, sync.getRuntimeMs()))
  sync.acceptSample(100, 10000, 0)
  advance(5000)
  advance(4000)
  assert.equal(sync.acceptSample(109, 19000, 2000), true, 'four seconds outbound and immediate inbound overestimates delivery age')
  assert.equal(sync.getRuntimeMs(), 111000n)
  advance(1000)
  const beforeCorrection = presentation.value
  assert.deepEqual(beforeCorrection, { paused: false, remainingMs: 18000n, completedFraction: .4, remainingText: '18s' })
  assert.equal(sync.acceptSample(110, 20000, 0), true)
  assert.deepEqual(presentation.value, beforeCorrection, 'correcting the anchor must not increase remaining time or rewind the sector')

  for (const [elapsed, runtime] of [[500, 110], [1000, 111], [1500, 111], [2000, 112]] as const) {
    advance(500)
    assert.equal(sync.estimateRuntimeMs(), 112000n, 'display holds until the corrected anchor catches up')
    assert.equal(sync.getRuntimeMs(), 112000n)
    assert.equal(sync.acceptSample(runtime, 20000 + elapsed, 0), true)
    assert.deepEqual(presentation.value, beforeCorrection, 'fresh samples during recovery must not keep advancing the display floor')
  }

  advance(500)
  assert.equal(sync.getRuntimeMs(), 112500n, 'normal-speed animation resumes once the anchor catches up')
  advance(2500)
  assert.equal(sync.acceptSample(115, 25000, 0), true)
  assert.equal(sync.getRuntimeMs(), 115000n, 'healthy samples remove the earlier lead instead of preserving it')
  assert.deepEqual(presentation.value, { paused: false, remainingMs: 15000n, completedFraction: .5, remainingText: '15s' })
  release()
})

test('one shared frame starts only with a sample and active consumers, and stops after releases/reset', t => {
  const { sync, frames, advance } = fixture(t)
  const observed = computed(() => sync.getRuntimeMs())
  const releaseFirst = sync.subscribe(), releaseSecond = sync.subscribe()
  assert.equal(frames.size, 0)
  sync.acceptSample(100, 10000, 0)
  assert.equal(frames.size, 1)
  assert.equal(observed.value, 100000n)
  advance(1500)
  assert.equal(frames.size, 1)
  assert.equal(observed.value, 101500n)
  releaseFirst()
  releaseFirst()
  assert.equal(frames.size, 1)
  releaseSecond()
  assert.equal(frames.size, 0)
  advance(30000, false)
  const releaseThird = sync.subscribe()
  assert.equal(observed.value, 131500n, 'opening a new display samples elapsed time immediately')
  assert.equal(frames.size, 1)
  sync.reset()
  assert.equal(observed.value, null)
  assert.equal(frames.size, 0)
  sync.acceptSample(0, 41500, 0)
  assert.equal(observed.value, 0n, 'new connection accepts a lower runtime')
  assert.equal(frames.size, 1, 'still-mounted consumers resume with the new session')
  releaseThird()
  assert.equal(frames.size, 0)
})
