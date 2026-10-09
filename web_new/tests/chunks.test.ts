import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { Assets, DOMAdapter, Mesh, Rectangle, Texture, TextureSource, type Spritesheet } from 'pixi.js'
import { VertexBuffer } from '../src/game/utils/VertexBuffer'
import { Chunk } from '../src/game/Chunk'
import { ChunkManager } from '../src/game/ChunkManager'
import { terrainManager } from '../src/game/terrain'
import { chunkCache, buildQueue } from '../src/game/cache'
import { CACHE_MAX_ENTRIES, CACHE_TTL_MS } from '../src/constants/cache'
import { ChunkStreamGuard } from '../src/network/ChunkStreamGuard'
import { proto } from '../src/network/proto/packets.js'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { GameFacade, gameFacade } from '../src/game/GameFacade'
import { Render } from '../src/game/Render'
import { moveController } from '../src/game/MoveController'
import { cameraController } from '../src/game/CameraController'
import type { MinimapFrame } from '../src/game/minimap/types'
import { useGameStore } from '../src/stores/gameStore'

// Use real Pixi meshes, geometry and buffers; only the browser capability probe
// and texture loading are replaced. No WebGL context is needed for lifecycle tests.
DOMAdapter.set({ ...DOMAdapter.get(), createCanvas: () => ({ getContext: () => null }) as unknown as HTMLCanvasElement })
const sheet = { textures: new Proxy({}, { get: () => Texture.EMPTY }), textureSource: Texture.EMPTY.source } as Spritesheet
const tiles = () => new Uint8Array(16).fill(35)
const identity = (seq: number, epoch = 1) => ({ streamEpoch: epoch, eventSeq: BigInt(seq) })

test('atlas trimming preserves the full tile scale and rotated UV corners', () => {
  const source = new TextureSource({ width: 2048, height: 2048 })
  for (const rotate of [0, 2]) {
    const texture = new Texture({
      source,
      frame: new Rectangle(100, 200, rotate ? 20 : 18, rotate ? 18 : 20),
      orig: new Rectangle(0, 0, 63, 32),
      trim: new Rectangle(22, 4, 18, 20),
      rotate,
    })
    const buffer = new VertexBuffer(1)
    buffer.addVertex(10, 20, 64, 48, texture)
    buffer.finish()

    // Cropping an atlas frame must leave its pixels at the same positions
    // they had when the original 63x32 canvas was drawn as a 64x48 tile.
    const left = 10 + 22 * 64 / 63
    const right = 10 + 40 * 64 / 63
    assert.deepEqual(Array.from(buffer.vertex), Array.from(new Float32Array([
      left, 26, right, 26, right, 56, left, 56,
    ])))
    const uv = texture.uvs
    assert.deepEqual(Array.from(buffer.uv), Array.from(new Float32Array([
      uv.x0, uv.y0, uv.x1, uv.y1, uv.x2, uv.y2, uv.x3, uv.y3,
    ])))
    texture.destroy()
  }
  source.destroy()
})

async function managerFixture(t: TestContext) {
  t.mock.method(Assets, 'load', async () => sheet)
  t.mock.method(terrainManager, 'generateTerrainForChunk', () => {})
  const manager = new ChunkManager()
  manager.setWorldParams(12, 4)
  t.after(() => manager.destroy())
  await manager.init()
  return manager
}

function geometry(chunk: Chunk) {
  const mesh = chunk.getSubchunkDataList()[0]!.container.children[0] as Mesh
  assert.ok(mesh instanceof Mesh)
  return mesh.geometry
}

function flushBorders(manager: ChunkManager) {
  ;(manager as unknown as { processBorderRefreshQueue(): void }).processBorderRefreshQueue()
  while (buildQueue.hasPendingTasks()) manager.update()
}

