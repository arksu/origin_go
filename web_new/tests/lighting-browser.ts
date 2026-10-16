import { Application, Container, Graphics, Point, Rectangle, Text, Ticker, type WebGLRenderer } from 'pixi.js'
import { LightingManager } from '../src/game/lighting/LightingManager'
import { ObjectManager } from '../src/game/ObjectManager'
import { ResourceLoader } from '../src/game/ResourceLoader'
import { cullingController } from '../src/game/culling'
import { coordScreen2Game } from '../src/game/utils/coordConvert'
import { setWorldParams } from './serverConstantsFixture'
import { resolveRenderResolution } from '../src/game/renderResolution'
import { ECONOMY_RENDERING_STORAGE_KEY, useEconomyRendering } from '../src/composables/useEconomyRendering'

type LightDefinition = Parameters<LightingManager['register']>[2]
type RGB = [number, number, number]
const result = document.querySelector<HTMLPreElement>('#result')!
const benchmarkResult = document.querySelector<HTMLPreElement>('#benchmark-result')!
const checks: string[] = []
const WIDTH = 640
const HEIGHT = 360
const frame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
const settle = async () => { for (let i = 0; i < 12; i++) await Promise.resolve() }
function check(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
  checks.push(`PASS: ${message}`)
  result.textContent = checks.join('\n')
  result.dataset.checks = String(checks.length)
}
const near = (a: RGB, b: RGB, tolerance = 5) => a.every((value, channel) => Math.abs(value - b[channel]!) <= tolerance)
const brightness = (value: RGB) => value.reduce((sum, channel) => sum + channel, 0)

async function createScene(resolution: number) {
  const app = new Application()
  await app.init({ width: WIDTH, height: HEIGHT, resolution, preference: 'webgl', antialias: false, autoDensity: true, background: '#909c90' })
  app.stop()
  Ticker.shared.stop()
  const world = new Container()
  const ground = new Graphics().rect(-4096, -4096, 8192, 8192).fill(0x909c90)
  world.addChild(ground)
  app.stage.addChild(world)
  const lighting = new LightingManager(app.renderer as WebGLRenderer)
  app.stage.addChild(lighting.getContainer())
  const worldHud = new Container()
  worldHud.addChild(new Graphics().rect(42, 24, 35, 16).fill(0xebc8b0))
  const label = new Text({ text: 'Campfire', resolution: 2, style: { fontFamily: 'Arial', fontSize: 16, fill: 0xffffff, stroke: { color: 0x000000, width: 2 } } })
  label.position.set(42, 45)
  worldHud.addChild(label)
  app.stage.addChild(worldHud)
  const hud = new Graphics().rect(WIDTH - 45, 10, 32, 32).fill(0xe8d8b8)
  app.stage.addChild(hud)
  const objects = new ObjectManager()
  objects.setParentContainer(world)
  objects.setLightingManager(lighting)
  let width = WIDTH
  let height = HEIGHT
  const draw = (phase: number | null = 0, active = true, nowMs = performance.now()) => {
    objects.update()
    worldHud.position.copyFrom(world.position)
    worldHud.scale.copyFrom(world.scale)
    lighting.update(nowMs, phase, world, width, height, active)
    app.render()
  }
  const pixels = () => app.renderer.extract.pixels({ target: app.stage, frame: new Rectangle(0, 0, width, height), resolution: 1 })
  const pixel = (x: number, y: number): RGB => {
    const output = pixels()
    const index = (Math.round(y) * output.width + Math.round(x)) * 4
    return [output.pixels[index]!, output.pixels[index + 1]!, output.pixels[index + 2]!]
  }
  const owner = (id: number, x: number, y: number, definition: LightDefinition) => {
    const container = new Container()
    container.position.set(x, y)
    world.addChild(container)
    lighting.register(id, container, definition)
    return container
  }
  const resize = (nextWidth: number, nextHeight: number) => {
    width = nextWidth
    height = nextHeight
    app.renderer.resize(width, height)
  }
  const dispose = () => {
    objects.clear()
    lighting.destroy()
    app.destroy(true, { children: true })
  }
  return { app, world, lighting, objects, label, draw, pixels, pixel, owner, resize, dispose }
}
type Scene = Awaited<ReturnType<typeof createScene>>

