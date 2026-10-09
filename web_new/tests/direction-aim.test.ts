import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useGameStore } from '../src/stores/gameStore'
import { useActionPresentation } from '../src/composables/useActionPresentation'
import { cancelActiveActionOnEscape } from '../src/game/hud/actionState'
import { directionSectorPoints, getDirectionSector } from '../src/game/hud/directionAim'
import { Render } from '../src/game/Render'
import { moveController } from '../src/game/MoveController'
import { gameFacade } from '../src/game/GameFacade'
import type { PointerClickEvent, PointerLongPressEvent } from '../src/game/InputController'
import { DROP_ITEM_TYPE_ID } from '../src/constants/render'
import { sendActivateAction, gameConnection } from '../src/network'
import { proto } from '../src/network/proto/packets.js'

const actions: proto.IActionDefinition[] = [
  { id: 'axe_sweep', targetKind: 'direction', sector: { range: 18, sectorAngle: Math.fround(Math.PI / 2) } },
  { id: 'axe_strike', targetKind: 'direction', sector: { range: 18, sectorAngle: Math.fround(Math.PI / 2) } },
  { id: 'lift', targetKind: 'object' },
]

function fixture(context: TestContext) {
  setActivePinia(createPinia())
  const game = useGameStore()
  game.setConnectionState('connected')
  game.setPlayerEnterWorld(1, 'Player', 12, 100, 7)
  game.setGameActionList(actions)
  const packets: proto.IClientMessage[] = []
  const order: string[] = []
  context.mock.method(gameConnection, 'send', (packet: proto.IClientMessage) => { order.push('send'); packets.push(packet) })
  context.mock.method(gameFacade, 'releaseKeyboardMovement', () => { order.push('release') })
  let click: (event: PointerClickEvent) => void = () => { throw new Error('input not initialized') }
  let longPress: (event: PointerLongPressEvent) => void = () => { throw new Error('input not initialized') }
  const render = Object.create(Render.prototype) as Record<string, unknown> & { setupInputController(): void }
  render.canvas = {}
  render.keyboardMovement = { release() { order.push('release') } }
  render.inputController = {
    init() {}, onDirection() {}, suppressMovementKeys() {},
    onClick(handler: typeof click) { click = handler }, onLongPress(handler: typeof longPress) { longPress = handler },
    onDragStart() {}, onDragMove() {}, onDragEnd() {}, onWheel() {}, onPinchMove() {}, onPointerMove() {},
  }
  render.screenToWorld = (x: number, y: number) => ({ x, y })
  render.objectManager = { getEntityAtScreen: () => ({ entityId: 888, typeId: DROP_ITEM_TYPE_ID }) }
  render.onClickCallback = () => false
  render.buildGhostController = { isActive: () => false }
  render.liftGhostController = { isActive: () => false }
  render.setupInputController()
  context.after(() => { game.reset(); moveController.clear() })
  return {
    game, render, packets, order, presentation: useActionPresentation(),
    click(button = 0) { click({ screenX: 118, screenY: 200, button, modifiers: 0 }) },
    longPress() { longPress({ screenX: 20, screenY: 30, modifiers: 0 }) },
  }
}

test('directional actions activate immediately with epoch and WASD release before sending', context => {
  const f = fixture(context)
  f.game.setGameActionState({ actionId: 'lift', phase: 'selecting', cursor: 'lift' })
  const serverState = f.game.gameActionState
  f.game.armBuildPlacement('campfire')
  f.game.openContextMenu(2, [{ actionId: 'open', title: 'Open' }])
  for (const id of ['axe_sweep', 'axe_strike', 'axe_strike']) {
    assert.equal(f.presentation.activate(id, sendActivateAction), true)
    const request = proto.ClientMessage.decode(proto.ClientMessage.encode(f.packets.at(-1)!).finish()).activateAction!
    assert.equal(request.actionId, id)
    assert.equal(request.streamEpoch, 7)
    assert.equal(Object.hasOwn(request, 'aimAngle'), false)
  }
  assert.deepEqual(f.order, ['release', 'send', 'release', 'send', 'release', 'send'])
  assert.equal(f.game.gameActionState, serverState, 'execution is confirmed only by the server')
  assert.equal(f.game.armedBuildKey, '')
  assert.equal(f.game.contextMenu, null)
})

