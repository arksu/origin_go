import { AnimationClip, AnimationMixer, Bone, Group, LoopOnce, PropertyBinding, Quaternion, Vector3, type AnimationAction, type Object3D } from 'three'
import { findRigBone } from './ActorSockets'
import type { ArmSide } from './equipment'

interface JointPose { position: Vector3; quaternion: Quaternion; scale: Vector3 }
interface PoseChannel { bone: Bone; sample: Bone; property: 'position' | 'quaternion' | 'scale' }
interface ArmLayer {
  bones: Bone[]
  sampler: Group
  mixer: AnimationMixer
  clips: Map<string, AnimationClip>
  channels: Map<string, PoseChannel[]>
  action?: AnimationAction
  name: string | null
  time: number
  from: JointPose[] | null
  startedAt: number
}

// Each sampler writes only one arm's local transforms after the locomotion
// mixer. Parent torso motion is inherited, while leg tracks cannot enter a mask.
export class ActorArmLayers {
  private readonly layers: Record<ArmSide, ArmLayer>
  constructor(model: Object3D, clips: readonly AnimationClip[], private readonly transitionMs = 120) {
    const create = (side: 'l' | 'r'): ArmLayer => {
      const bones: Bone[] = []
      findRigBone(model, `clavicle.${side}`).traverse((object) => { if (object instanceof Bone) bones.push(object) })
      // Mixer bindings cache their last value. Isolated sampling bones let a
      // constant pose be reapplied after every locomotion update.
      const sampler = new Group()
      const channelsByBone = new Map(bones.map((bone) => {
        const sample = bone.clone(false)
        sampler.add(sample)
        return [bone.name, { bone, sample }]
      }))
      const masked = new Map<string, AnimationClip>()
      const channels = new Map<string, PoseChannel[]>()
      for (const clip of clips) {
        const poseChannels: PoseChannel[] = []
        const tracks = clip.tracks.filter((track) => {
          const binding = PropertyBinding.parseTrackName(track.name)
          const pair = channelsByBone.get(binding.nodeName)
          const property = binding.propertyName
          if (!pair || binding.objectName || binding.propertyIndex !== undefined || !['position', 'quaternion', 'scale'].includes(property)) return false
          poseChannels.push({ ...pair, property: property as PoseChannel['property'] })
          return true
        })
        if (tracks.length) {
          masked.set(clip.name, new AnimationClip(`${clip.name}_${side}`, clip.duration, tracks))
          channels.set(clip.name, poseChannels)
        }
      }
      return { bones, sampler, mixer: new AnimationMixer(sampler), clips: masked, channels, name: null, time: 0, from: null, startedAt: 0 }
    }
    this.layers = { left: create('l'), right: create('r') }
  }

  validate(side: ArmSide, name: string | null): void {
    if (name !== null && !this.layers[side].clips.has(name)) throw new Error(`Character ${side} arm pose missing: ${name}`)
  }

  setPose(side: ArmSide, name: string | null, time = 0, now = performance.now()): boolean {
    this.validate(side, name)
    if (!Number.isFinite(time) || time < 0) throw new Error('Invalid arm pose sample time')
    const layer = this.layers[side]
    if (layer.name === name && layer.time === time) return false
    layer.from = layer.bones.map((bone) => ({ position: bone.position.clone(), quaternion: bone.quaternion.clone(), scale: bone.scale.clone() }))
    layer.action?.stop()
    layer.name = name
    layer.time = time
    layer.startedAt = now
    layer.action = name === null ? undefined : layer.mixer.clipAction(layer.clips.get(name)!)
    if (layer.action) {
      layer.action.setLoop(LoopOnce, 1).play()
      layer.action.clampWhenFinished = true
      layer.action.paused = true
    }
    return true
  }

  samplePhase(side: ArmSide, phase: number): void {
    if (!Number.isFinite(phase) || phase < 0 || phase > 1) throw new Error('Invalid arm pose phase')
    const layer = this.layers[side]
    // Distance sampling must not restart the equip/locomotion crossfade.
    if (layer.action) layer.time = phase * layer.action.getClip().duration
  }

  getPose(side: ArmSide): string | null { return this.layers[side].name }

  apply(now: number): void {
    for (const layer of Object.values(this.layers)) {
      if (layer.action) {
        layer.action.time = Math.min(layer.time, layer.action.getClip().duration)
        layer.mixer.update(0)
        for (const { bone, sample, property } of layer.channels.get(layer.name!)!) {
          if (property === 'quaternion') bone.quaternion.copy(sample.quaternion)
          else bone[property].copy(sample[property])
        }
      }
      if (!layer.from) continue
      const blend = Math.min(1, Math.max(0, (now - layer.startedAt) / this.transitionMs))
      if (blend === 1) { layer.from = null; continue }
      for (let index = 0; index < layer.bones.length; index++) {
        const bone = layer.bones[index]!
        const from = layer.from[index]!
        bone.position.lerp(from.position, 1 - blend)
        bone.quaternion.slerp(from.quaternion, 1 - blend)
        bone.scale.lerp(from.scale, 1 - blend)
      }
    }
  }

  get transitioning(): boolean { return Object.values(this.layers).some((layer) => layer.from !== null) }

  destroy(): void {
    for (const layer of Object.values(this.layers)) {
      layer.mixer.stopAllAction()
      layer.mixer.uncacheRoot(layer.sampler)
    }
  }
}
