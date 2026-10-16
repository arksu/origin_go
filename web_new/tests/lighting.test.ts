import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { Container, DOMAdapter, Mesh, type RenderOptions, RenderTexture, Sprite, type WebGLRenderer } from 'pixi.js'
import { LightingManager } from '../src/game/lighting/LightingManager'
import {
  createRadialLightPixels, getLightMapLayout, getNightOpacity, LIGHT_FLICKER_PERIOD_MS,
  lightFlickerSeed, lightFlickerAmount, lightSwayOffset, NIGHT_FLOOR, nightLightMultiplier,
  radialLightFalloff, resolveLightDefinition, shouldRefreshLightMap,
} from '../src/game/lighting/model'
import { validateLightDefinition } from '../src/game/lighting/validateDefinition.js'

function near(actual: number, expected: number): void {
  assert.ok(Math.abs(actual - expected) < 1e-8, `${actual} != ${expected}`)
}

function fixture(context: TestContext) {
  const adapter = DOMAdapter.get()
  DOMAdapter.set({ ...adapter, createCanvas: () => ({ getContext: () => null }) as unknown as HTMLCanvasElement })
  const canvas = new EventTarget()
  const passes: RenderOptions[] = []
  const maskTransforms: { a: number; d: number }[] = []
  const renderer = {
    canvas,
    render: (options: RenderOptions) => {
      // AbstractRenderer mutates reused options and only prepares the matrix once.
      if (!options.transform) {
        options.container.updateLocalTransform()
        options.transform = options.container.localTransform
      }
      maskTransforms.push({ a: options.transform.a, d: options.transform.d })
      passes.push(options)
    },
  } as unknown as WebGLRenderer
  const manager = new LightingManager(renderer)
  const world = new Container()
  const owner = new Container({ x: 80, y: 60 })
  world.addChild(owner)
  context.after(() => {
    manager.destroy()
    world.destroy({ children: true })
    DOMAdapter.set(adapter)
  })
  return { manager, world, owner, passes, maskTransforms, canvas }
}

function getUniforms(manager: LightingManager): { uNightOpacity: number; uHasLights: number } {
  const mesh = manager.getContainer() as Mesh
  return mesh.shader!.resources.lighting.uniforms
}

function firstSource(pass: RenderOptions): Sprite {
  return pass.container.children[0] as Sprite
}

test('night opacity uses the HUD boundaries and unknown samples leave the world unchanged', () => {
  for (const phase of [null, Number.NaN, Number.POSITIVE_INFINITY, -0.1, 1]) assert.equal(getNightOpacity(phase), 0)
  for (const hour of [0, 4, 5, 22, 23.999]) assert.equal(getNightOpacity(hour / 24), 1)
  for (const hour of [6, 12, 21]) assert.equal(getNightOpacity(hour / 24), 0)
  near(getNightOpacity(5.5 / 24), 0.5)
  near(getNightOpacity(21.5 / 24), 0.5)
})

