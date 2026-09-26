import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { Render } from '../src/game/Render'
import { InputController, type PointerClickEvent, type PointerLongPressEvent } from '../src/game/InputController'
import { cameraController } from '../src/game/CameraController'
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
    let longPress: (event: PointerLongPressEvent) => void = () => { throw new Error('input not registered') }
    let target: { entityId: number; typeId: number } | null = null
    let toolConsumes = false
    // Exercise the actual Render input callback without allocating a WebGL renderer.
    const render = Object.create(Render.prototype) as Record<string, unknown> & { setupInputController(): void }
    render.canvas = {}
    render.inputController = {
      init() {}, onClick(handler: typeof click) { click = handler },
      onLongPress(handler: typeof longPress) { longPress = handler }, onDragStart() {}, onDragMove() {}, onDragEnd() {},
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
      assert.equal(action.mapClick!.button, proto.MapClickButton.MAP_CLICK_BUTTON_PRIMARY)
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
    target = { entityId: 888, typeId: DROP_ITEM_TYPE_ID }
    click(event)
    assert.equal(packets.length, 1, 'dropped item must precede primary placement tools')
    assert.equal(Number(packets[0]!.playerAction!.mapClick!.targetEntityId), 888)
    target = { entityId: 777, typeId: 100 }
    toolConsumes = false
    packets.length = 0
    click({ ...event, button: 2 })
    assert.equal(packets.length, 1)
    assert.equal(Number(packets[0]!.playerAction!.mapClick!.targetEntityId), 777)
    assert.equal(packets[0]!.playerAction!.mapClick!.button, proto.MapClickButton.MAP_CLICK_BUTTON_SECONDARY)
    assert.equal(useGameStore().contextMenu, null)

    const gameStore = useGameStore()
    gameStore.setPlayerEnterWorld(1, 'Player', 1, 1, 1)
    gameStore.updateInventory({
      ref: { kind: proto.InventoryKind.INVENTORY_KIND_HAND, ownerId: 1, inventoryKey: 0 },
      revision: 1,
      hand: { item: { itemId: 900 } },
    })
    target = null
    gameStore.setGameActionList([
      { id: 'lift', targetKind: 'object' },
      { id: 'lift_down', targetKind: 'tile' },
      { id: 'plow_tile', targetKind: 'tile' },
    ])
    for (const actionId of ['lift', 'lift_down', 'plow_tile']) {
      for (const phase of ['selecting', 'approaching', 'executing']) {
        gameStore.setGameActionState({ actionId, phase, cursor: phase === 'selecting' ? 'dig' : '' })
        packets.length = 0
        click(event)
        assert.equal(packets.length, 1)
        assert.ok(packets[0]!.playerAction?.mapClick, `${actionId} must take the map click even with an item in hand`)
        assert.equal(packets[0]!.inventoryOp, undefined)
        assert.equal(Number(gameStore.handState?.item?.itemId), 900, 'target click must keep held item')
      }
    }
    render.buildGhostController = { isActive: () => true }
    render.liftGhostController = { isActive: () => true }
    render.updateBuildGhostAtScreen = () => { throw new Error('RMB entered primary build placement') }
    render.updateLiftGhostAtScreen = () => { throw new Error('RMB entered primary lift placement') }
    for (const phase of ['idle', 'selecting', 'approaching', 'executing']) {
      gameStore.setGameActionState({ actionId: phase === 'idle' ? '' : 'lift_down', phase, cursor: phase === 'selecting' ? 'lift_down' : '' })
      for (const object of [null, { entityId: 777, typeId: 100 }, { entityId: 888, typeId: DROP_ITEM_TYPE_ID }]) {
        target = object
        gameStore.openContextMenu(123, [{ actionId: 'open', title: 'Open' }])
        packets.length = 0
        click({ ...event, button: 2 })
        assert.equal(packets.length, 1, 'RMB must send one packet without a separate CancelAction or inventory request')
        assert.deepEqual(Object.keys(packets[0]!), ['playerAction'])
        const wire = proto.ClientMessage.encode(proto.ClientMessage.create(packets[0]!)).finish()
        const action = proto.ClientMessage.decode(wire).playerAction!
        assert.equal(action.mapClick?.button, proto.MapClickButton.MAP_CLICK_BUTTON_SECONDARY)
        assert.equal(Number(action.mapClick?.targetEntityId), object?.entityId ?? 0)
        assert.equal(action.mapClick?.x, 42)
        assert.equal(action.mapClick?.y, -99)
        assert.equal(action.modifiers, 5)
        assert.equal(gameStore.contextMenu, null, 'RMB ground and object both close stale context menus')
        assert.equal(Number(gameStore.handState?.item?.itemId), 900)
        const secondary = packets[0]
        packets.length = 0
        longPress(event)
        assert.deepEqual(packets, [secondary], 'long-press must match mouse RMB including modifiers')
      }
    }
    packets.length = 0
    click({ ...event, button: 1 })
    assert.equal(packets.length, 0, 'middle button must not send gameplay input')
    render.buildGhostController = { isActive: () => false }
    render.liftGhostController = { isActive: () => false }
    target = null
    gameStore.setGameActionState({ actionId: '', phase: 'idle', cursor: '' })
    packets.length = 0
    click(event)
    assert.equal(packets.length, 1)
    assert.ok(packets[0]!.inventoryOp?.op?.dropToWorld, 'idle click must keep ordinary hand drop')
  } finally { gameConnection.send = originalSend }
})

