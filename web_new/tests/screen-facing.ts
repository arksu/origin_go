import { OrthographicCamera, Vector3 } from 'three'
import { actorYawForFacing, facingFromDisplacement, FacingStabilizer } from '../src/game/actors/facing'
import { ACTOR_RENDER } from '../src/game/actors/config'
import { coordScreen2Game } from '../src/game/utils/coordConvert'

function check(value: boolean, message: string) { if (!value) throw new Error(message) }
export function verifyScreenFacing(): void {
  const camera = new OrthographicCamera(-2, 2, 2, -2, .1, 20)
  camera.position.set(0, 3, Math.sqrt(27))
  camera.lookAt(0, 0, 0)
  camera.updateMatrixWorld(true)
  const origin = new Vector3().project(camera)
  for (let direction = 0; direction < 8; direction++) {
    const angle = (direction - 1) * Math.PI / 4
    const screen = { x: Math.cos(angle), y: Math.sin(angle) }
    const world = coordScreen2Game(screen.x, screen.y)
    for (let previous = 0; previous < 8; previous++) {
      check(facingFromDisplacement(world.x, world.y, previous) === direction, `Screen ray ${direction} must select its facing`)
    }
    const yaw = actorYawForFacing(direction)
    const forward = new Vector3(Math.sin(yaw), 0, Math.cos(yaw)).project(camera).sub(origin)
    const actual = Math.atan2(-forward.y, forward.x)
    check(Math.abs(Math.atan2(Math.sin(actual - angle), Math.cos(actual - angle))) < 1e-8,
      `Model forward vector must project onto screen ray ${direction}`)
    check(facingFromDisplacement(0, 0, direction) === direction, 'Rest must preserve facing')
    const boundary = angle + Math.PI / 8
    for (const offset of [-1, 1, -2, 2]) {
      const jitter = boundary + offset * Math.PI / 180
      const delta = coordScreen2Game(Math.cos(jitter), Math.sin(jitter))
      check(facingFromDisplacement(delta.x, delta.y, direction) === direction, 'Boundary noise must not toggle facing')
    }
    const beyond = boundary + ACTOR_RENDER.facingHysteresis + .001
    const delta = coordScreen2Game(Math.cos(beyond), Math.sin(beyond))
    check(facingFromDisplacement(delta.x, delta.y, direction) === (direction + 1) % 8, 'Crossing dead band must turn')
  }
  for (const fps of [30, 60, 144]) {
    const stabilizer = new FacingStabilizer()
    let direction = stabilizer.update(1, 1, 0, true)
    for (let tick = 0; tick < fps * 2; tick++) {
      const now = tick * 1000 / fps
      const candidate = Math.floor(now / 50) % 2 ? 2 : 1
      direction = stabilizer.update(direction, candidate, now)
      check(direction === 1, 'Alternating adjacent sectors must not flicker')
    }
    direction = stabilizer.update(direction, direction, 2000)
    direction = stabilizer.update(direction, 2, 2100)
    check(direction === 1, 'A transient new direction must wait')
    direction = stabilizer.update(direction, 2, 2221)
    check(direction === 2, 'A sustained new direction must eventually turn')
    direction = stabilizer.update(direction, 1, 2230)
    direction = stabilizer.update(direction, 1, 2351)
    check(direction === 2, 'Minimum hold must prevent a rapid return')
    direction = stabilizer.update(direction, 1, 2472)
    check(direction === 1, 'Stable return must be allowed after hold')
    check(stabilizer.update(direction, 5, 2473) === 5, 'A deliberate reversal must be immediate')
  }
  const shallow = coordScreen2Game(Math.cos(Math.PI / 12), Math.sin(Math.PI / 12))
  check(facingFromDisplacement(shallow.x, shallow.y, 3) === 1, 'Shallow screen movement must face E')
}
