import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { test } from 'node:test'
import { ResourceLoader } from '../src/game/ResourceLoader'

const species = ['birch', 'pine', 'oak', 'apple', 'maple', 'yew', 'elm', 'hazel', 'willow', 'fir', 'autumn_oak', 'mberry']

function pngSize(path: string): [number, number] {
  const bytes = readFileSync(resolve('public/assets/game', path))
  assert.equal(bytes.subarray(0, 8).toString('hex'), '89504e470d0a1a0a', path)
  return [bytes.readUInt32BE(16), bytes.readUInt32BE(20)]
}

test('all generated tree species and chopping stumps resolve to published PNGs', () => {
  for (const tree of species) {
    for (const stage of ['6', 'stump']) {
      const key = `trees/${tree}/${stage}`
      const resource = ResourceLoader.getResourceDef(key)
      assert.ok(resource, key)
      assert.ok(resource.layers.some(layer => !layer.shadow), `${key} body`)
      assert.ok(resource.layers.some(layer => layer.shadow), `${key} shadow`)
      if (stage === '6') {
        assert.ok(resource.layers.some(layer => !layer.shadow && layer.interactive), `${key} selectable body`)
      }
      for (const layer of resource.layers) {
        assert.ok(layer.img, `${key} static image`)
        const [width, height] = pngSize(layer.img)
        assert.ok(width > 0 && height > 0, layer.img)
      }
    }
  }
})

test('mulberry body and shadow resources keep their ground anchors and canvas sizes', () => {
  const mature = ResourceLoader.getResourceDef('trees/mberry/6')!
  const stump = ResourceLoader.getResourceDef('trees/mberry/stump')!
  assert.deepEqual(mature.offset, [67, 114])
  assert.deepEqual(stump.offset, [40, 24])
  for (const [resource, bodySize, shadowSize, shadowOffset] of [
    [mature, [136, 136], [104, 40], [15, 103]],
    [stump, [80, 48], [80, 32], [0, 12]],
  ] as const) {
    const body = resource.layers.find(layer => !layer.shadow)!
    const shadow = resource.layers.find(layer => layer.shadow)!
    assert.deepEqual(body.offset, [0, 0])
    assert.deepEqual(shadow.offset, shadowOffset)
    assert.deepEqual(pngSize(body.img!), bodySize)
    assert.deepEqual(pngSize(shadow.img!), shadowSize)
    assert.equal(ResourceLoader.resolveLayerZ(shadow), -1)
  }
})

test('maple chopping retains its resource key and uses the complete oak stump artwork', () => {
  assert.deepEqual(ResourceLoader.getResourceDef('trees/maple/stump'), ResourceLoader.getResourceDef('trees/oak/stump'))
})
