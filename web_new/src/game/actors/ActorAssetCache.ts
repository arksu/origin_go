import { BufferGeometry, CompressedTexture, InterleavedBufferAttribute, LoadingManager, Material, Mesh, PropertyBinding, SkinnedMesh, Texture, type AnimationClip, type Group, type WebGLRenderer } from 'three'
import { GLTFLoader, type GLTF } from 'three/addons/loaders/GLTFLoader.js'
import { KTX2Loader } from 'three/addons/loaders/KTX2Loader.js'
import { MeshoptDecoder } from 'three/addons/libs/meshopt_decoder.module.js'
import { loadActorCatalog, validateAssetURL, type ActorCatalog, type ActorManifest } from './ActorAssetCatalog'
import { bindClips } from './ActorClipBinding'
import { ActorSockets } from './ActorSockets'
import { ACTOR_RENDER } from './config'

export interface ActorBundle { scene: Group; animations: AnimationClip[]; manifest: ActorManifest }
interface Lease<T> { asset: T; release(): void }
interface Entry<T> { promise: Promise<T>; asset?: T; references: number }

function resources(assets: Iterable<GLTF>) {
  const geometries = new Set<BufferGeometry>(); const materials = new Set<Material>(); const textures = new Set<Texture>()
  for (const asset of assets) asset.scene.traverse(object => {
    if (!(object instanceof Mesh)) return
    geometries.add(object.geometry)
    for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
      materials.add(material)
      for (const value of Object.values(material)) if (value instanceof Texture) textures.add(value)
    }
  })
  return { geometries, materials, textures }
}

/** Counts decoded CPU buffers and texture payloads, never compressed GLB transfer sizes. */
export function actorResidentBytes(assets: Iterable<GLTF>, extraTextures: Iterable<Texture> = []): number {
  const loaded = [...assets]; const { geometries, textures } = resources(loaded)
  for (const texture of extraTextures) textures.add(texture)
  const buffers = new Set<ArrayBufferLike>()
  for (const geometry of geometries) {
    const attributes = [...Object.values(geometry.attributes), ...Object.values(geometry.morphAttributes).flat()]
    if (geometry.index) attributes.push(geometry.index)
    for (const attribute of attributes) buffers.add(attribute instanceof InterleavedBufferAttribute ? attribute.data.array.buffer : attribute.array.buffer)
  }
  let imageBytes = 0
  for (const texture of textures) {
    if (texture instanceof CompressedTexture) {
      for (const mip of texture.mipmaps) if ('data' in mip && ArrayBuffer.isView(mip.data)) buffers.add(mip.data.buffer)
    } else {
      const image = texture.image as { width?: number; height?: number; data?: ArrayBufferView } | undefined
      if (image?.data) buffers.add(image.data.buffer)
      else imageBytes += (image?.width ?? 1) * (image?.height ?? 1) * 4 * (texture.generateMipmaps ? 4 / 3 : 1)
    }
  }
  for (const asset of loaded) for (const clip of asset.animations) for (const track of clip.tracks) {
    buffers.add(track.times.buffer); buffers.add(track.values.buffer)
  }
  return Math.ceil(imageBytes + [...buffers].reduce((sum, buffer) => sum + buffer.byteLength, 0))
}

export class ActorAssetCache {
  private readonly loader: GLTFLoader
  private readonly ktx: KTX2Loader
  private catalogPromise?: Promise<ActorCatalog>
  private readonly entries = new Map<string, Entry<GLTF>>()
  private readonly textures = new Map<string, Entry<CompressedTexture>>()
  private destroyed = false

  constructor(renderer: WebGLRenderer, private readonly catalogURL = '/assets/game/asset-catalog.json') {
    const manager = new LoadingManager()
    manager.setURLModifier(validateAssetURL)
    this.ktx = new KTX2Loader(manager).setTranscoderPath('/assets/game/decoders/basis/').detectSupport(renderer)
    const originalLoad = this.ktx.load.bind(this.ktx)
    this.loadTexture = url => new Promise((resolve, reject) => { originalLoad(url, resolve, undefined, reject) })
    // Preleased KTX textures are shared even when two model revisions reference them.
    this.ktx.load = (url, onLoad, _onProgress, onError) => {
      const entry = this.textures.get(url)
      if (!entry) { onError?.(new Error(`Undeclared actor texture: ${url}`)); return undefined }
      void entry.promise.then(onLoad).catch(error => onError?.(error))
      return undefined
    }
    this.loader = new GLTFLoader(manager).setMeshoptDecoder(MeshoptDecoder).setKTX2Loader(this.ktx)
    this.loader.register(parser => ({
      name: 'ACTOR_EXPORTED_NAMES',
      beforeRoot: async () => {
        // GLTFLoader otherwise repairs collisions with suffixes, hiding an invalid rig contract.
        const names = new Set<string>()
        for (const node of parser.json.nodes ?? []) {
          if (!node.name) continue
          const name = PropertyBinding.sanitizeNodeName(node.name)
          if (names.has(name)) throw new Error(`Duplicate sanitized exported node name: ${name}`)
          names.add(name)
        }
      },
    }))
  }
  private readonly loadTexture: (url: string) => Promise<CompressedTexture>

  get catalog(): Promise<ActorCatalog> {
    if (this.destroyed) return Promise.reject(new Error('Actor asset cache is disposed'))
    return this.catalogPromise ??= loadActorCatalog(this.catalogURL)
  }

