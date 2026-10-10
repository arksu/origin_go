import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { InputController } from '../src/game/InputController'
import { KeyboardMovementController } from '../src/game/KeyboardMovementController'
import { screenMovementDirection } from '../src/game/utils/movementDirection'
import { coordGame2Screen } from '../src/game/utils/coordConvert'
import { setWorldParams } from './serverConstantsFixture'
import { playerCommandController } from '../src/game/PlayerCommandController'
import { gameConnection } from '../src/network/GameConnection'
import { gameFacade } from '../src/game/GameFacade'
import { Render } from '../src/game/Render'
import { cameraController } from '../src/game/CameraController'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { sendActivateAction, sendStartCraftOne, sendStartCraftMany, sendStartBuild, sendBuildProgress, sendOpenWindow } from '../src/network'
import { DROP_ITEM_TYPE_ID } from '../src/constants/render'
import { DIRECTION_INPUT_TTL_MS, DIRECTION_REFRESH_MS } from '../src/constants/movement'
import './movement-protocol.test.mjs'
import './object-batches.test'

function inputFixture(context: TestContext) {
  context.mock.timers.enable({ apis: ['setTimeout', 'Date'], now: 1000 })
  const priorWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  const priorDocument = Object.getOwnPropertyDescriptor(globalThis, 'document')
  const windowEvents = new EventTarget()
  const documentEvents = Object.assign(new EventTarget(), { hidden: false, activeElement: null as unknown })
  Object.defineProperty(globalThis, 'window', { configurable: true, value: windowEvents })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: documentEvents })
  const canvas = Object.assign(new EventTarget(), { style: {}, setPointerCapture() {}, releasePointerCapture() {} }) as unknown as HTMLCanvasElement
  const input = new InputController()
  const packets: proto.IClientMessage[] = []
  context.mock.method(gameConnection, 'send', (packet: proto.IClientMessage) => packets.push(proto.ClientMessage.decode(proto.ClientMessage.encode(packet).finish())))
  let connected = true
  let clock: number | null = null
  const keyboard = new KeyboardMovementController(
    (x, y, revision, epoch) => playerCommandController.sendMoveDirection(x, y, revision, epoch),
    () => connected,
    () => input.suppressMovementKeys(),
    () => clock ?? Date.now(),
  )
  input.onDirection((x, y) => keyboard.setDirection(x, y))
  input.init(canvas)
  input.setKeyboardMovementEnabled(keyboard.configure(7, true, true))
  context.after(() => {
    keyboard.destroy()
    input.destroy()
    if (priorWindow) Object.defineProperty(globalThis, 'window', priorWindow)
    else Reflect.deleteProperty(globalThis, 'window')
    if (priorDocument) Object.defineProperty(globalThis, 'document', priorDocument)
    else Reflect.deleteProperty(globalThis, 'document')
    setWorldParams(32, 128)
  })
  const key = (type: 'keydown' | 'keyup', code: string, options: Record<string, unknown> = {}) => {
    const event = Object.assign(new Event(type, { cancelable: true }), {
      code, key: code === 'KeyW' ? 'ц' : code, repeat: false, shiftKey: false, ctrlKey: false, altKey: false, metaKey: false, isComposing: false,
      ...options,
    })
    windowEvents.dispatchEvent(event)
    return event
  }
  return { input, keyboard, packets, key, windowEvents, documentEvents, canvas,
    setConnected(value: boolean) { connected = value }, setClock(value: number) { clock = value },
    release() { keyboard.release(); input.suppressMovementKeys() },
  }
}

const direction = (packet: proto.IClientMessage | undefined) => packet!.playerAction!.moveDirection!