test('world tile lookup uses active payloads across negative coordinates, reload and cached unload', async (t) => {
  const manager = await managerFixture(t)
  const first = tiles()
  first[15] = 3
  manager.loadChunk(-1, -1, first, 1, identity(1))
  assert.equal(manager.getTileTypeAtWorld(-0.01, -0.01), 3)
  assert.equal(manager.getTileTypeAtWorld(-12, -12), 3)
  assert.equal(manager.getTileTypeAtWorld(-48, -48), 35)
  assert.equal(manager.getTileTypeAtWorld(0, 0), undefined)
  assert.equal(manager.getTileTypeAtWorld(NaN, 0), undefined)
  manager.unloadChunk(-1, -1, identity(2))
  assert.ok(manager.getChunk(-1, -1), 'the graphical cache retains this chunk')
  assert.equal(manager.getTileTypeAtWorld(-1, -1), undefined, 'cached water must not outlive its active payload')
  const replacement = tiles()
  replacement[15] = 1
  manager.loadChunk(-1, -1, replacement, 2, identity(3))
  assert.equal(manager.getTileTypeAtWorld(-1, -1), 1, 'replacement tile is visible immediately')
  manager.clear()
  assert.equal(manager.getTileTypeAtWorld(-1, -1), undefined)
})

test('minimap borrows active tiles before graphics are ready and prefers them over retained tiles', async t => {
  const manager = await managerFixture(t)
  const original = tiles()
  manager.loadChunk(-1, -1, original, 1, identity(1))
  manager.unloadChunk(-1, -1, identity(2))
  assert.equal(manager.getMinimapChunk(-1, -1)!.tiles, original)
  assert.equal(manager.getTileTypeAtWorld(-1, -1), undefined, 'world lookup stays active-only')

  // Delay graphics readiness while a new accepted payload replaces a retained one.
  ;(manager as unknown as { spritesheet: Spritesheet | null }).spritesheet = null
  const replacement = tiles()
  replacement[15] = 14
  manager.loadChunk(-1, -1, replacement, 2, identity(3))
  const borrowed = manager.getMinimapChunk(-1, -1)!
  assert.deepEqual(Object.keys(borrowed).sort(), ['tiles', 'version', 'x', 'y'])
  assert.equal(borrowed.tiles, replacement, 'tile bytes are borrowed without a clone')
  assert.equal(borrowed.version, 2)
  assert.equal(chunkCache.peek('-1,-1')!.tiles, original, 'unfinished graphics still have their older cache entry')
  assert.equal(manager.getTileTypeAtWorld(-1, -1), 14)

  manager.loadChunk(1, 0, replacement, 3, identity(4))
  assert.equal(manager.getMinimapChunk(1, 0)!.tiles, replacement, 'pending active chunks need no graphics')
  manager.unloadChunk(1, 0, identity(5))
  assert.equal(manager.getMinimapChunk(1, 0), undefined, 'unbuilt unloaded chunks have no retained source')
  assert.equal(manager.getMinimapChunk(NaN, 0), undefined)
  assert.equal(manager.getMinimapChunk(0.5, 0), undefined)
  manager.clear()
  assert.equal(manager.getMinimapChunk(-1, -1), undefined)
})

test('wire sequence ordering is lossless, per coordinate, and survives unload', () => {
  const guard = new ChunkStreamGuard()
  guard.reset(1)
  const acceptLoad = (seq: string, version = 7, x = 0, epoch = 1) => {
    const wire = proto.S2C_ChunkLoad.fromObject({ streamEpoch: epoch, eventSeq: seq, chunk: { version } })
    const decoded = proto.S2C_ChunkLoad.decode(proto.S2C_ChunkLoad.encode(wire).finish())
    return guard.accept(x, 0, decoded.streamEpoch, decoded.eventSeq, decoded.chunk!.version!)
  }
  assert.equal(acceptLoad('0'), null)
  assert.ok(guard.accept(0, 0, 1, 2)) // unload arrives before the old load
  assert.equal(acceptLoad('1'), null)
  assert.ok(acceptLoad('4')) // equal-version reload arrives before unload 3
  assert.equal(guard.accept(0, 0, 1, 3), null)
  assert.equal(acceptLoad('4'), null)
  assert.ok(acceptLoad('1', 7, 1), 'unrelated coordinates have independent guards')
  assert.ok(acceptLoad('5', 8))
  assert.equal(acceptLoad('6', 7), null, 'new sequence cannot regress tile history')
  assert.ok(guard.accept(0, 0, 1, 7))
  assert.equal(acceptLoad('8', 7), null, 'unload must retain version history')
  assert.ok(acceptLoad('9', 8), 'equal version after eviction is accepted')
  assert.ok(acceptLoad('9007199254740992', 8))
  assert.ok(acceptLoad('9007199254740993', 8))
  assert.equal(acceptLoad('9007199254740992', 8), null)
  assert.ok(acceptLoad('18446744073709551615', 8))
  assert.equal(guard.accept(0, 0, 1, Number.MAX_SAFE_INTEGER + 1, 8), null)
  guard.reset(2)
  assert.equal(acceptLoad('18446744073709551615', 9, 0, 1), null)
  assert.ok(acceptLoad('1', 0, 0, 2), 'new server stream can start with version zero')
})