  private async acquireEntry<T>(entries: Map<string, Entry<T>>, url: string, load: () => Promise<T>, dispose: (asset: T) => void): Promise<Lease<T>> {
    if (this.destroyed) throw new Error('Actor asset cache is disposed')
    let entry = entries.get(url)
    if (!entry) {
      const created: Entry<T> = { promise: undefined!, references: 0 }
      created.promise = load().then(asset => {
        created.asset = asset
        if (this.destroyed) throw new Error('Actor asset cache disposed during load')
        if (this.residentBytes > ACTOR_RENDER.maxResidentBytes) throw new Error('Actor asset memory budget exceeded')
        return asset
      }).catch(error => {
        if (created.asset) { dispose(created.asset); created.asset = undefined }
        entries.delete(url)
        throw error
      })
      entries.set(url, created); entry = created
    }
    entry.references++
    try {
      const asset = await entry.promise
      let released = false
      return { asset, release: () => {
        if (released) return
        released = true
        if (--entry.references === 0 && entry.asset) {
          entries.delete(url); dispose(entry.asset); entry.asset = undefined
        }
      } }
    } catch (error) { entry.references--; throw error }
  }

  private disposeArtifact(asset: GLTF): void {
    const owned = resources([asset])
    const remaining = resources([...this.entries.values()].flatMap(entry => entry.asset && entry.asset !== asset ? [entry.asset] : []))
    owned.geometries.forEach(geometry => { if (!remaining.geometries.has(geometry)) geometry.dispose() })
    owned.materials.forEach(material => { if (!remaining.materials.has(material)) material.dispose() })
    const leased = new Set([...this.textures.values()].flatMap(entry => entry.asset ? [entry.asset] : []))
    owned.textures.forEach(texture => { if (!remaining.textures.has(texture) && !leased.has(texture as CompressedTexture)) this.disposeTexture(texture) })
    asset.scene.traverse(object => { if (object instanceof SkinnedMesh) object.skeleton.dispose() })
    // AnimationClip has no dispose API. Dropping the final artifact releases its arrays.
  }
  private disposeTexture(texture: Texture): void {
    texture.dispose()
    const image = texture.image as { close?: () => void } | undefined
    image?.close?.()
  }

  async acquire(id: string): Promise<Lease<ActorBundle>> {
    const catalog = await this.catalog
    if (this.destroyed) throw new Error('Actor asset cache is disposed')
    const manifest = Object.hasOwn(catalog.manifests, id) ? catalog.manifests[id] : undefined
    if (!manifest) throw new Error(`Actor asset not in catalog: ${id}`)
    const leases: Lease<unknown>[] = []
    const release = () => { leases.splice(0).reverse().forEach(lease => lease.release()) }
    try {
      const textureResults = await Promise.allSettled(manifest.textures.map(texture => this.acquireEntry(this.textures, texture.url, () => this.loadTexture(texture.url), value => this.disposeTexture(value))))
      for (const result of textureResults) if (result.status === 'fulfilled') leases.push(result.value)
      const textureFailure = textureResults.find(result => result.status === 'rejected')
      if (textureFailure?.status === 'rejected') throw textureFailure.reason
      const urls = [manifest.model.url, ...Object.values(manifest.clips).map(clip => clip.artifact.url)]
      const results = await Promise.allSettled(urls.map(url => this.acquireEntry(this.entries, url, () => this.loader.loadAsync(url), asset => this.disposeArtifact(asset))))
      for (const result of results) if (result.status === 'fulfilled') leases.push(result.value)
      const failure = results.find(result => result.status === 'rejected')
      if (failure?.status === 'rejected') throw failure.reason
      if (this.destroyed) throw new Error('Actor asset cache disposed during load')
      const assets = results.map(result => (result as PromiseFulfilledResult<Lease<GLTF>>).value.asset)
      const model = assets[0]!
      if (model.animations.length) throw new Error('Catalog model contains embedded animations')
      const clips = assets.slice(1).flatMap(asset => asset.animations)
      if (clips.length !== Object.keys(manifest.clips).length) throw new Error('Catalog animation count mismatch')
      const animations = bindClips(model.scene, manifest, clips)
      if (manifest.kind === 'character') new ActorSockets(model.scene, manifest.sockets)
      return { asset: { scene: model.scene, animations, manifest }, release }
    } catch (error) { release(); throw new Error(`Unable to load actor asset ${id}`, { cause: error }) }
  }

  get residentBytes(): number {
    return actorResidentBytes([...this.entries.values()].flatMap(entry => entry.asset ? [entry.asset] : []), [...this.textures.values()].flatMap(entry => entry.asset ? [entry.asset] : []))
  }
  get loadedCount(): number { return [...this.entries.values()].filter(entry => entry.asset).length }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    // Pending decodes must settle before terminating workers, otherwise their promises never reject.
    const pending = [...this.entries.values(), ...this.textures.values()].map(entry => entry.promise)
    for (const [url, entry] of this.entries) if (entry.asset) { this.entries.delete(url); this.disposeArtifact(entry.asset); entry.asset = undefined }
    for (const entry of this.textures.values()) if (entry.asset) { this.disposeTexture(entry.asset); entry.asset = undefined }
    void Promise.allSettled(pending).then(() => { this.textures.clear(); this.entries.clear(); this.ktx.dispose() })
  }
}