async function checkPixels(resolution: number) {
  const scene = await createScene(resolution)
  const { world, lighting, draw, pixel } = scene
  try {
    draw(12 / 24)
    const day = pixel(200, 180)
    const hud = pixel(610, 25)
    const worldHud = pixel(50, 30)
    draw(21.5 / 24)
    const dusk = pixel(200, 180)
    draw(0)
    const night = pixel(200, 180)
    check(brightness(night) < brightness(dusk) && brightness(dusk) < brightness(day), `DPR ${resolution}: GPU pixels darken continuously from day through dusk to night`)
    const floors: RGB = [0.24, 0.28, 0.36]
    check(night.every((value, channel) => Math.abs(value / day[channel]! - floors[channel]!) < 0.04), `DPR ${resolution}: night multiply preserves the specified blue-grey ambient floor`)
    check(near(pixel(610, 25), hud), `DPR ${resolution}: HUD above the lighting pass retains its daytime colors`)
    check(near(pixel(50, 30), worldHud), `DPR ${resolution}: world HUD above the lighting pass stays bright`)
    draw(null)
    check(near(pixel(200, 180), day), `DPR ${resolution}: missing calendar sample keeps the world readable in daylight`)
    draw(0, false)
    check(near(pixel(200, 180), day), `DPR ${resolution}: inactive world bypasses darkness`)

    const fire: LightDefinition = { radius: 120, tint: 0xffd090, strength: 1, flicker: 0 }
    const first = scene.owner(10001, 200, 180, fire)
    draw(0)
    const warm = pixel(280, 180)
    check(brightness(warm) > brightness(night), `DPR ${resolution}: a light restores underlying world pixels`)
    check(warm[0] / day[0] > warm[2] / day[2] + 0.05, `DPR ${resolution}: light restoration is warm instead of a white cutout`)
    scene.owner(10002, 200, 180, fire)
    draw(0)
    const overlap = pixel(280, 180)
    check(brightness(overlap) >= brightness(warm), `DPR ${resolution}: overlapping lights combine`)
    const litPixels = scene.pixels()
    draw(12 / 24)
    const originalPixels = scene.pixels()
    let brighterThanOriginal = false
    for (let index = 0; index < litPixels.pixels.length; index += 4) {
      for (let channel = 0; channel < 3; channel++) {
        if (litPixels.pixels[index + channel]! > originalPixels.pixels[index + channel]! + 2) brighterThanOriginal = true
      }
    }
    check(!brighterThanOriginal, `DPR ${resolution}: overlapping lights saturate without brightening above the original scene`)
    draw(0)
    const centerBeforePan = pixel(200, 180)
    world.x = 80
    draw(0)
    check(near(pixel(280, 180), centerBeforePan), `DPR ${resolution}: panning moves the illumination with its owner`)
    world.scale.set(1.5)
    draw(0)
    check(brightness(pixel(380, 270)) > brightness(night), `DPR ${resolution}: zoom scales the source position and radius`)
    world.position.set(0, 0)
    world.scale.set(1)
    lighting.clear()
    first.destroy()
    const hidden = scene.owner(10003, -20, 180, { ...fire, radius: 160 })
    hidden.visible = false
    draw(0)
    check(lighting.getMetrics().intersecting === 1 && brightness(pixel(30, 180)) > brightness(night), `DPR ${resolution}: a culled offscreen owner still lights pixels inside the viewport`)
    hidden.x = -1000
    draw(0)
    check(lighting.getMetrics().intersecting === 0 && near(pixel(30, 180), night), `DPR ${resolution}: sources outside their full light radius do not contribute`)
    lighting.clear()
    scene.owner(10004, 160, 200, fire)
    draw(0)
    const cached = lighting.getMetrics().redrawCount
    draw(0)
    check(lighting.getMetrics().redrawCount === cached && cached > 0, `DPR ${resolution}: a stationary nonflickering source reuses its rendered map`)
    scene.owner(10005, 300, 200, { ...fire, strength: 0.7, flicker: 0.04 })
    lighting.update(2_000_000, 0, world, WIDTH, HEIGHT)
    const flickerStart = lighting.getMetrics().redrawCount
    for (let elapsed = 1; elapsed <= 65; elapsed++) lighting.update(2_000_000 + elapsed, 0, world, WIDTH, HEIGHT)
    check(lighting.getMetrics().redrawCount === flickerStart, `DPR ${resolution}: stationary flicker does not redraw faster than 15 Hz`)
    lighting.update(2_000_067, 0, world, WIDTH, HEIGHT)
    check(lighting.getMetrics().redrawCount === flickerStart + 1, `DPR ${resolution}: flicker redraw resumes after its bounded interval`)
    scene.resize(777, 401)
    draw(0)
    const resized = lighting.getMetrics()
    check(resized.mapWidth <= 512 && resized.mapHeight <= 512 && resized.mapBytes <= 512 * 512 * 4, `DPR ${resolution}: resize stays within the 512px RGB-map memory bound`)
    check(near(pixel(700, 350), night), `DPR ${resolution}: resized viewport has complete darkening coverage`)
    const beforeLargeResize = pixel(160, 200)
    scene.resize(1920, 1080)
    draw(0)
    const large = lighting.getMetrics()
    check(large.mapWidth === 512 && large.mapHeight === 288 && near(pixel(160, 200), beforeLargeResize, 7),
      `DPR ${resolution}: large lit resize preserves source coordinates while changing map scale`)
    check(near(pixel(1800, 1000), night), `DPR ${resolution}: large viewport corners keep the ambient floor`)
    lighting.clear()
    draw(null)
    check(lighting.getMetrics().sources === 0 && near(pixel(200, 180), day), `DPR ${resolution}: reset removes all lights and restores unsynchronized daylight`)
  } finally { scene.dispose() }
}

