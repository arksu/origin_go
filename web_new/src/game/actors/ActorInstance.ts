import { AnimationMixer, Bone, Group, Mesh, Skeleton, SkinnedMesh, type AnimationAction, type Object3D, type ShaderMaterial } from 'three'
import { clone } from 'three/addons/utils/SkeletonUtils.js'
import type { GLTF } from 'three/addons/loaders/GLTFLoader.js'
import type { ActorAssetCache } from './ActorAssetCache'
import { createActorMaterial } from './ActorMaterial'
import { DualQuaternionSkin } from './DualQuaternionSkin'
import { ACTOR_RENDER, COMMONER_MODEL } from './config'
import { DEFAULT_EQUIPMENT, EQUIPMENT, armForSlot, validateEquipment, type ArmSide, type EquipmentDefinition, type EquipmentBinding } from './equipment'
import { ActorSockets } from './ActorSockets'
import { ActorArmLayers } from './ActorArmLayers'
import { actorYawForFacing } from './facing'
import type { EquippedVisual, EquipmentSlot } from '../../types/characterVisual'

interface EquipmentInstance {
  root: Group
  parent: Object3D
  release: () => void
  meshes: Mesh[]
}

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
  private sockets: ActorSockets | null = null
  private armLayers: ActorArmLayers | null = null
  private readonly actions = new Map<string, AnimationAction>()
  private readonly bones = new Map<string, Bone>()
  private readonly skins = new Map<Skeleton, DualQuaternionSkin>()
  private readonly materials = new Set<ShaderMaterial>()
  private readonly equipment = new Map<EquipmentSlot, EquipmentInstance>()
  private armProfiles: Partial<Record<ArmSide, EquipmentBinding>> = {}
  private equipmentRevision = 0
  private requestedEquipment: readonly EquippedVisual[] = []
  private releaseModel: (() => void) | null = null
  private lastPose = ''
  private destroyed = false
  private lowDetail = false

  constructor(private readonly cache: Pick<ActorAssetCache, 'acquire'>, private readonly catalog: Readonly<Record<string, EquipmentDefinition>> = EQUIPMENT) {
    this.ready = this.load().catch((error: unknown) => {
      this.error = error instanceof Error ? error : new Error(String(error))
      throw this.error
    })
  }

  private prepareMesh(mesh: Mesh): void {
    let skin: DualQuaternionSkin | undefined
    if (mesh instanceof SkinnedMesh) {
      skin = this.skins.get(mesh.skeleton)
      if (!skin) {
        skin = new DualQuaternionSkin(mesh.skeleton)
        this.skins.set(mesh.skeleton, skin)
      }
    }
    const materials = (Array.isArray(mesh.material) ? mesh.material : [mesh.material]).map((source) => {
      const material = createActorMaterial(source, skin, mesh.userData.skinning === 'linear')
      this.materials.add(material)
      return material
    })
    mesh.material = materials.length === 1 ? materials[0]! : materials
    // The world culler owns visibility; the bind-pose sphere clips raised arms.
    mesh.frustumCulled = false
    if (mesh.userData.lod === 0) mesh.visible = !this.lowDetail
    if (mesh.userData.lod === 1) mesh.visible = this.lowDetail
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
    this.sockets = new ActorSockets(this.model)
    this.armLayers = new ActorArmLayers(this.model, lease.asset.animations)
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

  async setEquipment(items: readonly EquippedVisual[]): Promise<void> {
    validateEquipment(items, this.catalog)
    if (this.destroyed) return
    if (!this.model || !this.armLayers) throw new Error('Character is not loaded')
    const desired = items.map((item) => ({ ...item }))
    const poses: Record<ArmSide, string | null> = { left: null, right: null }
    const profiles: Partial<Record<ArmSide, EquipmentBinding>> = {}
    const renderable = desired.flatMap((item) => {
      const definition = Object.hasOwn(this.catalog, item.visualKey) ? this.catalog[item.visualKey] : undefined
      if (!definition || definition.kind === 'deferred') return []
      const side = armForSlot(item.slot)
      if (definition.kind === 'rigid' && side) {
        const binding = definition.bindings[item.slot]!
        poses[side] = binding.pose ?? null
        profiles[side] = binding
        this.armLayers!.validate(side, binding.walkPose ?? null)
      }
      return [{ item, definition }]
    })
    for (const side of ['left', 'right'] as const) this.armLayers.validate(side, poses[side])
    const revision = ++this.equipmentRevision
    const leases = await Promise.allSettled(renderable.map(({ definition }) => this.cache.acquire(definition.url)))
    const acquired = leases.flatMap((lease) => lease.status === 'fulfilled' ? [lease.value] : [])
    const failure = leases.find((lease) => lease.status === 'rejected')
    if (this.destroyed || revision !== this.equipmentRevision || failure) {
      acquired.forEach((lease) => lease.release())
      // Superseded failures do not belong to the current equipment request.
      if (!this.destroyed && revision === this.equipmentRevision && failure?.status === 'rejected') throw failure.reason
      return
    }
    const replacements = new Map<EquipmentSlot, EquipmentInstance>()
    try {
      for (let index = 0; index < renderable.length; index++) {
        const { item, definition } = renderable[index]!
        const lease = acquired[index]!
        replacements.set(item.slot, this.attachEquipment(lease.asset, lease.release, definition, item.slot))
      }
    } catch (error) {
      replacements.forEach((piece) => this.disposeEquipment(piece))
      acquired.forEach((lease) => lease.release())
      throw error
    }
    this.equipment.forEach((piece) => this.disposeEquipment(piece))
    this.equipment.clear()
    replacements.forEach((piece, slot) => {
      this.equipment.set(slot, piece)
      piece.parent.add(piece.root)
    })
    this.requestedEquipment = desired
    this.armProfiles = profiles
    for (const side of ['left', 'right'] as const) this.armLayers.setPose(side, poses[side])
    this.lastPose = ''
    this.updatePose()
  }

  private attachEquipment(asset: GLTF, release: () => void, definition: Exclude<EquipmentDefinition, { kind: 'deferred' }>, slot: EquipmentSlot): EquipmentInstance {
    const source = clone(asset.scene)
    source.updateMatrixWorld(true)
    const piece: EquipmentInstance = { root: new Group(), parent: this.model!, release, meshes: [] }
    const detachedSkeletons = new Set<Skeleton>()
    try {
      source.traverse((object) => { if (object instanceof Mesh) piece.meshes.push(object) })
      if (!piece.meshes.length) throw new Error('Equipment asset has no meshes')
      if (definition.kind === 'rigid') {
        if (piece.meshes.some((mesh) => mesh instanceof SkinnedMesh)) throw new Error('Rigid equipment contains a skinned mesh')
        const binding = definition.bindings[slot]!
        piece.parent = this.sockets!.get(binding.socket)
        this.applyAttachmentTransform(piece.root, binding)
        piece.root.add(source)
      } else {
        for (const mesh of piece.meshes) {
          if (!(mesh instanceof SkinnedMesh)) throw new Error('Skinned equipment contains a rigid mesh')
          const bones = mesh.skeleton.bones.map((bone) => {
            const shared = this.bones.get(bone.name)
            if (!shared) throw new Error(`Equipment bone ${bone.name} is not in the character skeleton`)
            return shared
          })
          detachedSkeletons.add(mesh.skeleton)
          piece.root.attach(mesh)
          mesh.bind(new Skeleton(bones, mesh.skeleton.boneInverses.map((matrix) => matrix.clone())), mesh.bindMatrix)
        }
      }
      piece.meshes.forEach((mesh) => this.prepareMesh(mesh))
      return piece
    } catch (error) {
      this.disposeEquipment(piece)
      throw error
    } finally {
      detachedSkeletons.forEach((skeleton) => skeleton.dispose())
    }
  }

  private applyAttachmentTransform(root: Group, binding: EquipmentBinding): void {
    const transform = binding.transform
    const position = transform?.position ?? [0, 0, 0]
    const rotation = transform?.rotation ?? [0, 0, 0]
    const quaternion = transform?.quaternion
    const scale = transform?.scale ?? 1
    if (![...position, ...rotation, ...(quaternion ?? []), scale].every(Number.isFinite) || scale <= 0 ||
        (quaternion && (transform?.rotation || Math.abs(Math.hypot(...quaternion) - 1) > .001))) throw new Error('Invalid equipment attachment transform')
    root.position.set(...position)
    if (quaternion) root.quaternion.set(...quaternion)
    else root.rotation.set(...rotation)
    root.scale.setScalar(scale)
  }

  private disposeEquipment(piece: EquipmentInstance): void {
    piece.root.removeFromParent()
    for (const mesh of piece.meshes) {
      if (mesh instanceof SkinnedMesh) {
        this.skins.get(mesh.skeleton)?.destroy()
        this.skins.delete(mesh.skeleton)
        mesh.skeleton.dispose()
      }
      for (const material of Array.isArray(mesh.material) ? mesh.material : [mesh.material]) {
        if (this.materials.delete(material as ShaderMaterial)) material.dispose()
      }
    }
    piece.release()
  }

  setArmPose(side: ArmSide, clip: string | null, sampleTime = 0): void {
    if (!this.armLayers) throw new Error('Character is not loaded')
    if (this.armLayers.setPose(side, clip, sampleTime)) this.lastPose = ''
    delete this.armProfiles[side]
  }

  updatePose(now = performance.now()): boolean {
    if (!this.mixer || this.destroyed) return false
    const phase = this.walking ? Math.floor((this.distanceTiles / ACTOR_RENDER.cycleDistanceTiles % 1) * ACTOR_RENDER.walkSamples + 1e-7) / ACTOR_RENDER.walkSamples : 0
    const name = `${this.carrying ? 'carry_' : ''}${this.walking ? 'walk' : 'idle'}`
    const key = `${name}/${phase}/${this.direction}/${this.hovered}`
    if (key === this.lastPose && (this.carrying || !this.armLayers?.transitioning)) return false
    this.lastPose = key
    this.root.rotation.y = actorYawForFacing(this.direction)
    if (!this.carrying && this.armLayers) {
      for (const side of ['left', 'right'] as const) {
        const profile = this.armProfiles[side]
        if (!profile) continue
        const clip = (this.walking ? profile.walkPose ?? profile.pose : profile.pose) ?? null
        // Capture the last displayed arm before locomotion overwrites it.
        if (this.armLayers.getPose(side) !== clip) this.armLayers.setPose(side, clip, 0, now)
        this.armLayers.samplePhase(side, this.walking && profile.walkPose ? phase : 0)
      }
    }
    const active = this.actions.get(name)!
    for (const action of this.actions.values()) action.stop()
    active.play()
    active.paused = true
    active.time = phase * active.getClip().duration
    this.mixer.update(0)
    if (!this.carrying) this.armLayers?.apply(now)
    for (const [slot, piece] of this.equipment) {
      piece.root.visible = !(this.carrying && armForSlot(slot))
      for (const mesh of piece.meshes) {
        if (!mesh.morphTargetInfluences || !mesh.morphTargetDictionary) continue
        mesh.morphTargetInfluences.fill(0)
        const index = mesh.morphTargetDictionary[`walk_cloth_${Math.round(phase * 8) % 8}`]
        if (this.walking && index !== undefined) mesh.morphTargetInfluences[index] = 1
      }
    }
    this.root.updateMatrixWorld(true)
    this.skins.forEach((skin) => skin.update())
    this.revision++
    return true
  }

  get isReady(): boolean { return this.loaded && !this.error }
  get equippedVisuals(): readonly EquippedVisual[] { return this.requestedEquipment }

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
    this.armLayers?.destroy()
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
