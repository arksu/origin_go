import { Application, Container, Graphics, Sprite, Text } from 'pixi.js'
import { ObjectManager } from '../src/game/ObjectManager'
import { ObjectView } from '../src/game/ObjectView'
import { MOVEMENT_DIRECTIONS, ResourceLoader } from '../src/game/ResourceLoader'
import { coordScreen2Game } from '../src/game/utils/coordConvert'
import { coordGame2Screen } from '../src/game/utils/coordConvert'
import { getCoordPerTile, setWorldParams } from '../src/game/tiles/Tile'
import { moveController } from '../src/game/MoveController'
import { timeSync } from '../src/network/TimeSync'
import { verifyMovementStopping } from './movement-stop'

const result = document.querySelector<HTMLPreElement>('#result')!
const checks: string[] = []
function check(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}
function spriteOf(view: ObjectView): Sprite {
  const sprite = view.getContainer().children.find((child) => child instanceof Sprite)
  check(sprite instanceof Sprite, 'Player sprite must be loaded')
  return sprite
}
const nextPaint = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))

async function main() {
  const app = new Application()
  await app.init({ width: 960, height: 510, background: '#313a2b', antialias: false, resolution: 1 })
  app.stop()
  document.querySelector('#preview')!.append(app.canvas)
  const world = new Container()
  app.stage.addChild(world)
  const manager = new ObjectManager()
  manager.setParentContainer(world)
  const options = { typeId: 1, resourcePath: 'player', position: { x: 0, y: 0 }, size: { x: 4, y: 4 } }
  const playerDef = ResourceLoader.getResourceDef('player')!
  const sheet = playerDef.layers[0]!.spriteSheet!
  setWorldParams(12, 128)
  const cycleDistance = sheet.cycleDistanceTiles * getCoordPerTile()
  const frameDistance = cycleDistance / 8

  // Exercise changes arriving before the shared asynchronous load finishes.
  manager.spawnObject({ ...options, entityId: 1 })
  manager.spawnObject({ ...options, entityId: 2 })
  manager.spawnObject({ ...options, entityId: 3 })
  const player = manager.getObject(1)!
  const stoppedDuringLoad = manager.getObject(2)!
  check(player.hasAnimatedFrames(), 'Animation must register synchronously')
  player.onMoved(1, frameDistance * 2)
  stoppedDuringLoad.onMoved(5)
  stoppedDuringLoad.onStopped()
  manager.despawnObject(3)
  const textures = await ResourceLoader.loadDirectionalSpriteSheet(sheet)
  await nextPaint()
  const sprite = spriteOf(player)
  check(sprite.texture.frame.y === 192, 'Move before load must face E')
  check(sprite.texture === textures[1]![2], 'Distance traveled during load must retain phase')
  check(spriteOf(stoppedDuringLoad).texture === textures[5]![8], 'Stop before load must retain W idle')
  check(manager.getObjectCount() === 2 && world.children.length === 2, 'Despawn during load must not attach a sprite')
  checks.push('PASS asynchronous move / turn / stop / despawn')

  const expectedRows = [3, 2, 1, 0, 7, 6, 5, 4]
  player.onStopped()
  const start = performance.now()
  for (let direction = 0; direction < 8; direction++) {
    player.onStopped()
    player.onMoved(direction)
    for (let frame = 0; frame < 8; frame++) {
      if (frame > 0) player.onMoved(direction, frameDistance)
      check(sprite.texture.frame.x === frame * 80 && sprite.texture.frame.y === expectedRows[direction]! * 96,
        `Wrong crop: ${MOVEMENT_DIRECTIONS[direction]} frame ${frame}`)
      check(sprite.texture.width === 80 && sprite.texture.height === 96, 'Native frame size')
      check(sprite.x === -40 && sprite.y === -84 && sprite.scale.x === 1, 'Stable ground anchor and native scale')
    }
    player.onStopped()
    player.updateAnimation(start + 9000)
    check(sprite.texture.frame.x === 640 && sprite.texture.frame.y === expectedRows[direction]! * 96,
      `Separate standing pose must hold: ${MOVEMENT_DIRECTIONS[direction]}`)
    player.onMoved(direction)
  }
  player.onMoved(7, cycleDistance)
  check(sprite.texture.frame.x === 0, 'Cycle must wrap after the calibrated distance')
  player.onMoved(5, frameDistance * 4)
  check(sprite.texture === textures[5]![4], 'Turning must preserve stride phase')
  player.onMoved(5, frameDistance)
  check(sprite.texture === textures[5]![5], 'Repeated movement must not restart stride')
  player.updateAnimation(start + 900000)
  player.onMoved(5, 0)
  check(sprite.texture === textures[5]![5], 'Elapsed time and zero displacement must not advance walking')
  player.onStopped()
  player.updateAnimation(start + 9000)
  check(sprite.texture === textures[5]![8], 'Idle must remain stable and retain facing')
  player.onMoved(5)
  check(sprite.texture === textures[5]![0], 'Restart from idle must begin a fresh stride')
  checks.push('PASS 64 walk + 8 standing crops / walk excludes idle / wrap / turn phase / stop / restart')

  for (const frameRate of [30, 60, 144]) {
    player.onStopped()
    for (let tick = 0; tick < frameRate; tick++) player.onMoved(1, cycleDistance * 2.375 / frameRate)
    check(sprite.texture === textures[1]![3], `Equal distance must produce equal phase at ${frameRate} render FPS`)
  }
  player.onStopped()
  for (let tick = 0; tick < 60; tick++) player.onMoved(1, cycleDistance * .3 / 60)
  check(sprite.texture === textures[1]![2], 'Slow movement must advance only its traveled distance')
  player.onStopped()
  for (let tick = 0; tick < 60; tick++) player.onMoved(1, cycleDistance * .6 / 60)
  check(sprite.texture === textures[1]![4], 'Double speed must advance double the phase')
  player.onMoved(5, cycleDistance * .0875)
  check(sprite.texture === textures[5]![5], 'Changing speed and direction must retain fractional phase')
  player.onStopped()
  setWorldParams(32, 128)
  player.onMoved(1, sheet.cycleDistanceTiles * 32 / 8)
  check(sprite.texture === textures[1]![1], 'Calibration must follow negotiated world units per tile')
  player.onStopped()
  setWorldParams(12, 128)
  player.onMoved(1, frameDistance)
  check(sprite.texture === textures[1]![1], 'One frame must cover the same tile distance with 12 units per tile')
  checks.push('PASS 30 / 60 / 144 render FPS / variable speed / fractional phase / world scale')

  player.onMoved(5)
  const bounds = player.computeScreenBounds()
  check(bounds.maxY >= 12 && bounds.minY <= -84, 'Culling must include feet below anchor')
  manager.setKnockedOutPose(1, true)
  player.onMoved(1, cycleDistance)
  player.updateAnimation(start + 10000)
  check(sprite.texture === textures[5]![8], 'KO must stop walking and ignore subsequent movement')
  const lyingBounds = player.computeScreenBounds()
  check(lyingBounds.minX <= -84 && lyingBounds.maxY >= 40, 'KO frame must fit culling bounds')
  manager.setKnockedOutPose(1, false)
  checks.push('PASS standing / KO culling bounds and stopped KO pose')

  player.onMoved(3)
  player.onStopped()
  app.render()
  check(!player.hitTestRmbScreenPoint(-39, -83, coordScreen2Game), 'Transparent corner must not catch clicks')
  check(player.hitTestRmbScreenPoint(0, -45, coordScreen2Game), 'Opaque torso must catch clicks')
  player.setHovered(true)
  player.onMoved(5, frameDistance * 3)
  player.updateAnimation(performance.now() + 300)
  app.render()
  player.setHovered(false)
  check(sprite.texture.source.scaleMode === 'nearest' && sprite.roundPixels, 'Pixel filtering and snapping')
  check(textures === await ResourceLoader.loadDirectionalSpriteSheet(sheet), 'Frame textures must be shared')
  manager.despawnObject(2)
  check(!sprite.texture.source.destroyed, 'Despawning another entity must retain shared atlas')
  checks.push('PASS alpha hit testing / animated hover / nearest sampling / shared atlas ownership')

  player.onStopped()
  manager.updateObjectPosition(1, 0, 0, false, 1)
  manager.updateObjectPosition(1, frameDistance, 0, true, 1)
  check(sprite.texture === textures[1]![1], 'ObjectManager must derive distance from world position')
  const before = sprite.texture
  await new Promise((resolve) => setTimeout(resolve, 150))
  manager.update()
  check(sprite.texture === before, 'Clock ticks without displacement must not animate walking')
  world.scale.set(3)
  world.position.set(71, -35)
  manager.updateObjectPosition(1, frameDistance, 0, true, 1)
  check(sprite.texture === before, 'Camera zoom and pan must not contribute distance')
  manager.updateObjectPosition(1, 100000, 100000, true, 1, 0)
  check(sprite.texture === before, 'Explicit zero distance must suppress teleport travel')
  manager.updateObjectPosition(1, 100000 + frameDistance, 100000, true, 1)
  check(sprite.texture === textures[1]![2], 'Walking after teleport must resume from the new position')
  manager.updateObjectPosition(1, 100000 + frameDistance, 100000, false, 1)
  const idle = sprite.texture
  await new Promise((resolve) => setTimeout(resolve, 150))
  manager.update()
  check(sprite.texture === idle, 'ObjectManager stop must hold idle')
  manager.despawnObject(1)
  world.position.set(0, 0)
  checks.push('PASS real ObjectManager distance / camera independence / teleport / stop / cleanup')

  const originalNow = Date.now
  try {
    let clientNow = originalNow()
    Date.now = () => clientNow
    const renderTime = timeSync.estimateServerNowMs(clientNow) - timeSync.getInterpolationDelayMs()
    moveController.initEntity(900, 0, 0)
    moveController.onObjectMove(900, renderTime - 100, 1, false, 0, 0, 32, 0, true, 1, 0)
    moveController.onObjectMove(900, renderTime + 100, 2, false, 6.4, 0, 32, 0, true, 1, 0)
    const interpolated = moveController.update().get(900)!
    check(interpolated.distanceMoved > 0 && interpolated.distanceMoved < 3.2,
      'Distance must measure the displayed smoothed position, not requested server speed')
    check(Math.abs(interpolated.distanceMoved - Math.hypot(interpolated.x, interpolated.y)) < 1e-9,
      'Reported distance must equal actual visual displacement')
    moveController.onObjectMove(900, renderTime, 3, true, 10000, 0, 0, 0, false, 1, 0)
    check(moveController.update().get(900)!.distanceMoved === 0, 'Explicit network teleport must not count as walking')
    clientNow += 1000
    moveController.onObjectMove(900, renderTime + 1000, 4, false, 50000, 0, 0, 0, true, 1, 0)
    const snapped = moveController.update().get(900)!
    check(snapped.x === 50000 && snapped.distanceMoved === 0, 'Large correction snap must not count as walking')
  } finally {
    Date.now = originalNow
    moveController.removeEntity(900)
  }
  checks.push('PASS real MoveController interpolation / explicit teleport / error-correction snap')

  await verifyMovementStopping(manager)
  checks.push('PASS server stop → decelerating walk → exact arrival / 8 directions / 30–144 FPS / restart / teleport')

  // Display production ObjectViews, with a shared parent zoom like the game camera.
  world.scale.set(2)
  const previewViews: ObjectView[] = []
  const previewGrounds: Graphics[] = []
  const previewOrigins: Array<{ x: number; y: number }> = []
  const serverPositions: Array<{ x: number; y: number }> = []
  const barrel = await ResourceLoader.loadTexture('obj/barrel/barrel.png')
  barrel.source.scaleMode = 'nearest'
  for (let direction = 0; direction < 8; direction++) {
    const column = direction % 4
    const row = Math.floor(direction / 4)
    const footX = column * 120 + 45
    const footY = row * 125 + 105
    const ground = new Graphics()
    world.addChild(ground)
    previewGrounds.push(ground)
    previewOrigins.push({ x: footX, y: footY })
    const reference = new Sprite(barrel)
    reference.position.set(footX + 30, footY - barrel.height)
    world.addChild(reference)
    manager.spawnObject({ ...options, position: coordScreen2Game(footX, footY), entityId: 100 + direction })
    const view = manager.getObject(100 + direction)!
    const position = view.getPosition()
    serverPositions.push({ ...position })
    moveController.initEntity(view.entityId, position.x, position.y, direction * Math.PI / 4 - Math.PI / 2)
    // Each card follows its character like a camera. World positions still travel.
    view.setScreenPositionOverride(footX, footY)
    view.onMoved(direction)
    previewViews.push(view)
    const label = new Text({ text: MOVEMENT_DIRECTIONS[direction], style: { fontFamily: 'monospace', fontSize: 10, fill: '#e3d7b9' } })
    label.position.set(footX - 6, row * 125 + 8)
    world.addChild(label)
  }
  const toggle = document.querySelector<HTMLButtonElement>('#toggle')!
  const zoom = document.querySelector<HTMLButtonElement>('#zoom')!
  const speed = document.querySelector<HTMLInputElement>('#speed')!
  const speedLabel = document.querySelector<HTMLOutputElement>('#speed-value')!
  const visualSpeedLabel = document.querySelector<HTMLOutputElement>('#visual-speed')!
  toggle.disabled = zoom.disabled = false
  let walking = true
  let moveSeq = 0
  let lastPacketMs = Date.now()
  const sendMovementSamples = () => {
    const velocity = walking ? Number(speed.value) : 0
    const serverTime = timeSync.estimateServerNowMs()
    moveSeq++
    previewViews.forEach((view, direction) => {
      const position = serverPositions[direction]!
      const heading = direction * Math.PI / 4 - Math.PI / 2
      moveController.onObjectMove(view.entityId, serverTime, moveSeq, false,
        position.x, position.y, Math.cos(heading) * velocity, Math.sin(heading) * velocity,
        velocity > 0, 1, heading)
    })
    lastPacketMs = Date.now()
  }
  toggle.onclick = () => {
    walking = !walking
    toggle.textContent = walking ? 'Остановить' : 'Идти'
    sendMovementSamples()
  }
  zoom.onclick = () => {
    const scale = world.scale.x === 2 ? 1 : 2
    world.scale.set(scale)
    zoom.textContent = `Масштаб ${scale === 2 ? 1 : 2}×`
  }
  const showSpeed = () => {
    const value = Number(speed.value)
    speedLabel.textContent = `${value.toFixed(1)} ед./с → ${(value / cycleDistance * 8).toFixed(2)} кадров/с`
  }
  speed.oninput = showSpeed
  document.querySelector<HTMLButtonElement>('#reference-speed')!.onclick = () => {
    speed.value = String(cycleDistance / .96)
    showSpeed()
  }
  showSpeed()
  sendMovementSamples()
  let previousVisualUpdateMs = Date.now()
  app.ticker.add((ticker) => {
    serverPositions.forEach((position, direction) => {
      const velocity = walking ? Number(speed.value) : 0
      const angle = direction * Math.PI / 4 - Math.PI / 2
      position.x += Math.cos(angle) * velocity * ticker.deltaMS / 1000
      position.y += Math.sin(angle) * velocity * ticker.deltaMS / 1000
    })
    const now = Date.now()
    if (walking && now - lastPacketMs >= 100) sendMovementSamples()
    const renderPositions = moveController.update()
    const elapsedMs = now - previousVisualUpdateMs
    previousVisualUpdateMs = now
    const firstPosition = renderPositions.get(100)!
    const actualSpeed = elapsedMs > 0 ? firstPosition.distanceMoved * 1000 / elapsedMs : 0
    visualSpeedLabel.textContent = `${actualSpeed.toFixed(2)} ед./с → ${(actualSpeed / cycleDistance * 8).toFixed(2)} кадров/с · ${firstPosition.isMoving ? 'ходьба' : 'стойка'}`
    previewViews.forEach((view, direction) => {
      const position = renderPositions.get(view.entityId)!
      manager.updateObjectPosition(view.entityId, position.x, position.y,
        position.isMoving, position.direction, position.distanceMoved)
      const current = view.getPosition()
      const origin = previewOrigins[direction]!
      const ground = previewGrounds[direction]!
      ground.clear().ellipse(origin.x, origin.y, 25, 10).fill({ color: 0x21271b, alpha: .45 })
      for (let column = -2; column <= 2; column++) {
        for (let row = -2; row <= 2; row++) {
          const point = coordGame2Screen(
            (column - current.x / getCoordPerTile() % 1) * getCoordPerTile(),
            (row - current.y / getCoordPerTile() % 1) * getCoordPerTile())
          if (Math.abs(point.x) < 40 && Math.abs(point.y) < 13) {
            ground.rect(Math.round(origin.x + point.x), Math.round(origin.y + point.y), 2, 1).fill(0x829168)
          }
        }
      }
    })
    manager.update()
  })
  app.start()
  result.textContent = checks.join('\n')
  result.dataset.status = 'passed'
}

main().catch((error: unknown) => {
  result.textContent = [...checks, `FAIL ${error instanceof Error ? error.stack : String(error)}`].join('\n')
  result.dataset.status = 'failed'
  console.error(error)
})