async function preloadCampfire() {
  for (const state of ['burning', 'unlit']) {
    const definition = ResourceLoader.getResourceDef(`campfire/${state}`)
    if (!definition) throw new Error(`Missing campfire/${state}`)
    await Promise.all(definition.layers.flatMap((layer) => layer.frames?.map((image) => ResourceLoader.loadTexture(image.img)) ?? (layer.img ? [ResourceLoader.loadTexture(layer.img)] : [])))
  }
}

async function checkFireFlicker(resolution: number) {
  const scene = await createScene(resolution)
  try {
    const fire = ResourceLoader.getResourceDef('campfire/burning')?.light
    if (!fire) throw new Error('Production burning campfire is missing its light definition')
    scene.draw(0.5, true, 0)
    const day = scene.pixel(280, 180)
    scene.draw(0, true, 0)
    const ambient = scene.pixel(280, 180)
    scene.owner(91001, 160, 180, fire)
    scene.owner(91002, 480, 180, fire)
    const first: number[] = []
    const second: number[] = []
    let initial: RGB | undefined
    let bounded = true
    for (let now = 0; now <= 10_000; now += 67) {
      scene.draw(0, true, now)
      const pixels = scene.pixels()
      const read = (x: number): RGB => {
        const index = (180 * pixels.width + x) * 4
        return [pixels.pixels[index]!, pixels.pixels[index + 1]!, pixels.pixels[index + 2]!]
      }
      const a = read(280)
      const b = read(360)
      initial ??= a
      first.push(brightness(a))
      second.push(brightness(b))
      bounded &&= a.every((value, channel) => value >= ambient[channel]! - 2 && value <= day[channel]! + 2) &&
        b.every((value, channel) => value >= ambient[channel]! - 2 && value <= day[channel]! + 2)
    }
    check(bounded, `DPR ${resolution}: all fire samples stay between ambient and daylight`)
    check(Math.max(...first) - Math.min(...first) > 8 && Math.max(...second) - Math.min(...second) > 8,
      `DPR ${resolution}: production campfires visibly flicker in GPU pixels`)
    check(first.some((value, index) => Math.abs(value - second[index]!) > 6),
      `DPR ${resolution}: two campfires have independent flicker`)
    scene.lighting.invalidate()
    scene.draw(0, true, 0)
    check(near(scene.pixel(280, 180), initial!, 1), `DPR ${resolution}: seeking to the same time reproduces the fire pixels`)
    const night = scene.pixel(280, 180)
    scene.draw(21.5 / 24, true, 0)
    const dusk = scene.pixel(280, 180)
    check(brightness(night) < brightness(dusk) && brightness(dusk) < brightness(day),
      `DPR ${resolution}: the same fire fades with the existing dusk multiplier`)
    scene.draw(0.5, true, 67)
    check(near(scene.pixel(280, 180), day, 1) && !scene.lighting.getContainer().visible,
      `DPR ${resolution}: fire lighting remains disabled in daylight`)
    scene.lighting.clear()
    scene.owner(91003, 320, 180, { ...fire, flicker: 0 })
    let minBias = Infinity
    let maxBias = -Infinity
    for (let now = 0; now <= 10_000; now += 67) {
      scene.draw(0, true, now)
      const pixels = scene.pixels()
      const lightAt = (x: number): number => {
        const index = (180 * pixels.width + x) * 4
        return pixels.pixels[index]! + pixels.pixels[index + 1]! + pixels.pixels[index + 2]!
      }
      const bias = lightAt(440) - lightAt(200)
      minBias = Math.min(minBias, bias)
      maxBias = Math.max(maxBias, bias)
    }
    check(minBias < -4 && maxBias > 4,
      `DPR ${resolution}: production halo sways to both sides with constant brightness and radius`)
  } finally { scene.dispose() }
}

