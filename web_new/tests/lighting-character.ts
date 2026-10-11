import { Application, Container, Graphics, Sprite, Texture, Ticker, type WebGLRenderer } from 'pixi.js'
import { ObjectManager } from '../src/game/ObjectManager'
import { ActorRenderer } from '../src/game/actors/ActorRenderer'
import { COMMONER_ASSET_ID } from '../src/game/actors/config'
import { LightingManager } from '../src/game/lighting/LightingManager'
import { NIGHT_FLOOR } from '../src/game/lighting/model'
import { setWorldParams } from './serverConstantsFixture'

type RGB = [number, number, number]
interface FrameSample {
  dpr: number
  lights: number
  state: string
  actorUpdated: boolean
  mapRedraws: number
  ambient: RGB
  lit: RGB
}

const WIDTH = 720
const HEIGHT = 360
const DAY: RGB = [144, 156, 144]
const result = document.querySelector<HTMLPreElement>('#result')!
const sampleOutput = document.querySelector<HTMLPreElement>('#samples')!
const checks: string[] = []
const failures: string[] = []
const samples: FrameSample[] = []
const paint = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
const near = (a: RGB, b: readonly number[]) => a.every((value, channel) => Math.abs(value - b[channel]!) <= 2)

function assert(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message)
}

function check(value: boolean, message: string): void {
  checks.push(`${value ? 'PASS' : 'FAIL'} ${message}`)
  if (!value) failures.push(message)
  result.textContent = checks.join('\n')
  result.dataset.checks = String(checks.length)
}