test('handlers gate store, renderer and bootstrap together', t => {
  setActivePinia(createPinia())
  const store = useGameStore()
  const loads = t.mock.method(gameFacade, 'loadChunk', () => {})
  const unloads = t.mock.method(gameFacade, 'unloadChunk', () => {})
  const resets = t.mock.method(gameFacade, 'resetWorld', () => {})
  const bootstraps = t.mock.method(store, 'markBootstrapFirstChunkLoaded', () => {})
  registerMessageHandlers()
  const dispatch = (packet: proto.IServerMessage) => messageDispatcher.dispatch(proto.ServerMessage.create(packet))
  const enter = (streamEpoch: number) => dispatch({ playerEnterWorld: { entityId: 1, streamEpoch, coordPerTile: 12, chunkSize: 4 } })
  const load = (eventSeq: number, version: number, streamEpoch = 1) => dispatch({ chunkLoad: { streamEpoch, eventSeq, chunk: { coord: { x: 0, y: 0 }, tiles: tiles(), version } } })
  enter(1)
  assert.equal(resets.mock.callCount(), 1, 'even the first entry unconditionally resets presentation')
  dispatch({ chunkUnload: { streamEpoch: 1, eventSeq: 2, coord: { x: 0, y: 0 } } })
  load(1, 1)
  assert.equal(store.chunks.size, 0)
  assert.equal(loads.mock.callCount(), 0)
  assert.equal(bootstraps.mock.callCount(), 0)
  load(4, 3)
  load(3, 2)
  load(5, 2)
  dispatch({ chunkUnload: { streamEpoch: 1, eventSeq: 3, coord: { x: 0, y: 0 } } })
  assert.equal(loads.mock.callCount(), 1)
  assert.equal(unloads.mock.callCount(), 1)
  assert.equal(store.chunks.get('0,0')!.version, 3)
  assert.equal(bootstraps.mock.callCount(), 1)
  enter(2)
  assert.equal(resets.mock.callCount(), 2)
  assert.equal(store.chunks.size, 0)
  load(6, 4, 1)
  assert.equal(loads.mock.callCount(), 1)
  load(1, 0, 2)
  assert.equal(loads.mock.callCount(), 2)
  assert.equal(store.chunks.get('0,0')!.version, 0)
})

test('minimap reads only accepted terrain and rejected packets cannot revive evicted terrain', async t => {
  const manager = await managerFixture(t)
  setActivePinia(createPinia())
  t.mock.method(gameFacade, 'loadChunk', manager.loadChunk.bind(manager))
  t.mock.method(gameFacade, 'unloadChunk', manager.unloadChunk.bind(manager))
  t.mock.method(gameFacade, 'resetWorld', manager.clear.bind(manager))
  registerMessageHandlers()
  const dispatch = (packet: proto.IServerMessage) => messageDispatcher.dispatch(proto.ServerMessage.create(packet))
  dispatch({ playerEnterWorld: { entityId: 1, streamEpoch: 1, coordPerTile: 12, chunkSize: 4 } })
  const load = (eventSeq: number, version: number, tileType: number, streamEpoch = 1) => dispatch({
    chunkLoad: { streamEpoch, eventSeq, chunk: { coord: { x: 0, y: 0 }, tiles: new Uint8Array(16).fill(tileType), version } },
  })
  load(1, 1, 35)
  load(2, 2, 14)
  assert.equal(manager.getMinimapChunk(0, 0)!.tiles[0], 14, 'accepted plowed terrain replaces the previous payload')
  load(1, 1, 35)
  load(2, 2, 35)
  load(3, 1, 35)
  load(4, 3, 35, 2)
  assert.equal(manager.getMinimapChunk(0, 0)!.tiles[0], 14, 'old events, duplicates, lower versions and other streams are rejected')
  dispatch({ chunkUnload: { streamEpoch: 1, eventSeq: 5, coord: { x: 0, y: 0 } } })
  assert.equal(manager.getMinimapChunk(0, 0)!.tiles[0], 14, 'normal unload keeps only the existing retained source')
  chunkCache.evict('0,0')
  load(4, 2, 14)
  load(5, 2, 14)
  load(6, 1, 35)
  assert.equal(manager.getMinimapChunk(0, 0), undefined)
  load(7, 2, 14)
  assert.equal(manager.getMinimapChunk(0, 0)!.tiles[0], 14, 'a newly accepted reload becomes available')
})