test('light definitions reject malformed data and resolve independent scalar snapshots', () => {
  for (const definition of [null, [], {}, { radius: 0 }, { radius: Number.NaN }, { radius: 1, other: true },
    { radius: 1, offset: [1] }, { radius: 1, offset: [0, Infinity] }, { radius: 1, tint: 0x1000000 },
    { radius: 1, tint: 2.5 }, { radius: 1, strength: -0.1 }, { radius: 1, strength: 1.1 },
    { radius: 1, flicker: NaN }, { radius: 1, flicker: 1.1 },
    { radius: 1, flickerHz: 0 }, { radius: 1, flickerHz: -1 }, { radius: 1, flickerHz: NaN },
    { radius: 1, flickerHz: Infinity }, { radius: 1, flickerHz: '6' },
    { radius: 1, flickerFastWeight: -0.1 }, { radius: 1, flickerFastWeight: 1.1 },
    { radius: 1, flickerFastWeight: NaN }, { radius: 1, flickerFastWeight: Infinity },
    { radius: 1, sway: [1] }, { radius: 1, sway: [-1, 0] }, { radius: 1, sway: [0, -1] },
    { radius: 1, sway: [0, NaN] }, { radius: 1, sway: [Infinity, 0] }, { radius: 1, sway: '8,3' },
    { radius: 1, swayHz: 0 }, { radius: 1, swayHz: -1 }, { radius: 1, swayHz: NaN },
    { radius: 1, swayHz: Infinity }, { radius: 1, swayHz: '1.2' }]) {
    assert.throws(() => validateLightDefinition(definition), /Invalid light:/)
  }
  assert.deepEqual(resolveLightDefinition({ radius: 160 }), {
    radius: 160, offsetX: 0, offsetY: 0, tint: 0xffffff, strength: 1, flicker: 0,
    flickerHz: 4, flickerFastWeight: 0.25,
    swayX: 0, swayY: 0, swayHz: 1,
  })
  const definition = {
    radius: 160, offset: [0, -12] as [number, number], tint: 0xffe6cc, strength: 0.96, flicker: 0.04,
    sway: [8, 3] as [number, number], swayHz: 1.2,
  }
  const resolved = resolveLightDefinition(definition)
  definition.offset[1] = -100
  definition.radius = 1000
  definition.sway[0] = 100
  assert.equal(resolved.offsetY, -12)
  assert.equal(resolved.radius, 160)
  assert.equal(resolved.swayX, 8)
  assert.equal(resolved.swayY, 3)
  assert.equal(resolved.swayHz, 1.2)
  for (const weight of [0, 0.65, 1]) {
    const tuned = resolveLightDefinition({ radius: 160, flickerHz: 6, flickerFastWeight: weight })
    assert.equal(tuned.flickerHz, 6)
    assert.equal(tuned.flickerFastWeight, weight)
  }
})

test('light-map dimensions use integer physical pixels and stay bounded independently of DPR', () => {
  assert.deepEqual(getLightMapLayout(390, 844), { width: 195, height: 422, bytes: 195 * 422 * 4 })
  assert.deepEqual(getLightMapLayout(1920, 1080), { width: 512, height: 288, bytes: 512 * 288 * 4 })
  assert.deepEqual(getLightMapLayout(1, 1), { width: 1, height: 1, bytes: 4 })
  for (const [width, height] of [[3840, 2160], [3000, 3000], [731.4, 1700.2]]) {
    const layout = getLightMapLayout(width!, height!)
    assert.ok(Number.isInteger(layout.width) && Number.isInteger(layout.height))
    assert.ok(layout.width <= 512 && layout.height <= 512)
    assert.ok(layout.bytes <= 512 * 512 * 4)
  }
  assert.throws(() => getLightMapLayout(0, 100), /positive and finite/)
})

test('legacy radial mask has a plateau, a linear edge, and premultiplied white pixels', () => {
  for (const radius of [0, 0.25, 0.5]) assert.equal(radialLightFalloff(radius), 1)
  near(radialLightFalloff(0.75), 0.5)
  assert.equal(radialLightFalloff(1), 0)
  assert.equal(radialLightFalloff(2), 0)
  const pixels = createRadialLightPixels()
  assert.equal(pixels.length, 256 * 256 * 4)
  assert.equal(pixels[0], 0)
  assert.equal(pixels[(128 * 256 + 128) * 4], 255)
  for (let offset = 0; offset < pixels.length; offset += 4) {
    assert.equal(pixels[offset], pixels[offset + 3])
    assert.equal(pixels[offset + 1], pixels[offset + 3])
    assert.equal(pixels[offset + 2], pixels[offset + 3])
  }
})

test('additive overlapping light restores daytime without brightening above it', () => {
  for (const floor of NIGHT_FLOOR) {
    near(nightLightMultiplier(1, 0, floor), floor)
    near(nightLightMultiplier(0, 0.7, floor), 1)
    near(nightLightMultiplier(1, 0.7 + 0.7, floor), 1)
    for (const night of [0, 0.1, 0.5, 1]) {
      for (const light of [0, 0.1, 0.5, 0.9, 1, 10]) {
        const multiplier = nightLightMultiplier(night, light, floor)
        assert.ok(multiplier >= floor && multiplier <= 1)
      }
    }
  }
  const red = nightLightMultiplier(1, 0.96, NIGHT_FLOOR[0])
  const green = nightLightMultiplier(1, 0.96 * 230 / 255, NIGHT_FLOOR[1])
  const blue = nightLightMultiplier(1, 0.96 * 204 / 255, NIGHT_FLOOR[2])
  assert.ok(red > green && green > blue, 'the campfire restores subtly warmer colors')
})