test('touch long-press sends one secondary packet, suppresses release tap, and middle drag only pans', context => {
  setActivePinia(createPinia())
  context.mock.timers.enable({ apis: ['setTimeout'] })
  const previousWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  const previousDocument = Object.getOwnPropertyDescriptor(globalThis, 'document')
  const keyboard = new EventTarget()
  Object.defineProperty(globalThis, 'window', { configurable: true, value: keyboard })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: new EventTarget() })
  const packets: proto.IClientMessage[] = []
  context.mock.method(gameConnection, 'send', (packet: proto.IClientMessage) => { packets.push(packet) })
  const startPan = context.mock.method(cameraController, 'startPan', () => {})
  const pan = context.mock.method(cameraController, 'pan', () => {})
  const endPan = context.mock.method(cameraController, 'endPan', () => {})
  const canvas = Object.assign(new EventTarget(), { style: {}, setPointerCapture() {}, releasePointerCapture() {} })
  const input = new InputController()
  const render = Object.create(Render.prototype) as Record<string, unknown> & { setupInputController(): void }
  render.canvas = canvas
  render.inputController = input
  render.screenToWorld = () => ({ x: 42.4, y: -98.6 })
  render.objectManager = { getEntityAtScreen: () => null }
  render.buildGhostController = { isActive: () => false }
  render.liftGhostController = { isActive: () => false }
  render.setupInputController()
  const pointer = (type: string, properties: Record<string, unknown>) => canvas.dispatchEvent(Object.assign(new Event(type, { cancelable: true }), {
    pointerId: 1, pointerType: 'touch', clientX: 10, clientY: 20, button: 0, ...properties,
  }))
  try {
    keyboard.dispatchEvent(Object.assign(new Event('keydown'), { shiftKey: true, ctrlKey: false, altKey: true }))
    pointer('pointerdown', {})
    context.mock.timers.tick(500)
    assert.equal(packets.length, 1)
    assert.equal(packets[0]!.playerAction!.mapClick!.button, proto.MapClickButton.MAP_CLICK_BUTTON_SECONDARY)
    assert.equal(packets[0]!.playerAction!.modifiers, 5)
    pointer('pointerup', {})
    assert.equal(packets.length, 1, 'long-press release must not emit another click')
    pointer('pointerdown', {})
    pointer('pointerup', {})
    assert.equal(packets.length, 2, 'the next ordinary tap must still work')
    assert.equal(packets[1]!.playerAction!.mapClick!.button, proto.MapClickButton.MAP_CLICK_BUTTON_PRIMARY)
    pointer('pointerdown', { pointerType: 'mouse', button: 1 })
    pointer('pointermove', { pointerType: 'mouse', button: 1, clientX: 100, movementX: 90, movementY: 0 })
    pointer('pointerup', { pointerType: 'mouse', button: 1, clientX: 100 })
    assert.equal(startPan.mock.callCount(), 1)
    assert.equal(pan.mock.callCount(), 1)
    assert.equal(endPan.mock.callCount(), 1)
    assert.equal(packets.length, 2, 'camera drag must not send a gameplay packet')
  } finally {
    input.destroy()
    if (previousWindow) Object.defineProperty(globalThis, 'window', previousWindow)
    else Reflect.deleteProperty(globalThis, 'window')
    if (previousDocument) Object.defineProperty(globalThis, 'document', previousDocument)
    else Reflect.deleteProperty(globalThis, 'document')
  }
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
