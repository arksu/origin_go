import assert from 'node:assert/strict'
import { after, test } from 'node:test'
import { Container, Sprite, Texture, Ticker } from 'pixi.js'
import { createPinia, setActivePinia } from 'pinia'
import { fxManager } from '../src/game/fx/FxManager'
import { SKULL_SMOKE } from '../src/game/fx/assets/skullSmoke'
import { ResourceLoader } from '../src/game/ResourceLoader'
import { Render } from '../src/game/Render'
import { gameFacade } from '../src/game/GameFacade'
import { soundManager } from '../src/game'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { proto } from '../src/network/proto/packets.js'
import { useGameStore } from '../src/stores/gameStore'
import { setWorldParams } from './serverConstantsFixture'
import { chunkCache } from '../src/game/cache/ChunkCache'

after(() => chunkCache.destroy())

const textures = SKULL_SMOKE.frames.map(() => new Texture({ source: Texture.WHITE.source }))
const tick = () => (fxManager as unknown as { update(ticker: Ticker): void }).update({ deltaMS: 16 } as Ticker)
const play = (container: Container, fxKey = 'skull_smoke') => fxManager.playWorldFx({ container, fxKey, x: 100, y: 200 })

test('loading is shared and never queues old events or creates sprites after reset/destroy', async t => {
  let resolve!: (texture: Texture) => void
  const loading = new Promise<Texture>(done => { resolve = done })
  const loader = t.mock.method(ResourceLoader, 'loadTexture', (path: string) =>
    loading.then(() => textures[SKULL_SMOKE.frames.indexOf(path as typeof SKULL_SMOKE.frames[number])]!))
  const parent = new Container()
  const destroyed = new Container()
  const pending = fxManager.preloadWorldFx()
  assert.equal(fxManager.preloadWorldFx(), pending)
  assert.equal(loader.mock.callCount(), 8)
  play(parent)
  play(destroyed)
  fxManager.clearWorldFx(parent)
  destroyed.destroy()
  resolve(textures[0]!)
  await pending
  assert.equal(parent.children.length, 0)
  tick()
  assert.equal(parent.children.length, 0, 'late asset readiness must not replay a dropped event')
  play(destroyed)
  assert.equal(destroyed.destroyed, true)
  parent.destroy()
})

test('one detached sprite follows the eight authored frames and expires by monotonic time', async t => {
  await fxManager.preloadWorldFx()
  let now = 1000
  t.mock.method(performance, 'now', () => now)
  const parent = new Container()
  const objectView = new Container()
  parent.addChild(objectView)
  t.after(() => { fxManager.clearWorldFx(parent); parent.destroy({ children: true }) })
  play(parent)
  const smoke = parent.children[1] as Sprite
  assert.equal(smoke.texture, textures[0])
  assert.equal(smoke.x, 100)
  assert.equal(smoke.y, 192)
  assert.equal(smoke.anchor.x, .5)
  assert.equal(smoke.anchor.y, 120 / 128)
  assert.equal(smoke.scale.x, 1)
  objectView.destroy()
  for (let frame = 0; frame < 8; frame++) {
    now = 1000 + frame * 250
    tick()
    assert.equal(smoke.texture, textures[frame])
    assert.equal(smoke.destroyed, false)
  }
  now = 2999
  tick()
  assert.equal(smoke.destroyed, false)
  now = 3000
  tick()
  assert.equal(smoke.destroyed, true)
  assert.equal(parent.children.length, 0)
  play(parent)
  now += 60_000
  tick()
  assert.equal(parent.children.length, 0, 'a suspended ticker must not extend playback')
})

test('capacity drops cosmetic work and owner-specific cleanup releases all slots', t => {
  const first = new Container()
  const second = new Container()
  t.after(() => {
    fxManager.clearWorldFx(first)
    fxManager.clearWorldFx(second)
    first.destroy()
    second.destroy()
  })
  play(first, 'unknown')
  assert.equal(first.children.length, 0)
  for (let index = 0; index < 63; index++) play(first)
  play(second)
  play(second)
  assert.equal(first.children.length, 63)
  assert.equal(second.children.length, 1)
  const sprites = [...first.children]
  fxManager.clearWorldFx(first)
  fxManager.clearWorldFx(first)
  assert.ok(sprites.every(sprite => sprite.destroyed))
  assert.equal(second.children.length, 1, 'another renderer owns its own effects')
  play(first)
  assert.equal(first.children.length, 1, 'cleanup frees capacity immediately')
  second.destroy({ children: true })
  tick()
})

