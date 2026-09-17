import { Application, Container, WebGLRenderer } from 'pixi.js'
import { ActorRenderer } from '../src/game/actors/ActorRenderer'
import type { EquipmentSlot } from '../src/types/characterVisual'

async function main() {
  const app = new Application()
  await app.init({ width: 1100, height: 600, background: '#334132', antialias: false, resolution: 1, preference: 'webgl' })
  document.querySelector('#preview')!.append(app.canvas)
  const renderer = new ActorRenderer(app.renderer as WebGLRenderer)
  const handles = Array.from({ length: 8 }, (_, index) => {
    const container = new Container()
    container.position.set(145 + index % 4 * 260, 280 + Math.floor(index / 4) * 280)
    container.scale.set(2)
    app.stage.addChild(container)
    const handle = renderer.create()
    container.addChild(handle.sprite)
    handle.priority = true
    return handle
  })
  await Promise.all(handles.map(handle => handle.actor.ready))
  let slot: EquipmentSlot = 'right_hand'
  let equipped = true
  let walking = true
  const equip = () => Promise.all(handles.map(handle => handle.actor.setEquipment(equipped ? [{ slot, visualKey: 'stone_axe' }] : [])))
  await equip()
  const button = (label: string, action: () => void) => {
    const element = document.createElement('button')
    element.textContent = label
    element.onclick = action
    document.querySelector('#controls')!.append(element)
  }
  button('Ходьба / стойка', () => { walking = !walking })
  button('Левая / правая', () => { slot = slot === 'right_hand' ? 'left_hand' : 'right_hand'; void equip() })
  button('Снять / надеть', () => { equipped = !equipped; void equip() })
  button('Переноска', () => { handles.forEach(handle => { handle.actor.carrying = !handle.actor.carrying }) })
  let distance = 0
  app.ticker.add(ticker => {
    if (walking) distance += ticker.deltaMS / 960 * handles[0]!.actor.cycleDistanceTiles
    handles.forEach((handle, direction) => {
      handle.actor.direction = direction
      handle.actor.walking = walking
      handle.actor.distanceTiles = distance
    })
    renderer.render(performance.now())
  }, undefined, 50)
  document.querySelector('#result')!.textContent = '8 направлений • 1200 треугольников • обычный хват из каталога • фаза от пройденного расстояния'
  Object.assign(window, { axeReview: { handles, renderer, app } })
}
void main().catch(error => { document.querySelector('#result')!.textContent = String(error); console.error(error) })
