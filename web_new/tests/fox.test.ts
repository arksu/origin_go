import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, test, type TestContext } from 'node:test'
import { Assets, Container, Sprite, Texture, TextureSource } from 'pixi.js'
import { ObjectManager } from '../src/game/ObjectManager'
import { ObjectView } from '../src/game/ObjectView'
import { MOVEMENT_DIRECTIONS, ResourceLoader } from '../src/game/ResourceLoader'
import { cullingController } from '../src/game/culling'
import { coordScreen2Game } from '../src/game/utils/coordConvert'
import { setWorldParams } from './serverConstantsFixture'

const fox = ResourceLoader.getResourceDef('fox')!
const sheet = fox.layers[0]!.spriteSheet!
// Published source indices are NE, N, E, SE, NW, W, S, SW.
const sourceRows = [0, 2, 3, 6, 7, 5, 4, 1]
const stride = 1.92 * 12

beforeEach(() => {
  setWorldParams(12, 128)
  ResourceLoader.clearCache()
  cullingController.clear()
})

function heading(direction: number): number {
  const angle = (direction - 1) * Math.PI / 4
  const delta = coordScreen2Game(Math.cos(angle), Math.sin(angle))
  return Math.atan2(delta.y, delta.x)
}

function fixture(t: TestContext) {
  const bytes = readFileSync(resolve('public/assets/game', sheet.img))
  const atlas = new Texture({ source: new TextureSource({ width: bytes.readUInt32BE(16), height: bytes.readUInt32BE(20) }) })
  let finish!: (texture: Texture) => void
  const pending = new Promise<Texture>(resolve => { finish = resolve })
  const loading = t.mock.method(Assets, 'load', () => pending)
  const parent = new Container()
  const manager = new ObjectManager()
  manager.setParentContainer(parent)
  t.after(() => {
    manager.clear()
    parent.destroy({ children: true })
    cullingController.clear()
    ResourceLoader.clearCache()
    atlas.destroy(true)
  })
  const spawn = (entityId: number, direction: number) => {
    manager.spawnObject({ entityId, typeId: 110, resourcePath: 'fox', position: { x: 0, y: 0, heading: heading(direction) }, size: { x: 6, y: 6 } })
    return manager.getObject(entityId)!
  }
  const loaded = async () => {
    finish(atlas)
    await new Promise<void>(resolve => setImmediate(resolve))
  }
  return { manager, spawn, loaded, loading, atlas }
}

function sprite(view: ObjectView): Sprite {
  const children = view.getContainer().children
  assert.equal(children.length, 1, 'fox must use one shared sheet sprite')
  assert.ok(children[0] instanceof Sprite)
  return children[0]
}

function assertFrame(view: ObjectView, direction: number, frame: number): void {
  const image = sprite(view)
  assert.deepEqual([image.texture.frame.x, image.texture.frame.y, image.texture.width, image.texture.height],
    [frame * 42, sourceRows[direction]! * 34, 42, 34], `${MOVEMENT_DIRECTIONS[direction]} frame ${frame}`)
  assert.deepEqual([image.x, image.y], [-19, -22], 'all frames retain the shared ground anchor')
}

test('fox resolves to a published sheet while legacy idle and walk resources remain available', () => {
  assert.deepEqual(fox.size, [42, 34])
  assert.deepEqual(fox.offset, [19, 22])
  assert.equal(fox.layers[0]!.interactive, true)
  assert.equal(sheet.frameCount, 4)
  assert.equal(sheet.idleFrame, 0, 'idle holds a clean walk pose; legacy standing PNGs have opaque backdrops')
  assert.equal(sheet.frameDurationMs, 180)
  assert.equal(sheet.cycleDistanceTiles, 32 / 12 * 4 * .18, 'stride preserves the original timing at fox walking speed')
  assert.deepEqual(sheet.directions, ['NE', 'N', 'E', 'SE', 'NW', 'W', 'S', 'SW'])
  const png = readFileSync(resolve('public/assets/game', sheet.img))
  assert.equal(png.subarray(0, 8).toString('hex'), '89504e470d0a1a0a')
  assert.deepEqual([png.readUInt32BE(16), png.readUInt32BE(20)], [168, 272])
  for (let direction = 0; direction < 8; direction++) {
    assert.ok(ResourceLoader.getResourceDef(`fox/${direction}`))
    assert.ok(ResourceLoader.getResourceDef(`fox/idle/${direction}`))
    assert.equal(ResourceLoader.getResourceDef(`fox/walk/${direction}`)!.layers[0]!.frames!.length, 4)
  }
})

