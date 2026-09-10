import { Sprite } from 'pixi.js'
import { moveController } from '../src/game/MoveController'
import { ObjectManager } from '../src/game/ObjectManager'
import { ResourceLoader } from '../src/game/ResourceLoader'
import { getCoordPerTile, setWorldParams } from '../src/game/tiles/Tile'
import { coordGame2Screen } from '../src/game/utils/coordConvert'
import { timeSync } from '../src/network/TimeSync'

function check(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

export async function verifyMovementStopping(manager: ObjectManager): Promise<void> {
  const entityId = 901
  const sheet = ResourceLoader.getResourceDef('player')!.layers[0]!.spriteSheet!
  const textures = await ResourceLoader.loadDirectionalSpriteSheet(sheet)
  manager.spawnObject({ entityId, typeId: 1, resourcePath: 'player', position: { x: 0, y: 0 }, size: { x: 4, y: 4 } })
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
  const view = manager.getObject(entityId)!
  const sprite = view.getContainer().children.find((child) => child instanceof Sprite)
  check(sprite instanceof Sprite, 'Stop regression needs the production player sprite')
  const originalNow = Date.now
  const originalCoordPerTile = getCoordPerTile()

  try {
    let clientNow = originalNow()
    Date.now = () => clientNow
    for (const unitsPerTile of [12, 32]) {
      setWorldParams(unitsPerTile, 128)
      const cycleDistance = sheet.cycleDistanceTiles * unitsPerTile
      for (let direction = 0; direction < 8; direction++) {
        const heading = direction * Math.PI / 4 - Math.PI / 2
        const targetX = Math.cos(heading) * cycleDistance * .9
        const targetY = Math.sin(heading) * cycleDistance * .9
        const settleDurations: number[] = []
        for (const frameRate of [30, 60, 144]) {
          const startTime = clientNow + 1000
          const renderStart = timeSync.estimateServerNowMs(startTime) - timeSync.getInterpolationDelayMs()
          // A moving sample followed by a stop, with render time halfway between
          // them, reproduces the premature idle before the stop timestamp.
          clientNow = startTime - 500
          moveController.initEntity(entityId, 0, 0, heading)
          moveController.onObjectMove(entityId, renderStart - 600, 1, false, 0, 0, 0, 0, true, 1, heading)
          clientNow = startTime
          moveController.onObjectMove(entityId, renderStart - 100, 2, false, 0, 0, 0, 0, true, 1, heading)
          moveController.onObjectMove(entityId, renderStart + 100, 3, false, targetX, targetY, 0, 0, false, 1, heading)
          view.onStopped()
          let totalDistance = 0
          let previousDistance = Infinity
          let reachedTarget = false
          const visitedFrames = new Set<number>()
          for (let tick = 0; tick <= frameRate * 2; tick++) {
            clientNow = startTime + tick * 1000 / frameRate
            const position = moveController.update().get(entityId)!
            totalDistance += position.distanceMoved
            manager.updateObjectPosition(entityId, position.x, position.y,
              position.isMoving, position.direction, position.distanceMoved)
            manager.update()
            const remaining = Math.hypot(targetX - position.x, targetY - position.y)
            if (remaining === 0) {
              check(!position.isMoving, 'Exact arrival must end visual movement')
              check(sprite.texture === textures[direction]![sheet.idleFrame], 'Arrival must select the standing pose')
              const finalCorrection = coordGame2Screen(
                Math.cos(heading) * position.distanceMoved, Math.sin(heading) * position.distanceMoved)
              check(Math.hypot(finalCorrection.x, finalCorrection.y) < .25, 'Final stop correction must stay below a native pixel')
              settleDurations.push(clientNow - startTime)
              reachedTarget = true
              break
            }
            check(position.isMoving, `Server stop must keep walking during visual travel (${direction}, ${frameRate} FPS)`)
            const expectedFrame = Math.floor(totalDistance / cycleDistance * sheet.frameCount + 1e-8) % sheet.frameCount
            check(sprite.texture === textures[direction]![expectedFrame], 'Deceleration must preserve the distance-based stride phase')
            visitedFrames.add(expectedFrame)
            if (clientNow > startTime + 100 + 1000 / frameRate) {
              check(position.distanceMoved < previousDistance, 'Travel and effective animation FPS must decrease during settling')
            }
            previousDistance = position.distanceMoved
            const repeated = moveController.update().get(entityId)!
            check(repeated.x === position.x && repeated.y === position.y && repeated.distanceMoved === 0,
              'An update without elapsed time must not add smoothing travel')
            check(repeated.isMoving, 'A repeated update must not flash idle during settling')
          }
          check(reachedTarget, 'Server stop must reach its exact endpoint in finite time')
          check(visitedFrames.size >= 4, 'Settling must advance actual walk frames instead of holding one walking pose')
          clientNow += 1000
          const stationary = moveController.update().get(entityId)!
          check(!stationary.isMoving && stationary.distanceMoved === 0, 'A completed stop must remain stationary')

          // A new movement after a completed stop starts from the exact endpoint.
          const restartTime = timeSync.estimateServerNowMs(clientNow) - timeSync.getInterpolationDelayMs()
          moveController.onObjectMove(entityId, restartTime, 4, false,
            targetX * 1.5, targetY * 1.5, 0, 0, true, 1, heading)
          clientNow += 1000 / frameRate
          const restarted = moveController.update().get(entityId)!
          check(restarted.isMoving && restarted.distanceMoved > 0, 'Walking must resume after a completed stop')
          manager.updateObjectPosition(entityId, restarted.x, restarted.y,
            restarted.isMoving, restarted.direction, restarted.distanceMoved)
          check(sprite.texture !== textures[direction]![sheet.idleFrame], 'Restart must leave the standing pose')
          moveController.onObjectMove(entityId, restartTime + 10, 5, true, 10000, 10000, 0, 0, false, 1, heading)
          const teleported = moveController.update().get(entityId)!
          check(!teleported.isMoving && teleported.distanceMoved === 0, 'Teleport must cancel pending visual settling')
          manager.updateObjectPosition(entityId, teleported.x, teleported.y,
            teleported.isMoving, teleported.direction, teleported.distanceMoved)
          check(sprite.texture === textures[direction]![sheet.idleFrame], 'A stopped teleport must immediately show idle')
        }
        check(Math.max(...settleDurations) - Math.min(...settleDurations) < 70,
          'Stop duration must not depend on rendering at 30, 60 or 144 FPS')
      }
    }

    const cycleDistance = sheet.cycleDistanceTiles * getCoordPerTile()
    clientNow += 1000
    let renderTime = timeSync.estimateServerNowMs(clientNow) - timeSync.getInterpolationDelayMs()
    moveController.initEntity(entityId, 0, 0)
    moveController.onObjectMove(entityId, renderTime - 100, 1, false, 0, 0, 32, 0, true, 1, 0)
    clientNow += 500
    renderTime = timeSync.estimateServerNowMs(clientNow) - timeSync.getInterpolationDelayMs()
    moveController.onObjectMove(entityId, renderTime, 2, false, cycleDistance * 1.5, 0, 0, 0, false, 1, 0)
    const settling = moveController.update().get(entityId)!
    check(settling.isMoving && settling.x < cycleDistance * 1.5, 'Rapid restart scenario must still be settling')
    view.onStopped()
    manager.updateObjectPosition(entityId, settling.x, settling.y,
      settling.isMoving, settling.direction, settling.distanceMoved)
    clientNow += 1000 / 60
    moveController.onObjectMove(entityId, renderTime + 1000 / 60, 3, false, cycleDistance * 2.5, 0, 32, 0, true, 1, 0)
    const resumed = moveController.update().get(entityId)!
    manager.updateObjectPosition(entityId, resumed.x, resumed.y,
      resumed.isMoving, resumed.direction, resumed.distanceMoved)
    const resumedFrame = Math.floor((settling.distanceMoved + resumed.distanceMoved) / cycleDistance * sheet.frameCount)
    check(resumed.isMoving && sprite.texture === textures[2]![resumedFrame],
      'Restart during deceleration must keep the accumulated stride without flashing idle')
    manager.setKnockedOutPose(entityId, true)
    clientNow += 1000 / 60
    const afterKO = moveController.update().get(entityId)!
    manager.updateObjectPosition(entityId, afterKO.x, afterKO.y,
      afterKO.isMoving, afterKO.direction, afterKO.distanceMoved)
    check(sprite.texture === textures[2]![sheet.idleFrame], 'KO must suppress walking even while the controller still settles')
    manager.setKnockedOutPose(entityId, false)

    // Even a very short buffered segment must not settle at an intermediate
    // target merely because the remaining interpolation error is subpixel.
    clientNow += 1000
    renderTime = timeSync.estimateServerNowMs(clientNow) - timeSync.getInterpolationDelayMs()
    moveController.initEntity(entityId, 0, 0)
    moveController.onObjectMove(entityId, renderTime - 100, 1, false, 0, 0, 0, 0, true, 1, 0)
    const tinyTarget = getCoordPerTile() / 1024
    moveController.onObjectMove(entityId, renderTime + 100, 2, false, tinyTarget, 0, 0, 0, false, 1, 0)
    const beforeStopTime = moveController.update().get(entityId)!
    check(beforeStopTime.isMoving && beforeStopTime.x < tinyTarget / 2,
      'A future stop sample must not trigger early subpixel completion')
    clientNow += 100
    const tinyStop = moveController.update().get(entityId)!
    check(!tinyStop.isMoving && tinyStop.x === tinyTarget, 'A tiny movement must finish at the authoritative endpoint')
    clientNow += 1
    moveController.onObjectMove(entityId, renderTime + 101, 3, false, 50000, 0, 0, 0, false, 1, 0)
    const correctedStop = moveController.update().get(entityId)!
    check(!correctedStop.isMoving && correctedStop.x === 50000 && correctedStop.distanceMoved === 0,
      'A large stopped correction must snap without starting a settling walk')
  } finally {
    Date.now = originalNow
    setWorldParams(originalCoordPerTile, 128)
    moveController.removeEntity(entityId)
    manager.despawnObject(entityId)
  }
}