test('KO and lying disable WASD until confirmed standing and require a fresh key press', context => {
  setActivePinia(createPinia())
  const fixture = inputFixture(context)
  const game = useGameStore()
  setWorldParams(12, 128, true)
  game.setConnectionState('connected')
  game.setPlayerEnterWorld(1, 'test', 7)
  const render = Object.create(Render.prototype) as Render
  Object.assign(render, { inputController: fixture.input, keyboardMovement: fixture.keyboard })
  render.setKeyboardMovementEnabled(true)
  fixture.key('keydown', 'KeyW')
  assert.equal(fixture.packets.length, 1)
  game.setPlayerStats({ isKnockedOut: true, isLying: true })
  render.setKeyboardMovementEnabled(true)
  assert.equal(fixture.packets.length, 2)
  assert.equal(direction(fixture.packets[1]).x, 0)
  fixture.key('keydown', 'KeyD')
  context.mock.timers.tick(1000)
  assert.equal(fixture.packets.length, 2)
  game.setPlayerStats({ isKnockedOut: false, isLying: true })
  render.setKeyboardMovementEnabled(true)
  fixture.key('keydown', 'KeyW')
  assert.equal(fixture.packets.length, 2, 'local KO completion must not enable movement')
  game.setPlayerStats({ isKnockedOut: false, isLying: false })
  render.setKeyboardMovementEnabled(true)
  context.mock.timers.tick(1000)
  assert.equal(fixture.packets.length, 2, 'old held input must not resume')
  fixture.key('keyup', 'KeyW')
  fixture.key('keydown', 'KeyW')
  assert.equal(fixture.packets.length, 3)
  assert.notEqual(direction(fixture.packets[2]).x, 0)
})

test('every physical key combination uses screen axes and constant world speed', () => {
  for (const coordPerTile of [12, 32, 64]) {
    setWorldParams(coordPerTile, 128)
    for (let mask = 0; mask < 16; mask++) {
      const screenX = Number(!!(mask & 8)) - Number(!!(mask & 2))
      const screenY = Number(!!(mask & 4)) - Number(!!(mask & 1))
      const world = screenMovementDirection(screenX, screenY)
      const length = Math.hypot(world.x, world.y)
      assert.ok(Math.abs(length - (screenX || screenY ? 1 : 0)) < 1e-7)
      for (const zoom of [.5, 1, 3]) {
        const projected = coordGame2Screen(world.x, world.y)
        assert.ok(Math.abs(projected.x * screenY * zoom - projected.y * screenX * zoom) < 1e-6)
        assert.ok(projected.x * screenX + projected.y * screenY >= 0)
      }
    }
  }
  setWorldParams(32, 128)
})

test('W+D serializes normalized floats, refreshes exactly five times per second and releases once', context => {
  const f = inputFixture(context)
  f.key('keydown', 'KeyW')
  f.key('keydown', 'KeyD')
  const initial = direction(f.packets[1])
  assert.ok(Math.abs(Math.hypot(initial.x!, initial.y!) - 1) < 1e-7)
  assert.ok(Math.abs(initial.x! + 1 / Math.sqrt(10)) < 1e-7)
  assert.ok(Math.abs(initial.y! + 3 / Math.sqrt(10)) < 1e-7)
  assert.equal(initial.inputRevision, 2)
  assert.equal(initial.streamEpoch, 7)
  for (let i = 0; i < 5; i++) {
    f.key('keydown', 'KeyW', { repeat: true })
    context.mock.timers.tick(DIRECTION_REFRESH_MS)
    assert.deepEqual(direction(f.packets.at(-1)), initial)
  }
  assert.equal(f.packets.length, 7)
  f.key('keyup', 'KeyW')
  f.key('keyup', 'KeyD')
  assert.equal(f.packets.length, 9)
  assert.equal(direction(f.packets.at(-1)).x, 0)
  assert.equal(direction(f.packets.at(-1)).y, 0)
  assert.equal(direction(f.packets.at(-1)).inputRevision, 4)
  context.mock.timers.tick(5000)
  assert.equal(f.packets.length, 9)
})

