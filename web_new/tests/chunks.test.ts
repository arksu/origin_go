import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { Assets, DOMAdapter, Mesh, Texture, type Spritesheet } from 'pixi.js'
import { Chunk } from '../src/game/Chunk'
import { ChunkManager } from '../src/game/ChunkManager'
import { terrainManager } from '../src/game/terrain'
import { chunkCache, buildQueue } from '../src/game/cache'
import { CACHE_MAX_ENTRIES, CACHE_TTL_MS } from '../src/constants/cache'
import { ChunkStreamGuard } from '../src/network/ChunkStreamGuard'
import { proto } from '../src/network/proto/packets.js'
import { registerMessageHandlers } from '../src/network/handlers'
import { messageDispatcher } from '../src/network/MessageDispatcher'
import { gameFacade } from '../src/game/GameFacade'
import { useGameStore } from '../src/stores/gameStore'

// Use real Pixi meshes, geometry and buffers; only the browser capability probe
// and texture loading are replaced. No WebGL context is needed for lifecycle tests.
DOMAdapter.set({ ...DOMAdapter.get(), createCanvas: () => ({ getContext: () => null }) as unknown as HTMLCanvasElement })
const sheet = { textures: new Proxy({}, { get: () => Texture.EMPTY }), textureSource: Texture.EMPTY.source } as Spritesheet
const tiles = () => new Uint8Array(16).fill(35)
const identity = (seq: number, epoch = 1) => ({ streamEpoch: epoch, eventSeq: BigInt(seq) })

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
  assert.equal(resets.mock.callCount(), 1)
  assert.equal(store.chunks.size, 0)
  load(6, 4, 1)
  assert.equal(loads.mock.callCount(), 1)
  load(1, 0, 2)
  assert.equal(loads.mock.callCount(), 2)
  assert.equal(store.chunks.get('0,0')!.version, 0)
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
  chunkCache.get('4,0')
  chunkCache.sweep(retainedAt + CACHE_TTL_MS)
  chunkCache.sweep(retainedAt + CACHE_TTL_MS * 2)
  assert.equal(manager.getChunk(4, 0), undefined)
  assert.equal(release.mock.callCount(), 1)
  assert.equal(manager.getChunk(0, 0), active)
  manager.loadChunk(4, 0, tiles(), 10, identity(4))
  assert.notEqual(manager.getChunk(4, 0), hidden)
  const oldest = manager.getChunk(4, 0)!
  const oldestRelease = t.mock.method(geometry(oldest), 'destroy')
  manager.unloadChunk(4, 0, identity(5))
  for (let index = 0; index < CACHE_MAX_ENTRIES; index++) {
    const x = 8 + index * 4
    manager.loadChunk(x, 0, tiles(), 10, identity(6 + index * 2))
    manager.unloadChunk(x, 0, identity(7 + index * 2))
  }
  assert.equal(manager.getChunk(4, 0), undefined)
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
