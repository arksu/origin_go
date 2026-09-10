import { Application, Container, Graphics, Text, WebGLRenderer } from 'pixi.js'
import { ActorRenderer, type ActorHandle } from '../src/game/actors/ActorRenderer'
import { ACTOR_RENDER, type EquipmentId } from '../src/game/actors/config'

const result = document.querySelector<HTMLPreElement>('#result')!
const state = document.querySelector<HTMLSelectElement>('#state')!
const count = document.querySelector<HTMLSelectElement>('#count')!
const speed = document.querySelector<HTMLInputElement>('#speed')!
const belt = document.querySelector<HTMLInputElement>('#belt')!
const wrap = document.querySelector<HTMLInputElement>('#wrap')!
const frames: number[] = []
const handles: ActorHandle[] = []
const directions = ['S', 'SE', 'E', 'NE', 'N', 'NW', 'W', 'SW']
const indices = [3, 2, 1, 0, 7, 6, 5, 4]

async function main() {
  const app = new Application()
  await app.init({ width: Math.min(1080, window.innerWidth - 48), height: 590, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  document.querySelector('#preview')!.append(app.canvas)
  const renderer = new ActorRenderer(app.renderer as WebGLRenderer)
  app.ticker.maxFPS = 30
  const world = new Container()
  app.stage.addChild(world)
  let previous = performance.now()
  let generation = 0
  let lastReport = 0
  let failure: unknown = null

  const equipped = (): EquipmentId[] => [...(wrap.checked ? ['linen_wrap' as const] : []), ...(belt.checked ? ['linen_belt' as const] : [])]
  async function populate() {
    const ownGeneration = ++generation
    for (const handle of handles.splice(0)) renderer.release(handle)
    for (const child of world.removeChildren()) child.destroy({ children: true })
    const number = Number(count.value)
    const scale = number > 8 ? 1 : 2
    const columns = number > 8 ? 10 : 4
    const spacing = (app.screen.width - 32) / columns
    for (let index = 0; index < number; index++) {
      const container = new Container()
      container.position.set(16 + spacing * (index % columns) + spacing / 2, (number > 8 ? 145 : 248) + Math.floor(index / columns) * (number > 8 ? 160 : 264))
      container.scale.set(scale)
      const shadow = new Graphics().ellipse(0, 0, 15, 5).fill({ color: '#17201b', alpha: .45 })
      container.addChild(shadow)
      const handle = renderer.create()
      handle.priority = true
      handle.actor.direction = indices[index % 8]!
      handle.actor.distanceTiles = index * .021
      container.addChild(handle.sprite)
      const label = new Text({ text: directions[index % 8], style: { fontSize: 8, fill: '#bec4a6', fontFamily: 'monospace' } })
      label.anchor.set(.5, 0)
      label.y = 12
      container.addChild(label)
      world.addChild(container)
      handles.push(handle)
    }
    await Promise.all(handles.map((handle) => handle.actor.ready))
    if (ownGeneration !== generation) return
    await Promise.all(handles.map((handle) => handle.actor.setEquipment(equipped())))
  }
  count.addEventListener('change', () => { void populate().catch((error) => { failure = error }) })
  for (const input of [belt, wrap]) input.addEventListener('change', () => {
    void Promise.all(handles.map((handle) => handle.actor.setEquipment(equipped()))).catch((error) => { failure = error })
  })
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
      handle.actor.walking = state.value.endsWith('walk')
      handle.actor.carrying = state.value.startsWith('carry')
      if (handle.actor.walking) handle.actor.distanceTiles += delta / 960 * ACTOR_RENDER.cycleDistanceTiles * Number(speed.value)
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
