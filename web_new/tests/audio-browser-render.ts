import { Application, Container, Text, WebGLRenderer } from 'pixi.js'
import { ActorRenderer } from '../src/game/actors/ActorRenderer'
import type { ActorCatalog } from '../src/game/actors/ActorAssetCatalog'
import { LocalAudioController } from '../src/game/LocalAudioController'
import type { SoundManager } from '../src/game/SoundManager'

export async function verifyRenderedVariants(catalog: ActorCatalog, manager: SoundManager, serverNow: number): Promise<unknown> {
  const check = (condition: unknown, message: string) => { if (!condition) throw new Error(message) }
  const app = new Application()
  await app.init({ width: 960, height: 450, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  app.stop()
  document.querySelector('#preview')!.replaceChildren(app.canvas)
  const renderer = new ActorRenderer(app.renderer as WebGLRenderer)
  const clips = ['chop_r', 'chop_l', 'walk', 'carry_walk']
  const handles = clips.map((clip, index) => {
    const container = new Container()
    container.position.set(120 + index * 240, 390)
    container.scale.set(2)
    const label = new Text({ text: clip, style: { fontSize: 10, fill: '#eddfba' } })
    label.anchor.set(.5, 0); label.y = 12
    app.stage.addChild(container)
    const handle = renderer.create()
    handle.priority = true
    container.addChild(handle.sprite, label)
    return handle
  })
  const dispose = () => { renderer.destroy(); app.destroy(true, { children: true }) }
  window.addEventListener('pagehide', dispose, { once: true })
  await Promise.all(handles.map(handle => handle.actor.ready))
  const chop = catalog.actionAnimations.tree_chop!
  await Promise.all(handles.slice(0, 2).map((handle, index) => {
    const variant = chop.variants.find(variant => variant.clip === clips[index])!
    return handle.actor.setEquipment(variant.equipment.map(item => ({ slot: item.slot, visualKey: item.visual_key })))
  }))
  const localContacts = { walk: 0, carry_walk: 0 }
  let locallyTriggeredWorldCues = 0
  const local = new LocalAudioController({ profile: key => manager.profile(key), play: (key, gain, options) => {
    if (manager.profile(key)?.mode === 'world') locallyTriggeredWorldCues++
    if (options?.sourceId === 3) localContacts.walk++
    if (options?.sourceId === 4) localContacts.carry_walk++
    return manager.play(key, gain, options)
  } })
  local.configure(catalog.locomotionAudio!, catalog.actionAnimations)
  local.setListener(3, 1); local.setListenerPosition({ x: 0, y: 0 })
  const poseSamples: Array<{ clip: string; cycle: number; opaquePixels: number; checksum: number }> = []
  let previousPhase = 0, previousCycle = 0, impacts = 0
  const initialPlayed = manager.metrics.played
  const start = performance.now()
  while (performance.now() - start < 4000) {
    const now = performance.now(), elapsed = now - start, cycle = Math.floor(elapsed / 2000), phase = elapsed % 2000 / 2000
    for (const [index, handle] of handles.entries()) {
      const actor = handle.actor
      actor.direction = 3
      actor.walking = index >= 2
      actor.carrying = index === 3
      actor.distanceTiles = elapsed / 960 * actor.cycleDistanceTiles
      actor.setActionAnimation(index < 2 ? { key: 'tree_chop', phase, facingAngle: -Math.PI / 2 } : null)
      local.update({ entityId: index + 1, actor: actor.assetId, position: { x: index === 3 ? 60 : 0, y: 0 }, ready: actor.isReady,
        moving: actor.walking, clip: clips[index]!, distanceTiles: actor.distanceTiles, discontinuity: false,
        action: index < 2 ? { generation: '1', revision: String(cycle + 1), animationKey: 'tree_chop', totalTicks: 20, elapsedTicks: 0,
          tickDurationMs: 100, serverTimeMs: serverNow + cycle * 2000, targetPosition: { x: 200, y: 100 } } : null,
        actionReady: actor.isActionAnimationSelected('tree_chop') }, now, serverNow + elapsed)
    }
    renderer.render(now)
    app.render()
    if ((cycle !== previousCycle || previousPhase < .6) && phase >= .6) {
      check(handles.slice(0, 2).every(handle => handle.actor.isActionAnimationSelected('tree_chop')), 'A rendered hand variant was ineligible')
      check(manager.play('chop', .104, { sourceId: 'preview-target' }), 'Rendered impact sound was rejected')
      impacts++
      for (const [index, handle] of handles.entries()) {
        const pixels = app.renderer.extract.pixels({ target: handle.sprite.texture }).pixels
        app.renderer.resetState()
        let opaquePixels = 0, checksum = 0
        for (let offset = 0; offset < pixels.length; offset++) {
          checksum = (checksum + pixels[offset]! * (offset + 1)) % 1000000007
          if (offset % 4 === 3 && pixels[offset]) opaquePixels++
        }
        check(opaquePixels > 500, `Rendered ${clips[index]} body is empty`)
        poseSamples.push({ clip: clips[index]!, cycle: cycle + 1, opaquePixels, checksum })
      }
    }
    previousPhase = phase; previousCycle = cycle
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
  }
  check(impacts === 2, 'Rendered chop cycles did not produce exactly two impacts')
  check(manager.metrics.played > initialPlayed + impacts, 'Rendered locomotion did not produce footstep contacts')
  check(localContacts.walk >= 2 && localContacts.carry_walk >= 2, 'A rendered locomotion variant missed contacts')
  check(locallyTriggeredWorldCues === 0, 'Rendered hand variants duplicated a server-owned cue locally')
  const glError = (app.renderer as WebGLRenderer).gl.getError()
  check(glError === 0, `Rendered audio preview reported GL error ${glError}`)
  // Leave the authored impact pose visible for the verification screenshot.
  handles.slice(0, 2).forEach(handle => handle.actor.setActionAnimation({ key: 'tree_chop', phase: .6, facingAngle: -Math.PI / 2 }))
  renderer.render(performance.now() + 1000); app.render()
  return { clips, poseSamples, impacts, admittedSounds: manager.metrics.played - initialPlayed, glError,
    locallyTriggeredWorldCues, localContacts, localContactsIndependentOfPoseSampling: true }
}