async function spawn(scene: Scene, id: number, state: 'burning' | 'unlit', screenX = 180, screenY = 220) {
  scene.objects.spawnObject({ entityId: id, typeId: 16, resourcePath: `campfire/${state}`, position: coordScreen2Game(screenX, screenY), size: { x: 12, y: 12 } })
  await settle()
  scene.draw(0)
}

async function checkObjectLifecycle(scene: Scene) {
  await spawn(scene, 1, 'unlit')
  check(scene.lighting.getMetrics().sources === 0, 'Production ObjectManager: unlit campfire registers no light')
  await spawn(scene, 1, 'burning')
  check(scene.lighting.getMetrics().sources === 1, 'Production ObjectManager: burning appearance registers one light')
  const lit = scene.pixel(180, 270)
  const priorOwner = scene.objects.getObject(1)!.getContainer()
  await spawn(scene, 1, 'burning', 380, 220)
  check(priorOwner.destroyed && scene.lighting.getMetrics().sources === 1, 'Production ObjectManager: same-ID replacement destroys the old owner without duplicating its light')
  check(brightness(scene.pixel(380, 270)) > brightness(scene.pixel(30, 270)), 'Production ObjectManager: replacement light follows the new owner')
  await spawn(scene, 1, 'unlit', 380, 220)
  check(scene.lighting.getMetrics().sources === 0 && brightness(scene.pixel(180, 270)) <= brightness(lit), 'Production ObjectManager: unlit appearance immediately removes light')
  for (let i = 0; i < 8; i++) { await spawn(scene, 1, 'burning'); await spawn(scene, 1, 'unlit') }
  check(scene.lighting.getMetrics().sources === 0, 'Production ObjectManager: repeated appearance changes do not leak light records')
  await spawn(scene, 1, 'burning')
  scene.objects.despawnObject(1)
  scene.draw(0)
  check(scene.lighting.getMetrics().sources === 0, 'Production ObjectManager: despawn removes its light')
  await spawn(scene, 1, 'burning')
  scene.objects.clear()
  scene.draw(0)
  check(scene.lighting.getMetrics().sources === 0, 'Production ObjectManager: world cleanup removes all light records')
}