test('flicker noise is stateless, source-specific, bounded, smooth, and does not loop each second', () => {
  const seed = lightFlickerSeed(123)
  const otherSeed = lightFlickerSeed(124)
  assert.equal(seed, lightFlickerSeed(123))
  assert.notEqual(seed, otherSeed)
  assert.equal(lightFlickerAmount(100, 0, seed), 0)
  const samples = Array.from({ length: 3001 }, (_, index) => lightFlickerAmount(index * 20, 0.1, seed))
  assert.ok(Math.min(...samples) < -0.04 && Math.max(...samples) > 0.04, 'the fire both dims and brightens')
  assert.ok(samples.some((value, index) => Math.abs(value - lightFlickerAmount(index * 20, 0.1, otherSeed)) > 0.04))
  assert.ok(samples.some((value, index) => Math.abs(value - lightFlickerAmount(index * 20 + 1000, 0.1, seed)) > 0.04))
  // Evaluate backwards to verify that seeking and skipped frames need no accumulated state.
  for (let index = samples.length - 1; index >= 0; index--) {
    const now = index * 20
    const amount = lightFlickerAmount(now, 0.1, seed)
    assert.equal(amount, samples[index])
    assert.ok(amount >= -0.1 && amount <= 0.1)
    assert.ok(Math.abs(amount - lightFlickerAmount(now + 1, 0.1, seed)) < 0.001)
  }
  for (let boundary = 250; boundary <= 60_000; boundary += 250) {
    assert.ok(Math.abs(lightFlickerAmount(boundary - 1, 0.1, seed) - lightFlickerAmount(boundary + 1, 0.1, seed)) < 0.001)
  }
})

test('geometry and invalid content bypass the 15 Hz flicker deadline without catch-up work', () => {
  assert.equal(shouldRefreshLightMap(10, 0, false, false, false), true)
  assert.equal(shouldRefreshLightMap(10, 0, true, true, false), true)
  assert.equal(shouldRefreshLightMap(10, 0, true, false, true), false)
  assert.equal(shouldRefreshLightMap(LIGHT_FLICKER_PERIOD_MS, 0, true, false, true), true)
  assert.equal(shouldRefreshLightMap(100_000, 0, true, false, false), false)
})

test('halo sway is bounded, smooth, deterministic, source-specific, and has its own frequency', () => {
  const seed = lightFlickerSeed(123)
  const samples = Array.from({ length: 1001 }, (_, index) => lightSwayOffset(index * 20, 8, seed, 1.2))
  assert.ok(Math.min(...samples) < -3 && Math.max(...samples) > 3, 'the halo sways to both sides')
  assert.ok(samples.some((value, index) => Math.abs(value - lightSwayOffset(index * 20, 8, lightFlickerSeed(124), 1.2)) > 3))
  for (let index = samples.length - 1; index >= 0; index--) {
    const now = index * 20
    const offset = lightSwayOffset(now, 8, seed, 1.2)
    assert.equal(offset, samples[index])
    assert.ok(offset >= -8 && offset <= 8)
    assert.ok(Math.abs(offset - lightSwayOffset(now + 1, 8, seed, 1.2)) < 0.05)
    near(lightSwayOffset(now, 8, seed, 2), lightSwayOffset(now * 2, 8, seed))
    assert.equal(lightSwayOffset(now, 0, seed, 1.2), 0)
  }
})

