import { Application, Container, WebGLRenderer } from 'pixi.js'
import { ActorRenderer, type ActorHandle } from '../src/game/actors/ActorRenderer'
import { screenFacingAngle } from '../src/game/actors/facing'

const result = document.querySelector<HTMLPreElement>('#result')!
const checks: string[] = []
function check(value: unknown, message: string): asserts value { if (!value) throw new Error(message) }
function pass(message: string) { checks.push(`PASS ${message}`); result.textContent = checks.join('\n') }

async function main() {
  const app = new Application()
  await app.init({ width: 600, height: 340, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  app.stop()
  document.querySelector('#preview')!.append(app.canvas)
  const renderer = new ActorRenderer(app.renderer as WebGLRenderer)
  const catalog = await renderer.cache.catalog
  const requestedBinding = new URLSearchParams(window.location.search).get('binding')
  const definition = Object.values(catalog.actionAnimations).find(binding => (!requestedBinding || binding.key === requestedBinding) &&
    binding.unbind_equipment_slots?.length && binding.variants[0]?.equipment.length === 0 && binding.preview?.equipment.length &&
    binding.preview.equipment.every(item => binding.unbind_equipment_slots!.includes(item.slot)))
  check(definition, 'Publish a matching unbind definition with unrestricted equipment and preview equipment before this review')
  const handles = [renderer.create(), renderer.create()]
  const [equipped, bare] = handles as [ActorHandle, ActorHandle]
  let now = performance.now() + 1000
  for (const [index, handle] of handles.entries()) {
    const container = new Container()
    container.position.set(150 + index * 300, 300)
    container.scale.set(2)
    container.addChild(handle.sprite)
    app.stage.addChild(container)
  }
  // Both local priority and observer throttling must apply attachment changes.
  equipped.priority = true
  bare.priority = false
  const render = () => { renderer.render(now += 1000); app.render(); check((app.renderer as WebGLRenderer).gl.getError() === 0, 'WebGL error') }
  const pixels = (handle: ActorHandle) => {
    const values = app.renderer.extract.pixels({ target: handle.sprite.texture }).pixels.slice()
    app.renderer.resetState()
    return values
  }
  const equal = (left: ArrayLike<number>, right: ArrayLike<number>) => left.length === right.length && Array.from(left).every((value, index) => value === right[index])
  try {
    await Promise.all(handles.map(handle => handle.actor.ready))
    await equipped.actor.setEquipment(definition.preview!.equipment.map(item => ({ slot: item.slot, visualKey: item.visual_key })))
    render(); render()
    const original = pixels(equipped)
    check(!equal(original, pixels(bare)), 'Equipment must be visible before the action')
    const bytes = renderer.metrics.assetBytes
    pass(`Published definition ${definition.key} / loaded equipment visible before action`)
    for (let direction = 0; direction < 8; direction++) {
      for (const phase of [0, .25, .5, 1]) {
        for (const handle of handles) {
          handle.actor.direction = direction
          handle.actor.setActionAnimation({ key: definition.key, phase, facingAngle: screenFacingAngle(direction) })
        }
        render(); render()
        check(equal(pixels(equipped), pixels(bare)), `Detached equipment still affects pixels: direction ${direction}, phase ${phase}`)
      }
    }
    check(renderer.metrics.assetBytes === bytes, 'Unbind must retain loaded resources')
    check(equipped.actor.equippedVisuals.length === definition.preview!.equipment.length, 'Unbind must retain equipped state')
    pass('Eight facings / four phases / terminal hold / exact pixels equal bare reference / retained equipment state')
    const frame = pixels(equipped), width = equipped.sprite.texture.width
    for (let index = 3; index < frame.length; index += 4) {
      if (!frame[index]) continue
      const pixel = (index - 3) / 4
      check(renderer.hitTest(equipped, pixel % width + equipped.sprite.x, Math.floor(pixel / width) - equipped.anchorY), 'Detached-state picking must match the body')
      break
    }
    const gl = (app.renderer as WebGLRenderer).gl, extension = gl.getExtension('WEBGL_lose_context')!
    const lost = new Promise<void>(resolve => app.canvas.addEventListener('webglcontextlost', () => resolve(), { once: true }))
    extension.loseContext(); await lost
    await new Promise(resolve => setTimeout(resolve, 100))
    const restored = new Promise<void>(resolve => app.canvas.addEventListener('webglcontextrestored', () => resolve(), { once: true }))
    extension.restoreContext()
    await Promise.race([restored, new Promise((_, reject) => setTimeout(() => reject(new Error('Context restore timed out')), 8000))])
    render()
    check(equal(frame, pixels(equipped)), 'Context restoration changed the detached pose')
    pass('Picking / WebGL context restoration while equipment is detached')
    for (const handle of handles) { handle.actor.direction = 3; handle.actor.setActionAnimation(null) }
    render(); render()
    check(equal(original, pixels(equipped)), 'Rebind must restore the original equipped pixels')
    check(!equal(pixels(equipped), pixels(bare)), 'Rebound equipment must be visible')
    pass('Cancellation / blend out / exact original equipped pixels restored')
  } finally {
    for (const handle of handles) renderer.release(handle)
    renderer.destroy()
  }
  check(renderer.metrics.assets === 0 && renderer.metrics.actors === 0, 'Teardown leaked resources')
  pass('Complete teardown')
  result.textContent += '\n\nALL CHECKS PASSED'
}
main().catch(error => { result.textContent += `\nFAIL ${error.stack ?? error}`; console.error(error) })