test('opposing keys stop and removing one starts fresh input; Shift leaves direction unchanged', context => {
  const f = inputFixture(context)
  f.key('keydown', 'KeyW', { shiftKey: true })
  f.key('keydown', 'KeyS')
  assert.equal(direction(f.packets.at(-1)).x, 0)
  f.key('keydown', 'KeyD')
  assert.deepEqual([direction(f.packets.at(-1)).x, direction(f.packets.at(-1)).y], Object.values(screenMovementDirection(1, 0)))
  f.key('keyup', 'KeyW')
  assert.deepEqual([direction(f.packets.at(-1)).x, direction(f.packets.at(-1)).y], Object.values(screenMovementDirection(1, 1)))
})

test('handoff suppresses held keys, repeat and old keyup; a fresh key recovers missed keyup', context => {
  const f = inputFixture(context)
  f.key('keydown', 'KeyW')
  f.release()
  assert.equal(f.packets.length, 2)
  f.key('keydown', 'KeyW', { repeat: true })
  f.key('keydown', 'KeyD')
  assert.deepEqual([direction(f.packets.at(-1)).x, direction(f.packets.at(-1)).y], Object.values(screenMovementDirection(1, 0)))
  f.key('keyup', 'KeyW')
  assert.equal(f.packets.length, 3)
  f.windowEvents.dispatchEvent(new Event('blur'))
  f.key('keydown', 'KeyD', { repeat: true })
  assert.equal(f.packets.length, 4)
  f.key('keydown', 'KeyD')
  assert.equal(f.packets.length, 5)
})

test('focus, composition and browser shortcuts release immediately and ignore text input', context => {
  const f = inputFixture(context)
  for (const target of [{ tagName: 'INPUT' }, { tagName: 'TEXTAREA' }, { tagName: 'SELECT' }, { tagName: 'DIV', isContentEditable: true }]) {
    f.key('keydown', 'KeyW')
    const before = f.packets.length
    f.documentEvents.activeElement = target
    f.documentEvents.dispatchEvent(Object.assign(new Event('focusin'), { composedPath: () => [target] }))
    assert.equal(f.packets.length, before + 1)
    assert.equal(direction(f.packets.at(-1)).x, 0)
    f.key('keyup', 'KeyW')
    assert.equal(f.key('keydown', 'KeyW').defaultPrevented, false)
    assert.equal(f.packets.length, before + 1)
    f.documentEvents.activeElement = null
  }
  f.key('keydown', 'KeyW')
  f.documentEvents.dispatchEvent(new Event('compositionstart'))
  let count = f.packets.length
  f.key('keydown', 'KeyD', { isComposing: true })
  assert.equal(f.packets.length, count)
  f.documentEvents.dispatchEvent(new Event('compositionend'))
  for (const modifier of ['ctrlKey', 'altKey', 'metaKey']) {
    f.key('keydown', 'KeyW')
    count = f.packets.length
    assert.equal(f.key('keydown', 'KeyD', { [modifier]: true }).defaultPrevented, false)
    assert.equal(f.packets.length, count + 1)
    assert.equal(direction(f.packets.at(-1)).x, 0)
  }
})

test('disabled sessions, disconnect, visibility and timer suspension require a fresh press', context => {
  const f = inputFixture(context)
  for (const [epoch, supported, enabled] of [[7, false, true], [0, true, true], [7, true, false]] as const) {
    f.input.setKeyboardMovementEnabled(f.keyboard.configure(epoch, supported, enabled))
    f.key('keydown', 'KeyW')
    context.mock.timers.tick(1000)
    assert.equal(f.packets.length, 0)
  }
  f.input.setKeyboardMovementEnabled(f.keyboard.configure(7, true, true))
  f.key('keydown', 'KeyW', { repeat: true })
  assert.equal(f.packets.length, 0)
  f.key('keydown', 'KeyW')
  f.documentEvents.hidden = true
  f.documentEvents.dispatchEvent(new Event('visibilitychange'))
  assert.equal(f.packets.length, 2)
  f.documentEvents.hidden = false
  context.mock.timers.tick(1000)
  assert.equal(f.packets.length, 2)
  f.key('keydown', 'KeyW')
  f.setClock(Date.now() + DIRECTION_INPUT_TTL_MS)
  context.mock.timers.tick(200)
  assert.equal(f.packets.length, 4, 'suspension sends one release, no refresh burst')
  context.mock.timers.tick(3000)
  assert.equal(f.packets.length, 4)
  f.key('keydown', 'KeyD')
  f.setConnected(false)
  f.keyboard.reset()
  context.mock.timers.tick(1000)
  assert.equal(f.packets.length, 5, 'offline reset must not send')
  f.input.setKeyboardMovementEnabled(f.keyboard.configure(8, true, true))
  f.setConnected(true)
  f.key('keydown', 'KeyD', { repeat: true })
  assert.equal(f.packets.length, 5)
  f.key('keydown', 'KeyD')
  assert.equal(direction(f.packets.at(-1)).streamEpoch, 8)
  assert.equal(direction(f.packets.at(-1)).inputRevision, 1)
})

