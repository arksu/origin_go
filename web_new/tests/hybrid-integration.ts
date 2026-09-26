import type { ActorHandle } from '../src/game/actors/ActorRenderer'
import { verifyMovementStopping } from './movement-stop'
import { verifyScreenFacing } from './screen-facing'
import { verifyCharacterEquipment, verifyStoneAxe } from './character-equipment'
import { Application, Container, Sprite, Texture, WebGLRenderer } from 'pixi.js'
import { ObjectManager } from '../src/game/ObjectManager'
import { ResourceLoader } from '../src/game/ResourceLoader'
import { ActorRenderer } from '../src/game/actors/ActorRenderer'
import { ACTOR_RENDER, COMMONER_ASSET_ID, DEFAULT_EQUIPMENT } from '../src/game/actors/config'
import { setWorldParams } from '../src/game/tiles/Tile'
import { coordScreen2Game } from '../src/game/utils/coordConvert'

const result = document.querySelector<HTMLPreElement>('#result')!
const checks: string[] = []
function check(value: unknown, message: string): asserts value { if (!value) throw new Error(message) }
function pass(message: string) { checks.push(`PASS ${message}`); result.textContent = checks.join('\n') }
const paint = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))

async function main() {
  const app = new Application()
  await app.init({ width: 720, height: 310, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  app.stop()
  document.querySelector('#preview')!.append(app.canvas)
  check((app.renderer as WebGLRenderer).gl.getError() === 0, 'GL error after Pixi initialization')
  // This scene validates the discrete export contract: eight headings and eight
  // gait samples are expected to hash to stable generated textures. It still
  // renders the current Three.js Meshy model, never a legacy atlas.
  const renderer = new ActorRenderer(app.renderer as WebGLRenderer, { mode: 'baked8' })
  check((app.renderer as WebGLRenderer).gl.getError() === 0, 'GL error after Three/Pixi initialization')
  const catalog = await renderer.cache.catalog
  const cycleDistanceTiles = catalog.manifests[COMMONER_ASSET_ID]!.clips.walk!.cycleDistanceTiles!
  const manager = new ObjectManager()
  const world = new Container()
  world.position.set(120, 250)
  world.scale.set(2)
  app.stage.addChild(world)
  manager.setParentContainer(world)
  manager.setActorRenderer(renderer)
  manager.setPlayerEntityId(101)
  setWorldParams(12, 128)
  const options = { typeId: 1, resourcePath: 'player', position: { x: 0, y: 0 }, size: { x: 4, y: 4 } }
  manager.spawnObject({ ...options, entityId: 101 })
  manager.spawnObject({ ...options, position: { x: 0, y: 0 }, entityId: 102 })
  manager.despawnObject(102)
  const view = manager.getObject(101)!
  view.onMoved(5, cycleDistanceTiles * 12 * .25)
  view.onStopped()
  const sprite = view.getContainer().children.find((child) => child instanceof Sprite)
  check(sprite instanceof Sprite, 'Production ObjectView must allocate the hybrid sprite synchronously')
  let renderTime = 0
  const render = (updateWorld = true) => {
    const gl = (app.renderer as WebGLRenderer).gl
    check(gl.getError() === 0, 'GL error left before test frame')
    if (updateWorld) manager.update()
    renderer.render(renderTime += 100)
    check(gl.getError() === 0, `GL error in actor pass at ${renderTime}`)
    app.render()
    check(gl.getError() === 0, `GL error in Pixi pass at ${renderTime}`)
  }
  for (let attempt = 0; attempt < 300 && sprite.texture === Texture.EMPTY; attempt++) { render(); await paint() }
  check(sprite.texture !== Texture.EMPTY && sprite.texture.width === 128, 'GLB must become a live 128-pixel GPU texture')
  check(renderer.metrics.actors === 1 && manager.getObjectCount() === 1, 'Despawn during loading must release its instance')
  check(renderer.metrics.assets === 9, 'Default actor must share one model and eight standalone animations')
  pass('Production ObjectView / async move, stop, despawn / shared model and standalone animation artifacts')

  const playerSprite = sprite
  function pixels(updateWorld = true) {
    render(updateWorld)
    let error = (app.renderer as WebGLRenderer).gl.getError()
    check(error === 0, `GL error after mixed render: ${error}; completed ${checks.length} groups`)
    const pixels = app.renderer.extract.pixels({ target: playerSprite.texture }).pixels
    // Pixi extraction leaves its read framebuffer bound; invalidate its state
    // before the next ordinary world render, as required for external GL work.
    app.renderer.resetState()
    error = (app.renderer as WebGLRenderer).gl.getError()
    check(error === 0, `GL error after test extraction: ${error}; completed ${checks.length} groups`)
    return pixels
  }
  function hash(updateWorld = true) {
    const values = pixels(updateWorld)
    let hash = 2166136261
    for (const value of values) hash = Math.imul(hash ^ value, 16777619)
    return hash >>> 0
  }
  view.onMoved(3)
  view.onStopped()
  const idle = hash()
  const values = pixels()
  const alpha = new Set<number>()
  let opaque = 0
  for (let index = 3; index < values.length; index += 4) { alpha.add(values[index]!); if (values[index]) opaque++ }
  check(alpha.size === 2 && alpha.has(0) && alpha.has(255), 'Pixel image must have binary coverage')
  check(opaque > 800 && opaque < 5000, 'Character silhouette must occupy a plausible native pixel area')
  check(values[3] === 0 && values[values.length - 1] === 0, 'Frame corners must remain transparent')
  check(view.hitTestRmbScreenPoint(120, 160, coordScreen2Game), 'Opaque torso must be selectable through production picking')
  check(!view.hitTestRmbScreenPoint(0, 30, coordScreen2Game), 'Transparent corner must not consume a world click')
  view.setHovered(true)
  const highlighted = hash()
  check(highlighted !== idle, 'Hover must follow the rendered silhouette')
  view.setHovered(false)
  check(hash() === idle, 'Leaving hover must restore the exact unhighlighted image')
  pass('Binary alpha / real silhouette picking / transparent clicks / GPU hover')

  const poses = new Set<number>()
  for (let direction = 0; direction < 8; direction++) {
    view.onStopped()
    view.onMoved(direction, 0)
    for (let frame = 0; frame < 6; frame++) render()
    for (let phase = 0; phase < 8; phase++) {
      view.onMoved(direction, phase === 0 ? 0 : cycleDistanceTiles * 12 / 8)
      poses.add(hash())
    }
  }
  check(poses.size === 64, `Eight directions and eight steps must produce 64 distinct images; got ${poses.size}`)
  let equalDistance = 0
  for (const fps of [30, 60, 144]) {
    view.onStopped()
    for (let step = 0; step < fps; step++) view.onMoved(1, cycleDistanceTiles * 12 * .375 / fps)
    for (let frame = 0; frame < 6; frame++) render()
    const image = hash()
    if (!equalDistance) equalDistance = image
    check(image === equalDistance, `Walk distance must be independent of ${fps} FPS`)
    check(hash() === image, 'Clock ticks alone must not advance gait')
  }
  view.onMoved(3)
  view.onStopped()
  for (let frame = 0; frame < 6; frame++) render()
  check(hash() === idle, 'Stopping must restore the standing pose')
  pass('64 skeletal walk poses / distance at 30, 60, 144 FPS / stationary clock / idle')

  await view.setActorEquipment([{ slot: 'legs', visualKey: 'linen_wrap' }])
  check(hash() === idle, 'Items without a registered 3D visual must preserve the body')
  let rejectedDuplicateSlot = false
  try { await view.setActorEquipment([{ slot: 'right_hand', visualKey: 'stone_axe' }, { slot: 'right_hand', visualKey: 'stone_axe' }]) } catch { rejectedDuplicateSlot = true }
  check(rejectedDuplicateSlot, 'Duplicate equipment slots must be rejected')
  check(hash() === idle, 'Rejected equipment must preserve the existing character')
  const staleSwap = view.setActorEquipment([])
  const latestSwap = view.setActorEquipment(DEFAULT_EQUIPMENT)
  await Promise.all([staleSwap, latestSwap])
  check(hash() === idle, 'Latest equipment request must win without changing the body pose')
  pass('Unavailable visual fallback / duplicate slots rejected / asynchronous empty replacement')

  manager.spawnObject({ entityId: 201, typeId: 10, resourcePath: 'box/normal', position: { x: 0, y: 0 }, size: { x: 4, y: 4 } })
  await ResourceLoader.loadTexture('obj/box/box.png')
  manager.setCarryVisualRelation(201, 101)
  manager.syncActiveCarryVisuals(56)
  const carry = hash()
  check(carry !== idle, 'Server carry relation must select raised arms')
  check(manager.getObject(201)!.getContainer().y === view.getContainer().y - 94, 'Carried prop must move to the new palm height')
  view.onMoved(5, cycleDistanceTiles * 12 * .375)
  check(hash() !== carry, 'Carry pose must retain leg locomotion')
  manager.clearCarryVisualRelation(201)
  view.onMoved(3)
  view.onStopped()
  for (let frame = 0; frame < 6; frame++) render()
  check(hash() === idle, 'Releasing a carried prop must restore ordinary idle')
  manager.setKnockedOutPose(101, true)
  const knockedOut = hash()
  check(knockedOut !== idle && view.getContainer().rotation === 0, 'KO must change the 3D body while leaving the ground container upright')
  const koView = view as unknown as { actorHandle: ActorHandle; shadowSprites: Sprite[]; knockedOutShadow: Container }
  check(koView.knockedOutShadow.visible && koView.shadowSprites.every((shadow) => !shadow.visible), 'KO must replace the standing cast shadow with ground contact')
  view.setShadowSuppressed(true)
  check(!koView.knockedOutShadow.visible, 'Shadow suppression must include the lying contact shadow')
  view.setShadowSuppressed(false)
  check(koView.knockedOutShadow.visible, 'Lying contact shadow must return when suppression ends')
  const koPixels = pixels()
  let minX = 128, maxX = 0, minY = 128, maxY = 0
  for (let y = 0; y < 128; y++) for (let x = 0; x < 128; x++) {
    if (!koPixels[(y * 128 + x) * 4 + 3]) continue
    minX = Math.min(minX, x); maxX = Math.max(maxX, x)
    minY = Math.min(minY, y); maxY = Math.max(maxY, y)
  }
  check(maxX - minX > (maxY - minY) * 2, 'KO silhouette must lie horizontally on the ground')
  check(minX > 0 && maxX < 127 && minY > 0 && maxY < 127, 'Lying body must fit entirely inside its texture')
  const bounds = view.computeScreenBounds()
  check(bounds.minX <= -ACTOR_RENDER.anchorX && bounds.maxX >= ACTOR_RENDER.cellSize - ACTOR_RENDER.anchorX &&
    bounds.maxY >= ACTOR_RENDER.cellSize - ACTOR_RENDER.knockedOutAnchorY, 'Culling must include the ground-centered KO frame')
  const hitX = Math.floor((minX + maxX) / 2), hitY = Math.floor((minY + maxY) / 2)
  check(koPixels[(hitY * 128 + hitX) * 4 + 3], 'KO picking fixture must land inside the torso')
  check(view.hitTestRmbScreenPoint(120 + (hitX - ACTOR_RENDER.anchorX) * 2,
    250 + (hitY - ACTOR_RENDER.knockedOutAnchorY) * 2, coordScreen2Game), 'Lying torso must use the new picking anchor')
  check(!view.hitTestRmbScreenPoint(120, 50, coordScreen2Game), 'Empty space above a knocked out body must not consume clicks')
  for (const mode of ['hybrid3d', 'baked8'] as const) {
    renderer.setSettings({ mode })
    koView.actorHandle.actor.walking = true
    koView.actorHandle.actor.carrying = true
    koView.actorHandle.actor.stopProgress = .5
    check(hash(false) === knockedOut, `3D knockout must override pending gait and carry directly in ${mode}`)
    for (let direction = 0; direction < 8; direction++) {
      view.onMoved(direction, cycleDistanceTiles * 12 * .25)
      check(hash() === knockedOut, `KO must suppress locomotion and preserve the horizontal body in ${mode}, heading ${direction}`)
    }
  }
  manager.setKnockedOutPose(101, false)
  check(hash() === idle && playerSprite.y === -ACTOR_RENDER.anchorY, 'Recovery must restore the exact standing body and anchor')
  check(!koView.knockedOutShadow.visible && koView.shadowSprites.every((shadow) => shadow.visible), 'Recovery must restore the standing shadow')
  pass('Server carry relation / universal hands / carry walk / release / 3D KO / ground picking / recovery')

  view.getContainer().renderable = false
  render()
  check(renderer.metrics.updated === 0 && sprite.texture === Texture.EMPTY, 'Culled characters must release their output slot and skip rendering')
  view.getContainer().renderable = true
  check(hash() === idle, 'A reused GPU slot must show the current pose, never another character')
  check((app.renderer as WebGLRenderer).gl.getError() === 0, 'Mixed render passes must not produce GL errors')
  pass('Offscreen updates / pooled output reuse / mixed GL state')

  world.scale.set(.5)
  app.render()
  check(hash() !== idle, 'Small screen scale must select the separately reduced game mesh')
  check(renderer.metrics.triangles > 3000 && renderer.metrics.triangles < 7000, 'LOD must render fewer than 7k triangles')
  world.scale.set(2)
  app.render()
  check(hash() === idle, 'Returning to normal scale must restore the original high-detail image')
  pass('Reduced mesh / triangle budget / lossless return to normal detail')

  const extension = (app.renderer as WebGLRenderer).gl.getExtension('WEBGL_lose_context')
  check(extension, 'Context-loss test extension is required for this review')
  const restored = new Promise<void>((resolve) => app.canvas.addEventListener('webglcontextrestored', () => resolve(), { once: true }))
  extension.loseContext()
  await new Promise<void>((resolve) => window.setTimeout(resolve, 100))
  extension.restoreContext()
  await Promise.race([restored, new Promise((_, reject) => window.setTimeout(() => reject(new Error('Context restoration timed out')), 8000))])
  check(hash() === idle, 'Context restoration must recreate the same character image')
  check(view.hitTestRmbScreenPoint(120, 160, coordScreen2Game), 'Picking framebuffer must survive context restoration')
  pass('Actual WebGL context loss / texture restoration / picking restoration')

  verifyScreenFacing()
  const liveActor = (view as unknown as { actorHandle: ActorHandle }).actorHandle.actor
  verifyCharacterEquipment(liveActor, () => hash(false))
  pass('Rigid attachment shader / actual GLTF arm masks / eight gait samples / clean removal')
  // This test authors actor samples directly; ObjectView must not overwrite
  // them with the stationary server fixture while reading the rendered pixels.
  await verifyStoneAxe(liveActor, () => hash(false))
  pass('Real stone axe / 64 equipped poses / distance-driven arm / free-hand independence / left hand / carry / removal')
  for (const scale of [.5, 1, 2]) {
    world.scale.set(scale)
    for (let direction = 0; direction < 8; direction++) {
      const angle = (direction - 1) * Math.PI / 4
      const delta = coordScreen2Game(Math.cos(angle) * 20, Math.sin(angle) * 20)
      manager.updateObjectPosition(101, 0, 0, false, 3, 0)
      // Deliberately wrong server direction: visible displacement owns facing.
      manager.updateObjectPosition(101, delta.x, delta.y, true, 3, Math.hypot(delta.x, delta.y))
      hash()
      check(liveActor.direction === direction, `Production facing must follow screen ray ${direction} at scale ${scale}`)
      manager.updateObjectPosition(101, delta.x, delta.y, false, 3, 0)
      check(liveActor.direction === direction, 'Stop must preserve the displayed facing')
      manager.updateObjectPosition(101, 5000, 5000, true, 3, 0)
      check(liveActor.direction === direction, 'Zero-distance teleport must not turn the character')
    }
  }
  world.scale.set(2)
  pass('Screen sectors / actual ObjectManager displacement / zoom / inverse camera projection / boundary hysteresis')
  await verifyMovementStopping(manager)
  pass('Live actor / server stop / deceleration / 8 directions / 30–144 FPS / restart / teleport / KO')
  manager.despawnObject(201)
  manager.despawnObject(101)
  renderer.destroy()
  check(Number(renderer.metrics.actors) === 0 && Number(renderer.metrics.assets) === 0, 'Teardown must release all actor resources')
  pass('Renderer teardown / no baked fallback')
  result.textContent += '\n\nALL CHECKS PASSED'
}
void main().catch((error: unknown) => { result.textContent += '\nFAIL ' + (error instanceof Error ? error.stack : String(error)); console.error(error) })
