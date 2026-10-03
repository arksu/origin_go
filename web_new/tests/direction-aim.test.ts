import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useGameStore } from '../src/stores/gameStore'
import { useActionPresentation } from '../src/composables/useActionPresentation'
import { cancelActiveActionOnEscape } from '../src/game/hud/actionState'
import { directionSectorPoints, getDirectionSector, normalizeAimAngle, resolveDirectionAimAngle } from '../src/game/hud/directionAim'
import { Render } from '../src/game/Render'
import { moveController } from '../src/game/MoveController'
import { cameraController } from '../src/game/CameraController'
import { gameFacade } from '../src/game/GameFacade'
import type { PointerClickEvent, PointerLongPressEvent } from '../src/game/InputController'
import { coordGame2ScreenWithCamera, coordScreen2GameWithCamera } from '../src/game/utils/coordConvert'
import { setWorldParams } from '../src/game/tiles/Tile'
import { DROP_ITEM_TYPE_ID } from '../src/constants/render'
import { sendActivateAction, gameConnection } from '../src/network'
import { proto } from '../src/network/proto/packets.js'
import { Container, Graphics } from 'pixi.js'
import { DirectionAimPreview } from '../src/game/DirectionAimPreview'
import { coordGame2Screen } from '../src/game/utils/coordConvert'

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
  moveController.initEntity(1, 100, 200, Math.PI / 2)
  setWorldParams(12, 100)
  const packets: proto.IClientMessage[] = []
  const preview: { origin: { x: number; y: number }; direction: number; zoom: number }[] = []
  let keyboardReleases = 0
  let previewClears = 0
  context.mock.method(gameConnection, 'send', (packet: proto.IClientMessage) => { packets.push(packet) })
  context.mock.method(gameFacade, 'releaseKeyboardMovement', () => { keyboardReleases++ })
  const camera = { x: 0, y: 0, zoom: 1 }
  let click: (event: PointerClickEvent) => void = () => { throw new Error('input not initialized') }
  let longPress: (event: PointerLongPressEvent) => void = () => { throw new Error('input not initialized') }
  const render = Object.create(Render.prototype) as Record<string, unknown> & {
    setupInputController(): void
    updateDirectionAim(): void
    resetWorld(): void
  }
  render.canvas = {}
  render.keyboardMovement = { release() { keyboardReleases++ }, reset() {} }
  render.inputController = {
    init() {}, onDirection() {}, suppressMovementKeys() {}, setKeyboardMovementEnabled() {},
    onClick(handler: typeof click) { click = handler }, onLongPress(handler: typeof longPress) { longPress = handler },
    onDragStart() {}, onDragMove() {}, onDragEnd() {}, onWheel() {}, onPinchMove() {}, onPointerMove() {},
  }
  render.screenToWorld = (x: number, y: number) => coordScreen2GameWithCamera(x, y, camera.x, camera.y, camera.zoom, 800, 600)
  render.objectManager = { getEntityAtScreen: () => { throw new Error('aim click attempted object lookup') }, clear() {} }
  render.onClickCallback = () => { throw new Error('aim click reached placement callbacks') }
  render.buildGhostController = { isActive: () => false, cancel() {} }
  render.liftGhostController = { isActive: () => false, cancel() {} }
  render.directionAimSelection = null
  render.directionAimAngle = 0
  render.directionAimPreview = {
    clear() { previewClears++ },
    update(origin: { x: number; y: number }, direction: number, _sector: unknown, zoom: number) { preview.push({ origin, direction, zoom }) },
  }
  context.mock.method(cameraController, 'getState', () => ({ ...camera, panOffsetX: 0, panOffsetY: 0 }))
  render.setupInputController()
  context.after(() => { game.reset(); moveController.clear() })
  return {
    game, render, camera, packets, preview,
    presentation: useActionPresentation(),
    keyboardReleases: () => keyboardReleases,
    previewClears: () => previewClears,
    clickWorld(x: number, y: number, button = 0) {
      const screen = coordGame2ScreenWithCamera(x, y, camera.x, camera.y, camera.zoom, 800, 600)
      click({ screenX: screen.x, screenY: screen.y, button, modifiers: 0 })
    },
    longPress() { longPress({ screenX: 20, screenY: 30, modifiers: 0 }) },
  }
}

test('direction selection stays local, replaces actions and respects shared cooldown checks', context => {
  const f = fixture(context)
  f.game.setGameActionState({ actionId: 'lift', phase: 'selecting', cursor: 'lift' })
  const serverState = f.game.gameActionState
  f.game.armBuildPlacement('campfire')
  f.game.openContextMenu(2, [{ actionId: 'open', title: 'Open' }])
  assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), true)
  const selection = f.game.directionAim
  assert.deepEqual(selection, { actionId: 'axe_sweep', streamEpoch: 7 })
  assert.equal(f.game.gameActionState, serverState)
  assert.equal(f.game.armedBuildKey, '')
  assert.equal(f.game.contextMenu, null)
  assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), true)
  assert.equal(f.game.directionAim, selection, 'reselecting the same action preserves aim direction')
  assert.equal(f.presentation.activate('axe_strike', sendActivateAction), true)
  assert.equal(f.game.directionAim?.actionId, 'axe_strike')
  assert.equal(f.packets.length, 0)
  assert.equal(f.keyboardReleases(), 0)
  f.game.setGameActionState({ phase: 'idle', serverTimeMs: 1000, cooldowns: [{ actionId: 'axe_sweep', startedAtMs: 1000, expiresAtMs: 5000 }] })
  assert.equal(f.presentation.activate('axe_sweep', sendActivateAction), false)
  assert.equal(f.game.directionAim?.actionId, 'axe_strike')
})