test('explicit action, context, craft and build requests release before sending', context => {
  setActivePinia(createPinia())
  const f = inputFixture(context)
  context.mock.method(gameFacade, 'releaseKeyboardMovement', () => f.release())
  for (const request of [
    () => sendActivateAction('dig'), () => playerCommandController.sendSelectContextAction(9, 'open'),
    () => sendStartCraftOne('test'), () => sendStartCraftMany('test', 3),
    () => sendStartBuild('test', { x: 10, y: 20 }), () => sendBuildProgress(9),
  ]) {
    f.key('keydown', 'KeyW')
    f.packets.length = 0
    request()
    assert.equal(f.packets.length, 2)
    assert.equal(direction(f.packets[0]).x, 0)
    assert.ok(f.packets[1]!.playerAction?.moveDirection == null)
    f.key('keyup', 'KeyW')
    context.mock.timers.tick(1000)
    assert.equal(f.packets.length, 2)
  }
  f.key('keydown', 'KeyW')
  f.packets.length = 0
  sendOpenWindow('inventory')
  context.mock.timers.tick(200)
  assert.equal(f.packets.length, 2)
  assert.ok(f.packets[0]!.openWindow)
  assert.notEqual(direction(f.packets[1]).x, 0)
})

test('Render sends stop before primary, secondary, long-press, placement and hand drop', context => {
  setActivePinia(createPinia())
  const f = inputFixture(context)
  const render = Object.create(Render.prototype) as Record<string, any>
  render.canvas = f.canvas
  render.inputController = f.input
  render.keyboardMovement = f.keyboard
  render.screenToWorld = () => ({ x: 200, y: 300 })
  let target: { entityId: number; typeId: number } | null = null
  render.objectManager = { getEntityAtScreen: () => target }
  render.buildGhostController = { isActive: () => false }
  render.liftGhostController = { isActive: () => false }
  render.setupInputController()
  f.input.setKeyboardMovementEnabled(true)
  const store = useGameStore()
  setWorldParams(12, 128, true)
  store.setConnectionState('connected')
  store.setPlayerEnterWorld(1, 'test', 7)
  const pointer = (type: string, button = 0, touch = false) => f.canvas.dispatchEvent(Object.assign(new Event(type, { cancelable: true }), {
    pointerId: 1, pointerType: touch ? 'touch' : 'mouse', clientX: 10, clientY: 20, button,
  }))
  for (const operation of ['primary', 'secondary', 'long-press', 'drop-item', 'placement', 'hand-drop']) {
    target = operation === 'drop-item' ? { entityId: 99, typeId: DROP_ITEM_TYPE_ID } : null
    render.onClickCallback = operation === 'placement' ? () => {
      playerCommandController.sendMapClick(42, 42, 0, 0)
      return true
    } : null
    if (operation === 'hand-drop') store.updateInventory({
      ref: { kind: proto.InventoryKind.INVENTORY_KIND_HAND, ownerId: 1, inventoryKey: 0 },
      revision: 1, hand: { item: { itemId: 900 } },
    })
    f.key('keydown', 'KeyW')
    f.packets.length = 0
    pointer('pointerdown', operation === 'secondary' ? 2 : 0, operation === 'long-press')
    if (operation === 'long-press') {
      context.mock.timers.tick(200)
      context.mock.timers.tick(200)
      context.mock.timers.tick(100)
      // Two scheduled refreshes precede the long-press trigger.
      f.packets.splice(0, 2)
    }
    pointer('pointerup', operation === 'secondary' ? 2 : 0, operation === 'long-press')
    assert.equal(f.packets.length, 2, operation)
    assert.equal(direction(f.packets[0]).x, 0, operation)
    if (operation === 'hand-drop') assert.ok(f.packets[1]!.inventoryOp?.op?.dropToWorld)
    else assert.ok(f.packets[1]!.playerAction?.mapClick)
    if (operation === 'drop-item') assert.equal(Number(f.packets[1]!.playerAction!.mapClick!.targetEntityId), 99)
    f.key('keydown', 'KeyW', { repeat: true })
    f.key('keyup', 'KeyW')
    context.mock.timers.tick(1000)
    assert.equal(f.packets.length, 2, 'old hold must not cancel the route')
  }
  const pan = context.mock.method(cameraController, 'pan', () => {})
  const zoom = context.mock.method(cameraController, 'adjustZoom', () => {})
  context.mock.method(cameraController, 'startPan', () => {})
  context.mock.method(cameraController, 'endPan', () => {})
  f.key('keydown', 'KeyW')
  f.packets.length = 0
  pointer('pointerdown', 1)
  f.canvas.dispatchEvent(Object.assign(new Event('pointermove'), {
    pointerType: 'mouse', clientX: 40, clientY: 50, movementX: 30, movementY: 30,
  }))
  pointer('pointerup', 1)
  f.canvas.dispatchEvent(Object.assign(new Event('wheel', { cancelable: true }), { deltaY: 1 }))
  f.canvas.dispatchEvent(Object.assign(new Event('pointermove'), { pointerType: 'mouse', clientX: 45, clientY: 55 }))
  f.key('keydown', 'KeyI')
  f.key('keydown', 'KeyC')
  context.mock.timers.tick(200)
  assert.equal(pan.mock.callCount(), 1)
  assert.equal(zoom.mock.callCount(), 1)
  assert.equal(f.packets.length, 1, 'pan, zoom, hover and window hotkeys must preserve the hold')
  assert.notEqual(direction(f.packets[0]).x, 0)
})

