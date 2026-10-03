import assert from 'node:assert/strict'
import { test } from 'node:test'
import { proto } from '../src/network/proto/packets.js'

test('combat protocol retains full uint64 identities and fractional HP', () => {
  const input = proto.ServerMessage.fromObject({
    combatResult: {
      entityId: '18446744073709551615', generation: '0:18446744073709551615',
      streamEpoch: 1, executionId: '18446744073709551614', eventSequence: '18446744073709551613',
      serverTimeMs: 600, hit: true,
      hits: [{ targetId: '18446744073709551612', damage: 0.4,
        target: { generation: '0:2', revision: '18446744073709551615', hp: 0.6, maxHp: 1 } }],
    },
  })
  const decoded = proto.ServerMessage.decode(proto.ServerMessage.encode(input).finish()).combatResult!
  assert.equal(decoded.entityId!.toString(), '18446744073709551615')
  assert.equal(decoded.executionId!.toString(), '18446744073709551614')
  assert.equal(decoded.hits![0]!.target!.hp, 0.6)
  assert.equal(decoded.hits![0]!.target!.revision!.toString(), '18446744073709551615')
  const legacy = proto.MapClick.decode(proto.MapClick.encode({ x: 1, y: 2 }).finish())
  assert.equal(legacy.combatAttempt, null)
  assert.equal(proto.S2C_PlayerEnterWorld.create().combatSupported, false)
})
