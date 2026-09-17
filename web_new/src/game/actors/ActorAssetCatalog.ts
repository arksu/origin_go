import { Matrix4, Quaternion, Vector3 } from 'three'
import { EQUIPMENT_SLOT_BY_ID, type EquipmentSlot } from '../../types/characterVisual'
import type { ArmMotion, EquipmentBinding, EquipmentDefinition, SocketId } from './equipment'

export interface Artifact { url: string; sha256: string; bytes: number }
export interface ClipManifest {
  artifact: Artifact
  rigHash: string
  channelMask: string[]
  duration: number
  loop: boolean
  playback: 'time' | 'distance'
  cycleDistanceTiles?: number
}
export interface GripBinding { slot: EquipmentSlot; socket: SocketId; grip: string; gripInverse: number[]; gripMatrix: number[]; policy: ArmMotion }
export interface ActorManifest {
  schema: number
  id: string
  kind: 'character' | 'equipment'
  rigHash: string | null
  model: Artifact
  metadata: Artifact
  textures: Artifact[]
  sockets: Partial<Record<SocketId, string>>
  clips: Record<string, ClipManifest>
  bindings: Partial<Record<EquipmentSlot, GripBinding>>
}
export interface ActorCatalog {
  readonly manifests: Readonly<Record<string, ActorManifest>>
  readonly equipment: Readonly<Record<string, EquipmentDefinition>>
}
const HASH = /^[a-f0-9]{64}$/
const SOCKETS: SocketId[] = ['grip_l', 'grip_r', 'forearm_l', 'forearm_r']
const NAME = /^[a-zA-Z0-9_.-]+$/
function record(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error(`Invalid ${label}`)
  return value as Record<string, unknown>
}
function positive(value: unknown): value is number { return typeof value === 'number' && Number.isFinite(value) && value > 0 }
export function validateAssetURL(value: unknown): string {
  if (typeof value !== 'string' || !/^\/assets\/game\/[a-zA-Z0-9_./-]+$/.test(value) || value.includes('//') || value.split('/').some(part => part === '..' || part === '.')) throw new Error(`Invalid actor asset URL: ${String(value)}`)
  return value
}
function artifact(value: unknown, extension: string): void {
  const item = record(value, 'artifact')
  const url = validateAssetURL(item.url)
  if (typeof item.sha256 !== 'string' || !HASH.test(item.sha256) || !url.endsWith(`/${item.sha256}.${extension}`) || !Number.isSafeInteger(item.bytes) || !positive(item.bytes)) throw new Error('Invalid immutable artifact')
}
function matrix(value: unknown): value is number[] { return Array.isArray(value) && value.length === 16 && value.every(v => typeof v === 'number' && Number.isFinite(v)) }
export function parseActorManifest(value: unknown): ActorManifest {
  const manifest = record(value, 'manifest')
  if (manifest.schema !== 1 || typeof manifest.kind !== 'string' || !['character', 'equipment'].includes(manifest.kind) || typeof manifest.id !== 'string' || !/^(character|equipment)\/[a-zA-Z0-9_-]+$/.test(manifest.id) || !manifest.id.startsWith(`${manifest.kind}/`)) throw new Error('Invalid actor manifest schema or id')
  if (manifest.kind === 'character' ? typeof manifest.rigHash !== 'string' || !HASH.test(manifest.rigHash) : manifest.rigHash !== null && (typeof manifest.rigHash !== 'string' || !HASH.test(manifest.rigHash))) throw new Error('Invalid rigHash')
  artifact(manifest.model, 'glb'); artifact(manifest.metadata, 'json')
  if (!Array.isArray(manifest.textures)) throw new Error('Invalid textures')
  manifest.textures.forEach(texture => artifact(texture, 'ktx2'))
  const sockets = record(manifest.sockets, 'sockets')
  for (const [name, node] of Object.entries(sockets)) if (!SOCKETS.includes(name as SocketId) || typeof node !== 'string' || !NAME.test(node)) throw new Error('Invalid socket')
  if (manifest.kind === 'character' && SOCKETS.some(name => !sockets[name])) throw new Error('Required character socket missing')
  const clips = record(manifest.clips, 'clips')
  for (const [name, value] of Object.entries(clips)) {
    const clip = record(value, `clip ${name}`)
    if (!NAME.test(name) || clip.rigHash !== manifest.rigHash || !positive(clip.duration) || typeof clip.loop !== 'boolean' || typeof clip.playback !== 'string' || !['time', 'distance'].includes(clip.playback)) throw new Error(`Invalid clip or rigHash: ${name}`)
    artifact(clip.artifact, 'glb')
    if (!Array.isArray(clip.channelMask) || !clip.channelMask.length || clip.channelMask.some(node => typeof node !== 'string' || !NAME.test(node)) || new Set(clip.channelMask).size !== clip.channelMask.length) throw new Error(`Invalid channel mask: ${name}`)
    if (clip.playback === 'distance' && !positive(clip.cycleDistanceTiles)) throw new Error(`Invalid cycleDistanceTiles: ${name}`)
  }
  if (manifest.kind === 'character') {
    for (const name of ['idle', 'walk', 'carry_idle', 'carry_walk']) if (!clips[name]) throw new Error(`Required animation missing: ${name}`)
    const walk = clips.walk as ClipManifest; const carry = clips.carry_walk as ClipManifest
    if (walk.playback !== 'distance' || carry.playback !== 'distance' || walk.cycleDistanceTiles !== carry.cycleDistanceTiles || walk.duration !== carry.duration || !walk.loop || !carry.loop) throw new Error('Incompatible carry_walk locomotion metadata')
  }
  const bindings = record(manifest.bindings, 'bindings')
  for (const [slot, value] of Object.entries(bindings)) {
    const binding = record(value, 'binding'); const policy = record(binding.policy, 'arm policy')
    if (!Object.values(EQUIPMENT_SLOT_BY_ID).includes(slot as EquipmentSlot) || binding.slot !== slot || !SOCKETS.includes(binding.socket as SocketId) || typeof binding.grip !== 'string' || !NAME.test(binding.grip) || !matrix(binding.gripInverse) || !matrix(binding.gripMatrix)) throw new Error('Invalid grip binding')
    if (policy.kind !== 'ordinary' && (policy.kind !== 'layered' || typeof policy.idlePose !== 'string' || !NAME.test(policy.idlePose) || policy.walkPose !== undefined && (typeof policy.walkPose !== 'string' || !NAME.test(policy.walkPose)))) throw new Error('Invalid arm policy')
    const inverse = new Matrix4().fromArray(binding.gripInverse)
    const product = inverse.clone().multiply(new Matrix4().fromArray(binding.gripMatrix))
    if (Math.abs(inverse.determinant()) < 1e-8 || product.elements.some((n, i) => Math.abs(n - (i % 5 === 0 ? 1 : 0)) > .001)) throw new Error('Invalid inverse grip matrix')
  }
  return manifest as unknown as ActorManifest
}
function freeze<T>(value: T): T {
  if (value && typeof value === 'object') { Object.values(value).forEach(freeze); Object.freeze(value) }
  return value
}
export async function loadActorCatalog(url = '/assets/game/asset-catalog.json', fetcher: typeof fetch = fetch): Promise<ActorCatalog> {
  async function json(path: string, cache: RequestCache) {
    const response = await fetcher(validateAssetURL(path), { cache, redirect: 'error', credentials: 'same-origin' })
    if (!response.ok) throw new Error(`Unable to load actor catalog resource ${path}: ${response.status}`)
    return response.json() as Promise<unknown>
  }
  const snapshot = record(await json(url, 'no-cache'), 'catalog')
  if (snapshot.schema !== 1) throw new Error('Invalid actor catalog schema')
  const assets = record(snapshot.assets, 'catalog assets')
  // Validate the complete snapshot before making any referenced requests.
  for (const reference of Object.values(assets)) artifact(reference, 'json')
  const entries = await Promise.all(Object.entries(assets).map(async ([id, reference]) => {
    const manifest = parseActorManifest(await json((reference as Artifact).url, 'force-cache'))
    if (manifest.id !== id) throw new Error(`Catalog manifest id mismatch: ${id}`)
    return [id, manifest] as const
  }))
  const manifests = Object.fromEntries(entries)
  const equipment: Record<string, EquipmentDefinition> = Object.create(null)
  for (const [id, manifest] of entries) {
    if (manifest.kind !== 'equipment') continue
    const bindings: Partial<Record<EquipmentSlot, EquipmentBinding>> = {}
    for (const [slot, binding] of Object.entries(manifest.bindings)) {
      const position = new Vector3(); const rotation = new Quaternion(); const scale = new Vector3()
      new Matrix4().fromArray(binding.gripInverse).decompose(position, rotation, scale)
      if (scale.x <= 0 || Math.abs(scale.x - scale.y) > .001 || Math.abs(scale.x - scale.z) > .001) throw new Error(`Nonuniform equipment grip scale: ${id}`)
      bindings[slot as EquipmentSlot] = { socket: binding.socket, armMotion: binding.policy,
        transform: { position: position.toArray(), quaternion: rotation.normalize().toArray(), scale: scale.x } }
    }
    equipment[id.slice('equipment/'.length)] = { kind: 'rigid', assetId: id, bindings }
  }
  return freeze({ manifests, equipment })
}