test('client revision wraps without serializing zero', context => {
  const f = inputFixture(context)
  Reflect.set(f.keyboard, 'revision', 0xffffffff)
  f.key('keydown', 'KeyW')
  assert.equal(direction(f.packets[0]).inputRevision, 1)
  f.key('keyup', 'KeyW')
  assert.equal(direction(f.packets[1]).inputRevision, 2)
})

test('destroy and re-init remove keyboard listeners and timers; modal reset requires fresh input', context => {
  const f = inputFixture(context)
  for (let index = 0; index < 3; index++) {
    f.input.init(f.canvas)
    f.input.setKeyboardMovementEnabled(f.keyboard.configure(7 + index, true, true))
    const before = f.packets.length
    f.key('keydown', 'KeyW')
    assert.equal(f.packets.length, before + 1)
    f.input.setKeyboardMovementEnabled(false)
    assert.equal(f.packets.length, before + 2)
    f.input.setKeyboardMovementEnabled(true)
    f.key('keydown', 'KeyW', { repeat: true })
    assert.equal(f.packets.length, before + 2)
    f.key('keydown', 'KeyW')
    assert.equal(f.packets.length, before + 3)
    f.keyboard.destroy()
    f.input.destroy()
    f.input.destroy()
    context.mock.timers.tick(1000)
    const count = f.packets.length
    f.key('keydown', 'KeyD')
    f.key('keyup', 'KeyW')
    assert.equal(f.packets.length, count)
  }
})
