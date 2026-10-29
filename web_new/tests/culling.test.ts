import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, test } from 'node:test'
import { Application, Container, type Sprite } from 'pixi.js'
import { ObjectManager } from '../src/game/ObjectManager'
import { ObjectView } from '../src/game/ObjectView'
import { ResourceLoader } from '../src/game/ResourceLoader'
import trees from '../src/game/objects/trees.json'
import { cullingController, ViewportCullingController } from '../src/game/culling'
import { setWorldParams } from './serverConstantsFixture'

beforeEach(() => setWorldParams(12, 128))

const app = { screen: { width: 800, height: 600 } } as Application

for (const [edge, bounds, outside, inside] of [
  ['bottom', { minX: 300, minY: 700, maxX: 400, maxY: 710 }, [0, 100], [0, -40]],
  ['top', { minX: 300, minY: -100, maxX: 400, maxY: -90 }, [0, -100], [0, 40]],
  ['left', { minX: -200, minY: 200, maxX: -180, maxY: 300 }, [-100, 0], [60, 0]],
  ['right', { minX: 1000, minY: 200, maxX: 1020, maxY: 300 }, [100, 0], [-80, 0]],
] as const) {
  test(`culling retains visibility history at the ${edge} edge without flickering`, () => {
    const controller = new ViewportCullingController()
    const world = new Container()
    const subchunk = world.addChild(new Container())
    const terrain = world.addChild(new Container())
    const object = world.addChild(new Container({ eventMode: 'static' }))
    controller.registerSubchunk('chunk', subchunk, bounds)
    controller.registerTerrain('terrain', terrain, bounds)
    controller.registerObject(1, object, bounds)

    const checkFrames = (visible: boolean) => {
      for (let frame = 0; frame < 8; frame++) {
        controller.update(app, world, world)
        for (const container of [subchunk, terrain, object]) assert.equal(container.visible, visible)
        assert.equal(object.eventMode, visible ? 'static' : 'none')
      }
    }

    checkFrames(true)
    world.position.set(...outside)
    checkFrames(false)
    world.position.set(0, 0)
    checkFrames(false)
    world.position.set(...inside)
    checkFrames(true)
    world.position.set(0, 0)
    checkFrames(true)
    world.destroy({ children: true })
  })
}

test('changing the culling margin preserves hysteresis and disabling it uses one threshold', () => {
  const controller = new ViewportCullingController()
  controller.setMarginTiles(6)
  const world = new Container()
  const object = world.addChild(new Container())
  controller.registerObject(1, object, { minX: 300, minY: 760, maxX: 400, maxY: 770 })
  for (let frame = 0; frame < 8; frame++) {
    controller.update(app, world, world)
    assert.equal(object.visible, true)
  }
  world.y = 100
  controller.update(app, world, world)
  assert.equal(object.visible, false)
  world.y = 0
  controller.update(app, world, world)
  assert.equal(object.visible, false)
  controller.setHysteresis(false)
  controller.update(app, world, world)
  assert.equal(object.visible, true)
  world.y = 100
  controller.update(app, world, world)
  assert.equal(object.visible, false)
  world.destroy({ children: true })
})

test('all tree growth stages include their published images in bounds before textures load', context => {
  context.mock.method(ResourceLoader, 'createSprite', () => new Promise<Sprite>(() => {}))
  for (const [species, stages] of Object.entries(trees.trees)) {
    for (const stage of Object.keys(stages)) {
      if (!/^\d+$/.test(stage)) continue
      const resourcePath = `trees/${species}/${stage}`
      const resource = ResourceLoader.getResourceDef(resourcePath)!
      const view = new ObjectView({ entityId: 1, typeId: 5, resourcePath, position: { x: 0, y: 0 }, size: { x: 10, y: 10 } })
      assert.equal(view.getContainer().children.length, 0, 'textures have not loaded')
      const bounds = view.computeScreenBounds()
      for (const layer of resource.layers) {
        const bytes = readFileSync(resolve('public/assets/game', layer.img!))
        const width = bytes.readUInt32BE(16)
        const height = bytes.readUInt32BE(20)
        const position = ResourceLoader.resolveLayerPosition(layer, resource)
        assert.ok(bounds.minX <= position.x && bounds.minY <= position.y, `${resourcePath}: image origin`)
        assert.ok(bounds.maxX >= position.x + width && bounds.maxY >= position.y + height, `${resourcePath}: image extent`)
      }
      view.destroy()
    }
  }
})

for (const zoom of [0.5, 1, 2]) {
  test(`a pine crown stays visible with its base below the viewport at zoom ${zoom}`, context => {
    context.mock.method(ResourceLoader, 'createSprite', () => new Promise<Sprite>(() => {}))
    cullingController.clear()
    const world = new Container()
    world.scale.set(zoom)
    const manager = new ObjectManager()
    manager.setParentContainer(world)
    context.after(() => {
      manager.clear()
      cullingController.clear()
      world.destroy({ children: true })
    })
    manager.spawnObject({ entityId: 1, typeId: 5, resourcePath: 'trees/pine/6', position: { x: 0, y: 0 }, size: { x: 10, y: 10 } })
    // Pine's body starts 279 local pixels above its ground anchor.
    world.position.set(400, 600 + 270 * zoom)
    for (let frame = 0; frame < 8; frame++) {
      cullingController.update(app, world, world)
      assert.equal(manager.getObject(1)!.getContainer().visible, true)
    }
    world.y += 300 * zoom
    cullingController.update(app, world, world)
    assert.equal(manager.getObject(1)!.getContainer().visible, false)
    world.y -= 300 * zoom
    cullingController.update(app, world, world)
    assert.equal(manager.getObject(1)!.getContainer().visible, true)
  })
}
