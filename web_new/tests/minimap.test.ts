import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { MINIMAP_ZOOM_LEVELS } from '../src/constants/minimap'
import { validateMinimapAtlas } from '../src/game/minimap/atlas'
import { MinimapRenderer } from '../src/game/minimap/MinimapRenderer'
import { MINIMAP_FALLBACK_COLORS, MINIMAP_UNKNOWN_COLOR } from '../src/game/minimap/palette'
import { rasterizeMinimap } from '../src/game/minimap/raster'
import type { MinimapChunk, MinimapFrame, MinimapPixels, MinimapTextureRegion } from '../src/game/minimap/types'
import { RENDERABLE_TILE_IDS, TILE_GRASS, TILE_PLOWED, TILE_VOID } from '../src/game/tiles/tileIds'

function pixels(width: number, height: number): MinimapPixels {
  return { width, height, data: new Uint8ClampedArray(width * height * 4) }
}

function atlasFixture() {
  const image = pixels(80, 64)
  const manifest: Record<string, MinimapTextureRegion> = {}
  RENDERABLE_TILE_IDS.forEach((tileId, index) => {
    const region = { x: index % 5 * 16, y: Math.floor(index / 5) * 16, width: 16, height: 16 }
    manifest[String(tileId)] = region
    for (let row = 0; row < 16; row++) {
      for (let column = 0; column < 16; column++) {
        image.data.set([tileId, column, row, 255], ((region.y + row) * image.width + region.x + column) * 4)
      }
    }
  })
  return { image, manifest }
}

function pixelAt(image: MinimapPixels, column: number, row: number): number[] {
  const offset = (row * image.width + column) * 4
  return Array.from(image.data.slice(offset, offset + 4))
}

function frameFixture(chunkSize = 4) {
  const chunks = new Map<string, MinimapChunk>()
  const queried: string[] = []
  const frame: MinimapFrame = {
    player: { x: 123.25, y: -50.5, heading: Math.PI / 2 },
    coordPerTile: 12,
    chunkSize,
    getChunk(x, y) {
      queried.push(`${x},${y}`)
      return chunks.get(`${x},${y}`)
    },
  }
  const addChunk = (x: number, y: number, tileId = TILE_GRASS) => {
    const chunk = { x, y, tiles: new Uint8Array(chunkSize * chunkSize).fill(tileId), version: 1 }
    chunks.set(`${x},${y}`, chunk)
    return chunk
  }
  return { frame, chunks, queried, addChunk }
}

test('atlas validation accepts all opaque regions, including real ImageData-style accessors', () => {
  const { image, manifest } = atlasFixture()
  const imageData = Object.create({ width: image.width, height: image.height, data: image.data }) as MinimapPixels
  const { atlas, invalidTileIds } = validateMinimapAtlas(manifest, imageData)
  assert.deepEqual(invalidTileIds, [])
  assert.equal(atlas.regions.size, 17)
  assert.equal(atlas.width, 80)
  assert.equal(atlas.height, 64)
  assert.equal(atlas.data, image.data)
  assert.equal(atlas.regions.has(TILE_VOID), false)
})

test('invalid atlas regions fall back independently: missing, fractional, out-of-bounds, overlap, size and opacity', () => {
  const { image, manifest } = atlasFixture()
  delete manifest['1']
  manifest['3'] = { ...manifest['3']!, x: 16.5 }
  manifest['12'] = { ...manifest['12']!, x: 79 }
  manifest['14'] = { ...manifest['20']! }
  manifest['25'] = { ...manifest['25']!, width: 15 }
  const transparent = manifest['30']!
  image.data[(transparent.y * image.width + transparent.x) * 4 + 3] = 0
  const { atlas, invalidTileIds } = validateMinimapAtlas(manifest, image)
  assert.deepEqual([...invalidTileIds].sort((left, right) => left - right), [1, 3, 12, 14, 20, 25, 30])
  assert.equal(atlas.regions.size, 10)
  assert.ok(atlas.regions.has(TILE_GRASS))
  assert.throws(() => validateMinimapAtlas(manifest, pixels(0, 0)), /dimensions/)
})