test('Render converts event coordinates without requiring an object, and reset/destroy clear playback', t => {
  setWorldParams(12, 128)
  const globals = ['window', 'document'].map(name => [name, Object.getOwnPropertyDescriptor(globalThis, name)] as const)
  for (const name of ['window', 'document']) Object.defineProperty(globalThis, name, { configurable: true, value: new EventTarget() })
  const render = new Render()
  t.after(() => {
    if (!render.getApp().stage.destroyed) render.destroy()
    for (const [name, descriptor] of globals) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor)
      else Reflect.deleteProperty(globalThis, name)
    }
  })
  const internals = render as unknown as { sceneInitialized: boolean; objectsContainer: Container }
  internals.sceneInitialized = true
  const initialChildren = internals.objectsContainer.children.length
  const calls = t.mock.method(fxManager, 'playWorldFx', () => {})
  render.playWorldFx('skull_smoke', 12, 24)
  assert.deepEqual(calls.mock.calls[0]?.arguments, [{ container: internals.objectsContainer, fxKey: 'skull_smoke', x: -32, y: 48 }])
  calls.mock.restore()
  render.playWorldFx('skull_smoke', 12, 24)
  assert.equal(internals.objectsContainer.children.length, initialChildren + 1)
  const resetSprite = internals.objectsContainer.children.at(-1)!
  render.resetWorld()
  assert.equal(resetSprite.destroyed, true)
  assert.equal(internals.objectsContainer.children.length, initialChildren)
  render.playWorldFx('skull_smoke', 12, 24)
  const sprite = internals.objectsContainer.children.at(-1)!
  render.destroy()
  assert.equal(sprite.destroyed, true)
})

test('world events validate epoch/position, preserve legacy discovery and never infer FX from snapshots', t => {
  setActivePinia(createPinia())
  setWorldParams(12, 128)
  const game = useGameStore()
  game.setConnectionState('connected')
  game.setPlayerEnterWorld(1, 'Observer', 7)
  t.mock.method(soundManager, 'initialize', async () => ({ manifests: {}, equipment: {}, actionAnimations: {} }))
  t.mock.method(gameFacade, 'resetWorld', () => {})
  t.mock.method(gameFacade, 'spawnObject', () => {})
  t.mock.method(gameFacade, 'despawnObject', () => {})
  const world = t.mock.method(gameFacade, 'playWorldFx', () => {})
  const legacy = t.mock.method(gameFacade, 'playFx', () => {})
  const feedback = t.mock.method(soundManager, 'playFeedback', () => {})
  registerMessageHandlers()
  const dispatch = (fx: proto.IS2C_Fx) => messageDispatcher.dispatch(proto.ServerMessage.create({ fx }))
  const fx = { fxKey: 'skull_smoke', position: { x: 321, y: -45 }, streamEpoch: 7 }
  const encoded = proto.ServerMessage.encode({ fx }).finish()
  messageDispatcher.dispatch(proto.ServerMessage.decode(encoded))
  assert.deepEqual(world.mock.calls[0]?.arguments, ['skull_smoke', 321, -45])
  assert.equal(legacy.mock.callCount(), 0)
  assert.equal(feedback.mock.callCount(), 0)
  for (const invalid of [
    { ...fx, streamEpoch: 0 }, { ...fx, streamEpoch: 6 }, { ...fx, streamEpoch: 8 },
    { ...fx, position: null }, { ...fx, position: { x: NaN, y: 0 } },
    { ...fx, position: { x: 0, y: Infinity } }, { ...fx, fxKey: '' },
  ]) dispatch(invalid)
  assert.equal(world.mock.callCount(), 1)
  dispatch({ fxKey: 'exp_gain' })
  assert.deepEqual(legacy.mock.calls[0]?.arguments, [1, 'exp_gain'])
  assert.deepEqual(feedback.mock.calls[0]?.arguments, ['exp_gain', 1])
  for (const reset of [() => game.setPlayerEnterWorld(1, 'Observer', 8),
    () => game.setPlayerLeaveWorld(), () => game.setConnectionState('disconnected')]) {
    reset()
    dispatch(fx)
  }
  assert.equal(world.mock.callCount(), 1)
  game.setConnectionState('connected')
  game.setPlayerEnterWorld(1, 'Observer', 7)
  messageDispatcher.dispatch(proto.ServerMessage.create({ objectSpawn: {
    entityId: 20, typeId: 16, resourcePath: 'campfire/unlit', position: { position: { x: 321, y: -45 } }, streamEpoch: 7,
  } }))
  messageDispatcher.dispatch(proto.ServerMessage.create({ objectDespawn: { entityId: 20, streamEpoch: 7 } }))
  assert.equal(world.mock.callCount(), 1, 'loading or removing a campfire does not synthesize a conversion event')
  game.reset()
})
