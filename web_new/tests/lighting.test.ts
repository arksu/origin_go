import assert from 'node:assert/strict'
import { test, type TestContext } from 'node:test'
import { Container, DOMAdapter, Mesh, type RenderOptions, RenderTexture, Sprite, type WebGLRenderer } from 'pixi.js'
import { LightingManager } from '../src/game/lighting/LightingManager'
import {
  createRadialLightPixels, getLightMapLayout, getNightOpacity, LIGHT_FLICKER_PERIOD_MS,
  lightFlickerPhase, lightFlickerStrength, NIGHT_FLOOR, nightLightMultiplier,
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
    { radius: 1, flicker: NaN }, { radius: 1, flicker: 1.1 }]) {
    assert.throws(() => validateLightDefinition(definition), /Invalid light:/)
  }
  assert.deepEqual(resolveLightDefinition({ radius: 160 }), {
    radius: 160, offsetX: 0, offsetY: 0, tint: 0xffffff, strength: 1, flicker: 0,
  })
  const definition = { radius: 160, offset: [0, -12] as [number, number], tint: 0xffe6cc, strength: 0.96, flicker: 0.04 }
  const resolved = resolveLightDefinition(definition)
  definition.offset[1] = -100
  definition.radius = 1000
  assert.equal(resolved.offsetY, -12)
  assert.equal(resolved.radius, 160)
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

test('flicker is deterministic, source-specific, smooth, and clamps Sprite strength', () => {
  const phase = lightFlickerPhase(123)
  assert.equal(phase, lightFlickerPhase(123))
  assert.notEqual(phase, lightFlickerPhase(124))
  assert.equal(lightFlickerStrength(100, 0.7, 0, phase), 0.7)
  let observedClamp = false
  for (let now = 0; now < 10_000; now += 20) {
    const strength = lightFlickerStrength(now, 0.96, 0.04, phase)
    assert.ok(strength >= 0.96 * 0.96 && strength <= 0.96 * 1.04)
    assert.ok(Math.abs(strength - lightFlickerStrength(now + 1, 0.96, 0.04, phase)) < 0.001)
    const strongest = lightFlickerStrength(now, 1, 1, phase)
    assert.ok(strongest >= 0 && strongest <= 1)
    observedClamp ||= strongest === 1
  }
  assert.equal(observedClamp, true)
})

test('geometry and invalid content bypass the 15 Hz flicker deadline without catch-up work', () => {
  assert.equal(shouldRefreshLightMap(10, 0, false, false, false), true)
  assert.equal(shouldRefreshLightMap(10, 0, true, true, false), true)
  assert.equal(shouldRefreshLightMap(10, 0, true, false, true), false)
  assert.equal(shouldRefreshLightMap(LIGHT_FLICKER_PERIOD_MS, 0, true, false, true), true)
  assert.equal(shouldRefreshLightMap(100_000, 0, true, false, false), false)
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
  f.manager.register(3, f.owner, { radius: 40, strength: 0.96, flicker: 0.04 })
  f.manager.update(100, 0, f.world, 320, 240)
  const firstAlpha = firstSource(f.passes[0]!).alpha
  f.manager.update(150, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 1)
  assert.equal(firstSource(f.passes[0]!).alpha, firstAlpha)
  f.owner.x++
  f.manager.update(152, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
  f.manager.update(218, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 2)
  f.manager.update(219, 0, f.world, 320, 240)
  assert.equal(f.passes.length, 3)
  assert.equal(f.passes[0], f.passes[1], 'render options are reused instead of allocated per refresh')
  assert.equal(f.passes[0]!.target, f.passes[2]!.target, 'the RT is reused')
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