async function checkDensity(scene: Scene) {
  const previous = localStorage.getItem(ECONOMY_RENDERING_STORAGE_KEY)
  const setting = useEconomyRendering()
  try {
    setting.setEnabled(true)
    check(useEconomyRendering().enabled.value === true, 'Economy rendering persists and restores through the real browser localStorage')
    setting.setEnabled(false)
    check(useEconomyRendering().enabled.value === false, 'Normal rendering persists and restores through the real browser localStorage')
    scene.app.stage.eventMode = 'static'
    scene.app.stage.hitArea = new Rectangle(0, 0, WIDTH, HEIGHT)
    let received: Point | null = null
    const onPointer = (event: { global: { x: number; y: number } }) => { received = new Point(event.global.x, event.global.y) }
    scene.app.stage.on('pointerdown', onPointer)
    for (const dpr of [1, 2]) {
      for (const economy of [false, true]) {
        const resolution = resolveRenderResolution(economy, dpr)
        scene.app.renderer.resize(WIDTH, HEIGHT, resolution)
        scene.draw(0)
        check(scene.app.screen.width === WIDTH && scene.app.screen.height === HEIGHT && scene.app.canvas.width === WIDTH * resolution && scene.app.canvas.height === HEIGHT * resolution,
          `DPR ${dpr}, economy ${economy}: live density switch preserves logical dimensions`)
        const rect = scene.app.canvas.getBoundingClientRect()
        const clientX = rect.left + 110 / WIDTH * rect.width
        const clientY = rect.top + 100 / HEIGHT * rect.height
        const mapped = new Point()
        scene.app.renderer.events.mapPositionToPoint(mapped, clientX, clientY)
        check(Math.abs(mapped.x - 110) < 0.01 && Math.abs(mapped.y - 100) < 0.01,
          `DPR ${dpr}, economy ${economy}: Pixi maps CSS pointer coordinates to unchanged logical coordinates`)
        received = null
        scene.app.canvas.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, clientX, clientY, pointerId: 1, pointerType: 'mouse', isPrimary: true, button: 0 }))
        const pointer = received as Point | null
        check(pointer !== null && Math.abs(pointer.x - 110) < 0.01 && Math.abs(pointer.y - 100) < 0.01,
          `DPR ${dpr}, economy ${economy}: dispatched pointer reaches the scene at the expected coordinates`)
        scene.draw(12 / 24)
        const day = scene.pixels()
        scene.draw(0)
        const night = scene.pixels()
        let whiteTextPixels = 0
        let textChanged = false
        for (let y = 45; y < 70; y++) for (let x = 42; x < 115; x++) {
          const index = (y * day.width + x) * 4
          if (day.pixels[index]! > 240 && day.pixels[index + 1]! > 240 && day.pixels[index + 2]! > 240) {
            whiteTextPixels++
            for (let channel = 0; channel < 3; channel++) if (Math.abs(day.pixels[index + channel]! - night.pixels[index + channel]!) > 2) textChanged = true
          }
        }
        check(scene.label.resolution === 2 && whiteTextPixels > 15 && !textChanged,
          `DPR ${dpr}, economy ${economy}: real HUD text retains its high-density texture and bright night pixels`)
      }
    }
    scene.app.stage.off('pointerdown', onPointer)
    scene.app.renderer.resize(WIDTH, HEIGHT, resolveRenderResolution(setting.enabled.value, window.devicePixelRatio))
  } finally {
    if (previous === null) localStorage.removeItem(ECONOMY_RENDERING_STORAGE_KEY)
    else localStorage.setItem(ECONOMY_RENDERING_STORAGE_KEY, previous)
  }
}