// Exercise the real bridge and loop with only unrelated graphics/UI work stubbed.
// The chunk manager, movement controller, handlers and lifecycle methods stay real.
function renderBridgeFixture(t: TestContext, manager: ChunkManager) {
  const noop = () => {}
  const minimap = { render: (_frame: MinimapFrame) => {}, clear: noop, destroy: noop, setZoom: (_zoom: number) => {} }
  const objectManager = {
    updateObjectPosition: noop, setPlayerEntityId: noop, update: noop,
    syncActiveCarryVisuals: noop, clear: noop, spawnObject: noop,
    getObject: () => undefined, setKnockedOutPose: noop, setCarryVisualRelation: noop,
  }
  const render: Render = Object.assign(Object.create(Render.prototype), {
    chunkManager: manager, objectManager, minimapRenderer: minimap, minimapCanvas: null,
    playerEntityId: null, actorRenderer: null,
    combatSectorPreview: { clear: noop },
    keyboardMovement: { reset: noop }, inputController: { setKeyboardMovementEnabled: noop },
    buildGhostController: { cancel: noop }, liftGhostController: { cancel: noop },
    nicknameManager: { clear: noop, update: noop }, chatBalloonManager: { clear: noop, update: noop },
    damageNumberManager: { clear: noop, update: noop, forgetSpawn: noop, rememberDespawn: noop },
  })
  const frameLoop = render as unknown as { update(): void }
  for (const method of ['updateCamera', 'updateBuildGhost', 'updateLiftGhost', 'updateCombatSector', 'updateChunkBuilds', 'updateCulling', 'updateHoverHighlight', 'updateDebugOverlay'] as const) {
    t.mock.method(render as unknown as Record<typeof method, () => void>, method, noop)
  }
  moveController.reset()
  t.after(() => { moveController.reset(); cameraController.reset() })
  return { render, frameLoop, minimap, objectManager }
}

test('minimap uses initialized and smoothed player pose once per frame independently of camera', async t => {
  const manager = await managerFixture(t)
  const { render, frameLoop, minimap } = renderBridgeFixture(t, manager)
  const facade = new GameFacade()
  ;(facade as unknown as { render: Render }).render = render
  assert.equal(facade.getMinimapPlayerPose(), null)
  render.setPlayerEntityId(42)
  assert.equal(facade.getMinimapPlayerPose(), null, 'entering a world does not invent an origin marker')
  moveController.initEntity(42, 1234, -5678, Math.PI / 2)
  assert.deepEqual(facade.getMinimapPlayerPose(), { x: 1234, y: -5678, heading: Math.PI / 2 })
  assert.equal(moveController.getRenderPosition(42), null, 'spawn pose is valid before the first interpolation frame')

  const movementUpdates = t.mock.method(moveController, 'update')
  const mapFrames = t.mock.method(minimap, 'render')
  moveController.onObjectMove(42, Date.now(), 1, false, 1300, -5670, 64, 8, true, 0, Math.PI / 4)
  frameLoop.update()
  assert.equal(movementUpdates.mock.callCount(), 1, 'the minimap must not advance movement a second time')
  const smoothed = moveController.getRenderPosition(42)!
  assert.deepEqual(mapFrames.mock.calls[0]!.arguments[0].player, { x: smoothed.x, y: smoothed.y, heading: smoothed.heading })
  assert.equal(mapFrames.mock.calls[0]!.arguments[0].coordPerTile, 12)
  assert.equal(mapFrames.mock.calls[0]!.arguments[0].chunkSize, 4)
  cameraController.setPosition(99999, 99999)
  cameraController.pan(80, -40)
  cameraController.setZoom(2)
  assert.deepEqual(facade.getMinimapPlayerPose(), { x: smoothed.x, y: smoothed.y, heading: smoothed.heading })

  moveController.initEntity(42, -800, 1600, Math.PI)
  assert.deepEqual(facade.getMinimapPlayerPose(), { x: -800, y: 1600, heading: Math.PI }, 'same-ID respawn replaces an older rendered pose')
  assert.equal(moveController.getRenderPosition(42), null)
  moveController.removeEntity(42)
  assert.equal(facade.getMinimapPlayerPose(), null)
})

