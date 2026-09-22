import { Application, Container, Sprite, Texture, Ticker } from 'pixi.js'
import { ObjectManager } from '../src/game/ObjectManager'
import { ResourceLoader } from '../src/game/ResourceLoader'
import { cullingController } from '../src/game/culling'
import { fxManager } from '../src/game/fx/FxManager'
import { ParticleEmitter } from '../src/game/fx/ParticleEmitter'
import { ParticlePool } from '../src/game/fx/ParticlePool'
import { smokePreset } from '../src/game/fx/presets/smoke'
import { validateFxDefinition } from '../src/game/fx/validateDefinition.js'
import { setWorldParams } from '../src/game/tiles/Tile'
import { coordScreen2Game } from '../src/game/utils/coordConvert'

const result = document.querySelector<HTMLPreElement>('#result')!
const checks: string[] = []
function check(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
  checks.push(`PASS: ${message}`)
  result.textContent = checks.join('\n')
}
const settle = async () => { for (let index = 0; index < 8; index++) await Promise.resolve() }
const snapshot = (container: Container) => JSON.stringify(container.children.map((child) => [child.x, child.y, child.alpha, child.scale.x, child.rotation]))
const activeEmitters = () => [...(fxManager as unknown as { activeFx: Set<unknown> }).activeFx].filter((fx): fx is ParticleEmitter => fx instanceof ParticleEmitter)
const step = (emitter: ParticleEmitter, count: number) => { for (let index = 0; index < count; index++) emitter.update(100) }

