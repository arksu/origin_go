import assert from 'node:assert/strict'
import { test } from 'node:test'
import { MessageDispatcher } from '../src/network/MessageDispatcher'
import { proto } from '../src/network/proto/packets.js'
import { util } from 'protobufjs/minimal'
import { AttackResultReceiver } from '../src/network/AttackResultReceiver'

test('activation preserves absent and zero aim and decodes legacy requests', () => {
  for (const aimAngle of [undefined, 0, Math.PI / 2]) {
    const request = { actionId: 'axe_aoe', streamEpoch: 7, ...(aimAngle === undefined ? {} : { aimAngle }) }
    const encoded = proto.ClientMessage.encode({ sequence: 42, activateAction: request }).finish()
    const decoded = proto.ClientMessage.decode(encoded)
    assert.equal(decoded.sequence, 42)
    assert.equal(decoded.activateAction!.actionId, 'axe_aoe')
    assert.equal(decoded.activateAction!.streamEpoch, 7)
    assert.equal(Object.hasOwn(decoded.activateAction!, 'aimAngle'), aimAngle !== undefined)
    if (aimAngle !== undefined) assert.equal(decoded.activateAction!.aimAngle, Math.fround(aimAngle))
    if (aimAngle === 0) {
      assert.equal(Buffer.from(encoded).toString('hex'), '082ad201100a076178655f616f6515000000001807')
    }
  }
  const legacy = proto.ClientMessage.decode(Buffer.from('082ad201060a046c696674', 'hex'))
  assert.equal(legacy.activateAction!.actionId, 'lift')
  assert.equal(Object.hasOwn(legacy.activateAction!, 'aimAngle'), false)
  assert.equal(legacy.activateAction!.streamEpoch, 0)
})

test('action catalog transmits the full sector in radians without changing legacy entries', () => {
  const decoded = proto.ServerMessage.decode(proto.ServerMessage.encode({ actionList: { actions: [
    { id: 'lift', targetKind: 'object' },
    { id: 'axe_aoe', targetKind: 'direction', stamina: 60, cooldownMs: 2000,
      sector: { range: 18, sectorAngle: Math.PI / 2 } },
  ] } }).finish())
  const [legacy, attack] = decoded.actionList!.actions!
  assert.equal(legacy!.sector, null)
  assert.equal(attack!.targetKind, 'direction')
  assert.equal(attack!.sector!.range, 18)
  assert.equal(attack!.sector!.sectorAngle, Math.fround(Math.PI / 2))
  assert.equal(attack!.stamina, 60)
  assert.equal(attack!.cooldownMs, 2000)
})

test('animation updates and visibility snapshots preserve fixed angle presence and timing', () => {
  for (const facingAngle of [undefined, 0, Math.PI / 2]) {
    const state = proto.CharacterActionAnimationState.fromObject({
      generation: '0:4294967297', revision: '9007199254740993', animationKey: 'axe_aoe',
      totalTicks: 6, elapsedTicks: 3, tickDurationMs: 100, serverTimeMs: '1790970000000',
      ...(facingAngle === undefined ? { targetPosition: { x: 18, y: -12 } } : { facingAngle: Math.fround(facingAngle) }),
    })
    for (const packet of [
      { characterActionAnimation: { entityId: 42, streamEpoch: 7, state } },
      { objectSpawn: { entityId: 42, streamEpoch: 7, actionAnimation: state } },
    ]) {
      const decoded = proto.ServerMessage.decode(proto.ServerMessage.encode(packet).finish())
      const animation = decoded.characterActionAnimation?.state ?? decoded.objectSpawn!.actionAnimation!
      assert.deepEqual(proto.CharacterActionAnimationState.create(animation).toJSON(), state.toJSON())
      assert.equal(Object.hasOwn(animation, 'facingAngle'), facingAngle !== undefined)
    }
  }
})

test('attack result dispatch preserves misses, zero damage, fractional damage and exact uint64 IDs', () => {
  const dispatcher = new MessageDispatcher()
  const received: proto.IS2C_AttackResult[] = []
  dispatcher.on('attackResult', result => received.push(result))
  for (const hits of [[], [
    { targetId: '9007199254740995', damage: 3.6 },
    { targetId: '9007199254740997', damage: 0 },
  ]]) {
    const packet = proto.ServerMessage.fromObject({ attackResult: {
      streamEpoch: 7, eventId: '18446744073709551615', attackerId: '9007199254740993', hits,
    } })
    const encoded = proto.ServerMessage.encode(packet).finish()
    assert.deepEqual([...encoded.subarray(0, 2)], [0xa2, 0x03])
    const decoded = proto.ServerMessage.decode(encoded)
    const result = decoded.attackResult!
    assert.equal(result.streamEpoch, 7)
    assert.equal(String(result.eventId), '18446744073709551615')
    assert.equal(String(result.attackerId), '9007199254740993')
    assert.ok(result.hits)
    assert.deepEqual(result.hits.map(hit => ({ targetId: String(hit.targetId), damage: hit.damage })), hits)
    dispatcher.dispatch(decoded)
    assert.equal(received.at(-1), result)
  }
  assert.equal(received.length, 2)
  assert.equal(dispatcher.getUnknownMessageCount(), 0)
})

