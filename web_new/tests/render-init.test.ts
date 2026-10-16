import assert from 'node:assert/strict'
import { after, beforeEach, test } from 'node:test'
import { Application } from 'pixi.js'
import { Render } from '../src/game/Render'
import { GameFacade } from '../src/game/GameFacade'
import { ActorRenderer } from '../src/game/actors/ActorRenderer'
import { chunkCache } from '../src/game/cache/ChunkCache'

const message = 'Unable to initialize WebGL 2. Enable hardware acceleration or try another browser.'
const canvas = { style: {} } as HTMLCanvasElement
const previousGlobals = ['window', 'document'].map(name => [name, Object.getOwnPropertyDescriptor(globalThis, name)] as const)

beforeEach(() => {
  for (const [name, value] of Object.entries({
    window: Object.assign(new EventTarget(), { devicePixelRatio: 1 }),
    document: new EventTarget(),
  })) {
    Object.defineProperty(globalThis, name, { configurable: true, value })
  }
})
after(() => {
  chunkCache.destroy()
  for (const [name, previous] of previousGlobals) {
    if (previous) Object.defineProperty(globalThis, name, previous)
    else Reflect.deleteProperty(globalThis, name)
  }
})

test('unavailable WebGL rejects clearly without falling back, and partial teardown releases containers', async () => {
  const render = new Render()
  const stage = render.getApp().stage
  await assert.rejects(render.init(canvas), error => {
    assert.ok(error instanceof Error)
    assert.equal(error.message, message)
    assert.ok(error.cause instanceof Error)
    assert.equal(error.cause.message, 'No available renderer for the current environment')
    return true
  })
  assert.equal(render.getApp().renderer, undefined)
  assert.doesNotThrow(() => render.destroy())
  assert.equal(stage.destroyed, true)
  const containers = render as unknown as Record<string, { destroyed: boolean }>
  for (const key of ['mapContainer', 'objectsContainer', 'worldHudContainer', 'uiContainer']) {
    assert.equal(containers[key]?.destroyed, true, key)
  }
})

test('Pixi initialization uses only WebGL and preserves the original failure as its cause', async context => {
  const original = new Error('Unable to create a graphics context')
  context.mock.method(Application.prototype, 'init', async (options: { preference: string[] }) => {
    assert.deepEqual(options.preference, ['webgl'])
    throw original
  })
  const render = new Render()
  await assert.rejects(render.init(canvas), error => {
    assert.ok(error instanceof Error)
    assert.equal(error.message, message)
    assert.equal(error.cause, original)
    return true
  })
  assert.doesNotThrow(() => render.destroy())
})

for (const version of [1, undefined]) {
  test(`WebGL version ${version} is rejected before constructing the actor renderer`, async context => {
    let actorCreated = false
    context.mock.method(ActorRenderer.prototype as unknown as { resetPixiState(): void }, 'resetPixiState', () => {
      actorCreated = true
      throw new Error('Unexpected actor construction')
    })
    context.mock.method(Application.prototype, 'init', async function(this: Application) {
      this.renderer = { context: version ? { webGLVersion: version } : undefined, off() {} } as unknown as Application['renderer']
    })
    const destroy = context.mock.method(Application.prototype, 'destroy', function(this: Application) {
      this.stage.destroy({ children: true })
    })
    const render = new Render()
    await assert.rejects(render.init(canvas), { message })
    assert.equal(actorCreated, false)
    assert.doesNotThrow(() => render.destroy())
    assert.equal(destroy.mock.callCount(), 1)
  })
}

test('errors after WebGL 2 initialization retain their own diagnostic', async context => {
  const original = new Error('Character resources failed to initialize')
  context.mock.method(Application.prototype, 'init', async function(this: Application) {
    this.renderer = { context: { webGLVersion: 2 }, off() {} } as unknown as Application['renderer']
  })
  context.mock.method(Application.prototype, 'destroy', function(this: Application) {
    this.stage.destroy({ children: true })
  })
  context.mock.method(ActorRenderer.prototype as unknown as { resetPixiState(): void }, 'resetPixiState', () => {
    throw original
  })
  const render = new Render()
  await assert.rejects(render.init(canvas), error => error === original)
  render.destroy()
})

test('facade tears down a failed initialization and can start again with a fresh renderer', async context => {
  const original = new Error('Graphics unavailable')
  context.mock.method(Application.prototype, 'init', async () => { throw original })
  const facade = new GameFacade()
  await assert.rejects(facade.init(canvas), { message })
  assert.equal(facade.isInitialized(), false)
  const failedApp = (facade as unknown as { render: Render }).render.getApp()

  context.mock.method(Render.prototype, 'init', async () => {})
  context.mock.method(Render.prototype, 'getApp', () => ({
    renderer: { events: { cursorStyles: {}, rootBoundary: { cursor: 'default' } } },
  }))
  await facade.init(canvas)
  assert.equal(failedApp.stage.destroyed, true)
  assert.equal(facade.isInitialized(), true)
  assert.doesNotThrow(() => facade.destroy())
  assert.equal(facade.isInitialized(), false)
  assert.doesNotThrow(() => facade.destroy())
})