async function checkScene(dpr: number): Promise<void> {
  const app = new Application()
  // Match Render.ts: a distinct opaque background exposes contamination of the
  // transparent-black light-map clear after Three.js touches the shared context.
  await app.init({ width: WIDTH, height: HEIGHT, resolution: dpr, autoDensity: true, background: '#353e67', antialias: false, preference: 'webgl' })
  app.stop()
  Ticker.shared.stop()
  document.querySelector('#preview')!.append(app.canvas)
  const pixi = app.renderer as WebGLRenderer
  const gl = pixi.gl
  const world = new Container()
  world.addChild(new Graphics().rect(-4096, -4096, 8192, 8192).fill(0x909c90))
  app.stage.addChild(world)
  const actors = new ActorRenderer(pixi)
  const lighting = new LightingManager(pixi)
  app.stage.addChild(lighting.getContainer())
  const objects = new ObjectManager()
  objects.setParentContainer(world)
  objects.setActorRenderer(actors)
  objects.setLightingManager(lighting)
  objects.setPlayerEntityId(101)
  setWorldParams(12, 128)
  objects.spawnObject({ entityId: 101, typeId: 1, resourcePath: 'player', position: { x: 0, y: 0 }, size: { x: 4, y: 4 } })
  const view = objects.getObject(101)!
  view.getContainer().position.set(520, 310)
  const sprite = view.getContainer().children.find((child) => child instanceof Sprite)
  assert(sprite instanceof Sprite, 'Production ObjectView must synchronously create an actor sprite')
  let nowMs = performance.now()
  const pixel = new Uint8Array(4)
  const readPixel = (x: number, y: number): RGB => {
    // readPixels does not submit an additional draw or alter blending state.
    // Reading the canvas immediately is also valid without preserveDrawingBuffer.
    gl.readPixels(Math.floor(x * dpr), gl.drawingBufferHeight - Math.floor(y * dpr) - 1, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, pixel)
    return [pixel[0]!, pixel[1]!, pixel[2]!]
  }
  const draw = (state: string, record = true): FrameSample => {
    objects.update()
    actors.render(nowMs += 100)
    lighting.update(nowMs, 0, world, WIDTH, HEIGHT)
    app.render()
    assert(gl.getParameter(gl.FRAMEBUFFER_BINDING) === null, 'The final stage render must target the canvas framebuffer')
    assert(gl.getError() === gl.NO_ERROR, `Unexpected GL error after ${state}`)
    const sample: FrameSample = {
      dpr, lights: lighting.getMetrics().sources, state, actorUpdated: actors.hasUpdatedPoses,
      mapRedraws: lighting.getMetrics().redrawCount, ambient: readPixel(650, 35), lit: readPixel(235, 180),
    }
    assert(gl.getError() === gl.NO_ERROR, `Unexpected GL error reading ${state}`)
    if (record) samples.push(sample)
    return sample
  }

  try {
    for (let attempt = 0; attempt < 600 && sprite.texture === Texture.EMPTY; attempt++) {
      draw('asset loading', false)
      await paint()
    }
    assert(sprite.texture !== Texture.EMPTY, 'The real GLB actor must finish loading')
    const catalog = await actors.cache.catalog
    const stride = catalog.manifests[COMMONER_ASSET_ID]!.clips.walk!.cycleDistanceTiles! * 12
    view.onMoved(3)
    view.onStopped()
    for (let frame = 0; frame < 20; frame++) draw('idle settling', false)
    check(sprite.texture.width === 128, `DPR ${dpr}: the production character has a live GPU texture`)

    for (const lightCount of [0, 1]) {
      if (lightCount) {
        const lightOwner = new Container()
        lightOwner.position.set(200, 180)
        world.addChild(lightOwner)
        lighting.register(201, lightOwner, { radius: 120, tint: 0xffe6cc, strength: 0.96, flicker: 0 })
        draw('build static light map', false)
      }
      const baseline = draw('cached idle baseline')
      const mapRedraws = lighting.getMetrics().redrawCount
      const expectedAmbient = DAY.map((channel, index) => Math.round(channel * NIGHT_FLOOR[index]!))
      check(!baseline.actorUpdated, `DPR ${dpr}, ${lightCount} lights: the idle reference reuses the cached character texture`)
      check(near(baseline.ambient, expectedAmbient), `DPR ${dpr}, ${lightCount} lights: idle night uses the specified ambient multiplier`)
      const sequence: FrameSample[] = []
      for (let cycle = 0; cycle < 3; cycle++) {
        sequence.push(draw(`cycle ${cycle}: cached idle`))
        for (let step = 0; step < 3; step++) {
          view.onMoved(3, stride / 8)
          sequence.push(draw(`cycle ${cycle}: moving ${step}`))
        }
        view.onStopped()
        for (let frame = 0; frame < 8; frame++) sequence.push(draw(`cycle ${cycle}: stopping ${frame}`))
        sequence.push(draw(`cycle ${cycle}: cached idle after stop`))
      }
      const dirty = sequence.filter((sample) => sample.state.includes('moving') && sample.actorUpdated)
      const cached = sequence.filter((sample) => sample.state.includes('cached idle') && !sample.actorUpdated)
      check(dirty.length > 0 && cached.length > 0, `DPR ${dpr}, ${lightCount} lights: frames alternate actual Three.js pose rendering with cached Pixi frames`)
      const ambientDrift = sequence.find((sample) => !near(sample.ambient, baseline.ambient))
      check(!ambientDrift, `DPR ${dpr}, ${lightCount} lights: movement preserves ambient pixels${ambientDrift ? `; idle=${baseline.ambient}, ${ambientDrift.state}=${ambientDrift.ambient}` : ''}`)
      const lightDrift = sequence.find((sample) => !near(sample.lit, baseline.lit))
      check(!lightDrift, `DPR ${dpr}, ${lightCount} lights: movement preserves lit pixels${lightDrift ? `; idle=${baseline.lit}, ${lightDrift.state}=${lightDrift.lit}` : ''}`)
      check(lighting.getMetrics().redrawCount === mapRedraws, `DPR ${dpr}, ${lightCount} lights: stationary geometry retains its light-map cache throughout movement`)
      if (lightCount) check(baseline.lit[0] > baseline.ambient[0] + 50, `DPR ${dpr}: the cached source illuminates the sampled world pixel`)

      if (lightCount) {
        // Follow-camera movement dirties the map exactly when a new actor pose
        // causes the Three/Pixi handoff. Keep sampled pixels within the uniform
        // ground and the flat inner light radius so their expected RGB is fixed.
        const refreshed: FrameSample[] = []
        for (let cycle = 0; cycle < 3; cycle++) {
          world.x += 2
          refreshed.push(draw(`camera cycle ${cycle}: cached idle + map refresh`))
          for (let step = 0; step < 3; step++) {
            view.onMoved(3, stride / 8)
            world.x += 2
            refreshed.push(draw(`camera cycle ${cycle}: moving ${step} + map refresh`))
          }
          view.onStopped()
          for (let frame = 0; frame < 8; frame++) draw(`camera cycle ${cycle}: stop settling`, false)
        }
        check(refreshed.some((sample) => sample.actorUpdated) && refreshed.some((sample) => !sample.actorUpdated),
          `DPR ${dpr}: camera refresh covers both dirty Three.js poses and cached character frames`)
        const refreshedAmbientDrift = refreshed.find((sample) => !near(sample.ambient, baseline.ambient))
        check(!refreshedAmbientDrift, `DPR ${dpr}: camera-follow movement preserves ambient pixels during map refresh${refreshedAmbientDrift ? `; idle=${baseline.ambient}, ${refreshedAmbientDrift.state}=${refreshedAmbientDrift.ambient}` : ''}`)
        const refreshedLightDrift = refreshed.find((sample) => !near(sample.lit, baseline.lit))
        check(!refreshedLightDrift, `DPR ${dpr}: camera-follow movement preserves lit pixels during map refresh${refreshedLightDrift ? `; idle=${baseline.lit}, ${refreshedLightDrift.state}=${refreshedLightDrift.lit}` : ''}`)
        check(lighting.getMetrics().redrawCount === mapRedraws + refreshed.length,
          `DPR ${dpr}: every camera movement redraws the map immediately`)

        // Pixel picking binds an external framebuffer and resets Pixi state too.
        // Test it with a cached actor so the following frame cannot accidentally
        // repair that handoff by running the Three.js render pass again.
        view.hitTestRmbScreenPoint(world.x + view.getContainer().x, view.getContainer().y - 60,
          (x, y) => ({ x, y }))
        const beforePickingRefresh = lighting.getMetrics().redrawCount
        world.x += 2
        const afterPicking = draw('pixel picking + cached idle + map refresh')
        check(!afterPicking.actorUpdated && afterPicking.mapRedraws === beforePickingRefresh + 1,
          `DPR ${dpr}: picking is followed by an immediate light-map refresh with a cached actor`)
        check(near(afterPicking.ambient, baseline.ambient),
          `DPR ${dpr}: character pixel picking preserves subsequent ambient brightness`)
        check(near(afterPicking.lit, baseline.lit),
          `DPR ${dpr}: character pixel picking preserves subsequent light brightness`)
      }
    }
  } finally {
    objects.destroy()
    lighting.destroy()
    actors.destroy()
    app.destroy(true, { children: true })
  }
}

async function main(): Promise<void> {
  for (const dpr of [1, 2]) await checkScene(dpr)
  sampleOutput.textContent = JSON.stringify(samples, null, 2)
  result.dataset.status = failures.length ? 'failed' : 'passed'
  result.dataset.failures = JSON.stringify(failures)
  result.textContent += failures.length ? `\n${failures.length} checks failed.` : '\nAll checks passed.'
}

void main().catch((error: unknown) => {
  const message = error instanceof Error ? error.stack ?? error.message : String(error)
  result.dataset.status = 'failed'
  result.dataset.failures = JSON.stringify([message])
  result.textContent += `\nERROR ${message}`
  sampleOutput.textContent = JSON.stringify(samples, null, 2)
})