test('sway without brightness flicker respects the refresh limit and follows camera and owner transforms', context => {
  const f = fixture(context)
  f.world.position.set(20, 30)
  f.world.scale.set(2, 0.5)
  f.manager.register(123, f.owner, { radius: 40, offset: [5, -2], strength: 0.7, sway: [8, 3], swayHz: 1.2 })
  f.manager.update(0, 0, f.world, 320, 240)
  const sprite = firstSource(f.passes[0]!)
  const firstX = sprite.x
  const firstY = sprite.y
  for (let now = 1; now <= 66; now++) f.manager.update(now, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 1)
  near(sprite.x, firstX)
  near(sprite.y, firstY)
  let minX = Infinity
  let maxX = -Infinity
  let minY = Infinity
  let maxY = -Infinity
  for (let now = 67; now < 20_000; now += 67) {
    f.manager.update(now, 0, f.world, 320, 240)
    assert.ok(sprite.x >= 174 && sprite.x <= 206)
    assert.ok(sprite.y >= 57.5 && sprite.y <= 60.5)
    near(sprite.alpha, 0.7)
    near(sprite.width, 160)
    near(sprite.height, 40)
    minX = Math.min(minX, sprite.x)
    maxX = Math.max(maxX, sprite.x)
    minY = Math.min(minY, sprite.y)
    maxY = Math.max(maxY, sprite.y)
  }
  assert.ok(maxX - minX > 8 && maxY - minY > 1, 'both axes move while brightness and radius stay constant')
  const localX = (sprite.x - 20) / 2
  const localY = (sprite.y - 30) / 0.5
  const draws = f.passes.length
  const lastRefresh = (draws - 1) * 67
  f.world.x += 10
  f.world.scale.set(4, 1)
  f.manager.update(lastRefresh, 0, f.world, 320, 240)
  assert.equal(f.passes.length, draws + 1)
  near(sprite.x, 30 + localX * 4)
  near(sprite.y, 30 + localY)
  const beforeMove = sprite.x
  f.owner.x++
  f.manager.update(lastRefresh, 0, f.world, 320, 240)
  assert.equal(f.passes.length, draws + 2)
  near(sprite.x, beforeMove + 4)
  near(f.owner.x, 81)
  near(f.owner.y, 60)
  assert.equal(f.passes[0]!.target, f.passes.at(-1)!.target)
})

test('viewport selection includes the full sway envelope even without brightness flicker', context => {
  const f = fixture(context)
  f.owner.visible = false
  f.manager.register(123, f.owner, { radius: 40, sway: [8, 3] })
  for (const [x, y] of [[-47.5, 60], [367.5, 60], [80, -42.5], [80, 282.5]]) {
    f.owner.position.set(x, y)
    f.manager.update(0, 0, f.world, 320, 240)
    assert.equal(f.manager.getMetrics().intersecting, 1)
  }
  for (const [x, y] of [[-48.1, 60], [368.1, 60], [80, -43.1], [80, 283.1]]) {
    f.owner.position.set(x, y)
    f.manager.update(1, 0, f.world, 320, 240)
    assert.equal(f.manager.getMetrics().intersecting, 0)
    assert.equal(getUniforms(f.manager).uHasLights, 0)
  }
})

test('authored flicker frequency scales time and fast-noise weight makes changes sharper', () => {
  const seed = lightFlickerSeed(123)
  let gentleChange = 0
  let sharpChange = 0
  for (let now = 0; now < 60_000; now += 67) {
    near(lightFlickerAmount(now, 0.2, seed, 8), lightFlickerAmount(now * 2, 0.2, seed))
    const gentle = lightFlickerAmount(now, 0.2, seed)
    const sharp = lightFlickerAmount(now, 0.2, seed, 6, 0.65)
    gentleChange += Math.abs(gentle - lightFlickerAmount(now + 67, 0.2, seed))
    sharpChange += Math.abs(sharp - lightFlickerAmount(now + 67, 0.2, seed, 6, 0.65))
    for (const weight of [0, 0.65, 1]) {
      const amount = lightFlickerAmount(now, 0.2, seed, 6, weight)
      assert.ok(amount >= -0.2 && amount <= 0.2)
      assert.equal(lightFlickerAmount(now, 0, seed, 6, weight), 0)
    }
  }
  assert.ok(sharpChange > gentleChange * 1.5, 'the campfire preset changes faster between light-map refreshes')
})