test('every renderable type has a defined fallback; unknown IDs are diagnostic and void is transparent', () => {
  assert.deepEqual(Object.keys(MINIMAP_FALLBACK_COLORS).map(Number).sort((left, right) => left - right), RENDERABLE_TILE_IDS)
  const { frame, addChunk } = frameFixture(5)
  const chunk = addChunk(0, 0)
  chunk.tiles.fill(TILE_VOID)
  chunk.tiles.set([...RENDERABLE_TILE_IDS, 222])
  const output = pixels(5, 5)
  const unknown: number[] = []
  rasterizeMinimap(output, frame, 0, 0, null, tileId => unknown.push(tileId))
  RENDERABLE_TILE_IDS.forEach((tileId, index) => {
    assert.deepEqual(pixelAt(output, index % 5, Math.floor(index / 5)), [...MINIMAP_FALLBACK_COLORS[tileId]!, 255])
  })
  assert.deepEqual(pixelAt(output, 2, 3), [...MINIMAP_UNKNOWN_COLOR, 255])
  assert.deepEqual(pixelAt(output, 3, 3), [0, 0, 0, 0])
  assert.deepEqual(unknown, [222])
})

test('world texture phase crosses negative nondefault chunk boundaries and survives viewport movement', () => {
  const { image, manifest } = atlasFixture()
  const { atlas } = validateMinimapAtlas(manifest, image)
  const { frame, addChunk, queried } = frameFixture(3)
  for (let chunkY = -2; chunkY <= 1; chunkY++) {
    for (let chunkX = -2; chunkX <= 1; chunkX++) addChunk(chunkX, chunkY)
  }
  const output = pixels(7, 7)
  rasterizeMinimap(output, frame, -4, -4, atlas, () => assert.fail('known tile'))
  assert.deepEqual(pixelAt(output, 0, 0), [TILE_GRASS, 12, 12, 255])
  assert.deepEqual(pixelAt(output, 3, 3), [TILE_GRASS, 15, 15, 255])
  assert.deepEqual(pixelAt(output, 4, 4), [TILE_GRASS, 0, 0, 255])
  assert.deepEqual(pixelAt(output, 6, 6), [TILE_GRASS, 2, 2, 255])
  assert.equal(new Set(queried).size, queried.length, 'each intersecting chunk is borrowed once')
  const moved = pixels(7, 7)
  rasterizeMinimap(moved, frame, -3, -3, atlas, () => assert.fail('known tile'))
  assert.deepEqual(pixelAt(moved, 2, 2), pixelAt(output, 3, 3), 'the same world tile retains texture phase')
})

test('reused raster clears removed chunks while stationary and reflects replacements immediately', () => {
  const { frame, addChunk, chunks } = frameFixture()
  addChunk(0, 0)
  const output = pixels(8, 4)
  rasterizeMinimap(output, frame, 0, 0, null, () => {})
  assert.deepEqual(pixelAt(output, 0, 0), [...MINIMAP_FALLBACK_COLORS[TILE_GRASS]!, 255])
  assert.deepEqual(pixelAt(output, 7, 3), [0, 0, 0, 0], 'missing chunks are transparent')
  addChunk(0, 0, TILE_PLOWED)
  rasterizeMinimap(output, frame, 0, 0, null, () => {})
  assert.deepEqual(pixelAt(output, 0, 0), [...MINIMAP_FALLBACK_COLORS[TILE_PLOWED]!, 255])
  chunks.clear()
  rasterizeMinimap(output, frame, 0, 0, null, () => {})
  assert.equal(output.data.some(channel => channel !== 0), false)
})

