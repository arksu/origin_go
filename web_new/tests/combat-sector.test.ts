import assert from 'node:assert/strict'
import { test } from 'node:test'
import { Container, Graphics } from 'pixi.js'
import { CombatSectorPreview, COMBAT_SECTOR_DURATION_MS } from '../src/game/CombatSectorPreview'
import { Render } from '../src/game/Render'
import { GameFacade } from '../src/game/GameFacade'
import { directionSectorPoints, type DirectionSector } from '../src/game/hud/directionAim'
import { coordGame2Screen } from '../src/game/utils/coordConvert'
import { setWorldParams } from '../src/game/tiles/Tile'
import { cameraController } from '../src/game/CameraController'

const sector: DirectionSector = { range: 18, angle: Math.PI / 2 }

function near(actual: number, expected: number): void {
  assert.ok(Math.abs(actual - expected) < 1e-9, `${actual} != ${expected}`)
}

function fixture() {
  setWorldParams(12, 100)
  const parent = new Container()
  const preview = new CombatSectorPreview(parent)
  const shape = parent.children[0] as Graphics
  return { parent, preview, shape, destroy() { preview.destroy(); parent.destroy() } }
}

test('confirmed sector projects catalog range around the visual anchor and does not intercept input', context => {
  const f = fixture()
  context.after(f.destroy)
  const origin = coordGame2Screen(100, 200)
  f.preview.show(origin, 0, sector, 2, 1000)
  assert.equal(f.shape.visible, true)
  assert.equal(f.shape.eventMode, 'none')
  assert.equal(f.shape.zIndex, 1900000000)
  assert.equal(f.shape.context.strokeStyle.width * 2, 1.5)
  near(f.shape.x, origin.x)
  near(f.shape.y, origin.y)
  const bounds = f.shape.getLocalBounds()
  for (const point of directionSectorPoints({ x: 0, y: 0 }, 0, sector)) {
    const screen = coordGame2Screen(point.x, point.y)
    assert.ok(screen.x >= bounds.minX && screen.x <= bounds.maxX)
    assert.ok(screen.y >= bounds.minY && screen.y <= bounds.maxY)
  }
})

test('deadline is exactly 1000 ms and expires independently of frame count or background pauses', context => {
  const f = fixture()
  context.after(f.destroy)
  const origin = { x: 100, y: 200 }
  f.preview.show(origin, 0, sector, 1, 250)
  for (let now = 250; now < 1249; now += 16) f.preview.update(origin, 1, now)
  f.preview.update(origin, 1, 250 + COMBAT_SECTOR_DURATION_MS - 1)
  assert.equal(f.shape.visible, true)
  f.preview.update(origin, 1, 250 + COMBAT_SECTOR_DURATION_MS)
  assert.equal(f.shape.visible, false)
  f.preview.show(origin, 0, sector, 1, 2000)
  f.preview.update(origin, 1, 60_000)
  assert.equal(f.shape.visible, false)
})

test('following and camera pan move the sector without recomputing geometry or changing the angle', context => {
  const f = fixture()
  context.after(f.destroy)
  const clears = context.mock.method(f.shape, 'clear')
  const polygons = context.mock.method(f.shape, 'poly')
  const origin = { x: 100, y: 200 }
  f.preview.show(origin, Math.PI / 3, sector, 1, 1000)
  const instruction = f.shape.context.instructions[0]
  const bounds = f.shape.getLocalBounds()
  const geometryBounds = [bounds.minX, bounds.minY, bounds.maxX, bounds.maxY]
  for (let frame = 0; frame < 20; frame++) {
    origin.x += 2
    origin.y -= 1
    f.preview.update(origin, 1, 1000 + frame * 16)
  }
  near(f.shape.x, origin.x)
  near(f.shape.y, origin.y)
  assert.equal(clears.mock.callCount(), 1)
  assert.equal(polygons.mock.callCount(), 1)
  assert.equal(f.shape.context.instructions[0], instruction)
  f.parent.position.set(250, 400)
  const globalOrigin = f.shape.toGlobal({ x: 0, y: 0 })
  near(globalOrigin.x, origin.x + 250)
  near(globalOrigin.y, origin.y + 400)
  const finalBounds = f.shape.getLocalBounds()
  assert.deepEqual([finalBounds.minX, finalBounds.minY, finalBounds.maxX, finalBounds.maxY], geometryBounds)
})

test('zoom preserves screen stroke thickness and redraws only when zoom changes', context => {
  const f = fixture()
  context.after(f.destroy)
  const clears = context.mock.method(f.shape, 'clear')
  const origin = { x: 100, y: 200 }
  f.preview.show(origin, 0, sector, 1, 1000)
  for (const zoom of [0.5, 2, 4]) {
    f.parent.scale.set(zoom)
    f.preview.update(origin, zoom, 1100)
    near(f.shape.context.strokeStyle.width * zoom, 1.5)
    const count = clears.mock.callCount()
    f.preview.update(origin, zoom, 1101)
    assert.equal(clears.mock.callCount(), count)
    const projected = f.shape.toGlobal({ x: 0, y: 0 })
    near(projected.x, origin.x * zoom)
    near(projected.y, origin.y * zoom)
  }
  assert.equal(clears.mock.callCount(), 4)
})

