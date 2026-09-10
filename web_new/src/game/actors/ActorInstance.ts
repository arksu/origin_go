import { AnimationMixer, Bone, Group, Skeleton, SkinnedMesh, type AnimationAction, type Object3D, type ShaderMaterial } from 'three'
import { clone } from 'three/addons/utils/SkeletonUtils.js'
import type { GLTF } from 'three/addons/loaders/GLTFLoader.js'
import type { ActorAssetCache } from './ActorAssetCache'
import { createActorMaterial } from './ActorMaterial'
import { DualQuaternionSkin } from './DualQuaternionSkin'
import { ACTOR_RENDER, COMMONER_MODEL, DEFAULT_EQUIPMENT, EQUIPMENT, type EquipmentId } from './config'

interface EquipmentInstance { root: Group; release: () => void; meshes: SkinnedMesh[] }
const ANGLES = [135, 90, 45, 0, 315, 270, 225, 180]

export class ActorInstance {
  readonly root = new Group()
  readonly ready: Promise<void>
  revision = 0
  direction = 3
  distanceTiles = 0
  walking = false
  carrying = false
  hovered = false
  error: Error | null = null
  private model: Object3D | null = null
  private loaded = false
  private mixer: AnimationMixer | null = null
  private readonly actions = new Map<string, AnimationAction>()
  private readonly bones = new Map<string, Bone>()
  private readonly skins = new Map<Skeleton, DualQuaternionSkin>()
  private readonly materials = new Set<ShaderMaterial>()
  private readonly equipment = new Map<string, EquipmentInstance>()
  private equipmentRevision = 0
  private releaseModel: (() => void) | null = null
  private lastPose = ''
  private destroyed = false
  private lowDetail = false

  constructor(private readonly cache: ActorAssetCache) {
    this.ready = this.load().catch((error: unknown) => {
      this.error = error instanceof Error ? error : new Error(String(error))
      throw this.error
    })
  }

  private prepareMesh(mesh: SkinnedMesh): void {
    let skin = this.skins.get(mesh.skeleton)
    if (!skin) {
      skin = new DualQuaternionSkin(mesh.skeleton)
      this.skins.set(mesh.skeleton, skin)
    }
    const materials = (Array.isArray(mesh.material) ? mesh.material : [mesh.material]).map((source) => {
      const material = createActorMaterial(source, skin!)
      this.materials.add(material)
      return material
    })
    mesh.material = materials.length === 1 ? materials[0]! : materials
    // The world culler owns visibility; the bind-pose sphere clips raised arms.
    mesh.frustumCulled = false
  }

  private async load(): Promise<void> {
    const lease = await this.cache.acquire(COMMONER_MODEL)
    if (this.destroyed) { lease.release(); return }
    this.releaseModel = lease.release
    this.model = clone(lease.asset.scene)
    this.model.traverse((object) => {
      if (object.userData.lod === 1) object.visible = false
      if (object instanceof Bone) this.bones.set(object.name, object)
      if (object instanceof SkinnedMesh) this.prepareMesh(object)
    })
    if (this.bones.size === 0) throw new Error('Character has no skeleton')
    this.root.add(this.model)
    this.mixer = new AnimationMixer(this.model)
    for (const name of ['idle', 'walk', 'carry_idle', 'carry_walk']) {
      const clip = lease.asset.animations.find((candidate) => candidate.name === name)
      if (!clip) throw new Error(`Character animation missing: ${name}`)
      this.actions.set(name, this.mixer.clipAction(clip))
    }
    await this.setEquipment(DEFAULT_EQUIPMENT)
    this.loaded = true
    this.updatePose()
  }

  async setEquipment(ids: readonly EquipmentId[]): Promise<void> {
    if (new Set(ids).size !== ids.length || ids.some((id) => !Object.hasOwn(EQUIPMENT, id))) throw new Error('Invalid character equipment')
    const slots = ids.map((id) => EQUIPMENT[id].slot)
    if (new Set(slots).size !== slots.length) throw new Error('Two equipment pieces occupy the same slot')
    const revision = ++this.equipmentRevision
    const leases = await Promise.allSettled(ids.map((id) => this.cache.acquire(EQUIPMENT[id].url)))
    const acquired = leases.filter((lease) => lease.status === 'fulfilled').map((lease) => lease.value)
    const failure = leases.find((lease) => lease.status === 'rejected')
    if (this.destroyed || revision !== this.equipmentRevision || failure) {
      acquired.forEach((lease) => lease.release())
      if (failure?.status === 'rejected') throw failure.reason
      return
    }
    const replacements = new Map<string, EquipmentInstance>()
    try {
      for (let index = 0; index < ids.length; index++) {
        const id = ids[index]!
        const lease = acquired[index]!
        replacements.set(id, this.attachEquipment(lease.asset, lease.release))
      }
    } catch (error) {
      replacements.forEach((piece) => this.disposeEquipment(piece))
      acquired.forEach((lease) => lease.release())
      throw error
    }
    this.equipment.forEach((piece) => this.disposeEquipment(piece))
    this.equipment.clear()
    replacements.forEach((piece, id) => {
      this.equipment.set(id, piece)
      this.model!.add(piece.root)
    })
    this.lastPose = ''
    this.updatePose()
  }

