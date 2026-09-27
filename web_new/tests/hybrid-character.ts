import { facingFromDisplacement, screenFacingAngle } from '../src/game/actors/facing'
import { coordScreen2Game } from '../src/game/utils/coordConvert'
import { Application, Container, Graphics, Sprite, Text, WebGLRenderer } from 'pixi.js'
import { ResourceLoader } from '../src/game/ResourceLoader'
import { ActorRenderer, type ActorHandle } from '../src/game/actors/ActorRenderer'

const result = document.querySelector<HTMLPreElement>('#result')!
const state = document.querySelector<HTMLSelectElement>('#state')!
const count = document.querySelector<HTMLSelectElement>('#count')!
const speed = document.querySelector<HTMLInputElement>('#speed')!
const chopPlay = document.querySelector<HTMLInputElement>('#chop-play')!
const chopFrame = document.querySelector<HTMLInputElement>('#chop-frame')!
const initialState = new URLSearchParams(window.location.search).get('state')
if (initialState && [...state.options].some(option => option.value === initialState)) state.value = initialState
const frames: number[] = []
const handles: ActorHandle[] = []
const shadows = new Map<ActorHandle, Graphics>()
const directions = ['S', 'SE', 'E', 'NE', 'N', 'NW', 'W', 'SW']
const indices = [3, 2, 1, 0, 7, 6, 5, 4]

