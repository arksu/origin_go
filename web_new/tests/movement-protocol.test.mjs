import assert from 'node:assert/strict'
import { test } from 'node:test'
import { proto } from '../src/network/proto/packets.js'

test('directional start, turn and release survive the wire with revision and epoch', () => {
  for (const direction of [
    { x: -1 / Math.sqrt(10), y: -3 / Math.sqrt(10), inputRevision: 1, streamEpoch: 7 },
    { x: 1, y: 0, inputRevision: 2, streamEpoch: 7 },
    { x: 0, y: 0, inputRevision: 3, streamEpoch: 7 },
    { x: 0, y: -1, inputRevision: 0xffffffff, streamEpoch: 0xffffffff },
  ]) {
    const packet = { sequence: 42, playerAction: { moveDirection: direction } }
    const decoded = proto.ClientMessage.decode(proto.ClientMessage.encode(packet).finish())
    const result = decoded.playerAction.moveDirection
    assert.equal(decoded.sequence, 42)
    assert.equal(result.x, Math.fround(direction.x))
    assert.equal(result.y, Math.fround(direction.y))
    assert.equal(result.inputRevision, direction.inputRevision)
    assert.equal(result.streamEpoch, direction.streamEpoch)
  }
})

test('legacy enter-world disables WASD and existing map buttons retain their meaning', () => {
  const legacy = proto.S2C_PlayerEnterWorld.decode(Uint8Array.of(8, 42, 72, 7))
  assert.equal(legacy.directionalMovementSupported, false)
  assert.equal(legacy.streamEpoch, 7)
  const supported = proto.S2C_PlayerEnterWorld.decode(proto.S2C_PlayerEnterWorld.encode({
    entityId: 42, streamEpoch: 7, directionalMovementSupported: true,
  }).finish())
  assert.equal(supported.directionalMovementSupported, true)
  for (const button of [0, 2]) {
    const action = proto.C2S_PlayerAction.decode(proto.C2S_PlayerAction.encode({
      mapClick: { x: 42, y: -99, targetEntityId: 777, button },
    }).finish())
    assert.equal(action.moveDirection, null)
    assert.equal(action.mapClick.button, button)
    assert.equal(action.mapClick.x, 42)
  }
  const legacyClick = proto.MapClick.decode(Uint8Array.of(8, 42))
  assert.equal(legacyClick.button, 0)
})