test('confirmation sends one directional request with an explicit zero angle before other map tools', context => {
  const f = fixture(context)
  f.game.updateInventory({ ref: { kind: proto.InventoryKind.INVENTORY_KIND_HAND, ownerId: 1, inventoryKey: 0 }, revision: 1, hand: { item: { itemId: 900 } } })
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.clickWorld(118, 200)
  assert.equal(f.packets.length, 1)
  assert.deepEqual(Object.keys(f.packets[0]!), ['activateAction'])
  const request = proto.ClientMessage.decode(proto.ClientMessage.encode(f.packets[0]!).finish()).activateAction!
  assert.equal(request.actionId, 'axe_sweep')
  assert.equal(request.aimAngle, 0)
  assert.equal(Object.hasOwn(request, 'aimAngle'), true)
  assert.equal(request.streamEpoch, 7)
  assert.equal(f.game.directionAim, null)
  assert.equal(Number(f.game.handState?.item?.itemId), 900)
  assert.equal(f.keyboardReleases(), 0)
  // The next primary click uses the ordinary route rather than confirming again.
  f.render.objectManager = { getEntityAtScreen: () => ({ entityId: 888, typeId: DROP_ITEM_TYPE_ID }) }
  f.render.onClickCallback = () => true
  f.clickWorld(118, 200)
  assert.equal(f.packets.filter(packet => packet.activateAction).length, 1)
  assert.equal(Number(f.packets.at(-1)?.playerAction?.mapClick?.targetEntityId), 888)
})

test('Escape and secondary input only cancel local aim without changing server action state', context => {
  const f = fixture(context)
  f.game.setGameActionState({ actionId: 'lift', phase: 'selecting' })
  let serverCancels = 0
  for (const input of ['escape', 'right', 'long press']) {
    f.presentation.activate('axe_sweep', sendActivateAction)
    if (input === 'escape') {
      assert.equal(cancelActiveActionOnEscape('selecting', () => { serverCancels++ }, f.game.cancelDirectionAim), true)
    } else if (input === 'right') {
      f.clickWorld(118, 200, 2)
    } else {
      f.longPress()
    }
    assert.equal(f.game.directionAim, null)
    assert.equal(f.game.gameActionState.phase, 'selecting')
  }
  assert.equal(serverCancels, 0)
  assert.equal(f.packets.length, 0)
  assert.equal(f.keyboardReleases(), 0)
})

test('direction confirmations preserve world angles across pan and zoom', context => {
  const f = fixture(context)
  for (const zoom of [1, 2]) {
    f.camera.x = 25
    f.camera.y = -10
    f.camera.zoom = zoom
    for (const [target, expected] of [
      [{ x: 100, y: 218 }, Math.PI / 2], [{ x: 82, y: 200 }, Math.PI], [{ x: 100, y: 182 }, 1.5 * Math.PI],
    ] as const) {
      f.presentation.activate('axe_strike', sendActivateAction)
      f.clickWorld(target.x, target.y)
      const packet = proto.ClientMessage.decode(proto.ClientMessage.encode(f.packets.at(-1)!).finish())
      assert.equal(packet.activateAction?.aimAngle, Math.fround(expected))
      assert.equal(packet.activateAction?.streamEpoch, 7)
      assert.equal(f.game.directionAim, null)
    }
  }
  assert.equal(f.packets.length, 6)
  assert.equal(f.keyboardReleases(), 0)
})

test('ordinary action activation and building retain their previous routes', context => {
  const f = fixture(context)
  f.presentation.activate('axe_sweep', sendActivateAction)
  assert.equal(f.presentation.activate('lift', sendActivateAction), true)
  assert.equal(f.game.directionAim, null)
  assert.equal(f.packets.length, 1)
  const request = proto.ClientMessage.decode(proto.ClientMessage.encode(f.packets[0]!).finish()).activateAction!
  assert.equal(request.actionId, 'lift')
  assert.equal(Object.hasOwn(request, 'aimAngle'), false)
  assert.equal(request.streamEpoch, 0)
  assert.equal(f.keyboardReleases(), 1)
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.game.armBuildPlacement('campfire')
  assert.equal(f.game.directionAim, null)
  assert.equal(f.game.armedBuildKey, 'campfire')
})