  private attachEquipment(asset: GLTF, release: () => void): EquipmentInstance {
    const source = clone(asset.scene)
    source.updateMatrixWorld(true)
    const root = new Group()
    const meshes: SkinnedMesh[] = []
    source.traverse((object) => { if (object instanceof SkinnedMesh) meshes.push(object) })
    for (const mesh of meshes) {
      const bones = mesh.skeleton.bones.map((bone) => {
        const shared = this.bones.get(bone.name)
        if (!shared) throw new Error(`Equipment bone ${bone.name} is not in the character skeleton`)
        return shared
      })
      root.attach(mesh)
      mesh.bind(new Skeleton(bones, mesh.skeleton.boneInverses.map((matrix) => matrix.clone())), mesh.bindMatrix)
      this.prepareMesh(mesh)
    }
    return { root, release, meshes }
  }

  private disposeEquipment(piece: EquipmentInstance): void {
    piece.root.removeFromParent()
    for (const mesh of piece.meshes) {
      this.skins.get(mesh.skeleton)?.destroy()
      this.skins.delete(mesh.skeleton)
      mesh.skeleton.dispose()
      const materials = Array.isArray(mesh.material) ? mesh.material : [mesh.material]
      for (const material of materials) { material.dispose(); this.materials.delete(material as ShaderMaterial) }
    }
    piece.release()
  }

  updatePose(): boolean {
    if (!this.mixer || this.destroyed) return false
    const phase = this.walking ? Math.floor((this.distanceTiles / ACTOR_RENDER.cycleDistanceTiles % 1) * ACTOR_RENDER.walkSamples + 1e-7) / ACTOR_RENDER.walkSamples : 0
    const name = `${this.carrying ? 'carry_' : ''}${this.walking ? 'walk' : 'idle'}`
    const key = `${name}/${phase}/${this.direction}/${this.hovered}`
    if (key === this.lastPose) return false
    this.lastPose = key
    this.root.rotation.y = (ANGLES[this.direction] ?? 0) * Math.PI / 180
    const active = this.actions.get(name)!
    for (const action of this.actions.values()) action.stop()
    active.play()
    active.paused = true
    active.time = phase * active.getClip().duration
    this.mixer.update(0)
    for (const piece of this.equipment.values()) for (const mesh of piece.meshes) {
      if (!mesh.morphTargetInfluences || !mesh.morphTargetDictionary) continue
      mesh.morphTargetInfluences.fill(0)
      const index = mesh.morphTargetDictionary[`walk_cloth_${Math.round(phase * 8) % 8}`]
      if (this.walking && index !== undefined) mesh.morphTargetInfluences[index] = 1
    }
    this.root.updateMatrixWorld(true)
    this.skins.forEach((skin) => skin.update())
    this.revision++
    return true
  }

  get isReady(): boolean { return this.loaded && !this.error }

  setLowDetail(enabled: boolean): void {
    if (this.lowDetail === enabled || !this.model) return
    this.lowDetail = enabled
    this.model.traverse((object) => {
      if (object.userData.lod === 0) object.visible = !enabled
      if (object.userData.lod === 1) object.visible = enabled
    })
    this.revision++
  }

  destroy(): void {
    this.destroyed = true
    this.equipmentRevision++
    this.equipment.forEach((piece) => this.disposeEquipment(piece))
    this.equipment.clear()
    this.mixer?.stopAllAction()
    if (this.model) this.mixer?.uncacheRoot(this.model)
    this.skins.forEach((skin) => { skin.destroy(); skin.skeleton.dispose() })
    this.skins.clear()
    this.materials.forEach((material) => material.dispose())
    this.materials.clear()
    this.root.removeFromParent()
    this.releaseModel?.()
  }
}
