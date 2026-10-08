import { Application, Assets, BitmapFont, Cache, Container, Graphics, Sprite } from 'pixi.js'
import { DamageNumberManager } from '../src/game/DamageNumberManager'
import { CAPACITY, type DamageNumberHit } from '../src/game/hud/damageNumbers'
import { AttackResultReceiver } from '../src/network/AttackResultReceiver'
import { proto } from '../src/network/proto/packets.js'

// Standalone visual fixture, intentionally not part of the application bundle.
const app = new Application()
await app.init({ resizeTo: window, background: '#172d21', resolution: 2, autoDensity: true, preference: 'webgl' })
document.body.append(app.canvas)
const world = new Container()
world.sortableChildren = true
app.stage.addChild(world)
const objects = new Map<number, { getContainer(): Container }>()
const lookup = { getObject: (id: number) => objects.get(id) }
const manager = new DamageNumberManager(world)
manager.installFont()
const receiver = new AttackResultReceiver()
receiver.reset(1)
const treeTexture = await Assets.load('/assets/game/obj/trees/willow/06.png')
const status = document.querySelector<HTMLOutputElement>('#status')!
const zoomInput = document.querySelector<HTMLSelectElement>('#zoom')!
const frameInput = document.querySelector<HTMLSelectElement>('#frame')!
let eventId = 0
let startedAt = performance.now()
let measurement = ''
let sweeping = false

function addTarget(id: number, x: number, y: number, tree = false): void {
  const container = new Container()
  container.position.set(x, y)
  if (tree) {
    const sprite = new Sprite(treeTexture)
    sprite.anchor.set(0.5, 1)
    container.addChild(sprite)
  } else {
    container.addChild(new Graphics().roundRect(-8, -16, 16, 16, 3).fill(0x647846))
  }
  container.zIndex = y
  world.addChild(container)
  objects.set(id, { getContainer: () => container })
}

function reset(): void {
  manager.clear()
  for (const object of objects.values()) object.getContainer().destroy({ children: true })
  objects.clear()
  addTarget(1, 0, 0, true)
  measurement = ''
  sweeping = false
}

function present(hits: readonly DamageNumberHit[], fresh = true): void {
  if (fresh) startedAt = performance.now()
  const packet = proto.S2C_AttackResult.fromObject({ streamEpoch: 1, eventId: ++eventId, attackerId: 9999, hits })
  if (receiver.accept(packet)) manager.show(hits, lookup, startedAt, Number(zoomInput.value))
}

document.querySelector('#hit')!.addEventListener('click', () => present([{ targetId: '1', damage: 81 / 13 }]))
document.querySelector('#repeat')!.addEventListener('click', () => {
  manager.clear()
  startedAt = performance.now()
  for (const damage of [3.6, 81 / 13, 0, 0.001]) present([{ targetId: '1', damage }], false)
})
document.querySelector('#fatal')!.addEventListener('click', () => {
  reset()
  startedAt = performance.now()
  manager.rememberDespawn(1, lookup, startedAt)
  objects.get(1)!.getContainer().destroy({ children: true })
  objects.delete(1)
  present([{ targetId: '1', damage: 30 }], false)
})
document.querySelector('#sweep')!.addEventListener('click', () => {
  reset()
  sweeping = true
  const hits: DamageNumberHit[] = []
  for (let index = 0; index < CAPACITY; index++) {
    const id = index + 2
    addTarget(id, (index % 32 - 15.5) * 27, 120 + Math.floor(index / 32) * 24)
    hits.push({ targetId: String(id), damage: (index % 99) + 0.6 })
  }
  present(hits)
})
document.querySelector('#reset')!.addEventListener('click', reset)
document.querySelector('#snapshot')!.addEventListener('click', () => {
  app.renderer.render(app.stage)
  const link = document.createElement('a')
  link.href = app.canvas.toDataURL('image/png')
  link.download = 'damage-numbers.png'
  link.click()
})
document.querySelector<HTMLSelectElement>('#resolution')!.addEventListener('change', event => {
  const resolution = Number((event.target as HTMLSelectElement).value)
  app.renderer.resize(window.innerWidth, window.innerHeight, resolution)
})
document.querySelector('#measure')!.addEventListener('click', () => {
  const samples: string[] = []
  for (const count of [0, 1, 10, 512]) {
    manager.clear()
    for (let index = 0; index < count; index++) manager.presentation.emit(index + 1, 3.6, 0, 0, 0)
    manager.update(0, 1)
    for (let iteration = 0; iteration < 1000; iteration++) manager.update(iteration % 600, 1)
    const before = performance.now()
    for (let iteration = 0; iteration < 10000; iteration++) manager.update(iteration % 600, 1)
    const updateUs = (performance.now() - before) * 1000 / 10000
    for (let iteration = 0; iteration < 5; iteration++) app.renderer.render(app.stage)
    const frameSamples: number[] = []
    for (let sample = 0; sample < 5; sample++) {
      const frameStart = performance.now()
      for (let iteration = 0; iteration < 100; iteration++) {
        manager.update(iteration % 600, 1)
        app.renderer.render(app.stage)
      }
      frameSamples.push((performance.now() - frameStart) * 1000 / 100)
    }
    frameSamples.sort((a, b) => a - b)
    samples.push(`${count}: update ${updateUs.toFixed(2)} / frame ${frameSamples[2]!.toFixed(2)} µs`)
  }
  for (let cycle = 0; cycle < 100; cycle++) {
    manager.clear()
    for (let index = 0; index < CAPACITY; index++) manager.presentation.emit(index + 1, cycle + 0.6, 0, 0, 0)
    manager.update(450, 1)
  }
  measurement = `${samples.join('\n')}\nFrame includes scene draw submission, not GPU completion.\nAfter 100 full cycles: ${manager.getContainer().children.length} pooled views`
  manager.clear()
})

reset()
app.ticker.add(() => {
  const zoom = Number(zoomInput.value)
  world.scale.set(zoom)
  world.position.set(app.screen.width / 2, sweeping ? app.screen.height * 0.28 - 120 * zoom : app.screen.height * 0.55 + treeTexture.height * zoom)
  const now = frameInput.value === 'live' ? performance.now() : startedAt + Number(frameInput.value)
  manager.update(now, zoom)
  const font = Cache.get<BitmapFont>('OriginDamageNumbers-bitmap')
  let atlasBytes = 0
  for (const page of font.pages) atlasBytes += page.texture.source.pixelWidth * page.texture.source.pixelHeight * 4
  status.value = `Active: ${manager.presentation.activeCount} / ${CAPACITY} | Zoom: ${zoom} | DPR: ${app.renderer.resolution}\nAtlas: ${font.pages.length} page(s), ${(atlasBytes / 1024).toFixed(1)} KiB RGBA\n${measurement}`
})

window.addEventListener('pagehide', () => {
  app.ticker.stop()
  manager.destroy()
  app.destroy(true, { children: true })
}, { once: true })