test('preview follows displayed movement and camera changes with a stationary pointer', context => {
  const f = fixture(context)
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.render.lastPointerScreen = null
  f.render.updateDirectionAim()
  assert.equal(f.preview.at(-1)!.direction, Math.PI / 2, 'initial aim uses the visual heading')
  f.render.lastPointerScreen = coordGame2ScreenWithCamera(118, 200, 0, 0, 1, 800, 600)
  f.render.updateDirectionAim()
  assert.equal(f.preview.at(-1)!.direction, 0)
  moveController.initEntity(1, 110, 210, Math.PI)
  f.render.updateDirectionAim()
  assert.deepEqual({ x: f.preview.at(-1)!.origin.x, y: f.preview.at(-1)!.origin.y }, { x: 110, y: 210 })
  assert.ok(Math.abs(f.preview.at(-1)!.direction - normalizeAimAngle(Math.atan2(-10, 8))) < 1e-12)
  f.camera.x = 25
  f.camera.y = -10
  f.camera.zoom = 2
  f.render.updateDirectionAim()
  assert.equal(f.preview.at(-1)!.zoom, 2)
  assert.notEqual(f.preview.at(-1)!.direction, f.preview.at(-2)!.direction)
  assert.equal(f.packets.length, 0)
})

test('stale worlds, disconnect and removed definitions discard aim without a request', context => {
  const f = fixture(context)
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.game.worldParams!.streamEpoch = 8
  f.clickWorld(118, 200)
  assert.equal(f.game.directionAim, null)
  assert.equal(f.packets.length, 0)
  sendActivateAction('axe_sweep', { aimAngle: 0, streamEpoch: 7 })
  assert.equal(f.packets.length, 0)
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.game.setGameActionList([{ id: 'lift', targetKind: 'object' }])
  assert.equal(f.game.directionAim, null)
  f.game.setGameActionList(actions)
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.game.setConnectionState('disconnected')
  assert.equal(f.game.directionAim, null)
  sendActivateAction('axe_sweep', { aimAngle: 0, streamEpoch: 8 })
  assert.equal(f.packets.length, 0)
  f.game.setConnectionState('connected')
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.game.setPlayerLeaveWorld()
  assert.equal(f.game.directionAim, null)
  assert.equal(f.packets.length, 0)
})

test('renderer reset and destruction clear local selection and preview resources', context => {
  const f = fixture(context)
  f.render.clearMinimap = () => {}
  f.render.chatBalloonManager = { clear() {}, destroy() {} }
  f.render.nicknameManager = { clear() {}, destroy() {} }
  f.render.chunkManager = { clear() {}, destroy() {} }
  f.presentation.activate('axe_sweep', sendActivateAction)
  f.render.resetWorld()
  assert.equal(f.game.directionAim, null)
  assert.ok(f.previewClears() > 0)
  let previewDestroyed = false
  f.render.directionAimPreview = { clear() {}, destroy() { previewDestroyed = true } }
  f.render.keyboardMovement = { destroy() {} }
  f.render.inputController = { destroy() {} }
  f.render.buildGhostController = { destroy() {} }
  f.render.liftGhostController = { destroy() {} }
  f.render.objectManager = { destroy() {} }
  f.render.debugOverlay = { destroy() {} }
  f.render.app = { ticker: { stop() {} }, destroy() {} }
  f.presentation.activate('axe_sweep', sendActivateAction)
  Render.prototype.destroy.call(f.render as unknown as Render)
  assert.equal(f.game.directionAim, null)
  assert.equal(previewDestroyed, true)
  assert.equal(f.packets.length, 0)
})

test('Pixi preview projects the world sector and keeps stroke width constant under zoom', () => {
  setWorldParams(12, 100)
  const parent = new Container()
  const preview = new DirectionAimPreview(parent)
  const shape = parent.children[0] as Graphics
  const origin = { x: 100, y: 200 }
  const sector = { range: 18, angle: Math.PI / 2 }
  preview.update(origin, 0, sector, 2)
  assert.equal(shape.eventMode, 'none')
  assert.equal(shape.context.strokeStyle.width * 2, 1.5)
  const bounds = shape.getLocalBounds()
  for (const point of directionSectorPoints(origin, 0, sector)) {
    const screen = coordGame2Screen(point.x, point.y)
    assert.ok(screen.x >= bounds.minX && screen.x <= bounds.maxX)
    assert.ok(screen.y >= bounds.minY && screen.y <= bounds.maxY)
  }
  preview.clear()
  assert.equal(shape.context.instructions.length, 0)
  preview.destroy()
  assert.equal(parent.children.length, 0)
  parent.destroy()
})

test('direction geometry uses radians and rejects missing or invalid sector definitions', () => {
  const origin = { x: 100, y: 200 }
  for (const [pointer, angle] of [
    [{ x: 110, y: 200 }, 0], [{ x: 100, y: 210 }, Math.PI / 2],
    [{ x: 90, y: 200 }, Math.PI], [{ x: 100, y: 190 }, Math.PI * 1.5],
  ] as const) assert.equal(resolveDirectionAimAngle(origin, pointer, 0), angle)
  assert.equal(resolveDirectionAimAngle(origin, origin, Math.PI), Math.PI)
  assert.equal(resolveDirectionAimAngle(origin, null, NaN), 0)
  const sector = { range: 18, angle: Math.PI / 2 }
  const points = directionSectorPoints(origin, 0, sector)
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
