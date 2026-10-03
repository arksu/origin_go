import assert from 'node:assert/strict'
import { test } from 'node:test'
import { CombatStateCache, combatAttempt, formatCombatValue } from '../src/types/combat'
import { decodeActionAnimation, actionAnimationPhase, acceptActionAnimation } from '../src/types/actionAnimation'
import { parseActionAnimationFile } from '../src/types/actionAnimationDefs'
import { readFileSync } from 'node:fs'
import { proto } from '../src/network/proto/packets'

const active = { generation: '0:1', revision: 1, executionId: 1, actionId: 'axe_aoe', phase: 'windup', lockedDirection: { x: 1, y: 0 }, range: 18, sectorAngleDegrees: 90, elapsedMs: 200, durationMs: 1000, serverTimeMs: 200, strikeAtMs: 600, recoveryEndMs: 1000, startEventSequence: 1 }
test('combat snapshots reject stale epochs, incarnations and revisions; results are once-only', () => {
  const cache = new CombatStateCache(); cache.reset(7)
  cache.spawn(1, '0:1', active)
  cache.spawn('18446744073709551615', '0:2', null, { generation: '0:2', revision: 1, hp: 1, maxHp: 1 })
  assert.equal(cache.execution(1, { ...active, revision: 3 }, 6), false)
  assert.equal(cache.execution(1, { ...active, generation: '0:8', revision: 3 }, 7), false)
  assert.equal(cache.execution(1, { ...active, revision: 0 }, 7), false)
  const message = proto.S2C_CombatResult.fromObject({ entityId: '1', generation: '0:1', streamEpoch: 7, executionId: '1', eventSequence: '2', serverTimeMs: 600, hit: true, hits: [{ targetId: '18446744073709551615', eventSequence: '3', damage: .4, target: { generation: '0:2', revision: '2', hp: .6, maxHp: 1 } }] })
  assert.equal(cache.result(message), true)
  assert.equal(cache.result(message), false)
  assert.equal(cache.entities.get('18446744073709551615')?.target?.hp, .6)
  assert.equal(cache.entities.get('18446744073709551615')?.feedback?.text, '−0.4')
  assert.equal(cache.execution(1, { ...active, serverTimeMs: 400, lockedDirection: { x: 0, y: 1 } }, 7), false, 'equal revision cannot change locked aim')
  cache.execution(1, { ...active, revision: 2, executionId: 2, startEventSequence: 10 }, 7)
  cache.execution(1, { generation: '0:1', revision: 3, phase: 'idle' }, 7)
  assert.equal(cache.result(message), false, 'late results remain stale after recovery has cleared execution')
  cache.spawn(1, '0:9', { ...active, generation: '0:9' })
  assert.equal(cache.result(message), false)
  cache.reset(8)
  assert.equal(cache.entities.size, 0)
  assert.equal(cache.result(message), false)
})
test('combat input is discrete, identity-bearing and gated by support', () => {
  const cache = new CombatStateCache(); cache.reset(7)
  const action = { id: 'axe_aoe', targetKind: 'direction', combat: {} }
  const selection = proto.S2C_ActionStateChanged.fromObject({ actionId: 'axe_aoe', phase: 'selecting', selectionGeneration: '18446744073709551614' })
  assert.equal(combatAttempt(selection, action, false, cache), undefined)
  assert.equal(cache.requestRevision, '0')
  const first = combatAttempt(selection, action, true, cache)!
  assert.deepEqual(first, { actionId: 'axe_aoe', selectionGeneration: '18446744073709551614', requestRevision: '1', streamEpoch: 7 })
  assert.equal(combatAttempt(selection, action, true, cache)?.requestRevision, '2')
  assert.equal(combatAttempt({ ...selection, phase: 'windup' }, action, true, cache), undefined)
  assert.equal(combatAttempt({ ...selection, phase: 'recovery' }, action, true, cache), undefined)
})
test('fractional display never rounds a positive sub-tenth hit to zero', () => {
  assert.equal(formatCombatValue(.09), '<0.1')
  assert.equal(formatCombatValue(.6), '0.6')
  assert.equal(formatCombatValue(6), '6.0')
  assert.equal(formatCombatValue(0), '0.0')
  assert.throws(() => formatCombatValue(NaN))
})
test('cooldowns stay independent and are presentation state', () => {
  const cache = new CombatStateCache(); cache.reset(7)
  assert.equal(cache.owner({ generation: '0:1', revision: 1, streamEpoch: 7, cooldowns: [{ actionId: 'axe_aoe', readyAtMs: 2000 }, { actionId: 'axe_single', readyAtMs: 3000 }] }), true)
  assert.equal(cache.cooldowns.get('axe_aoe'), 2000)
  assert.equal(cache.cooldowns.get('axe_single'), 3000)
  assert.equal(cache.request().requestRevision, '1', 'cooldown must not disable activation')
  assert.equal(cache.owner({ generation: '0:1', revision: 0, streamEpoch: 7 }), false)
})
test('combat animation fits the full clip on milliseconds and locked direction', () => {
  const sample = decodeActionAnimation({ generation: '0:1', revision: 1, animationKey: 'axe_aoe', durationMs: 1000, elapsedMs: 300, serverTimeMs: 300, executionId: 1, lockedDirection: { x: 1, y: 0 } })
  assert.equal(actionAnimationPhase(sample, 600), .6)
  assert.equal(actionAnimationPhase(sample, 900), .9, 'asset readiness or culling must seek current phase')
  assert.equal(actionAnimationPhase(sample, 1200), 1)
  assert.equal(acceptActionAnimation(sample, { ...sample, serverTimeMs: 600, elapsedMs: 600 }, '0:1'), true)
  assert.equal(acceptActionAnimation(sample, { ...sample, revision: '0' }, '0:1'), false)
  assert.equal(acceptActionAnimation(sample, sample, '0:2'), false)
  assert.throws(() => decodeActionAnimation({ ...sample, lockedDirection: { x: 2, y: 0 }, revision: 1, executionId: 1 }))
  const stopped = decodeActionAnimation({ generation: '0:1', revision: 2, serverTimeMs: 700 })
  assert.equal(acceptActionAnimation(sample, stopped, '0:1'), true)
  assert.equal(actionAnimationPhase(stopped, 800), 0)
})
test('combat bindings are moving, direction-facing and do not copy tree sound cues', () => {
  const input = JSON.parse(readFileSync('data/action_animations/combat.json', 'utf8'))
  const bindings = parseActionAnimationFile(input, 'combat.json')
  assert.equal(bindings.length, 2)
  assert.equal(bindings[0]!.variants[0]!.equipment[0]!.slot, 'right_hand')
  assert.equal(bindings[0]!.variants[1]!.equipment[0]!.slot, 'left_hand')
  for (const binding of bindings) { assert.equal(binding.facing, 'direction'); assert.equal(binding.eligibility.includes('stationary'), false); assert.equal(binding.sound_cues, undefined) }
  input.bindings[0].eligibility.push('stationary')
  assert.throws(() => parseActionAnimationFile(input, 'invalid.json'))
})
