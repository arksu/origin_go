import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { Render } from '../src/game/Render'
import type { PointerClickEvent } from '../src/game/InputController'
import { playerCommandController } from '../src/game/PlayerCommandController'
import { gameConnection } from '../src/network/GameConnection'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { DROP_ITEM_TYPE_ID } from '../src/constants/render'

test('primary map clicks preserve targets, rounding, modifiers and tool routing', () => {
  setActivePinia(createPinia())
  const packets: proto.IClientMessage[] = []
  const originalSend = gameConnection.send
  gameConnection.send = packet => { packets.push(packet) }
  try {
    let click: (event: PointerClickEvent) => void = () => { throw new Error('input not registered') }
    let target: { entityId: number; typeId: number } | null = null
    let toolConsumes = false
    // Exercise the actual Render input callback without allocating a WebGL renderer.
    const render = Object.create(Render.prototype) as Record<string, unknown> & { setupInputController(): void }
    render.canvas = {}
    render.inputController = {
      init() {}, onClick(handler: typeof click) { click = handler },
      onLongPress() {}, onDragStart() {}, onDragMove() {}, onDragEnd() {},
      onZoom() {}, onPinch() {}, onPinchMove() {}, onPointerMove() {}, onWheel() {},
    }
    render.screenToWorld = () => ({ x: 42.4, y: -98.6 })
    render.objectManager = { getEntityAtScreen: () => target }
    render.buildGhostController = { isActive: () => false }
    render.liftGhostController = { isActive: () => false }
    render.onClickCallback = () => toolConsumes
    render.setupInputController()
    const event = { screenX: 10, screenY: 20, button: 0, modifiers: 5 }
    for (const object of [null, { entityId: 777, typeId: 100 }, { entityId: 888, typeId: DROP_ITEM_TYPE_ID }]) {
      target = object
      packets.length = 0
      click(event)
      assert.equal(packets.length, 1)
      const wire = proto.ClientMessage.encode(proto.ClientMessage.create(packets[0]!)).finish()
      const action = proto.ClientMessage.decode(wire).playerAction!
      assert.ok(action.mapClick)
      assert.equal(action.interact, null)
      assert.equal(action.mapClick!.x, 42)
      assert.equal(action.mapClick!.y, -99)
      assert.equal(Number(action.mapClick!.targetEntityId), object?.entityId ?? 0)
      assert.equal(action.modifiers, 5)
    }
    target = { entityId: 777, typeId: 100 }
    toolConsumes = true
    packets.length = 0
    click(event)
    assert.equal(packets.length, 0, 'placement-consumed click must not also send map click')
    toolConsumes = false
    packets.length = 0
    click({ ...event, button: 2 })
    assert.equal(packets.length, 1)
    assert.equal(Number(packets[0]!.playerAction!.interact!.entityId), 777)
    assert.equal(packets[0]!.playerAction!.mapClick, null)
    assert.equal(useGameStore().contextMenu, null)

    const gameStore = useGameStore()
    gameStore.setPlayerEnterWorld(1, 'Player', 1, 1, 1)
    gameStore.updateInventory({
      ref: { kind: proto.InventoryKind.INVENTORY_KIND_HAND, ownerId: 1, inventoryKey: 0 },
      revision: 1,
      hand: { item: { itemId: 900 } },
    })
    target = null
    for (const actionId of ['lift', 'lift_down']) {
      gameStore.setGameActionState({ actionId, phase: 'selecting', cursor: actionId })
      packets.length = 0
      click(event)
      assert.equal(packets.length, 1)
      assert.ok(packets[0]!.playerAction?.mapClick, `${actionId} must take the map click even with an item in hand`)
      assert.equal(packets[0]!.inventoryOp, undefined)
    }
    gameStore.setGameActionState({ actionId: '', phase: 'idle', cursor: '' })
    packets.length = 0
    click(event)
    assert.equal(packets.length, 1)
    assert.ok(packets[0]!.inventoryOp?.op?.dropToWorld, 'idle click must keep ordinary hand drop')
  } finally { gameConnection.send = originalSend }
})

test('map-click sender is independent of chat messages', () => {
  const packets: proto.IClientMessage[] = []
  const originalSend = gameConnection.send
  gameConnection.send = packet => { packets.push(packet) }
  try {
    playerCommandController.sendMapClick(1, 2, 3, 0)
    const first = packets[0]
    gameConnection.send({ chat: { text: '/info' } })
    gameConnection.send({ chat: { text: '/destroy' } })
    playerCommandController.sendMapClick(1, 2, 3, 0)
    assert.deepEqual(packets[3], first)
  } finally { gameConnection.send = originalSend }
})
