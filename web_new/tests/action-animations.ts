import { Application, Texture, WebGLRenderer } from 'pixi.js'
import { ActorRenderer, type ActorHandle } from '../src/game/actors/ActorRenderer'
import { ACTOR_RENDER } from '../src/game/actors/config'
import { screenFacingAngle } from '../src/game/actors/facing'

export async function verifyActionAnimations(app: Application, renderer: ActorRenderer, handle: ActorHandle): Promise<void> {
  const check = (value: unknown, message: string) => { if (!value) throw new Error(message) }
  const settings = renderer.getSettings(), actor = handle.actor, immersionPx = handle.immersionPx
  const gl = (app.renderer as WebGLRenderer).gl
  const checkGL = (stage: string) => { const error = gl.getError(); check(error === 0, `GL error ${error} at ${stage}`) }
  checkGL('before action review')
  const catalog = await renderer.cache.catalog
  handle.immersionPx = 0
  actor.walking = actor.carrying = actor.knockedOut = false
  actor.stopProgress = undefined
  renderer.setSettings({ mode: 'hybrid3d', turnDurationMs: 1 })
  let now = performance.now() + 1000
  try {
    check(Object.keys(catalog.actionAnimations).length > 0, 'Review requires at least one published action definition')
    for (const binding of Object.values(catalog.actionAnimations)) {
      for (const variant of binding.variants) {
        await actor.setEquipment(variant.equipment.map(item => ({ slot: item.slot, visualKey: item.visual_key })))
        checkGL(`equipment update ${binding.key}/${variant.clip}`)
        for (let direction = 0; direction < 8; direction++) {
          actor.direction = direction
          for (let sample = 0; sample <= 32; sample++) {
            actor.setActionAnimation({ key: binding.key, phase: sample / 32, facingAngle: screenFacingAngle(direction) })
            renderer.render(now)
            now += binding.blend_ms + 1000
            renderer.render(now)
            checkGL(`render ${binding.key}/${variant.clip}/${direction}/${sample}`)
            const texture = handle.sprite.texture, frame = actor.outputFrame
            check(texture.width === frame.width && texture.height === frame.height, 'Output must match declared frame')
            check(handle.anchorY === frame.origin_y && handle.sprite.x === -frame.origin_x, 'Frame expansion must preserve the ground anchor')
            const { pixels, width, height } = app.renderer.extract.pixels({ target: texture })
            app.renderer.resetState()
            checkGL('extract')
            let opaque = 0, firstOpaque = -1
            for (let index = 3; index < pixels.length; index += 4) if (pixels[index]) { opaque++; if (firstOpaque < 0) firstOpaque = (index - 3) / 4 }
            check(opaque > 500, 'Sample must contain the rendered body')
            for (let column = 0; column < width; column++) {
              check(!pixels[column * 4 + 3] && !pixels[((height - 1) * width + column) * 4 + 3], `Vertical clipping: ${binding.key}/${variant.clip}, direction ${direction}, sample ${sample}`)
            }
            for (let row = 0; row < height; row++) {
              check(!pixels[(row * width) * 4 + 3] && !pixels[(row * width + width - 1) * 4 + 3], `Horizontal clipping: ${binding.key}/${variant.clip}, direction ${direction}, sample ${sample}`)
            }
            check(renderer.hitTest(handle, firstOpaque % width - frame.origin_x, Math.floor(firstOpaque / width) - frame.origin_y), 'Picking must use the complete output dimensions')
            check(!renderer.hitTest(handle, -frame.origin_x, -frame.origin_y), 'Transparent expanded corner must not capture a click')
            checkGL('picking')
            check(renderer.metrics.outputBytes >= width * height * 4, 'Pool accounting must include actual dimensions')
            now += 33
          }
        }
      }
      const beforeRestore = app.renderer.extract.pixels({ target: handle.sprite.texture }).pixels
      app.renderer.resetState()
      const extension = gl.getExtension('WEBGL_lose_context')!
      const lost = new Promise<void>(resolve => app.canvas.addEventListener('webglcontextlost', () => resolve(), { once: true }))
      extension.loseContext()
      await lost
      await new Promise(resolve => setTimeout(resolve, 100))
      const restored = new Promise<void>(resolve => app.canvas.addEventListener('webglcontextrestored', () => resolve(), { once: true }))
      extension.restoreContext()
      await Promise.race([restored, new Promise((_, reject) => setTimeout(() => reject(new Error('Action context restore timed out')), 8000))])
      renderer.render(now += 1000)
      const afterRestore = app.renderer.extract.pixels({ target: handle.sprite.texture }).pixels
      app.renderer.resetState()
      check(beforeRestore.every((value, index) => value === afterRestore[index]), 'Restoration must preserve the exact expanded frame and pose')
      checkGL('expanded frame context restoration')
      actor.setActionAnimation(null)
      renderer.render(now)
      check(handle.sprite.texture.height > ACTOR_RENDER.cellSize, 'Blend-out retains outgoing bounds')
      renderer.render(now += binding.blend_ms + 1000)
      checkGL('return to base frame')
      check(handle.sprite.texture !== Texture.EMPTY && handle.sprite.texture.height === ACTOR_RENDER.cellSize, 'Cancel must return to ordinary dimensions')
      check(handle.anchorY === ACTOR_RENDER.anchorY, 'Cancel restores the ordinary anchor')
    }
    check((app.renderer as WebGLRenderer).gl.getError() === 0, 'Action passes must preserve shared WebGL state')
  } finally {
    actor.setActionAnimation(null)
    await actor.setEquipment([])
    renderer.setSettings(settings)
    handle.immersionPx = immersionPx
  }
}