test('shared cooldown checks reject without releasing movement or sending', context => {
  const f = fixture(context)
  f.game.setGameActionState({ phase: 'idle', serverTimeMs: 1000, cooldowns: [{ actionId: 'axe_sweep', startedAtMs: 1000, expiresAtMs: 5000 }] })
  assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), false)
  assert.deepEqual(f.order, [])
  assert.deepEqual(f.packets, [])
})

test('primary click, RMB, long press and Escape retain ordinary routes after immediate activation', context => {
  const f = fixture(context)
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.click()
  assert.equal(Number(f.packets.at(-1)?.playerAction?.mapClick?.targetEntityId), 888)
  f.click(2)
  assert.equal(f.packets.at(-1)?.playerAction?.mapClick?.button, proto.MapClickButton.MAP_CLICK_BUTTON_SECONDARY)
  f.longPress()
  assert.equal(f.packets.at(-1)?.playerAction?.mapClick?.button, proto.MapClickButton.MAP_CLICK_BUTTON_SECONDARY)
  assert.equal(f.packets.filter(packet => packet.activateAction).length, 1)
  let cancels = 0
  assert.equal(cancelActiveActionOnEscape('executing', () => { cancels++ }), true)
  assert.equal(cancelActiveActionOnEscape('idle', () => { cancels++ }), false)
  assert.equal(cancels, 1)
})

test('client movement heading and pointer coordinates never select the attack angle', context => {
  const f = fixture(context)
  for (const heading of [0, -Math.PI / 2, Math.PI, 1.25]) {
    moveController.initEntity(1, 100, 200, heading)
    f.presentation.activate('axe_strike', sendActivateAction)
    assert.deepEqual({ ...f.packets.at(-1)!.activateAction }, { actionId: 'axe_strike', streamEpoch: 7 })
  }
})

test('ordinary activation retains its existing packet contract', context => {
  const f = fixture(context)
  assert.equal(f.presentation.activate('lift', sendActivateAction), true)
  assert.equal(f.packets.length, 1)
  assert.deepEqual({ ...f.packets[0]!.activateAction }, { actionId: 'lift' })
  assert.deepEqual(f.order, ['release', 'send'])
})

test('direction activation requires an active world, epoch and valid catalog sector', context => {
  const f = fixture(context)
  for (const epoch of [0, -1, 1.5, NaN, Infinity, 0x100000000]) {
    f.game.worldParams!.streamEpoch = epoch
    assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), false)
    sendActivateAction('axe_sweep')
  }
  f.game.worldParams!.streamEpoch = 7
  f.game.setGameActionList([{ id: 'axe_sweep', targetKind: 'direction' }])
  assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), false)
  sendActivateAction('axe_sweep')
  f.game.setGameActionList(actions)
  f.game.setConnectionState('disconnected')
  assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), false)
  sendActivateAction('axe_sweep')
  f.game.setConnectionState('connected')
  f.game.setPlayerLeaveWorld()
  assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), false)
  assert.deepEqual(f.packets, [])
  assert.deepEqual(f.order, [])
})

test('catalog geometry uses world radians, range and full sector width', () => {
  const origin = { x: 100, y: 200 }
  const points = directionSectorPoints(origin, 0, { range: 18, angle: Math.PI / 2 })
  assert.deepEqual(points[0], origin)
  for (const point of points.slice(1)) assert.ok(Math.abs(Math.hypot(point.x - origin.x, point.y - origin.y) - 18) < 1e-12)
  assert.ok(Math.abs(Math.atan2(points[1]!.y - origin.y, points[1]!.x - origin.x) + Math.PI / 4) < 1e-12)
  assert.ok(Math.abs(Math.atan2(points.at(-1)!.y - origin.y, points.at(-1)!.x - origin.x) - Math.PI / 4) < 1e-12)
  for (const action of [
    { targetKind: 'direction' }, { targetKind: 'direction', sector: { range: 18, sectorAngle: 0 } },
    { targetKind: 'direction', sector: { range: NaN, sectorAngle: 1 } },
    { targetKind: 'direction', sector: { range: 18, sectorAngle: Infinity } },
    { targetKind: 'direction', sector: { range: 18, sectorAngle: 7 } },
    { targetKind: 'object', sector: { range: 18, sectorAngle: 1 } },
  ]) assert.equal(getDirectionSector(action), null)
  assert.equal(getDirectionSector({ targetKind: 'direction', sector: { range: 18, sectorAngle: Math.fround(2 * Math.PI) } })?.angle, 2 * Math.PI)
})