test('facade controls the attached minimap and stale detach cannot release a newer canvas', async t => {
  const manager = await managerFixture(t)
  const { render, frameLoop, minimap } = renderBridgeFixture(t, manager)
  const facade = new GameFacade()
  ;(facade as unknown as { render: Render }).render = render
  const attachedCanvas = {} as HTMLCanvasElement
  ;(render as unknown as { minimapCanvas: HTMLCanvasElement }).minimapCanvas = attachedCanvas
  const destroys = t.mock.method(minimap, 'destroy')
  const zooms = t.mock.method(minimap, 'setZoom')
  const clears = t.mock.method(minimap, 'clear')
  const frames = t.mock.method(minimap, 'render')
  facade.attachMinimap(attachedCanvas)
  facade.setMinimapZoom(2)
  facade.clearMinimap()
  assert.equal(zooms.mock.calls[0]!.arguments[0], 2)
  assert.equal(clears.mock.callCount(), 1)
  facade.detachMinimap({} as HTMLCanvasElement)
  assert.equal(destroys.mock.callCount(), 0)
  frameLoop.update()
  assert.equal(frames.mock.callCount(), 1, 'stale unmount leaves the current presentation attached')
  facade.detachMinimap(attachedCanvas)
  facade.detachMinimap(attachedCanvas)
  assert.equal(destroys.mock.callCount(), 1)
  frameLoop.update()
  assert.equal(frames.mock.callCount(), 1, 'detached renderer receives no further world references')
})

test('every world entry clears the minimap before same-coordinate surface and mine-layer streams', async t => {
  const manager = await managerFixture(t)
  const { render, frameLoop, minimap } = renderBridgeFixture(t, manager)
  setActivePinia(createPinia())
  const facadeState = gameFacade as unknown as { render: Render | null }
  const originalRender = facadeState.render
  facadeState.render = render
  t.after(() => { facadeState.render = originalRender })
  const clears = t.mock.method(minimap, 'clear')
  const frames = t.mock.method(minimap, 'render')
  registerMessageHandlers()
  const dispatch = (packet: proto.IServerMessage) => messageDispatcher.dispatch(proto.ServerMessage.create(packet))

  for (const [index, layer] of ['surface', 'mine entrance', 'deeper mine', 'surface return'].entries()) {
    const streamEpoch = index + 1
    dispatch({ playerEnterWorld: { entityId: 42, streamEpoch, coordPerTile: 12, chunkSize: 4 } })
    assert.equal(clears.mock.callCount(), streamEpoch, `${layer}: entry clears synchronously without a leave packet`)
    assert.equal(manager.getMinimapChunk(0, 0), undefined, `${layer}: matching coordinates cannot reuse the previous layer`)
    assert.equal(gameFacade.getMinimapPlayerPose(), null)
    assert.equal(moveController.getRenderPosition(42), null)
    frameLoop.update()
    assert.equal(frames.mock.calls.at(-1)!.arguments[0].player, null)
    dispatch({ chunkLoad: { streamEpoch, eventSeq: 1, chunk: { coord: { x: 0, y: 0 }, tiles: new Uint8Array(16).fill(index === 0 ? 35 : 14), version: 0 } } })
    assert.equal(manager.getMinimapChunk(0, 0)!.tiles[0], index === 0 ? 35 : 14)
    dispatch({ objectSpawn: { streamEpoch, entityId: 42, position: { position: { x: 24, y: 36, heading: Math.PI } } } })
    assert.deepEqual(gameFacade.getMinimapPlayerPose(), { x: 24, y: 36, heading: Math.PI })
    frameLoop.update()
    assert.deepEqual(frames.mock.calls.at(-1)!.arguments[0].player, { x: 24, y: 36, heading: Math.PI })
    manager.unloadChunk(0, 0, identity(2, streamEpoch))
    assert.ok(manager.getMinimapChunk(0, 0), 'retained terrain must also be cleared at the next entry')
  }
  gameFacade.resetWorld()
  assert.equal(clears.mock.callCount(), 5)
  assert.equal(gameFacade.getMinimapPlayerPose(), null)
  assert.equal(manager.getMinimapChunk(0, 0), undefined)
})