async function eventWithin(target: EventTarget, name: string, action: () => void, timeoutMs = 4000) {
  return new Promise<boolean>((resolve) => {
    const listener = () => { clearTimeout(timeout); target.removeEventListener(name, listener); resolve(true) }
    const timeout = setTimeout(() => { target.removeEventListener(name, listener); resolve(false) }, timeoutMs)
    target.addEventListener(name, listener)
    action()
  })
}
async function checkContextRestore(scene: Scene) {
  const gl = (scene.app.renderer as WebGLRenderer).gl
  const extension = gl.getExtension('WEBGL_lose_context')
  if (!extension) {
    checks.push('SKIP: WEBGL_lose_context is unavailable')
    document.body.dataset.contextStatus = 'unsupported'
    return
  }
  scene.owner(90001, 280, 180, { radius: 120, strength: 1, tint: 0xffd090, flicker: 0 })
  scene.draw(0)
  const expected = scene.pixel(280, 180)
  const before = scene.lighting.getMetrics().redrawCount
  scene.app.canvas.addEventListener('webglcontextlost', (event) => event.preventDefault(), { once: true })
  check(await eventWithin(scene.app.canvas, 'webglcontextlost', () => extension.loseContext()), 'WebGL context loss event is observable')
  await frame()
  check(await eventWithin(scene.app.canvas, 'webglcontextrestored', () => extension.restoreContext()), 'WebGL context restores')
  await frame()
  scene.draw(0)
  check(scene.lighting.getMetrics().redrawCount > before && near(scene.pixel(280, 180), expected, 7), 'Restored context redraws the cached light map and recovers the same pixels')
  scene.lighting.clear()
  document.body.dataset.contextStatus = 'passed'
}