test('registered light definitions pass frequency and fast-noise weight into the rendered light', context => {
  const f = fixture(context)
  const definition = { radius: 40, strength: 0.7, flicker: 0.2 }
  f.manager.register(123, f.owner, definition)
  f.manager.update(517, 0, f.world, 320, 240)
  const gentleAlpha = firstSource(f.passes[0]!).alpha
  f.manager.register(123, f.owner, { ...definition, flickerHz: 6, flickerFastWeight: 0.65 })
  f.manager.update(517, 0, f.world, 320, 240)
  const sprite = firstSource(f.passes[1]!)
  const flicker = lightFlickerAmount(517, 0.2, lightFlickerSeed(123), 6, 0.65)
  near(sprite.alpha, 0.7 * (1 + flicker))
  near(sprite.width, 80 * (1 + flicker / 4))
  assert.notEqual(sprite.alpha, gentleAlpha)
  f.manager.update(583, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2, 'a faster authored frequency still respects the 15 Hz refresh limit')
})

test('daylight and ambient-only night need no map allocation; entry into night redraws immediately', context => {
  const f = fixture(context)
  f.manager.update(0, 0, f.world, 320, 240)
  assert.equal(f.manager.getContainer().visible, true)
  assert.equal(getUniforms(f.manager).uHasLights, 0)
  assert.equal(f.manager.getMetrics().mapBytes, 0)
  assert.equal(f.passes.length, 0)
  f.manager.register(1, f.owner, { radius: 40 })
  f.manager.update(1, 0.5, f.world, 320, 240)
  assert.equal(f.manager.getContainer().visible, false)
  assert.equal(f.manager.getMetrics().mapBytes, 0)
  f.manager.update(2, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 1)
  assert.equal(f.manager.getMetrics().mapBytes, 160 * 120 * 4)
  assert.equal(f.manager.getContainer().blendMode, 'multiply')
  assert.equal(f.manager.getContainer().eventMode, 'none')
  f.manager.update(3, 0.5, f.world, 320, 240)
  f.manager.update(4, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
})

test('offscreen owner culling does not remove an intersecting halo and real camera offsets stay aligned', context => {
  const f = fixture(context)
  f.owner.visible = false
  f.owner.position.set(-15, 5)
  f.world.position.set(20, 30)
  f.world.scale.set(2)
  f.manager.register(2, f.owner, { radius: 40, offset: [5, -2] })
  f.manager.update(0, 0, f.world, 320, 240)
  assert.equal(f.manager.getMetrics().intersecting, 1)
  const pass = f.passes[0]!
  const sprite = firstSource(pass)
  near(sprite.x, 0)
  near(sprite.y, 36)
  near(sprite.width, 160)
  near(sprite.height, 160)
  assert.equal(sprite.blendMode, 'add')
  assert.equal(sprite.eventMode, 'none')
  assert.equal(sprite.texture.source.alphaMode, 'premultiplied-alpha')
  near(pass.container.scale.x, 0.5)
  near(pass.container.scale.y, 0.5)
  f.owner.x = -1000
  f.manager.update(1, 0, f.world, 320, 240)
  assert.equal(f.manager.getMetrics().intersecting, 0)
  assert.equal(getUniforms(f.manager).uHasLights, 0)
  assert.equal(f.passes.length, 1)
  f.owner.x = -15
  f.manager.update(2, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2, 'zero to light cannot reuse stale map content')
})

test('stationary flicker is throttled but geometry changes redraw immediately and reset its deadline', context => {
  const f = fixture(context)
  f.manager.register(3, f.owner, { radius: 40, strength: 0.9, flicker: 0.1 })
  f.manager.update(100, 0, f.world, 320, 240)
  const sprite = firstSource(f.passes[0]!)
  const firstAlpha = sprite.alpha
  const firstWidth = sprite.width
  for (let now = 101; now <= 166; now++) f.manager.update(now, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 1)
  assert.equal(sprite.alpha, firstAlpha)
  assert.equal(sprite.width, firstWidth)
  assert.equal(sprite.height, firstWidth)
  f.owner.x++
  f.manager.update(166, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
  near(sprite.x, f.owner.x)
  const movedAlpha = sprite.alpha
  const movedWidth = sprite.width
  f.manager.update(232, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
  assert.equal(sprite.alpha, movedAlpha)
  assert.equal(sprite.width, movedWidth)
  f.manager.update(233, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 3)
  assert.notEqual(sprite.alpha, movedAlpha)
  assert.notEqual(sprite.width, movedWidth)
  assert.equal(f.passes[0], f.passes[1], 'render options are reused instead of allocated per refresh')
  assert.equal(f.passes[0]!.target, f.passes[2]!.target, 'the RT is reused')
})

test('fire brightness and radius vary together within bounds and zoom redraws immediately', context => {
  const f = fixture(context)
  f.manager.register(123, f.owner, { radius: 160, strength: 0.9, flicker: 0.1 })
  let minimumWidth = Infinity
  let maximumWidth = 0
  for (let now = 0; now < 30_000; now += 67) {
    f.manager.update(now, 0, f.world, 320, 240)
    const sprite = firstSource(f.passes[0]!)
    assert.ok(sprite.alpha >= 0.81 && sprite.alpha <= 0.99)
    assert.ok(sprite.width >= 312 && sprite.width <= 328)
    near(sprite.height, sprite.width)
    near(sprite.width / 320 - 1, (sprite.alpha / 0.9 - 1) / 4)
    minimumWidth = Math.min(minimumWidth, sprite.width)
    maximumWidth = Math.max(maximumWidth, sprite.width)
  }
  assert.ok(maximumWidth - minimumWidth > 4, 'the radius visibly breathes over time')
  const draws = f.passes.length
  const sprite = firstSource(f.passes[0]!)
  const previousWidth = sprite.width
  const previousAlpha = sprite.alpha
  f.world.scale.set(2, 0.5)
  const lastRefresh = (draws - 1) * 67
  f.manager.update(lastRefresh, 0, f.world, 320, 240)
  assert.equal(f.passes.length, draws + 1)
  near(sprite.width, previousWidth * 2)
  near(sprite.height, previousWidth * 0.5)
  near(sprite.alpha, previousAlpha)
  assert.equal(f.passes[0]!.target, f.passes.at(-1)!.target)
})

test('flickering lights use their maximum radius for viewport selection on every edge', context => {
  const f = fixture(context)
  f.owner.visible = false
  f.manager.register(123, f.owner, { radius: 40, strength: 0.9, flicker: 0.1 })
  for (const [x, y] of [[-40.5, 60], [360.5, 60], [80, -40.5], [80, 280.5]]) {
    f.owner.position.set(x, y)
    f.manager.update(0, 0, f.world, 320, 240)
    assert.equal(f.manager.getMetrics().intersecting, 1, 'the expanded halo is selected even outside the base radius')
  }
  for (const [x, y] of [[-41.1, 60], [361.1, 60], [80, -41.1], [80, 281.1]]) {
    f.owner.position.set(x, y)
    f.manager.update(1, 0, f.world, 320, 240)
    assert.equal(f.manager.getMetrics().intersecting, 0)
    assert.equal(getUniforms(f.manager).uHasLights, 0)
  }
})

test('full-amplitude flicker clamps brightness without collapsing the radius', context => {
  const f = fixture(context)
  f.manager.register(123, f.owner, { radius: 40, strength: 1, flicker: 1 })
  let observedClamp = false
  for (let now = 0; now < 10_000; now += 67) {
    f.manager.update(now, 0, f.world, 320, 240)
    const sprite = firstSource(f.passes[0]!)
    assert.ok(sprite.alpha >= 0 && sprite.alpha <= 1)
    assert.ok(sprite.width >= 60 && sprite.width <= 100)
    observedClamp ||= sprite.alpha === 1
  }
  assert.equal(observedClamp, true)
})

test('static lights reuse the map while dusk opacity changes; pan, zoom, and viewport changes redraw', context => {
  const f = fixture(context)
  f.manager.register(4, f.owner, { radius: 40 })
  f.manager.update(100, 21.25 / 24, f.world, 320, 240)
  f.manager.update(100_000, 21.75 / 24, f.world, 320, 240)
  assert.equal(f.passes.length, 1)
  near(getUniforms(f.manager).uNightOpacity, 0.75)
  f.world.x = 1
  f.manager.update(100_001, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
  f.world.scale.set(0.5)
  f.manager.update(100_002, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 3)
  f.manager.update(100_003, 0, f.world, 1920, 1080)
  assert.equal(f.passes.length, 4)
  assert.equal(f.manager.getMetrics().mapWidth, 512)
  assert.equal(f.manager.getMetrics().mapHeight, 288)
  const target = f.passes[0]!.target as RenderTexture
  assert.equal(target.source.pixelWidth, 512)
  assert.equal(target.source.pixelHeight, 288)
})

test('a capped-map resize refreshes the detached root matrix and preserves screen light anchor and radius', context => {
  const f = fixture(context)
  f.owner.position.set(320, 180)
  f.manager.register(40, f.owner, { radius: 40 })
  f.manager.update(0, 0, f.world, 640, 360)
  near(f.maskTransforms[0]!.a, 0.5)
  near(f.maskTransforms[0]!.d, 0.5)
  f.manager.update(1, 0, f.world, 1920, 1080)
  const transform = f.maskTransforms[1]!
  const metrics = f.manager.getMetrics()
  const sprite = firstSource(f.passes[1]!)
  near(transform.a, 512 / 1920)
  near(transform.d, 288 / 1080)
  near(transform.a * sprite.x / metrics.mapWidth * 1920, 320)
  near(transform.d * sprite.y / metrics.mapHeight * 1080, 180)
  near(transform.a * sprite.width / 2 / metrics.mapWidth * 1920, 40)
  near(transform.d * sprite.height / 2 / metrics.mapHeight * 1080, 40)
})

test('replacement, removal, reset and inactive/unknown-time exits cannot retain previous light', context => {
  const f = fixture(context)
  f.manager.register(5, f.owner, { radius: 40 })
  f.manager.update(0, 0, f.world, 320, 240)
  const oldSprite = firstSource(f.passes[0]!)
  const replacement = new Container({ x: 140, y: 100 })
  f.world.addChild(replacement)
  f.manager.register(5, replacement, { radius: 20 })
  assert.equal(oldSprite.destroyed, true)
  assert.equal(f.manager.getMetrics().sources, 1)
  f.manager.update(1, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
  near(firstSource(f.passes[1]!).x, 140)
  f.manager.update(2, null, f.world, 320, 240)
  assert.equal(f.manager.getContainer().visible, false)
  f.manager.update(3, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 3)
  f.manager.update(4, 0, f.world, 320, 240, false)
  assert.equal(f.manager.getContainer().visible, false)
  f.manager.update(5, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 4)
  f.manager.unregister(5)
  f.manager.update(6, 0, f.world, 320, 240)
  assert.equal(getUniforms(f.manager).uHasLights, 0)
  assert.equal(f.passes.length, 4)
  f.manager.clear()
  assert.equal(f.manager.getContainer().visible, false)
  assert.equal(f.manager.getMetrics().sources, 0)
  assert.equal(f.manager.getMetrics().redrawCount, 0)
})

test('context restoration invalidates an identical map and destruction releases owned GPU sources and listeners', context => {
  const f = fixture(context)
  f.manager.register(6, f.owner, { radius: 40 })
  f.manager.update(0, 0, f.world, 320, 240)
  const target = f.passes[0]!.target as RenderTexture
  const mapSource = target.source
  const radialSource = firstSource(f.passes[0]!).texture.source
  const geometryBuffers = (f.manager.getContainer() as Mesh).geometry.buffers.slice()
  f.canvas.dispatchEvent(new Event('webglcontextlost'))
  f.manager.update(1, 0, f.world, 320, 240)
  assert.equal(f.manager.getContainer().visible, false)
  assert.equal(f.passes.length, 1)
  f.canvas.dispatchEvent(new Event('webglcontextrestored'))
  f.manager.update(2, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
  const removals = context.mock.method(f.canvas, 'removeEventListener')
  f.manager.destroy()
  assert.equal(removals.mock.callCount(), 2)
  assert.equal(mapSource.destroyed, true)
  assert.equal(radialSource.destroyed, true)
  assert.ok(geometryBuffers.every(buffer => buffer.destroyed))
  assert.equal(f.manager.getContainer().destroyed, true)
  f.manager.destroy()
  assert.equal(removals.mock.callCount(), 2)
  assert.equal(f.manager.getMetrics().mapBytes, 0)
})