test('stationary spawn and heading changes select all eight authored rows from one shared load', async t => {
  const { spawn, loaded, loading, atlas } = fixture(t)
  const views = MOVEMENT_DIRECTIONS.map((_, direction) => spawn(direction + 1, direction))
  for (const view of views) {
    assert.equal(view.hasAnimatedFrames(), true, 'animation registration must precede async texture loading')
    const bounds = view.computeScreenBounds()
    assert.ok(bounds.minX <= -19 && bounds.minY <= -22 && bounds.maxX >= 23 && bounds.maxY >= 12)
  }
  await loaded()
  assert.equal(loading.mock.callCount(), 1)
  assert.deepEqual(loading.mock.calls[0]!.arguments, ['/assets/game/animals/fox/fox.png'])
  assert.equal(atlas.source.scaleMode, 'nearest')
  for (const [direction, view] of views.entries()) {
    assertFrame(view, direction, 0)
    assert.equal(sprite(view).eventMode, 'static')
    assert.equal(sprite(view).roundPixels, true)
    assert.equal(sprite(view).texture.source, atlas.source)
    view.setHeading(heading((direction + 4) % 8))
    assertFrame(view, (direction + 4) % 8, 0)
  }
})

test('walking follows distance at different frame rates, turns retain phase and stopping selects idle', async t => {
  const { spawn, loaded } = fixture(t)
  const views = [30, 60, 144].map((fps) => ({ fps, view: spawn(fps, 1) }))
  await loaded()
  for (const { fps, view } of views) {
    for (let frame = 0; frame < fps; frame++) view.onMoved(5, stride * .75 / fps)
    assertFrame(view, 1, 3)
    view.updateAnimation(1e9)
    assertFrame(view, 1, 3)
    view.setHeading(heading(7))
    assertFrame(view, 7, 3)
    view.onMoved(5, stride * .25)
    assertFrame(view, 7, 0)
    view.onStopped()
    assertFrame(view, 7, 0)
    view.updateAnimation(2e9)
    assertFrame(view, 7, 0)
    view.onMoved(5, stride * .25)
    assertFrame(view, 7, 1)
  }
})

test('async loading uses the latest movement state and AOI reentry cannot resurrect a despawned sprite', async t => {
  const { manager, spawn, loaded, loading } = fixture(t)
  const walking = spawn(1, 0)
  const stopped = spawn(2, 0)
  const removed = spawn(3, 0)
  manager.updateObjectPosition(1, 2, 4, true, 0, stride * .5, undefined, 1, heading(6))
  manager.updateObjectPosition(2, 2, 4, true, 0, stride * .5, undefined, 1, heading(3))
  manager.updateObjectPosition(2, 2, 4, false, 0, 0)
  manager.despawnObject(3)
  const replacement = spawn(3, 5)
  await loaded()
  assertFrame(walking, 6, 2)
  assertFrame(stopped, 3, 0)
  assert.equal(removed.getContainer().destroyed, true)
  assertFrame(replacement, 5, 0)
  const sharedTexture = sprite(replacement).texture
  manager.despawnObject(3)
  const reentered = spawn(3, 5)
  await loaded()
  assertFrame(reentered, 5, 0)
  assert.equal(sprite(reentered).texture, sharedTexture, 'despawn must retain shared frames for later observers')
  assert.equal(loading.mock.callCount(), 1)
})