async function main() {
  const app = new Application()
  await app.init({ width: Math.min(1080, window.innerWidth - 48), height: 730, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  document.querySelector('#preview')!.append(app.canvas)
  const renderer = new ActorRenderer(app.renderer as WebGLRenderer)
  app.ticker.maxFPS = 30
  const world = new Container()
  app.stage.addChild(world)
  const barrel = await ResourceLoader.loadTexture('obj/barrel/barrel.png')
  barrel.source.scaleMode = 'nearest'
  let previous = performance.now()
  let generation = 0
  let lastReport = 0
  let failure: unknown = null
  let actionStart = performance.now()

  async function applyState() {
    actionStart = performance.now()
    for (const handle of handles) {
      const chopping = state.value.startsWith('chop_')
      await handle.actor.setEquipment(chopping ? [{ slot: state.value === 'chop_l' ? 'left_hand' : 'right_hand', visualKey: 'stone_axe' }] : [])
      handle.actor.setChopCycle(chopping ? { startMs: actionStart, durationMs: 2000 } : null)
    }
  }
  state.addEventListener('change', () => { void applyState().catch(error => { failure = error }) })

  async function populate() {
    const ownGeneration = ++generation
    shadows.clear()
    frames.length = 0
    previous = performance.now()
    for (const handle of handles.splice(0)) renderer.release(handle)
    for (const child of world.removeChildren()) child.destroy({ children: true })
    const number = Number(count.value)
    const scale = number > 8 ? 1 : 2
    const columns = number > 8 ? 10 : 4
    const spacing = (app.screen.width - 32) / columns
    for (let index = 0; index < number; index++) {
      const container = new Container()
      container.position.set(16 + spacing * (index % columns) + spacing / 2, (number > 8 ? 185 : 360) + Math.floor(index / columns) * (number > 8 ? 160 : 330))
      container.scale.set(scale)
      const shadow = new Graphics().ellipse(0, 0, 15, 5).fill({ color: '#17201b', alpha: .45 })
      const angle = screenFacingAngle(indices[index % 8]!)
      const rayX = Math.cos(angle) * 38
      const rayY = Math.sin(angle) * 38
      const ray = new Graphics().moveTo(-rayX, -rayY).lineTo(rayX, rayY).stroke({ color: '#83a7a0', width: 1 })
      ray.circle(rayX, rayY, 2).fill('#d5e9ae')
      container.addChild(ray, shadow)
      const handle = renderer.create()
      handle.priority = true
      const delta = coordScreen2Game(rayX, rayY)
      handle.actor.direction = facingFromDisplacement(delta.x, delta.y, 3)
      handle.actor.distanceTiles = index * .021
      container.addChild(handle.sprite)
      const reference = new Sprite(barrel)
      reference.position.set(22, -barrel.height)
      container.addChild(reference)
      const label = new Text({ text: directions[index % 8], style: { fontSize: 8, fill: '#bec4a6', fontFamily: 'monospace' } })
      label.anchor.set(.5, 0)
      label.y = 12
      container.addChild(label)
      world.addChild(container)
      handles.push(handle)
      shadows.set(handle, shadow)
    }
    await Promise.all(handles.map((handle) => handle.actor.ready))
    if (ownGeneration !== generation) return
    await applyState()
  }
  count.addEventListener('change', () => { void populate().catch((error) => { failure = error }) })
  app.canvas.addEventListener('pointermove', (event) => {
    const rect = app.canvas.getBoundingClientRect()
    const point = { x: (event.clientX - rect.left) * app.screen.width / rect.width, y: (event.clientY - rect.top) * app.screen.height / rect.height }
    for (const handle of handles) {
      const local = handle.sprite.parent!.toLocal(point)
      handle.actor.hovered = renderer.hitTest(handle, local.x, local.y)
    }
  })
  document.querySelector('#context')!.addEventListener('click', () => {
    const extension = (app.renderer as WebGLRenderer).gl.getExtension('WEBGL_lose_context')
    extension?.loseContext()
    window.setTimeout(() => extension?.restoreContext(), 1000)
  })
  app.ticker.add(() => {
    const now = performance.now()
    const delta = Math.min(now - previous, 100)
    frames.push(now - previous)
    if (frames.length > 300) frames.shift()
    previous = now
    for (const handle of handles) {
      if (!handle.actor.isReady) continue
      handle.actor.walking = state.value.endsWith('walk')
      handle.actor.carrying = state.value.startsWith('carry')
      handle.actor.knockedOut = state.value === 'knocked_out'
      if (state.value.startsWith('chop_') && !chopPlay.checked) {
        handle.actor.setChopCycle({ startMs: now - Number(chopFrame.value) / 69 * 2000, durationMs: 2000 })
      }
      if (handle.actor.walking) handle.actor.distanceTiles += delta / 960 * handle.actor.cycleDistanceTiles * Number(speed.value)
      const angle = screenFacingAngle(handle.actor.direction)
      const travel = handle.actor.walking ? ((handle.actor.distanceTiles / handle.actor.cycleDistanceTiles) % 1) * 32 - 16 : 0
      const offsetX = Math.cos(angle) * travel
      const offsetY = Math.sin(angle) * travel
      handle.sprite.position.set(-handle.anchorX + offsetX, -handle.anchorY + offsetY)
      const shadow = shadows.get(handle)!
      shadow.clear()
      if (handle.actor.knockedOut) shadow.ellipse(3, 4, 46, 8).fill({ color: '#17201b', alpha: .3 })
      else shadow.ellipse(0, 0, 15, 5).fill({ color: '#17201b', alpha: .45 })
      shadow.position.set(offsetX, offsetY)
    }
    try { renderer.render(now) } catch (error) { failure = error }
    if (now - lastReport > 500) {
      const ordered = [...frames].sort((first, second) => first - second)
      const average = frames.reduce((sum, value) => sum + value, 0) / frames.length
      result.textContent = failure ? String(failure instanceof Error ? failure.stack : failure) : JSON.stringify({
        status: handles.every((handle) => handle.actor.isReady) ? 'ready' : 'loading',
        fps: +(1000 / average).toFixed(1), p95FrameMs: +(ordered[Math.floor(ordered.length * .95)] ?? 0).toFixed(1),
        ...renderer.metrics, glError: (app.renderer as WebGLRenderer).gl.getError(),
      }, null, 2)
      lastReport = now
    }
  })
  await populate()
  Object.assign(window, { hybridReview: { app, renderer, handles } })
}
void main().catch((error: unknown) => { result.textContent = error instanceof Error ? error.stack ?? error.message : String(error) })
