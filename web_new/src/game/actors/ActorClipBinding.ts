import { AnimationClip, PropertyBinding, type Object3D } from 'three'
import type { ActorManifest } from './ActorAssetCatalog'

export function bindClips(model: Object3D, manifest: ActorManifest, clips: readonly AnimationClip[]): AnimationClip[] {
  const names = new Map<string, Object3D>()
  model.traverse(node => {
    if (!node.name) return
    const name = PropertyBinding.sanitizeNodeName(node.name)
    if (names.has(name)) throw new Error(`Duplicate sanitized node name: ${name}`)
    names.set(name, node)
  })
  const seen = new Set<string>()
  return clips.map(clip => {
    const metadata = manifest.clips[clip.name]
    if (!metadata || metadata.rigHash !== manifest.rigHash) throw new Error(`Animation rigHash mismatch: ${clip.name}`)
    if (seen.has(clip.name)) throw new Error(`Duplicate animation: ${clip.name}`)
    seen.add(clip.name)
    if (Math.abs(clip.duration - metadata.duration) > .0001) throw new Error(`Animation duration mismatch: ${clip.name}`)
    const mask = new Set(metadata.channelMask.map(name => PropertyBinding.sanitizeNodeName(name)))
    const tracks = clip.tracks.map(track => {
      const parsed = PropertyBinding.parseTrackName(track.name)
      const target = PropertyBinding.sanitizeNodeName(parsed.nodeName)
      if (parsed.objectName || parsed.objectIndex !== undefined || parsed.propertyIndex !== undefined || !['position', 'quaternion', 'scale'].includes(parsed.propertyName)) throw new Error(`Unsupported animation track: ${track.name}`)
      if (!names.has(target)) throw new Error(`Animation target missing: ${target}`)
      if (!mask.has(target)) throw new Error(`Animation target outside channel mask: ${target}`)
      const bound = track.clone()
      bound.name = `${names.get(target)!.name}.${parsed.propertyName}`
      // Values are immutable; share the decoded arrays between actor instances.
      bound.times = track.times; bound.values = track.values
      return bound
    })
    return new AnimationClip(clip.name, clip.duration, tracks, clip.blendMode)
  })
}