class FakeContext {
  imageSmoothingEnabled = true
  fillStyle = ''
  strokeStyle = ''
  lineWidth = 1
  lineJoin = 'miter'
  image: MinimapPixels | null = null
  cleared = false
  readonly draws: unknown[][] = []
  readonly translations: number[][] = []
  readonly rotations: number[] = []
  readonly transforms: number[][] = []
  constructor(private readonly atlasImage: MinimapPixels) {}
  createImageData(width: number, height: number) { return pixels(width, height) }
  putImageData(image: MinimapPixels) { this.image = image; this.cleared = false }
  getImageData() { return this.atlasImage }
  clearRect() { this.cleared = true }
  drawImage(...args: unknown[]) { this.draws.push(args); this.cleared = false }
  setTransform(...args: number[]) { this.transforms.push(args) }
  translate(...args: number[]) { this.translations.push(args) }
  rotate(heading: number) { this.rotations.push(heading) }
  save() {}
  restore() {}
  beginPath() {}
  moveTo() {}
  lineTo() {}
  closePath() {}
  stroke() {}
  fill() {}
}

class FakeCanvas {
  width = 200
  height = 200
  clientWidth = 200
  clientHeight = 200
  readonly context: FakeContext
  constructor(atlasImage: MinimapPixels) { this.context = new FakeContext(atlasImage) }
  getContext() { return this.context }
}

function browserFixture(t: TestContext) {
  const { image, manifest } = atlasFixture()
  const canvases: FakeCanvas[] = []
  const images: Array<{ onload: (() => void) | null; onerror: (() => void) | null }> = []
  let resolveResponse!: (response: Response) => void
  let rejectResponse!: (error: Error) => void
  const response = new Promise<Response>((resolve, reject) => { resolveResponse = resolve; rejectResponse = reject })
  const windowMock = { devicePixelRatio: 1 }
  const originalGlobals = ['document', 'window', 'Image', 'fetch'].map(name => [name, Object.getOwnPropertyDescriptor(globalThis, name)] as const)
  Object.defineProperties(globalThis, {
    document: { configurable: true, value: { createElement: () => {
      const canvas = new FakeCanvas(image)
      canvases.push(canvas)
      return canvas
    } } },
    window: { configurable: true, value: windowMock },
    Image: { configurable: true, value: class {
      naturalWidth = image.width
      naturalHeight = image.height
      src = ''
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      constructor() { images.push(this) }
    } },
    fetch: { configurable: true, value: () => response },
  })
  t.after(() => {
    for (const [name, descriptor] of originalGlobals) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor)
      else Reflect.deleteProperty(globalThis, name)
    }
  })
  const warnings = t.mock.method(console, 'warn', () => {})
  const canvas = new FakeCanvas(image)
  const renderer = new MinimapRenderer(canvas as unknown as HTMLCanvasElement)
  t.after(() => renderer.destroy())
  return {
    canvas, canvases, images, renderer, windowMock, warnings, manifest,
    complete: async () => {
      resolveResponse({ ok: true, json: async () => manifest } as Response)
      images[0]!.onload?.()
      await new Promise(resolve => setImmediate(resolve))
    },
    fail: async () => {
      rejectResponse(new Error('fixture asset failure'))
      images[0]!.onerror?.()
      await new Promise(resolve => setImmediate(resolve))
    },
  }
}

test('renderer centers fractional world pose at every zoom, honors DPR and world heading without retaining frames', async t => {
  const { canvas, canvases, renderer, windowMock, complete } = browserFixture(t)
  const { frame, addChunk, queried } = frameFixture(3)
  addChunk(3, -2)
  windowMock.devicePixelRatio = 2
  canvas.clientWidth = 160
  canvas.clientHeight = 144
  await complete()
  for (const zoom of MINIMAP_ZOOM_LEVELS) {
    renderer.setZoom(zoom)
    renderer.render(frame)
    assert.equal(canvas.width, 320)
    assert.equal(canvas.height, 288)
    assert.deepEqual(canvas.context.translations.at(-1), [80, 72])
    assert.equal(canvas.context.rotations.at(-1), Math.PI / 2)
    assert.equal(canvas.context.imageSmoothingEnabled, false)
    const raster = canvases[0]!.context.image!
    assert.equal(raster.width, Math.ceil(160 / zoom) + 1)
    assert.equal(raster.height, Math.ceil(144 / zoom) + 1)
    const leftTile = frame.player!.x / frame.coordPerTile - 80 / zoom
    const topTile = frame.player!.y / frame.coordPerTile - 72 / zoom
    const lastDraw = canvas.context.draws.at(-1)!
    assert.equal(lastDraw[1], (Math.floor(leftTile) - leftTile) * zoom)
    assert.equal(lastDraw[2], (Math.floor(topTile) - topTile) * zoom)
  }
  assert.ok(queried.length)
  assert.throws(() => renderer.setZoom(3), /Unsupported minimap zoom/)
  const drawCount = canvas.context.draws.length
  renderer.render({ ...frame, player: null })
  assert.equal(canvas.context.cleared, true)
  assert.equal(canvas.context.draws.length, drawCount, 'no origin marker or terrain before spawn')
  assert.equal(canvases[0]!.context.image!.data.some(channel => channel !== 0), false)
})