test('new starts replace the only sector; cancellation and a missing owner hide it without resurrection', context => {
  const f = fixture()
  context.after(f.destroy)
  const origin = { x: 0, y: 0 }
  f.preview.show(origin, 0, sector, 1, 1000)
  f.preview.show(origin, Math.PI, { range: 50, angle: Math.PI }, 1, 1500)
  assert.equal(f.parent.children.length, 1)
  assert.equal(f.parent.children[0], f.shape)
  f.preview.update(origin, 1, 2000)
  assert.equal(f.shape.visible, true, 'the newer confirmed start owns the deadline')
  f.preview.clear()
  assert.equal(f.shape.visible, false)
  f.preview.update(origin, 1, 2001)
  assert.equal(f.shape.visible, false)
  f.preview.show(origin, 0, sector, 1, 3000)
  f.preview.update(null, 1, 3001)
  assert.equal(f.shape.visible, false)
  f.preview.update(origin, 1, 3002)
  assert.equal(f.shape.visible, false)
})

test('destroy releases the graphics context and parent ownership', () => {
  const f = fixture()
  f.preview.show({ x: 0, y: 0 }, 0, sector, 1, 1000)
  const graphicsContext = f.shape.context
  f.preview.destroy()
  assert.equal(f.shape.destroyed, true)
  assert.equal(graphicsContext.destroyed, true)
  assert.equal(f.parent.children.length, 0)
  f.parent.destroy()
})

test('Render uses the displayed ObjectView container and clears on owner despawn or owner replacement', context => {
  const f = fixture()
  context.after(f.destroy)
  const owner = new Container()
  owner.position.set(321, 654)
  const objects = new Map([[42, { getContainer: () => owner }]])
  const render = Object.create(Render.prototype) as Record<string, unknown> & {
    showAttackSector(angle: number, sector: DirectionSector): void
    clearAttackSector(): void
    updateCombatSector(now: number): void
    despawnObject(entityId: number): void
    setPlayerEntityId(entityId: number | null): void
  }
  render.playerEntityId = 42
  render.combatSectorPreview = f.preview
  render.objectManager = {
    getObject: (id: number) => objects.get(id),
    despawnObject: (id: number) => objects.delete(id),
    setPlayerEntityId() {},
  }
  render.damageNumberManager = { rememberDespawn() {} }
  render.nicknameManager = { remove() {} }
  context.mock.method(performance, 'now', () => 1000)
  context.mock.method(cameraController, 'getZoom', () => 1)
  context.mock.method(cameraController, 'setTargetEntity', () => {})
  render.showAttackSector(0, sector)
  near(f.shape.x, owner.x)
  near(f.shape.y, owner.y)
  owner.position.set(333, 666)
  render.updateCombatSector(1100)
  near(f.shape.x, 333)
  near(f.shape.y, 666)
  render.despawnObject(42)
  assert.equal(f.shape.visible, false)
  render.showAttackSector(0, sector)
  assert.equal(f.shape.visible, false, 'an absent view cannot create a sector')
  objects.set(42, { getContainer: () => owner })
  render.showAttackSector(0, sector)
  render.setPlayerEntityId(43)
  assert.equal(f.shape.visible, false)
  owner.destroy()
})

test('GameFacade forwards confirmed sector presentation and cleanup without an initialized renderer', context => {
  const facade = new GameFacade()
  facade.showAttackSector(0, sector)
  facade.clearAttackSector()
  const calls: string[] = []
  const renderer = {
    showAttackSector(angle: number, shape: DirectionSector) {
      assert.equal(angle, Math.PI)
      assert.equal(shape, sector)
      calls.push('show')
    },
    clearAttackSector() { calls.push('clear') },
  }
  const state = facade as unknown as { render: typeof renderer | null }
  state.render = renderer
  context.after(() => { state.render = null })
  facade.showAttackSector(Math.PI, sector)
  facade.clearAttackSector()
  assert.deepEqual(calls, ['show', 'clear'])
})

test('world reset hides the sector and renderer destruction releases its resources', () => {
  const f = fixture()
  const noop = () => {}
  const lifecycle = { clear: noop, destroy: noop }
  const render: Render = Object.assign(Object.create(Render.prototype), {
    combatSectorPreview: f.preview, playerEntityId: 42,
    keyboardMovement: { reset: noop, destroy: noop },
    inputController: { setKeyboardMovementEnabled: noop, destroy: noop },
    buildGhostController: { cancel: noop, destroy: noop },
    liftGhostController: { cancel: noop, destroy: noop },
    chatBalloonManager: lifecycle, nicknameManager: lifecycle, damageNumberManager: lifecycle,
    objectManager: lifecycle, chunkManager: lifecycle,
    debugOverlay: { destroy: noop }, app: { ticker: { stop: noop }, destroy: noop },
  })
  const origin = { x: 100, y: 200 }
  f.preview.show(origin, 0, sector, 1, 1000)
  render.resetWorld()
  assert.equal(f.shape.visible, false)
  f.preview.show(origin, Math.PI / 2, sector, 1, 2000)
  render.destroy()
  assert.equal(f.shape.destroyed, true)
  assert.equal(f.parent.children.length, 0)
  f.parent.destroy()
})