test('equal version retains actual Chunk and geometry; higher version rebuilds', async t => {
  const manager = await managerFixture(t)
  const builds = t.mock.method(Chunk.prototype, 'buildTiles')
  manager.loadChunk(0, 0, tiles(), 7, identity(1))
  const chunk = manager.getChunk(0, 0)!
  const original = geometry(chunk)
  const release = t.mock.method(original, 'destroy')
  const bufferReleases = original.buffers.map(buffer => t.mock.method(buffer, 'destroy'))
  const shader = (chunk.getSubchunkDataList()[0]!.container.children[0] as Mesh).shader!
  const releaseShader = t.mock.method(shader, 'destroy')
  const program = shader.glProgram
  manager.unloadChunk(0, 0, identity(2))
  assert.equal(chunk.visible, false)
  assert.equal(release.mock.callCount(), 0)
  manager.loadChunk(0, 0, tiles(), 7, identity(3))
  assert.equal(manager.getChunk(0, 0), chunk)
  assert.equal(geometry(chunk), original)
  assert.equal(chunk.visible, true)
  assert.equal(builds.mock.callCount(), 1)
  const changed = tiles(); changed[0] = 14
  manager.loadChunk(0, 0, changed, 8, identity(4))
  assert.equal(builds.mock.callCount(), 2)
  assert.notEqual(geometry(chunk), original)
  assert.equal(release.mock.callCount(), 1)
  assert.ok(bufferReleases.every(releaseBuffer => releaseBuffer.mock.callCount() === 1))
  assert.equal(releaseShader.mock.callCount(), 1)
  assert.ok(program!.vertex, 'chunk disposal must preserve the shared shader program')
  assert.equal(chunkCache.peek('0,0')!.version, 8)
})

test('neighbor version and presence refresh borders; hidden refresh waits for reload', async t => {
  const manager = await managerFixture(t)
  manager.loadChunk(0, 0, tiles(), 7, identity(1))
  manager.loadChunk(1, 0, tiles(), 3, identity(2))
  flushBorders(manager)
  const chunk = manager.getChunk(0, 0)!
  const first = geometry(chunk)
  manager.loadChunk(1, 0, tiles(), 4, identity(3))
  flushBorders(manager)
  assert.notEqual(geometry(chunk), first)
  assert.equal(chunkCache.peek('0,0')!.version, 7, 'border refresh cannot reset own version')
  assert.equal(chunkCache.peek('0,0')!.neighborVersions.get('1,0'), 4)
  manager.unloadChunk(0, 0, identity(4))
  const hidden = geometry(chunk)
  const retainedAt = chunkCache.peek('0,0')!.retainedAt
  manager.loadChunk(1, 0, tiles(), 5, identity(5))
  flushBorders(manager)
  assert.equal(geometry(chunk), hidden)
  assert.equal(chunkCache.peek('0,0')!.retainedAt, retainedAt)
  manager.loadChunk(0, 0, tiles(), 7, identity(6))
  assert.notEqual(geometry(chunk), hidden)
  const withNeighbor = geometry(chunk)
  manager.unloadChunk(1, 0, identity(7))
  flushBorders(manager)
  assert.notEqual(geometry(chunk), withNeighbor)
  assert.equal(chunkCache.peek('0,0')!.version, 7)
})

