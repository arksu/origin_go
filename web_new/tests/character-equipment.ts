import { BoxGeometry, Mesh, MeshStandardMaterial } from 'three'
import type { ActorInstance } from '../src/game/actors/ActorInstance'
import { ActorSockets, findRigBone } from '../src/game/actors/ActorSockets'
import { createActorMaterial } from '../src/game/actors/ActorMaterial'

// Exercise the actual exported rig and shared Three/Pixi WebGL path using an
// in-memory box, not a production weapon or a newly authored animation asset.
export function verifyCharacterEquipment(actor: ActorInstance, hash: () => number): void {
  const check = (value: boolean, message: string) => { if (!value) throw new Error(message) }
  actor.walking = false
  actor.direction = 3
  actor.updatePose()
  const bare = hash()
  const source = new MeshStandardMaterial({ color: 0xd8d1b3 })
  const geometry = new BoxGeometry(.18, .4, .18)
  const material = createActorMaterial(source)
  const fixture = new Mesh(geometry, material)
  fixture.position.y = .15
  new ActorSockets(actor.root).get('grip_r').add(fixture)
  actor.revision++
  try {
    check(hash() !== bare, 'Rigid socket mesh must be visible through the pixel actor shader')
    const left = findRigBone(actor.root, 'forearm.l')
    const right = findRigBone(actor.root, 'forearm.r')
    const baselineLeft = left.quaternion.clone()
    const baselineRight = right.quaternion.clone()
    actor.setArmPose('right', 'carry_idle')
    actor.updatePose(performance.now() + 200)
    check(left.quaternion.angleTo(baselineLeft) < 1e-6, 'Right-arm mask must not modify the actual left arm')
    check(right.quaternion.angleTo(baselineRight) > .01, 'Actual GLTF arm tracks must resolve through the mask')
    const held = right.quaternion.clone()
    for (let step = 0; step < 8; step++) {
      actor.walking = true
      actor.distanceTiles = step / 8 * actor.cycleDistanceTiles
      actor.updatePose(performance.now() + 300)
      check(right.quaternion.angleTo(held) < 1e-6, 'Actual held arm must survive every gait sample')
      hash() // Also asserts no GL errors in the shared renderer.
    }
  } finally {
    fixture.removeFromParent()
    geometry.dispose()
    material.dispose()
    source.dispose()
    actor.setArmPose('right', null)
    actor.walking = false
    actor.distanceTiles = 0
    actor.updatePose(performance.now() + 300)
    actor.updatePose(performance.now() + 800)
    actor.revision++
  }
  check(hash() === bare, 'Removing a rigid mesh and arm mask must restore the original actor image')
}

export async function verifyStoneAxe(actor: ActorInstance, hash: () => number): Promise<void> {
  const check = (value: boolean, message: string) => { if (!value) throw new Error(message) }
  actor.walking = false
  actor.direction = 3
  actor.updatePose()
  const bare = hash()
  const freeArm = findRigBone(actor.root, 'forearm.l')
  const freeHandPoses = []
  actor.walking = true
  actor.updatePose()
  actor.updatePose(performance.now() + 500)
  for (let phase = 0; phase < 8; phase++) {
    actor.walking = true
    actor.distanceTiles = phase / 8 * actor.cycleDistanceTiles
    actor.updatePose(performance.now() + 500)
    freeHandPoses.push(freeArm.quaternion.clone())
  }
  actor.walking = false
  actor.updatePose()
  actor.updatePose(performance.now() + 500)
  await actor.setEquipment([{ slot: 'right_hand', visualKey: 'stone_axe' }])
  const now = performance.now() + 200
  actor.updatePose(now)
  check(hash() !== bare, 'Real stone axe must be visible through the production shader')
  const socket = actor.root.getObjectByName('grip_r')!
  check(socket.children.length === 1, 'Real axe must attach to the right grip')
  const heldArm = findRigBone(actor.root, 'upper_arm.r')
  const rotations: string[] = []
  actor.walking = true
  actor.updatePose(now - 100)
  actor.updatePose(now + 400)
  for (let phase = 0; phase < 8; phase++) {
    actor.walking = true
    actor.distanceTiles = phase / 8 * actor.cycleDistanceTiles
    actor.updatePose(now + 400)
    rotations.push(heldArm.quaternion.toArray().map(value => value.toFixed(5)).join(','))
    check(freeArm.quaternion.angleTo(freeHandPoses[phase]!) < 1e-6, `Equipped right hand changed the free arm gait at phase ${phase}: ${freeArm.quaternion.angleTo(freeHandPoses[phase]!)}`)
    for (let direction = 0; direction < 8; direction++) {
      actor.direction = direction
      actor.walking = true
      actor.updatePose(now + 400)
      hash()
    }
  }
  check(new Set(rotations).size >= 4, 'Travel arm must swing over the gait, not freeze')
  actor.carrying = true
  actor.updatePose(now + 600)
  check(socket.children[0]!.visible === false, 'Carry must temporarily hide the axe')
  actor.carrying = false
  await actor.setEquipment([{ slot: 'left_hand', visualKey: 'stone_axe' }])
  actor.updatePose(performance.now() + 300)
  check(socket.children.length === 0 && actor.root.getObjectByName('grip_l')!.children.length === 1, 'Axe must support the left hand independently')
  hash()
  await actor.setEquipment([])
  actor.direction = 3
  actor.walking = false
  actor.distanceTiles = 0
  actor.updatePose(performance.now() + 300)
  actor.updatePose(performance.now() + 800)
  check(hash() === bare, 'Removing the real axe must restore the exact bare character')
}
