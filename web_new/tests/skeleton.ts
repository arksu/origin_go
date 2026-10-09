import { Application, Container, Graphics, Text, TextureStyle, WebGLRenderer } from 'pixi.js'
import { ActorRenderer } from '../src/game/actors/ActorRenderer'
import { ObjectView } from '../src/game/ObjectView'
import { coordScreen2Game } from '../src/game/utils/coordConvert'

async function main() {
  TextureStyle.defaultOptions.scaleMode = 'nearest'
  const app = new Application()
  await app.init({ width: 1200, height: 460, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  document.querySelector('#preview')!.append(app.canvas)
  const renderer = new ActorRenderer(app.renderer as WebGLRenderer)
  const world = new Container()
  app.stage.addChild(world)
  const cases = [
    { label: 'Standing', resourcePath: 'player', typeId: 11, heading: 0, lying: false },
    { label: 'Body / east', resourcePath: 'player', typeId: 15, heading: -Math.PI / 4, lying: true },
    { label: 'Body / southeast', resourcePath: 'player', typeId: 15, heading: 0, lying: true },
    { label: 'With skull', resourcePath: 'player_skeleton', typeId: 17, heading: 0, lying: false },
    { label: 'Without skull', resourcePath: 'player_skeleton_without_skull', typeId: 18, heading: 0, lying: false },
  ]
  const views = cases.map((entry, index) => {
    const container = new Container()
    container.position.set(120 + index * 240, 270)
    container.scale.set(2)
    const view = new ObjectView({ entityId: index + 1, typeId: entry.typeId, resourcePath: entry.resourcePath,
      position: { x: 0, y: 0, heading: entry.heading }, size: { x: 9, y: 9 } }, renderer)
    view.setKnockedOutPose(entry.lying)
    const label = new Text({ text: entry.label, style: { fontFamily: 'monospace', fontSize: 8, fill: '#d8e3cc' } })
    label.anchor.set(.5)
    label.position.set(0, 78)
    const anchor = new Graphics().moveTo(-4, 0).lineTo(4, 0).moveTo(0, -4).lineTo(0, 4).stroke({ color: '#d56a55', width: 1 })
    container.addChild(view.getContainer(), anchor, label)
    world.addChild(container)
    return view
  })
  app.canvas.addEventListener('pointermove', (event) => {
    const rect = app.canvas.getBoundingClientRect()
    const screen = { x: (event.clientX - rect.left) * app.screen.width / rect.width, y: (event.clientY - rect.top) * app.screen.height / rect.height }
    for (const view of views) {
      view.setHovered(view.hitTestRmbScreenPoint(screen.x, screen.y, coordScreen2Game))
    }
  })
  app.ticker.add(() => {
    for (const view of views) view.updateAnimation(performance.now())
    renderer.render(performance.now())
    document.querySelector('#result')!.textContent = JSON.stringify(renderer.metrics, null, 2)
  })
}

void main().catch((error: unknown) => {
  document.querySelector('#result')!.textContent = error instanceof Error ? error.stack ?? error.message : String(error)
})