async function main() {
  const app = new Application()
  await app.init({ width: 900, height: 380, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  app.stop()
  Ticker.shared.stop()
  document.querySelector('#preview')!.append(app.canvas)
  const texture = await ResourceLoader.loadTexture('fx/smoke_puff.png')
  check(texture !== Texture.WHITE && texture.width === 64 && texture.height === 64, '64px puff loads through ResourceLoader')
  const pixels = app.renderer.extract.pixels({ target: texture }).pixels
  let transparent = 0
  let translucent = 0
  for (let index = 3; index < pixels.length; index += 4) {
    if (pixels[index] === 0) transparent++
    else if (pixels[index]! < 255) translucent++
  }
  check(transparent > 2000 && translucent > 0, 'Puff has transparent padding and translucent edges')

  const pool = new ParticlePool(texture)
  const particle = pool.acquire()
  const pooledSprite = particle.sprite
  pool.release(particle)
  const reused = pool.acquire()
  check(reused === particle && reused.sprite === pooledSprite, 'Pool reuses both the record and sprite')
  pool.release(reused)
  let duplicateReleaseRejected = false
  try { pool.release(reused) } catch { duplicateReleaseRejected = true }
  check(duplicateReleaseRejected, 'Pool rejects double release')
  pool.destroy()
  check(pooledSprite.destroyed && !texture.destroyed, 'Pool destroys sprites while preserving the shared texture')

  for (const params of [{ typo: 1 }, { density: -1 }, { windResponse: [2, 1] }, { tint: 0x1000000 }]) {
    let rejected = false
    try { validateFxDefinition({ preset: 'smoke', params }) } catch { rejected = true }
    check(rejected, `Malformed smoke parameters rejected: ${JSON.stringify(params)}`)
  }
  for (const offset of [[5], [5, 'up']]) {
    let rejected = false
    try { validateFxDefinition({ preset: 'smoke', offset }) } catch { rejected = true }
    check(rejected, `Malformed smoke offset rejected: ${JSON.stringify(offset)}`)
  }
  const config = smokePreset({ preset: 'smoke' })
  check(config.linger, 'Smoke defaults to a fading tail when its source state ends')
  check(!smokePreset({ preset: 'smoke', linger: false }).linger,
    'A definition can explicitly request instant smoke removal')
  const offsetConfig = smokePreset({ preset: 'smoke', offset: [9, -13] })
  check(offsetConfig.position.x === 9 && offsetConfig.position.y === -55,
    'Smoke definition offset adjusts the preset source in local pixels')
  const owner = new Container()
  const emitter = new ParticleEmitter({ ...config, rate: 100, maxParticles: 3 }, texture, owner)
  step(emitter, 20)
  check(emitter.particleCount === 3 && emitter.container.children.length === 3, 'Live particle count stays capped')
  const beforeCull = snapshot(emitter.container)
  owner.visible = false
  step(emitter, 40)
  check(snapshot(emitter.container) === beforeCull, 'Culling pauses positions, age-dependent alpha, scale and rotation')
  owner.visible = true
  emitter.update(100)
  check(snapshot(emitter.container) !== beforeCull, 'Visible owner resumes simulation without catch-up')
  emitter.stop()
  check(emitter.update(100), 'stop() preserves living particles')
  step(emitter, 40)
  check(!emitter.update(100) && emitter.container.destroyed && owner.children.length === 0, 'Stopped emitter finishes and removes its container')
  owner.destroy()

  const motionOwner = new Container()
  const motion = new ParticleEmitter({ ...config, rate: 10, maxParticles: 1, speed: [0, 0], buoyancy: 0, windResponse: [1, 1], lifetimeMs: [2000, 2000], wander: { speed: 0, frequency: 0 } }, texture, motionOwner)
  motion.update(100)
  motion.stop()
  const puff = motion.container.children[0]!
  check(puff.alpha === 0, 'New puff starts transparent')
  const startX = puff.x
  motion.update(100, { x: 14, y: 0 })
  check(puff.x > startX && puff.alpha > 0, 'Wind moves smoke rightward during soft fade-in')
  const rightX = puff.x
  motion.update(100, { x: -14, y: 0 })
  check(puff.x < rightX, 'Wind vector is read on every update')
  step(motion, 2)
  const peakAlpha = puff.alpha
  step(motion, 14)
  check(puff.alpha < peakAlpha && puff.alpha > 0, 'Smoke fades away before its lifetime ends')
  motion.destroy()
  motionOwner.destroy()

  const scene = new Container({ sortableChildren: true })
  scene.position.set(30, 20)
  scene.scale.set(2)
  app.stage.addChild(scene)
  const lingerOwner = new Container()
  lingerOwner.position.set(120, 80)
  lingerOwner.scale.set(1.2, 0.8)
  lingerOwner.rotation = 0.15
  scene.addChild(lingerOwner)
  const linger = await fxManager.attach({ preset: 'smoke', linger: true }, lingerOwner)
  check(linger, 'FxManager attaches an emitter')
  step(linger, 10)
  const beforeRelease = linger.container.toGlobal({ x: 0, y: 0 })
  scene.removeChild(lingerOwner)
  fxManager.detach(linger)
  lingerOwner.destroy({ children: true })
  const afterRelease = linger.container.toGlobal({ x: 0, y: 0 })
  check(linger.container.parent === scene && Math.hypot(afterRelease.x - beforeRelease.x, afterRelease.y - beforeRelease.y) < 0.001, 'Linger survives remove-before-destroy without changing its world transform')
  step(linger, 40)
  check(linger.container.destroyed && scene.children.length === 0, 'Linger tail expires without orphan sprites')

  const pendingOwner = new Container()
  const pending = fxManager.attach({ preset: 'smoke' }, pendingOwner)
  pendingOwner.destroy({ children: true })
  check(await pending === null, 'Destroy during texture load cannot create a late emitter')

  const manager = new ObjectManager()
  manager.setParentContainer(scene)
  setWorldParams(12, 128)
  const burning = ResourceLoader.getResourceDef('campfire/burning')!
  await Promise.all(burning.layers.flatMap((layer) => layer.frames?.map((frame) => ResourceLoader.loadTexture(frame.img)) ?? (layer.img ? [ResourceLoader.loadTexture(layer.img)] : [])))
  let tickTime = performance.now()
  const tick = () => { Ticker.shared.update(tickTime += 100); manager.update(); app.render() }
  tick()
  check(activeEmitters().length === 0, 'FxManager removes completed emitter entries')
  const spawn = async (entityId: number, state: 'burning' | 'unlit') => {
    manager.spawnObject({ entityId, typeId: 100, resourcePath: `campfire/${state}`, position: coordScreen2Game(80 + (entityId - 1) * 140, 155), size: { x: 12, y: 12 } })
    await settle()
  }
  await spawn(1, 'burning')
  for (let index = 0; index < 20; index++) tick()
  const burningEmitter = activeEmitters()[0]!
  check(activeEmitters().length === 1 && burningEmitter.particleCount > 0 && burningEmitter.container.zIndex === 2, 'Burning ObjectView emits above flame z=1')
  scene.x = -5000
  cullingController.update(app, scene, scene)
  const culledSnapshot = snapshot(burningEmitter.container)
  for (let index = 0; index < 30; index++) tick()
  check(!manager.getObject(1)!.getContainer().visible && snapshot(burningEmitter.container) === culledSnapshot, 'Panning the viewport away freezes production smoke')
  scene.x = 30
  cullingController.update(app, scene, scene)
  tick()
  check(manager.getObject(1)!.getContainer().visible && snapshot(burningEmitter.container) !== culledSnapshot, 'Panning back resumes production smoke')
  await spawn(1, 'unlit')
  tick()
  check(activeEmitters().length === 1 && burningEmitter.container.parent === scene &&
    manager.getObject(1)!.getContainer().children.every((child) => child !== burningEmitter.container),
  'Burning-to-unlit stops spawning and reparents the live smoke tail outside the new unlit view')
  const tailParticleCount = burningEmitter.particleCount
  tick()
  check(burningEmitter.particleCount <= tailParticleCount, 'Unlit state does not spawn replacement smoke puffs')
  step(burningEmitter, 50)
  tick()
  check(burningEmitter.container.destroyed && activeEmitters().length === 0, 'Smoke tail fades out and self-removes after the fire goes unlit')
  const unlitChildren = manager.getObject(1)!.getContainer().children.length
  for (let cycle = 0; cycle < 20; cycle++) {
    await spawn(1, 'burning')
    for (let index = 0; index < 5; index++) tick()
    await spawn(1, 'unlit')
    const fadingEmitter = activeEmitters()[0]
    if (fadingEmitter) step(fadingEmitter, 50)
    tick()
    if (activeEmitters().length !== 0 || Number(scene.children.length) !== 1 || manager.getObject(1)!.getContainer().children.length !== unlitChildren) {
      throw new Error(`Appearance rebuild leaked at cycle ${cycle}`)
    }
  }
  check(true, '20 light/extinguish appearance cycles keep sprite and emitter counts stable')
  await spawn(1, 'burning')
  manager.clear()
  tick()
  check(activeEmitters().length === 0 && scene.children.length === 0, 'World cleanup releases all attached particles')

  // Leave a live production preview and explicit debug controls for manual review.
  await spawn(1, 'burning')
  await spawn(2, 'burning')
  await spawn(3, 'unlit')
  for (let index = 0; index < 25; index++) tick()
  let firstBurning = true
  let pannedAway = false
  document.querySelector('#toggle')!.addEventListener('click', () => {
    firstBurning = !firstBurning
    void spawn(1, firstBurning ? 'burning' : 'unlit')
    document.querySelector('#toggle')!.textContent = firstBurning ? 'Extinguish first fire' : 'Light first fire'
  })
  document.querySelector('#pan')!.addEventListener('click', () => {
    pannedAway = !pannedAway
    scene.x = pannedAway ? -5000 : 30
    cullingController.update(app, scene, scene)
    document.querySelector('#pan')!.textContent = pannedAway ? 'Pan back' : 'Pan away'
  })
  const firstEmitter = () => activeEmitters().find((effect) => effect.container.parent === manager.getObject(1)?.getContainer())
  document.querySelector('#stop')!.addEventListener('click', () => firstEmitter()?.stop())
  document.querySelector('#linger')!.addEventListener('click', () => {
    const effect = firstEmitter()
    if (effect) { effect.config.linger = true; manager.despawnObject(1) }
  })
  app.ticker.add(() => {
    cullingController.update(app, scene, scene)
    manager.update()
    const emitters = activeEmitters()
    document.querySelector('#metrics')!.textContent = `Emitters: ${emitters.length}; live puffs: ${emitters.reduce((count, effect) => count + effect.particleCount, 0)}; visible fires: ${cullingController.getMetrics().objectsVisible}`
  })
  const previewPuff = new Sprite(texture)
  previewPuff.position.set(825, 20)
  app.stage.addChild(previewPuff)
  check(true, 'All checks passed; live preview ready (two burning fires, one unlit)')
  Ticker.shared.start()
  app.start()
}

void main().catch((error: unknown) => {
  result.textContent = `${checks.join('\n')}\nFAIL: ${error instanceof Error ? error.stack : error}`
  console.error(error)
})