interface TimerExtension { TIME_ELAPSED_EXT: number; GPU_DISJOINT_EXT: number }
interface BenchmarkRow { mode: string; sources: number; motion: string; cpuP50Ms: number; cpuP95Ms: number; gpuP50Ms: number | null; gpuP95Ms: number | null; gpuSamples: number; rafP50Ms: number; rafP95Ms: number; mapBytes: number; mapRedraws: number }
const percentile = (values: number[], fraction: number): number | null => {
  if (!values.length) return null
  const sorted = [...values].sort((a, b) => a - b)
  return Math.round(sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * fraction))]! * 1000) / 1000
}
async function runBenchmark(scene: Scene) {
  scene.objects.clear()
  scene.lighting.clear()
  scene.world.position.set(0, 0)
  scene.world.scale.set(1)
  const gl = (scene.app.renderer as WebGLRenderer).gl
  const gl2 = typeof WebGL2RenderingContext !== 'undefined' && gl instanceof WebGL2RenderingContext ? gl : null
  const extension = gl2?.getExtension('EXT_disjoint_timer_query_webgl2') as TimerExtension | null
  const rows: BenchmarkRow[] = []
  const gpuStatus = extension ? 'supported' : 'unavailable'
  const fire = ResourceLoader.getResourceDef('campfire/burning')?.light
  if (!fire) throw new Error('Production burning campfire is missing its light definition')
  benchmarkResult.dataset.status = 'running'
  benchmarkResult.dataset.gpu = gpuStatus
  for (const moving of [false, true]) {
    for (const count of [0, 1, 8, 16, 64]) {
      scene.lighting.clear()
      const owners = Array.from({ length: count }, (_, index) => scene.owner(80000 + index, 60 + (index % 8) * 74, 50 + Math.floor(index / 8) * 42, fire))
      for (const active of [false, true]) {
        const cpu: number[] = []
        const gpu: number[] = []
        const intervals: number[] = []
        let previousFrame = performance.now()
        const pending: WebGLQuery[] = []
        const startRedraw = scene.lighting.getMetrics().redrawCount
        const poll = () => {
          if (!gl2 || !extension) return
          const disjoint = Boolean(gl2.getParameter(extension.GPU_DISJOINT_EXT))
          for (let index = pending.length - 1; index >= 0; index--) {
            const query = pending[index]!
            if (!gl2.getQueryParameter(query, gl2.QUERY_RESULT_AVAILABLE)) continue
            if (!disjoint) gpu.push(Number(gl2.getQueryParameter(query, gl2.QUERY_RESULT)) / 1_000_000)
            gl2.deleteQuery(query)
            pending.splice(index, 1)
          }
          if (disjoint) gpu.length = 0
        }
        for (let index = 0; index < 110; index++) {
          do { await frame() } while (performance.now() - previousFrame < 1000 / 30)
          const frameTime = performance.now()
          if (index >= 20) intervals.push(frameTime - previousFrame)
          previousFrame = frameTime
          if (moving) { scene.world.x = Math.sin(index / 15) * 32; scene.world.y = Math.cos(index / 19) * 16 }
          else scene.world.position.set(0, 0)
          const measured = index >= 20
          const query = measured && index % 8 === 0 && pending.length < 8 && extension && gl2 ? gl2.createQuery() : null
          if (query && extension && gl2) gl2.beginQuery(extension.TIME_ELAPSED_EXT, query)
          const started = performance.now()
          scene.lighting.update(performance.now(), 0, scene.world, WIDTH, HEIGHT, active)
          scene.app.render()
          const elapsed = performance.now() - started
          if (query && extension && gl2) { gl2.endQuery(extension.TIME_ELAPSED_EXT); pending.push(query) }
          if (measured) cpu.push(elapsed)
          poll()
        }
        for (let retry = 0; pending.length && retry < 30; retry++) { await frame(); poll() }
        for (const query of pending) gl2?.deleteQuery(query)
        rows.push({ mode: active ? 'lighting' : 'baseline', sources: count, motion: moving ? 'moving' : 'stationary', cpuP50Ms: percentile(cpu, 0.5)!, cpuP95Ms: percentile(cpu, 0.95)!, gpuP50Ms: percentile(gpu, 0.5), gpuP95Ms: percentile(gpu, 0.95), gpuSamples: gpu.length, rafP50Ms: percentile(intervals, 0.5)!, rafP95Ms: percentile(intervals, 0.95)!, mapBytes: scene.lighting.getMetrics().mapBytes, mapRedraws: scene.lighting.getMetrics().redrawCount - startRedraw })
        benchmarkResult.textContent = JSON.stringify({ status: 'running', renderer: gl.getParameter(gl.RENDERER), resolution: scene.app.renderer.resolution, gpuTimer: gpuStatus, frameClock: 'browser requestAnimationFrame paced to at most 30 submissions per second', rows }, null, 2)
      }
      scene.lighting.clear()
      for (const owner of owners) owner.destroy()
    }
  }
  scene.world.position.set(0, 0)
  benchmarkResult.textContent = JSON.stringify({ status: 'passed', userAgent: navigator.userAgent, viewport: [WIDTH, HEIGHT], resolution: scene.app.renderer.resolution, mapLimit: 512, gpuTimer: gpuStatus, frameClock: 'browser requestAnimationFrame paced to at most 30 submissions per second', rows }, null, 2)
  benchmarkResult.dataset.status = 'passed'
}

