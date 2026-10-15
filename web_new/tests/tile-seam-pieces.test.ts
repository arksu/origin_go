import assert from 'node:assert/strict'
import { test } from 'node:test'
import { DOMAdapter, Texture, type Spritesheet } from 'pixi.js'
import { Chunk } from '../src/game/Chunk'
import { getTileSet, registerTileSet, TileArray, TileSet } from '../src/game/tiles/TileSet'
import water from '../src/game/tiles/configs/water.json'
import { pieceNeighborhood, transitionPieces } from './tile-seam-pieces'
import { setWorldParams } from './serverConstantsFixture'

test('piece enumeration includes variants outside a seed sample, excluding disabled and duplicate frames', () => {
  const array = new TileArray([
    { img: 'common.png', w: 1000 }, { img: 'rare.png', w: 1 },
    { img: 'disabled.png', w: 0 }, { img: 'common.png', w: 1 },
  ])
  assert.equal(array.get(255), 'common.png')
  assert.deepEqual(array.getTextureNames(), ['common.png', 'rare.png'])
  const names = array.getTextureNames()
  names.length = 0
  assert.deepEqual(array.getTextureNames(), ['common.png', 'rare.png'])
})

test('water gallery contains every border and corner variant with its exact source filename', () => {
  const pieces = transitionPieces(new TileSet(water))
  assert.equal(pieces.length, 102)
  for (const kind of ['border', 'corner'] as const) for (let mask = 1; mask <= 15; mask++) {
    const entries = pieces.filter(piece => piece.kind === kind && piece.mask === mask)
    const prefix = kind === 'border' ? 'b' : 'c'
    const count = kind === 'border' && [1, 2, 4, 8].includes(mask) ? 6 : 3
    assert.deepEqual(entries.map(piece => piece.texture), Array.from({ length: count }, (_, index) =>
      `water/${prefix}${mask}${index === 0 ? '' : `_${index + 1}`}.png`))
    assert.deepEqual(entries.map(piece => piece.variant), Array.from({ length: count }, (_, index) => index + 1))
  }
})

test('each 3x3 fixture selects exactly its labeled mask in the real chunk renderer', t => {
  DOMAdapter.set({ ...DOMAdapter.get(), createCanvas: () => ({ getContext: () => null }) as unknown as HTMLCanvasElement })
  setWorldParams(32, 16)
  const overlay = 249
  const base = 250
  const variants = (prefix: string) => Array.from({ length: 15 }, (_, index) => [{ img: `${prefix}${index + 1}`, w: 1 }])
  registerTileSet(overlay, { ground: [{ img: 'overlay', w: 1 }], borders: variants('b'), corners: variants('c') })
  registerTileSet(base, { ground: [{ img: 'base', w: 1 }], borders: [], corners: [] })
  const set = getTileSet(overlay)!
  const selected: { kind: string; mask: number }[] = []
  for (const [method, kind] of [['getBorderTexture', 'border'], ['getCornerTexture', 'corner']] as const) {
    const original = set[method].bind(set)
    t.mock.method(set, method, (mask: number, x: number, y: number) => {
      if (x === 8 && y === 8) selected.push({ kind, mask })
      return original(mask, x, y)
    })
  }
  const sheet = { textures: new Proxy({}, { get: () => Texture.EMPTY }), textureSource: Texture.EMPTY.source } as Spritesheet
  for (const kind of ['border', 'corner'] as const) for (let mask = 1; mask <= 15; mask++) {
    selected.length = 0
    const tiles = pieceNeighborhood(kind, mask, overlay, base, 16)
    assert.equal(tiles[8 * 16 + 8], base)
    const chunk = new Chunk(0, 0)
    try {
      chunk.buildTiles(tiles, sheet)
      assert.deepEqual(selected, [{ kind, mask }], `${kind} mask ${mask}`)
    } finally {
      chunk.destroy()
    }
  }
})