test('asset completion after clear paints nothing, then only a fresh current-world frame can draw', async t => {
  const { canvas, canvases, renderer, complete } = browserFixture(t)
  const { frame, addChunk, chunks } = frameFixture()
  addChunk(2, -2)
  renderer.render(frame)
  assert.ok(canvases[0]!.context.image!.data.some(channel => channel !== 0))
  renderer.clear()
  const drawCount = canvas.context.draws.length
  chunks.clear()
  await complete()
  assert.equal(canvas.context.draws.length, drawCount)
  assert.equal(canvas.context.cleared, true)
  assert.equal(canvases[0]!.context.image!.data.some(channel => channel !== 0), false)
  renderer.render(frame)
  assert.equal(canvases[0]!.context.image!.data.some(channel => channel !== 0), false, 'old source was not retained by the loader')
  addChunk(2, -2, TILE_PLOWED)
  renderer.render(frame)
  assert.ok(canvases[0]!.context.image!.data.some(channel => channel === TILE_PLOWED), 'new terrain uses the loaded static atlas')
})

test('detach aborts loading and prevents late completion or render from reviving the attachment', async t => {
  const { canvas, canvases, renderer, complete, warnings, images } = browserFixture(t)
  const { frame, addChunk } = frameFixture()
  addChunk(2, -2)
  renderer.render(frame)
  renderer.destroy()
  const drawCount = canvas.context.draws.length
  assert.equal(images[0]!.onload, null)
  await complete()
  renderer.render(frame)
  assert.equal(canvas.context.draws.length, drawCount)
  assert.equal(canvas.context.cleared, true)
  assert.equal(canvases[0]!.width, 0)
  assert.equal(warnings.mock.callCount(), 0, 'normal detach is not a failed asset load')
})

test('failed assets keep fallback terrain and marker working with bounded diagnostics across frames', async t => {
  const { canvas, canvases, renderer, fail, warnings } = browserFixture(t)
  const { frame, addChunk } = frameFixture()
  addChunk(2, -2, 222)
  await fail()
  for (let frameNumber = 0; frameNumber < 10; frameNumber++) renderer.render(frame)
  assert.equal(warnings.mock.callCount(), 2, 'one asset failure and one unknown-type warning, never per pixel or frame')
  assert.equal(canvas.context.rotations.length, 10, 'asset failures do not hide the player')
  assert.ok(canvases[0]!.context.image!.data.some(channel => channel === 255))
  addChunk(2, -2, TILE_GRASS)
  renderer.render(frame)
  assert.ok(canvases[0]!.context.image!.data.some(channel => channel === MINIMAP_FALLBACK_COLORS[TILE_GRASS]![0]))
})

test('partial atlas validation warns once and preserves valid materials', async t => {
  const { renderer, complete, warnings, manifest } = browserFixture(t)
  delete manifest[String(TILE_PLOWED)]
  await complete()
  const { frame, addChunk } = frameFixture()
  addChunk(2, -2, TILE_PLOWED)
  for (let frameNumber = 0; frameNumber < 5; frameNumber++) renderer.render(frame)
  assert.equal(warnings.mock.callCount(), 1)
  assert.match(String(warnings.mock.calls[0]!.arguments[0]), /14/)
})
