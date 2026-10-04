import { Application, Assets, Container, Graphics, Sprite, Text, WebGLRenderer, type Spritesheet } from 'pixi.js'
import { ActorRenderer, type ActorHandle } from '../src/game/actors/ActorRenderer'
import { ACTOR_RENDER } from '../src/game/actors/config'
import { SHALLOW_WATER } from '../src/game/actors/shallowWaterConfig'
import { ChunkManager } from '../src/game/ChunkManager'
import { ObjectManager } from '../src/game/ObjectManager'
import { type ObjectView } from '../src/game/ObjectView'
import { coordScreen2Game } from '../src/game/utils/coordConvert'
import { TILE_GRASS, TILE_SHALLOW_WATER } from '../src/game/tiles/tileIds'
import { setWorldParams } from '../src/game/tiles/Tile'

const report = document.querySelector<HTMLPreElement>('#result')!
const metrics = document.querySelector<HTMLPreElement>('#metrics')!
const check = (condition: unknown, message: string) => { if (!condition) throw new Error(message) }
// Background WebViews may suspend RAF while assets are still loading.
const paint = () => new Promise<void>(resolve => window.setTimeout(resolve, 16))

async function main() {
  const app = new Application()
  await app.init({ width: 1080, height: 440, background: '#2a3d2c', antialias: false, resolution: 1, preference: 'webgl' })
  app.stop()
  document.querySelector('#preview')!.append(app.canvas)
  const pixi = app.renderer as WebGLRenderer
  const renderer = new ActorRenderer(pixi)
  const manager = new ObjectManager()
  const chunks = new ChunkManager()
  setWorldParams(32, 16)
  const world = new Container()
  world.scale.set(2)
  world.sortableChildren = true
  app.stage.addChild(new Graphics().rect(0, 228, 1080, 212).fill('#084869'), world)
  manager.setParentContainer(world)
  manager.setActorRenderer(renderer)
  await manager.initShallowWater((x, y) => chunks.getTileTypeAtWorld(x, y))
  const identities = (eventSeq: number) => ({ streamEpoch: 1, eventSeq: BigInt(eventSeq) })
  const payloads = new Map<string, Uint8Array>()
  function fillMap(type: number) {
    for (let chunkX = -2; chunkX <= 2; chunkX++) for (let chunkY = -2; chunkY <= 2; chunkY++) {
      const tiles = new Uint8Array(16 * 16).fill(type)
      payloads.set(`${chunkX},${chunkY}`, tiles)
      chunks.loadChunk(chunkX, chunkY, tiles, 1, identities(1))
    }
  }
  fillMap(TILE_GRASS)
  const views: ObjectView[] = []
  for (let index = 0; index < 16; index++) {
    const screenX = 40 + (index % 8) * 66
    const screenY = index < 8 ? 100 : 208
    const position = coordScreen2Game(screenX, screenY)
    manager.spawnObject({ entityId: index + 1, typeId: 1, resourcePath: 'player', position, size: { x: 4, y: 4 } })
    const view = manager.getObject(index + 1)!
    view.onMoved(index % 8)
    view.onStopped()
    views.push(view)
    // Warm the shared model before allocating the rest of the review crowd.
    if (index === 0) await (view as unknown as { actorHandle: ActorHandle }).actorHandle.actor.ready
    const label = new Text({ text: String(index % 8), style: { fontSize: 8, fill: '#cedac3' } })
    label.position.set(screenX - 2, screenY + 9)
    world.addChild(label)
  }
  const handle = (view: ObjectView) => (view as unknown as { actorHandle: ActorHandle }).actorHandle
  const body = (view: ObjectView) => handle(view).sprite
  const rippleTextures = Assets.get<Spritesheet>(SHALLOW_WATER.textureURL).animations.ripples!
  const ripple = (view: ObjectView) => view.getContainer().children.find(child => child instanceof Sprite && rippleTextures.includes(child.texture)) as Sprite
  let now = performance.now()
  function frame(elapsed = 100) {
    now += elapsed
    manager.update(now)
    manager.syncActiveCarryVisuals(94)
    renderer.render(now)
    app.render()
    check(pixi.gl.getError() === 0, 'Mixed rendering must not leave a GL error')
  }
  report.textContent = 'Loading the model and animations…'
  for (let attempt = 0; attempt < 600 && views.some(view => !handle(view).actor.isReady); attempt++) {
    frame()
    if (attempt % 60 === 0) report.textContent = `Loading the model and animations… ${renderer.metrics.assets} assets, frame ${attempt}`
    await paint()
  }
  check(views.every(view => handle(view).actor.isReady), 'Character assets did not finish loading')
  report.textContent = 'Checking immersion, clicks, and carrying…'
  for (let index = 0; index < 6; index++) frame()
  function extract(view: ObjectView) {
    const values = app.renderer.extract.pixels({ target: body(view).texture }).pixels
    pixi.resetState()
    return values
  }
  const baseline = extract(views[8]!)
  function setTile(view: ObjectView, type: number) {
    const position = view.getPosition()
    const tileX = Math.floor(position.x / 32)
    const tileY = Math.floor(position.y / 32)
    const chunkX = Math.floor(tileX / 16)
    const chunkY = Math.floor(tileY / 16)
    const tiles = payloads.get(`${chunkX},${chunkY}`)!
    tiles[(tileY - chunkY * 16) * 16 + tileX - chunkX * 16] = type
    chunks.loadChunk(chunkX, chunkY, tiles, 2, identities(2))
  }
  for (const view of views.slice(8)) setTile(view, TILE_SHALLOW_WATER)
  frame(SHALLOW_WATER.transitionMs / 2)
  check(handle(views[8]!).immersionPx > 0 && handle(views[8]!).immersionPx < SHALLOW_WATER.immersionPx, 'Stationary terrain change must start a transition')
  frame(SHALLOW_WATER.transitionMs)
  const before = views[8]!.getContainer().position.clone()
  for (const view of views.slice(8)) {
    const actor = handle(view)
    check(actor.immersionPx === SHALLOW_WATER.immersionPx, 'All eight facings must reach knee depth')
    check(actor.sprite.y === -ACTOR_RENDER.anchorY + SHALLOW_WATER.immersionPx, 'Only the body moves down')
    const values = extract(view)
    const cropRow = ACTOR_RENDER.anchorY - SHALLOW_WATER.immersionPx
    let upperPixels = 0
    for (let row = 0; row < ACTOR_RENDER.cellSize; row++) for (let column = 0; column < ACTOR_RENDER.cellSize; column++) {
      const alpha = values[(row * ACTOR_RENDER.cellSize + column) * 4 + 3]!
      if (row >= cropRow) check(alpha === 0, 'Underwater pixels, including outlines, must be transparent')
      else if (alpha) upperPixels++
    }
    check(upperPixels > 300, 'The visible torso must remain intact')
    check(ripple(view).eventMode === 'none' && ripple(view).visible, 'Water overlay must remain noninteractive')
    const shadows = (view as unknown as { shadowSprites: Sprite[] }).shadowSprites
    check(shadows.every(shadow => !shadow.visible), 'Ground shadows must remain hidden in water')
  }
  check(views[8]!.getContainer().position.equals(before), 'Immersion must preserve the world/depth anchor')
  const submerged = views[8]!
  const transformed = submerged.getContainer().toGlobal({ x: 0, y: 10 })
  check(!submerged.hitTestRmbScreenPoint(transformed.x, transformed.y, coordScreen2Game), 'Submerged legs must not capture a click')
  submerged.setHovered(true)
  frame()
  const hovered = extract(submerged)
  const firstHidden = (ACTOR_RENDER.anchorY - SHALLOW_WATER.immersionPx) * ACTOR_RENDER.cellSize * 4
  for (let index = firstHidden + 3; index < hovered.length; index += 4) check(hovered[index] === 0, 'Hover must not reveal the clipped body')
  submerged.setHovered(false)
  frame()
  const wetPixels = extract(submerged)
  const frozenRevision = handle(submerged).actor.revision
  const seenRippleFrames = new Set()
  for (let index = 0; index < SHALLOW_WATER.frameCount; index++) {
    seenRippleFrames.add(ripple(submerged).texture)
    frame(1000 / SHALLOW_WATER.framesPerSecond)
  }
  check(seenRippleFrames.size === SHALLOW_WATER.frameCount, 'The loaded atlas must animate all five frames while stationary')
  check(handle(submerged).actor.revision === frozenRevision, 'Stationary ripples must reuse the cached actor frame')
  setTile(submerged, TILE_GRASS)
  frame(SHALLOW_WATER.transitionMs)
  const restored = extract(submerged)
  check(restored.every((value, index) => value === baseline[index]), 'Exiting water must restore the exact dry frame')
  setTile(submerged, TILE_SHALLOW_WATER)
  frame(SHALLOW_WATER.transitionMs)
  const p = submerged.getPosition()
  const chunkX = Math.floor(p.x / 32 / 16)
  const chunkY = Math.floor(p.y / 32 / 16)
  chunks.unloadChunk(chunkX, chunkY, identities(3))
  frame(1)
  check(handle(submerged).immersionPx === 0 && !ripple(submerged).visible, 'Unknown/unloaded terrain must clear immersion immediately')
  chunks.loadChunk(chunkX, chunkY, payloads.get(`${chunkX},${chunkY}`)!, 2, identities(4))
  frame(SHALLOW_WATER.transitionMs)
  manager.spawnObject({ entityId: 100, typeId: 6, resourcePath: 'barrel', position: p, size: { x: 4, y: 4 } })
  manager.setCarryVisualRelation(100, submerged.entityId)
  frame(SHALLOW_WATER.transitionMs)
  check(manager.getObject(100)!.getContainer().y === submerged.getContainer().y - (94 - SHALLOW_WATER.immersionPx), 'Carried prop must follow submerged hands')
  manager.clearCarryVisualRelation(100)
  manager.setCarryVisualRelation(submerged.entityId, views[0]!.entityId)
  frame(1)
  check(handle(submerged).immersionPx === 0, 'A carried character must not wade at its stored ground position')
  manager.clearCarryVisualRelation(submerged.entityId)
  frame(SHALLOW_WATER.transitionMs)
  submerged.setKnockedOutPose(true)
  frame(1)
  check(handle(submerged).immersionPx === 0 && !ripple(submerged).visible, 'Standing waterline must not clip the KO pose')
  submerged.setKnockedOutPose(false)
  frame(SHALLOW_WATER.transitionMs)
  const extension = pixi.gl.getExtension('WEBGL_lose_context')
  check(extension, 'Context loss extension is required for restoration test')
  const lost = new Promise<void>(resolve => app.canvas.addEventListener('webglcontextlost', () => resolve(), { once: true }))
  extension!.loseContext()
  await lost
  report.textContent = 'Checking WebGL context restoration…'
  await new Promise<void>(resolve => window.setTimeout(resolve, 100))
  const recovered = new Promise<void>(resolve => app.canvas.addEventListener('webglcontextrestored', () => resolve(), { once: true }))
  extension!.restoreContext()
  await Promise.race([recovered, new Promise((_, reject) => window.setTimeout(() => reject(new Error('Context restoration timed out')), 8000))])
  frame(SHALLOW_WATER.transitionMs)
  const afterRestore = extract(submerged)
  check(afterRestore.every((value, index) => value === wetPixels[index]), 'Context restoration must preserve the exact clipped frame')
  manager.despawnObject(100)
  report.textContent = 'ALL CHECKS PASSED\n5-frame ripples at 5 FPS · 8 facings · stationary entry/exit · binary GPU clipping · hover/picking · frame cache · chunk unload/reload · carry · KO · context restore'
  let walking = false
  let carrying = false
  let sceneCount = 16
  const crowdViews: ObjectView[] = []
  const frameIntervals: number[] = []
  let previousMs = performance.now()
  let lastReportMs = 0
  document.querySelector('#walk')!.addEventListener('click', () => { walking = !walking; if (!walking) for (const view of [...views, ...crowdViews]) view.onStopped() })
  document.querySelector('#carry')!.addEventListener('click', () => { carrying = !carrying; for (const view of [...views, ...crowdViews]) view.setCarrying(carrying) })
  document.querySelector<HTMLSelectElement>('#count')!.addEventListener('change', event => {
    sceneCount = Number((event.target as HTMLSelectElement).value)
    frameIntervals.length = 0
    for (const view of crowdViews.splice(0)) manager.despawnObject(view.entityId)
    for (const view of views) view.getContainer().visible = sceneCount === 16
    if (sceneCount === 16) return
    for (let index = 0; index < sceneCount; index++) {
      const position = coordScreen2Game(sceneCount === 1 ? 260 : 30 + (index % 15) * 33, 175 + Math.floor(index / 15) * 22)
      const entityId = 200 + index
      manager.spawnObject({ entityId, typeId: 1, resourcePath: 'player', position, size: { x: 4, y: 4 } })
      const view = manager.getObject(entityId)!
      view.setCarrying(carrying)
      setTile(view, TILE_SHALLOW_WATER)
      crowdViews.push(view)
    }
  })
  app.ticker.maxFPS = 30
  app.ticker.add(() => {
    const currentMs = performance.now()
    frameIntervals.push(currentMs - previousMs)
    if (frameIntervals.length > 300) frameIntervals.shift()
    const elapsed = Math.min(100, currentMs - previousMs)
    previousMs = currentMs
    for (const view of views) if (walking) view.onMoved((view.entityId - 1) % 8, elapsed / 1000 * 32)
    if (walking) for (let index = 0; index < crowdViews.length; index++) crowdViews[index]!.onMoved(index % 8, elapsed / 1000 * 32)
    frame(elapsed)
    if (currentMs - lastReportMs > 500) {
      const ordered = [...frameIntervals].sort((a, b) => a - b)
      const average = frameIntervals.reduce((sum, interval) => sum + interval, 0) / frameIntervals.length
      metrics.textContent = JSON.stringify({ visibleActors: sceneCount, fps: +(1000 / average).toFixed(1),
        p95FrameMs: +(ordered[Math.floor(ordered.length * .95)] ?? 0).toFixed(1), ...renderer.metrics, glError: pixi.gl.getError() }, null, 2)
      lastReportMs = currentMs
    }
  })
  app.start()
}

void main().catch(error => { report.textContent = 'FAIL ' + (error instanceof Error ? error.stack : String(error)); console.error(error) })