function attackResult(overrides: Record<string, unknown> = {}): proto.IS2C_AttackResult {
  return { streamEpoch: 7, eventId: 11, attackerId: 21, hits: [{ targetId: 31, damage: 0.6 }], ...overrides } as proto.IS2C_AttackResult
}

test('attack receiver consumes misses and contacts without modifying the packet', () => {
  const receiver = new AttackResultReceiver()
  receiver.reset(7)
  const miss = attackResult({ hits: [] })
  assert.equal(receiver.accept(miss), true)
  const contacts = attackResult({ eventId: 12, hits: [{ targetId: 31, damage: 0 }, { targetId: 41, damage: 81 / 13 }] })
  const before = structuredClone(contacts)
  assert.equal(receiver.accept(contacts), true)
  assert.deepEqual(contacts, before)
  assert.equal(receiver.accept(attackResult({ eventId: 13, hits: undefined })), true)
})

test('attack receiver preserves exact uint64 ordering through wire decoding', () => {
  const receiver = new AttackResultReceiver(), dispatcher = new MessageDispatcher(), accepted: boolean[] = []
  receiver.reset(7)
  dispatcher.on('attackResult', message => accepted.push(receiver.accept(message)))
  for (const eventId of ['9007199254740992', '9007199254740993', '9007199254740992', '18446744073709551615', '18446744073709551615']) {
    const packet = proto.ServerMessage.fromObject({ attackResult: {
      streamEpoch: 7, eventId, attackerId: '9007199254740995', hits: [{ targetId: '18446744073709551615', damage: 3.6 }],
    } })
    dispatcher.dispatch(proto.ServerMessage.decode(proto.ServerMessage.encode(packet).finish()))
  }
  assert.deepEqual(accepted, [true, true, false, true, false])
  receiver.reset(7)
  for (const eventId of [9, 10, 99, 100]) assert.equal(receiver.accept(attackResult({ eventId })), true)
})

test('attack receiver resets its watermark on world and connection changes', () => {
  const receiver = new AttackResultReceiver()
  assert.equal(receiver.accept(attackResult()), false)
  receiver.reset(7)
  assert.equal(receiver.accept(attackResult({ eventId: 100 })), true)
  assert.equal(receiver.accept(attackResult({ eventId: 99 })), false)
  receiver.reset()
  assert.equal(receiver.accept(attackResult({ eventId: 101 })), false)
  receiver.reset(7)
  assert.equal(receiver.accept(attackResult({ eventId: 1 })), true)
  receiver.reset(8)
  assert.equal(receiver.accept(attackResult({ eventId: 1000 })), false)
  assert.equal(receiver.accept(attackResult({ streamEpoch: 8, eventId: 1 })), true)
  for (const epoch of [NaN, Infinity, -1, 0.5, 4294967296]) {
    receiver.reset(epoch)
    assert.equal(receiver.accept(attackResult({ streamEpoch: epoch })), false)
  }
})

test('attack receiver rejects malformed identities without consuming their event ID', () => {
  const malformedLong = new util.Long(11, 0, true)
  malformedLong.low = NaN
  const fractionalLong = new util.Long(11, 0, true)
  fractionalLong.high = 0.5
  const malformedValues: unknown[] = [
    undefined, null, false, {}, { toString: () => '11' }, { low: 11, high: 0, unsigned: true },
    0, -1, NaN, Infinity, -Infinity, 1.5, Number.MAX_SAFE_INTEGER + 1,
    '', '0', '-1', '01', '+1', '1.0', '1e2', ' 11', '11 ', '0x0b', '18446744073709551616',
    malformedLong, fractionalLong, new util.Long(-1, -1, false),
  ]
  for (const value of malformedValues) {
    for (const field of ['eventId', 'attackerId', 'targetId']) {
      const receiver = new AttackResultReceiver()
      receiver.reset(7)
      const packet = field === 'targetId' ? attackResult({ hits: [{ targetId: value, damage: 0.6 }] }) : attackResult({ [field]: value })
      assert.equal(receiver.accept(packet), false, `${field}: ${String(value)}`)
      assert.equal(receiver.accept(attackResult()), true, 'invalid packet must not advance the watermark')
    }
  }
})

test('attack receiver validates the entire hit list before advancing the watermark', () => {
  const invalidLists: unknown[] = [
    'hits', {}, [null], [undefined],
    [{ targetId: 31, damage: 1 }, { targetId: '31', damage: 2 }],
    [{ targetId: 21, damage: 1 }],
    ...[NaN, Infinity, -Infinity, -0.1, '0.6'].map(damage => [{ targetId: 31, damage }]),
    Array.from({ length: 513 }, (_, index) => ({ targetId: index + 100, damage: 1 })),
  ]
  for (const hits of invalidLists) {
    const receiver = new AttackResultReceiver()
    receiver.reset(7)
    assert.equal(receiver.accept(attackResult({ eventId: 1000, hits })), false)
    assert.equal(receiver.accept(attackResult()), true)
    assert.equal(receiver.accept(attackResult({ eventId: 1000 })), true)
  }
  const receiver = new AttackResultReceiver()
  receiver.reset(7)
  assert.equal(receiver.accept(attackResult({ hits: Array.from({ length: 512 }, (_, index) => ({ targetId: index + 100, damage: 0 })) })), true)
})
