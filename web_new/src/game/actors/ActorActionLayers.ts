import { AnimationMixer, LoopOnce, PropertyBinding, SkinnedMesh, type AnimationAction, type AnimationClip, type Object3D } from 'three'
import { clone } from 'three/addons/utils/SkeletonUtils.js'
import type { ActionPoseSample } from './ActionAnimationPlayer'

interface Channel { target: Object3D; sample: Object3D; property: 'position' | 'quaternion' | 'scale' }

// An isolated sampler preserves the current locomotion/equipment pose as the
// blend destination, including channels not animated by the selected clip.
export class ActorActionLayers {
  private readonly sampler: Object3D
  private readonly mixer: AnimationMixer
  private readonly actions = new Map<string, AnimationAction>()
  private readonly channels = new Map<string, Channel[]>()

  constructor(model: Object3D, clips: readonly AnimationClip[], names: ReadonlySet<string>) {
    this.sampler = clone(model)
    this.mixer = new AnimationMixer(this.sampler)
    for (const clip of clips) {
      if (!names.has(clip.name)) continue
      const channels = clip.tracks.map(track => {
        const binding = PropertyBinding.parseTrackName(track.name)
        const target = model.getObjectByName(binding.nodeName), sample = this.sampler.getObjectByName(binding.nodeName)
        if (!target || !sample || binding.objectName || binding.propertyIndex !== undefined || !['position', 'quaternion', 'scale'].includes(binding.propertyName)) throw new Error(`Unsupported action animation channel: ${track.name}`)
        return { target, sample, property: binding.propertyName as Channel['property'] }
      })
      this.channels.set(clip.name, channels)
      this.actions.set(clip.name, this.mixer.clipAction(clip))
    }
    for (const name of names) if (!this.actions.has(name)) throw new Error(`Action animation clip not loaded: ${name}`)
  }

  apply(samples: readonly ActionPoseSample[], baked: boolean, sampleCount: number): void {
    this.mixer.stopAllAction()
    const total = samples.reduce((sum, sample) => sum + sample.weight, 0)
    if (total <= 0) return
    const channels = new Map<string, Channel>()
    // Several variants can share one clip during a transition; combine its weights.
    const byClip = new Map<string, { phase: number; weight: number }>()
    for (const sample of samples) {
      const previous = byClip.get(sample.clip)
      byClip.set(sample.clip, { phase: sample.phase, weight: (previous?.weight ?? 0) + sample.weight })
    }
    for (const [clip, sample] of byClip) {
      const action = this.actions.get(clip)
      if (!action) throw new Error(`Unknown action animation clip: ${clip}`)
      const phase = baked && sample.phase < 1 ? Math.floor(sample.phase * sampleCount) / sampleCount : sample.phase
      action.setLoop(LoopOnce, 1).play()
      action.clampWhenFinished = true
      action.paused = true
      action.setEffectiveWeight(sample.weight / total)
      action.time = phase * action.getClip().duration
      for (const channel of this.channels.get(clip)!) channels.set(`${channel.target.uuid}/${channel.property}`, channel)
    }
    this.mixer.update(0)
    const weight = Math.min(1, total)
    for (const { target, sample, property } of channels.values()) {
      if (property === 'quaternion') target.quaternion.slerp(sample.quaternion, weight)
      else target[property].lerp(sample[property], weight)
    }
  }

  destroy(): void {
    this.mixer.stopAllAction()
    this.mixer.uncacheRoot(this.sampler)
    const skeletons = new Set<SkinnedMesh['skeleton']>()
    this.sampler.traverse(node => { if (node instanceof SkinnedMesh) skeletons.add(node.skeleton) })
    for (const skeleton of skeletons) skeleton.dispose()
    this.sampler.clear()
  }
}
