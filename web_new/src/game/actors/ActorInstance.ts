import { AnimationMixer, Bone, Group, Mesh, Skeleton, SkinnedMesh, type AnimationAction, type Object3D, type ShaderMaterial } from 'three'
import { clone } from 'three/addons/utils/SkeletonUtils.js'
import type { ActorAssetCache, ActorBundle } from './ActorAssetCache'
import { bindClips } from './ActorClipBinding'
import { createActorMaterial } from './ActorMaterial'
import { DualQuaternionSkin } from './DualQuaternionSkin'
import { ACTOR_RENDER, COMMONER_ASSET_ID, DEFAULT_ACTOR_RENDER_SETTINGS, type ActorRenderSettings } from './config'
import { DEFAULT_EQUIPMENT, armForSlot, validateEquipment, type ArmMotion, type ArmSide, type EquipmentDefinition, type EquipmentBinding } from './equipment'
import { ActorSockets } from './ActorSockets'
import { ActorArmLayers } from './ActorArmLayers'
import { actorYawForScreenAngle, screenFacingAngle } from './facing'
import type { EquippedVisual, EquipmentSlot } from '../../types/characterVisual'
import type { ActionAnimationFrame } from '../../types/actionAnimationDefs'
import { ActionAnimationPlayer, type ActionAnimationInput } from './ActionAnimationPlayer'
import { ActorActionLayers } from './ActorActionLayers'
import { GAIT_CLIPS, locomotionClip, type GaitClip, type LocomotionClip } from './locomotion'

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
  private facingDirection = 3
  distanceTiles = 0
  walking = false
  movementMode = 1
  stopProgress: number | undefined
  carrying = false
  knockedOut = false
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
  private armProfiles: Partial<Record<ArmSide, Extract<ArmMotion, { kind: 'layered' }>>> = {}
  private equipmentRevision = 0
  private requestedEquipment: readonly EquippedVisual[] = []
  private releaseModel: (() => void) | null = null
  private lastPose = ''
  private lastPoseState = ''
  private immediateRender = true
  private facingAngle = screenFacingAngle(3)
  private targetFacingAngle = screenFacingAngle(3)
  private lastFacingUpdateMs: number | null = null
  private destroyed = false
  private lowDetail = false
  private walkWeight = 0
  private walkTarget = 0
  private walkBlendFrom = 0
  private walkBlendStarted = 0
  private walkPhase = 0
  private walkPhaseOffset = 0
  private stopStartWeight: number | undefined
  private readonly cycleDistances = new Map<LocomotionClip, number>()
  private gait: GaitClip = 'walk'
  private readonly gaitWeights: Record<GaitClip, number> = { crawl: 0, walk: 1, run: 0, fast_run: 0 }
  private readonly gaitBlendFrom: Record<GaitClip, number> = { ...this.gaitWeights }
  private gaitBlendStarted = 0
  private catalog: Readonly<Record<string, EquipmentDefinition>> = {}
  private equipmentReady = false
  private actionLayers: ActorActionLayers | null = null
  private readonly actionPlayer = new ActionAnimationPlayer({ width: ACTOR_RENDER.cellSize, height: ACTOR_RENDER.cellSize, origin_x: ACTOR_RENDER.anchorX, origin_y: ACTOR_RENDER.anchorY })

  constructor(private readonly cache: Pick<ActorAssetCache, 'acquire' | 'catalog'>) {
    this.ready = this.load().catch((error: unknown) => {
      this.error = error instanceof Error ? error : new Error(String(error))
      this.destroy()
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
    this.updateLod(mesh, this.lowDetail)
  }

  private updateLod(object: Object3D, lowDetail: boolean): void {
    if (object.userData.lod === 0) object.visible = !lowDetail
    if (object.userData.lod === 1) object.visible = lowDetail
  }

  private updateEquipmentLod(piece: EquipmentInstance): void {
    // A single-LOD item must keep its only model when the camera zooms out.
    const lowDetail = this.lowDetail && piece.meshes.some((mesh) => mesh.userData.lod === 1)
    piece.root.traverse((object) => this.updateLod(object, lowDetail))
  }

  private async load(): Promise<void> {
    const catalog = await this.cache.catalog
    this.catalog = catalog.equipment
    if (this.destroyed) return
    const lease = await this.cache.acquire(COMMONER_ASSET_ID)
    if (this.destroyed) { lease.release(); return }
    this.releaseModel = lease.release
    this.model = clone(lease.asset.scene)
    const animations = bindClips(this.model, lease.asset.manifest, lease.asset.animations)
    this.actionPlayer.configure(catalog.actionAnimations, lease.asset.manifest.id)
    const actionClips = new Set(Object.values(catalog.actionAnimations).filter(binding => binding.actor === lease.asset.manifest.id).flatMap(binding => binding.variants.map(variant => variant.clip)))
    this.actionLayers = new ActorActionLayers(this.model, animations, actionClips)
    const walk = lease.asset.manifest.clips.walk
    if (!walk?.cycleDistanceTiles || walk.cycleDistanceTiles <= 0 || !Number.isFinite(walk.cycleDistanceTiles) || lease.asset.manifest.clips.carry_walk?.cycleDistanceTiles !== walk.cycleDistanceTiles) throw new Error('Invalid walk distance metadata')
    for (const name of [...GAIT_CLIPS, 'carry_walk'] as const) {
      const clip = lease.asset.manifest.clips[name]
      if (!clip || clip.playback !== 'distance' || !clip.loop || !clip.cycleDistanceTiles || !Number.isFinite(clip.cycleDistanceTiles) || clip.cycleDistanceTiles <= 0) throw new Error(`Invalid locomotion metadata: ${name}`)
      this.cycleDistances.set(name, clip.cycleDistanceTiles)
    }
    this.model.traverse((object) => {
      if (object.userData.lod === 1) object.visible = false
      if (object instanceof Bone) this.bones.set(object.name, object)
      if (object instanceof SkinnedMesh) this.prepareMesh(object)
    })
    if (this.bones.size === 0) throw new Error('Character has no skeleton')
    this.sockets = new ActorSockets(this.model, lease.asset.manifest.sockets)
    this.armLayers = new ActorArmLayers(this.model, animations)
    this.root.add(this.model)
    this.mixer = new AnimationMixer(this.model)
    for (const name of ['idle', ...GAIT_CLIPS, 'carry_idle', 'carry_walk']) {
      const clip = animations.find((candidate) => candidate.name === name)
      if (!clip) throw new Error(`Character animation missing: ${name}`)
      this.actions.set(name, this.mixer.clipAction(clip))
    }
    await this.setEquipment(DEFAULT_EQUIPMENT)
    if (this.destroyed) return
    this.loaded = true
    this.updatePose()
  }

  async setEquipment(items: readonly EquippedVisual[]): Promise<void> {
    validateEquipment(items, this.catalog)
    if (this.destroyed) return
    if (!this.model || !this.armLayers) throw new Error('Character is not loaded')
    const desired = items.map((item) => ({ ...item }))
    const poses: Record<ArmSide, string | null> = { left: null, right: null }
    const profiles: Partial<Record<ArmSide, Extract<ArmMotion, { kind: 'layered' }>>> = {}
    const renderable = desired.flatMap((item) => {
      const definition = Object.hasOwn(this.catalog, item.visualKey) ? this.catalog[item.visualKey] : undefined
      if (!definition || definition.kind === 'deferred') return []
      const side = armForSlot(item.slot)
      if (definition.kind === 'rigid' && side) {
        const binding = definition.bindings[item.slot]!
        const motion = binding.armMotion
        if (motion?.kind === 'layered') {
          poses[side] = motion.idlePose
          profiles[side] = motion
          this.armLayers!.validate(side, motion.walkPose ?? null)
        }
      }
      return [{ item, definition }]
    })
    for (const side of ['left', 'right'] as const) this.armLayers.validate(side, poses[side])
    const revision = ++this.equipmentRevision
    this.equipmentReady = false
    const leases = await Promise.allSettled(renderable.map(({ definition }) => this.cache.acquire(definition.assetId)))
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
      this.updateEquipmentAttachment(slot, piece)
    })
    this.requestedEquipment = desired
    this.equipmentReady = true
    this.armProfiles = profiles
    for (const side of ['left', 'right'] as const) this.armLayers.setPose(side, poses[side])
    this.lastPose = ''
    this.immediateRender = true
    this.updatePose()
  }

  private attachEquipment(asset: ActorBundle, release: () => void, definition: Exclude<EquipmentDefinition, { kind: 'deferred' }>, slot: EquipmentSlot): EquipmentInstance {
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
      this.updateEquipmentLod(piece)
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

  private updateEquipmentAttachment(slot: EquipmentSlot, piece: EquipmentInstance): void {
    // Retain the loaded object and local attachment transform while the clip owns the slot.
    if (this.actionPlayer.unboundEquipmentSlots.has(slot)) piece.root.removeFromParent()
    else if (piece.root.parent !== piece.parent) piece.parent.add(piece.root)
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
    if (this.armLayers.setPose(side, clip, sampleTime)) {
      this.lastPose = ''
      this.immediateRender = true
    }
    delete this.armProfiles[side]
  }

  get direction(): number { return this.facingDirection }

  set direction(direction: number) {
    if (!Number.isInteger(direction) || direction < 0 || direction > 7) throw new Error(`Invalid facing direction: ${direction}`)
    if (direction === this.facingDirection) return
    this.facingDirection = direction
    this.setFacingAngle(screenFacingAngle(direction))
  }

  setFacingAngle(angle: number): void {
    if (!Number.isFinite(angle)) throw new Error('Invalid screen-facing angle')
    const target = Math.atan2(Math.sin(angle), Math.cos(angle))
    if (Math.abs(Math.atan2(Math.sin(target - this.targetFacingAngle), Math.cos(target - this.targetFacingAngle))) < 1e-6) return
    this.targetFacingAngle = target
    this.lastPose = ''
    this.immediateRender = true
  }

  private updateFacing(now: number, settings: ActorRenderSettings): boolean {
    const desiredFacing = this.actionPlayer.facingAngle ?? this.targetFacingAngle
    if (settings.mode === 'baked8') {
      const direction = ((Math.floor(desiredFacing / (Math.PI / 4) + .5) + 1) % 8 + 8) % 8
      const next = screenFacingAngle(direction)
      const changed = Math.abs(Math.atan2(Math.sin(next - this.facingAngle), Math.cos(next - this.facingAngle))) > 1e-6
      this.facingDirection = direction
      this.facingAngle = next
      this.lastFacingUpdateMs = now
      return changed
    }
    const previousUpdate = this.lastFacingUpdateMs
    this.lastFacingUpdateMs = now
    if (previousUpdate === null) {
      // A new model starts in its supplied facing, rather than turning from the default.
      this.facingAngle = desiredFacing
      return true
    }
    const difference = Math.atan2(Math.sin(desiredFacing - this.facingAngle), Math.cos(desiredFacing - this.facingAngle))
    const maximumStep = Math.PI * Math.max(0, now - previousUpdate) / settings.turnDurationMs
    const step = Math.max(-maximumStep, Math.min(maximumStep, difference))
    if (Math.abs(step) < 1e-6) return false
    this.facingAngle = Math.atan2(Math.sin(this.facingAngle + step), Math.cos(this.facingAngle + step))
    return true
  }

  private updateGait(now: number, bakedMode: boolean): void {
    const next = locomotionClip(this.movementMode, false) as GaitClip
    const progress = Math.max(0, Math.min(1, (now - this.gaitBlendStarted) / ACTOR_RENDER.locomotionBlendMs))
    for (const clip of GAIT_CLIPS) this.gaitWeights[clip] = this.gaitBlendFrom[clip] + ((clip === this.gait ? 1 : 0) - this.gaitBlendFrom[clip]) * progress
    if (next !== this.gait) {
      for (const clip of GAIT_CLIPS) this.gaitBlendFrom[clip] = this.gaitWeights[clip]
      this.gait = next
      this.gaitBlendStarted = now
      // Each gait owns its stride; do not carry a restart offset into another clip.
      this.walkPhaseOffset = 0
    }
    if (bakedMode || !this.walking || this.knockedOut || this.carrying) {
      for (const clip of GAIT_CLIPS) this.gaitWeights[clip] = this.gaitBlendFrom[clip] = clip === next ? 1 : 0
    }
  }

  updatePose(now = performance.now(), settings: ActorRenderSettings = DEFAULT_ACTOR_RENDER_SETTINGS): boolean {
    if (!this.mixer || this.destroyed) return false
    this.prepareActionAnimation(now)
    const carrying = this.carrying && !this.knockedOut
    this.updateGait(now, settings.mode === 'baked8')
    const continuousPhase = this.walking && !this.knockedOut ? ((this.distanceTiles / this.cycleDistanceTiles) % 1 + 1) % 1 : 0
    const phase = settings.mode === 'baked8' ? Math.floor(continuousPhase * ACTOR_RENDER.walkSamples + 1e-7) / ACTOR_RENDER.walkSamples : continuousPhase
    const bakedMode = settings.mode === 'baked8'
    const holdingBakedWalkFrame = !this.knockedOut && bakedMode && this.stopProgress !== undefined && this.stopProgress < 1
    const visualWalking = !this.knockedOut && (holdingBakedWalkFrame || (this.walking && !(bakedMode && this.stopProgress === 1)))
    const renderedPhase = holdingBakedWalkFrame ? this.walkPhase : phase
    const facingChanged = this.updateFacing(now, settings)
    const name = this.knockedOut ? 'knocked_out' : visualWalking ? this.locomotionClip : carrying ? 'carry_idle' : 'idle'
    const blendDuration = this.walkTarget === 0 ? ACTOR_RENDER.locomotionStopMs : ACTOR_RENDER.locomotionBlendMs
    const progress = Math.max(0, Math.min(1, (now - this.walkBlendStarted) / blendDuration))
    if (!bakedMode && this.stopStartWeight === undefined) {
      this.walkWeight = this.walkBlendFrom + (this.walkTarget - this.walkBlendFrom) * progress
      if (progress === 1) this.walkBlendFrom = this.walkTarget
    }
    const target = visualWalking ? 1 : 0
    if (this.knockedOut) {
      // Knockout must cancel a pending gait blend immediately, including a buffered stop.
      this.stopStartWeight = undefined
      this.walkWeight = this.walkBlendFrom = this.walkTarget = 0
      this.walkPhase = this.walkPhaseOffset = 0
      this.walkBlendStarted = now
    } else if (bakedMode) {
      this.stopStartWeight = undefined
      if (visualWalking) {
        this.walkWeight = 1
        this.walkBlendFrom = 1
        this.walkTarget = 1
      } else {
        this.walkWeight = 0
        this.walkBlendFrom = 0
        this.walkTarget = 0
        this.walkBlendStarted = now
      }
    } else if (this.stopProgress !== undefined) {
      this.stopStartWeight ??= this.walkWeight
      this.walkWeight = this.stopStartWeight * (1 - this.stopProgress)
      this.walkBlendFrom = this.walkWeight
      this.walkTarget = 0
      this.walkBlendStarted = now
    } else if (target !== this.walkTarget) {
      // A reversal starts from the current mixture, not either endpoint.
      this.walkBlendFrom = this.walkWeight
      this.walkTarget = target
      this.walkBlendStarted = now
      // ObjectView resets its distance on stop; a quick restart must not reset the visible gait.
      if (this.walking) this.walkPhaseOffset = this.walkWeight > 0 ? this.walkPhase - phase : 0
    }
    if (this.stopProgress === undefined) this.stopStartWeight = undefined
    // Baked8 holds one authored pose through position settling instead of blending it at display rate.
    if (visualWalking && !holdingBakedWalkFrame) this.walkPhase = ((phase + this.walkPhaseOffset) % 1 + 1) % 1
    const frame = this.outputFrame
    const actionIdentity = `${this.actionPlayer.samples.map(sample => sample.clip).join(',')}/${frame.width}/${frame.height}/${frame.origin_x}/${frame.origin_y}/${[...this.actionPlayer.unboundEquipmentSlots].sort().join(',')}`
    const actionPose = this.actionPlayer.samples.map(sample => `${sample.clip}:${sample.phase}:${sample.weight}`).join(',')
    const gaitMix = `${this.gaitWeights.crawl},${this.gaitWeights.walk},${this.gaitWeights.run},${this.gaitWeights.fast_run}`
    const key = `${settings.mode}/${name}/${gaitMix}/${renderedPhase}/${this.walkWeight}/${this.facingAngle}/${this.hovered}/${actionPose}/${actionIdentity}`
    if (!facingChanged && key === this.lastPose && (this.knockedOut || carrying || !this.armLayers?.transitioning)) return false
    const state = `${settings.mode}/${name}/${this.hovered}/${actionIdentity}`
    if (state !== this.lastPoseState) this.immediateRender = true
    this.lastPose = key
    this.lastPoseState = state
    if (this.knockedOut) {
      // Rotate the body in world space: face up, head left, centered over its ground position.
      this.root.rotation.set(-Math.PI / 2, 0, Math.PI / 2)
      this.root.position.set(ACTOR_RENDER.knockedOutBodyCenter, ACTOR_RENDER.knockedOutGroundHeight, 0)
    } else {
      this.root.rotation.set(0, actorYawForScreenAngle(this.facingAngle), 0)
      this.root.position.set(0, 0, 0)
    }
    if (!this.knockedOut && !carrying && this.armLayers) {
      for (const side of ['left', 'right'] as const) {
        const profile = this.armProfiles[side]
        if (!profile) continue
        const clip = visualWalking ? profile.walkPose ?? profile.idlePose : profile.idlePose
        // Capture the last displayed arm before locomotion overwrites it.
        if (this.armLayers.getPose(side) !== clip) this.armLayers.setPose(side, clip, 0, now)
        this.armLayers.samplePhase(side, visualWalking && profile.walkPose ? renderedPhase : 0)
      }
    }
    for (const action of this.actions.values()) action.stop()
    const prefix = carrying ? 'carry_' : ''
    const idleAction = this.actions.get(`${prefix}idle`)!
    idleAction.play()
    idleAction.paused = true
    idleAction.setEffectiveWeight(1 - this.walkWeight)
    idleAction.time = 0
    for (const clip of carrying ? ['carry_walk'] as const : GAIT_CLIPS) {
      const weight = this.walkWeight * (carrying ? 1 : this.gaitWeights[clip as GaitClip])
      if (weight === 0) continue
      const action = this.actions.get(clip)!
      action.play()
      action.paused = true
      action.setEffectiveWeight(weight)
      let sample = this.walkPhase
      if (clip !== this.locomotionClip && visualWalking && !holdingBakedWalkFrame) {
        sample = ((this.distanceTiles / this.cycleDistances.get(clip)! + this.walkPhaseOffset) % 1 + 1) % 1
        if (bakedMode) sample = Math.floor(sample * ACTOR_RENDER.walkSamples + 1e-7) / ACTOR_RENDER.walkSamples
      }
      action.time = sample * action.getClip().duration
    }
    this.mixer.update(0)
    if (!this.knockedOut && !carrying) this.armLayers?.apply(now)
    if (!this.knockedOut) this.actionLayers?.apply(this.actionPlayer.samples, bakedMode, ACTOR_RENDER.actionSamples)
    for (const [slot, piece] of this.equipment) {
      this.updateEquipmentAttachment(slot, piece)
      piece.root.visible = !(carrying && armForSlot(slot))
      for (const mesh of piece.meshes) {
        if (!mesh.morphTargetInfluences || !mesh.morphTargetDictionary) continue
        mesh.morphTargetInfluences.fill(0)
        const index = mesh.morphTargetDictionary[`walk_cloth_${Math.round(renderedPhase * 8) % 8}`]
        if (visualWalking && index !== undefined) mesh.morphTargetInfluences[index] = 1
      }
    }
    this.root.updateMatrixWorld(true)
    this.skins.forEach((skin) => skin.update())
    this.revision++
    return true
  }

  get isReady(): boolean { return this.loaded && !this.error }
  get assetId(): string { return COMMONER_ASSET_ID }
  isActionAnimationSelected(key: string): boolean { return this.actionPlayer.isSelected(key) }
  get locomotionClip(): LocomotionClip { return locomotionClip(this.movementMode, this.carrying && !this.knockedOut) }
  get cycleDistanceTiles(): number { return this.cycleDistances.get(this.locomotionClip) ?? 0 }
  get needsImmediateRender(): boolean { return this.immediateRender }
  acknowledgeRender(): void { this.immediateRender = false }
  invalidateRender(): void { this.lastPose = ''; this.immediateRender = true }
  get equippedVisuals(): readonly EquippedVisual[] { return this.requestedEquipment }

  setActionAnimation(input: ActionAnimationInput | null): void { this.actionPlayer.setInput(input) }

  prepareActionAnimation(now: number): void {
    // The stop ease keeps reporting walking; the server-confirmed stop (stopProgress set)
    // is enough, otherwise the pose waits out interpolation delay + LOCOMOTION_STOP_MS.
    this.actionPlayer.update({ stationary: !this.walking || this.stopProgress !== undefined,
      carrying: this.carrying, knockedOut: this.knockedOut, equipment: this.requestedEquipment, equipmentReady: this.equipmentReady }, now)
  }

  get outputFrame(): ActionAnimationFrame {
    return this.knockedOut ? { width: ACTOR_RENDER.cellSize, height: ACTOR_RENDER.cellSize, origin_x: ACTOR_RENDER.anchorX, origin_y: ACTOR_RENDER.knockedOutAnchorY } : this.actionPlayer.frame
  }

  releaseGPUResources(): void {
    // Per-instance skin textures also hold disposal listeners from the old context.
    for (const skin of this.skins.values()) {
      skin.texture.dispose()
      skin.skeleton.boneTexture?.dispose()
    }
  }

  setLowDetail(enabled: boolean): void {
    if (this.lowDetail === enabled || !this.model) return
    this.lowDetail = enabled
    this.model.traverse((object) => this.updateLod(object, enabled))
    for (const piece of this.equipment.values()) this.updateEquipmentLod(piece)
    this.immediateRender = true
    this.revision++
  }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    this.equipmentRevision++
    this.equipment.forEach((piece) => this.disposeEquipment(piece))
    this.equipment.clear()
    this.armLayers?.destroy()
    this.actionLayers?.destroy()
    this.mixer?.stopAllAction()
    if (this.model) this.mixer?.uncacheRoot(this.model)
    this.skins.forEach((skin) => { skin.destroy(); skin.skeleton.dispose() })
    this.skins.clear()
    this.materials.forEach((material) => material.dispose())
    this.materials.clear()
    this.root.removeFromParent()
    this.root.clear()
    this.releaseModel?.()
    this.releaseModel = null
    this.actions.clear()
    this.bones.clear()
    this.model = null
    this.mixer = null
    this.armLayers = null
    this.sockets = null
  }
}