test('TTL/LRU dispose hidden geometry once, preserve active chunks, and equal cache misses build', async t => {
  const manager = await managerFixture(t)
  manager.loadChunk(0, 0, tiles(), 10, identity(1))
  manager.loadChunk(4, 0, tiles(), 10, identity(2))
  const active = manager.getChunk(0, 0)!
  const hidden = manager.getChunk(4, 0)!
  const release = t.mock.method(geometry(hidden), 'destroy')
  manager.unloadChunk(4, 0, identity(3))
  const retainedAt = chunkCache.peek('4,0')!.retainedAt!
  const metricsBeforeReads = chunkCache.getMetrics()
  for (let frame = 0; frame < 100; frame++) assert.ok(manager.getMinimapChunk(4, 0))
  assert.deepEqual(chunkCache.getMetrics(), metricsBeforeReads, 'minimap reads do not affect cache metrics or storage')
  assert.equal(chunkCache.peek('4,0')!.retainedAt, retainedAt)
  chunkCache.sweep(retainedAt + CACHE_TTL_MS)
  chunkCache.sweep(retainedAt + CACHE_TTL_MS * 2)
  assert.equal(manager.getChunk(4, 0), undefined)
  assert.equal(manager.getMinimapChunk(4, 0), undefined, 'stationary reads cannot preserve expired terrain')
  assert.equal(release.mock.callCount(), 1)
  assert.equal(manager.getChunk(0, 0), active)
  manager.loadChunk(4, 0, tiles(), 10, identity(4))
  assert.notEqual(manager.getChunk(4, 0), hidden)
  const oldest = manager.getChunk(4, 0)!
  const oldestRelease = t.mock.method(geometry(oldest), 'destroy')
  manager.unloadChunk(4, 0, identity(5))
  for (let index = 0; index < CACHE_MAX_ENTRIES; index++) {
    assert.ok(manager.getMinimapChunk(4, 0), 'reading the oldest entry must not change eviction order')
    const x = 8 + index * 4
    manager.loadChunk(x, 0, tiles(), 10, identity(6 + index * 2))
    manager.unloadChunk(x, 0, identity(7 + index * 2))
  }
  assert.equal(manager.getChunk(4, 0), undefined)
  assert.equal(manager.getMinimapChunk(4, 0), undefined)
  assert.equal(oldestRelease.mock.callCount(), 1)
  assert.equal(manager.getChunk(0, 0), active)
  assert.equal(chunkCache.size, CACHE_MAX_ENTRIES + 1)
  manager.clear()
  assert.equal(chunkCache.size, 0)
  assert.equal(manager.getContainer().children.length, 0)
  assert.equal(oldestRelease.mock.callCount(), 1)
})

test('buffered and queued loads cannot resurrect after unload or reset', async t => {
  t.mock.method(terrainManager, 'generateTerrainForChunk', () => {})
  let resolve!: (sheet: Spritesheet) => void
  t.mock.method(Assets, 'load', () => new Promise<Spritesheet>(ready => { resolve = ready }))
  const manager = new ChunkManager()
  manager.setWorldParams(12, 4)
  t.after(() => manager.destroy())
  const ready = manager.init()
  manager.loadChunk(0, 0, tiles(), 1, identity(1))
  manager.unloadChunk(0, 0, identity(2))
  manager.loadChunk(1, 0, tiles(), 1, identity(3))
  manager.clear()
  resolve(sheet)
  await ready
  assert.equal(manager.getLoadedChunksCount(), 0)
  assert.equal(manager.getContainer().children.length, 0)
  manager.loadChunk(0, 0, tiles(), 1, identity(1, 2))
  manager.loadChunk(1, 0, tiles(), 1, identity(2, 2))
  ;(manager as unknown as { processBorderRefreshQueue(): void }).processBorderRefreshQueue()
  const dequeued = buildQueue.getTasksForFrame()
  assert.ok(dequeued.length)
  manager.unloadChunk(0, 0, identity(3, 2))
  for (const task of dequeued) {
    ;(manager as unknown as { processBuildTask(task: unknown): void }).processBuildTask(task)
    buildQueue.buildComplete()
  }
  assert.equal(manager.getChunk(0, 0)!.visible, false)
  manager.clear()
  for (const task of dequeued) (manager as unknown as { processBuildTask(task: unknown): void }).processBuildTask(task)
  manager.update()
  assert.equal(manager.getContainer().children.length, 0)
  assert.equal(chunkCache.size, 0)
})

test('a failed incomplete build cannot become reusable geometry', async t => {
  const manager = await managerFixture(t)
  const original = Chunk.prototype.buildTiles
  const failing = t.mock.method(Chunk.prototype, 'buildTiles', function (this: Chunk, ...args: Parameters<typeof original>) {
    original.apply(this, args)
    throw new Error('injected build failure')
  })
  assert.throws(() => manager.loadChunk(0, 0, tiles(), 1, identity(1)), /injected/)
  assert.equal(manager.getChunk(0, 0), undefined)
  assert.equal(chunkCache.peek('0,0'), undefined)
  failing.mock.restore()
  manager.loadChunk(0, 0, tiles(), 1, identity(2))
  assert.ok(geometry(manager.getChunk(0, 0)!))
})