async function main() {
  setWorldParams(12, 128)
  await preloadCampfire()
  await checkPixels(1)
  await checkPixels(2)
  await checkFireFlicker(1)
  await checkFireFlicker(2)
  const scene = await createScene(1)
  document.querySelector('#preview')!.append(scene.app.canvas)
  await checkObjectLifecycle(scene)
  await checkDensity(scene)
  await checkContextRestore(scene)
  await spawn(scene, 1, 'burning', 180, 220)
  await spawn(scene, 2, 'burning', 380, 220)
  await spawn(scene, 3, 'unlit', 500, 220)
  let burning = true
  let panned = false
  let zoomed = false
  let benchmarking = false
  const hour = document.querySelector<HTMLInputElement>('#hour')!
  const tick = () => {
    if (benchmarking) return
    cullingController.update(scene.app, scene.world, scene.world)
    scene.objects.update()
    const phase = Number(hour.value) / 24 % 1
    scene.lighting.update(performance.now(), phase, scene.world, WIDTH, HEIGHT)
    scene.app.render()
    document.querySelector('#clock')!.textContent = `${String(Math.floor(Number(hour.value)) % 24).padStart(2, '0')}:${String(Math.floor(Number(hour.value) * 60) % 60).padStart(2, '0')}`
    document.querySelector('#metrics')!.textContent = JSON.stringify(scene.lighting.getMetrics())
  }
  const loop = () => { tick(); requestAnimationFrame(loop) }
  document.querySelectorAll<HTMLButtonElement>('button').forEach((button) => { button.disabled = false })
  document.querySelector('#toggle')!.addEventListener('click', () => { burning = !burning; void spawn(scene, 1, burning ? 'burning' : 'unlit'); document.querySelector('#toggle')!.textContent = burning ? 'Extinguish first fire' : 'Light first fire' })
  document.querySelector('#pan')!.addEventListener('click', () => { panned = !panned; scene.world.x = panned ? -200 : 0; document.querySelector('#pan')!.textContent = panned ? 'Pan back' : 'Pan away' })
  document.querySelector('#zoom')!.addEventListener('click', () => { zoomed = !zoomed; scene.world.scale.set(zoomed ? 1.5 : 1); document.querySelector('#zoom')!.textContent = zoomed ? 'Zoom out' : 'Zoom in' })
  document.querySelector('#reset')!.addEventListener('click', () => { scene.objects.clear(); scene.lighting.clear(); scene.draw(null) })
  const economy = useEconomyRendering()
  const economyControl = document.querySelector<HTMLInputElement>('#economy')!
  economyControl.checked = economy.enabled.value
  scene.app.renderer.resize(WIDTH, HEIGHT, resolveRenderResolution(economy.enabled.value, window.devicePixelRatio))
  economyControl.addEventListener('change', () => {
    economy.setEnabled(economyControl.checked)
    scene.app.renderer.resize(WIDTH, HEIGHT, resolveRenderResolution(economy.enabled.value, window.devicePixelRatio))
    tick()
  })
  let clickCount = 0
  scene.app.stage.on('pointerdown', (event) => {
    document.querySelector('#pointer')!.textContent = `Canvas click ${++clickCount}: logical (${event.global.x.toFixed(1)}, ${event.global.y.toFixed(1)})`
  })
  document.querySelector('#benchmark')!.addEventListener('click', () => {
    if (benchmarking) return
    benchmarking = true
    economyControl.disabled = true
    document.querySelectorAll<HTMLButtonElement>('button').forEach((button) => { button.disabled = true })
    void runBenchmark(scene).catch((error: unknown) => { benchmarkResult.dataset.status = 'failed'; benchmarkResult.textContent = error instanceof Error ? error.stack ?? error.message : String(error) }).finally(() => {
      benchmarking = false
      economyControl.disabled = false
      document.querySelectorAll<HTMLButtonElement>('button').forEach((button) => { button.disabled = false })
    })
  })
  check(true, 'All GPU and lifecycle checks passed; production campfire preview is ready')
  document.body.dataset.status = 'passed'
  requestAnimationFrame(loop)
}
void main().catch((error: unknown) => {
  document.body.dataset.status = 'failed'
  result.textContent = `${checks.join('\n')}\nFAIL: ${error instanceof Error ? error.stack : error}`
  console.error(error)
})
