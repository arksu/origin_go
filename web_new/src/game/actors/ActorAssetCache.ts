import { BufferGeometry, Material, Mesh, Texture } from 'three'
import { GLTFLoader, type GLTF } from 'three/addons/loaders/GLTFLoader.js'
import { ACTOR_RENDER } from './config'

interface Entry {
  promise: Promise<GLTF>
  asset?: GLTF
  references: number
  bytes: number
  lastUsed: number
}

function resources(asset: GLTF) {
  const geometries = new Set<BufferGeometry>()
  const materials = new Set<Material>()
  const textures = new Set<Texture>()
  asset.scene.traverse((object) => {
    if (!(object instanceof Mesh)) return
    geometries.add(object.geometry)
    for (const material of Array.isArray(object.material) ? object.material : [object.material]) {
      materials.add(material)
      for (const value of Object.values(material)) if (value instanceof Texture) textures.add(value)
    }
  })
  return { geometries, materials, textures }
}

function bytesUsed(asset: GLTF): number {
  const { geometries, textures } = resources(asset)
  const arrays = new Set<ArrayBufferLike>()
  for (const geometry of geometries) {
    const attributes = [...Object.values(geometry.attributes), ...Object.values(geometry.morphAttributes).flat()]
    if (geometry.index) attributes.push(geometry.index)
    for (const attribute of attributes) if ('array' in attribute) arrays.add(attribute.array.buffer)
  }
  let bytes = [...arrays].reduce((sum, buffer) => sum + buffer.byteLength, 0)
  for (const texture of textures) {
    const image = texture.image as { width?: number; height?: number } | undefined
    bytes += (image?.width ?? 1) * (image?.height ?? 1) * 4 * (texture.generateMipmaps ? 4 / 3 : 1)
  }
  for (const clip of asset.animations) for (const track of clip.tracks) bytes += track.times.byteLength + track.values.byteLength
  return Math.ceil(bytes)
}

function disposeAsset(asset: GLTF): void {
  const { geometries, materials, textures } = resources(asset)
  geometries.forEach((geometry) => geometry.dispose())
  materials.forEach((material) => material.dispose())
  textures.forEach((texture) => {
    texture.dispose()
    const image = texture.image as { close?: () => void } | undefined
    image?.close?.()
  })
}

export class ActorAssetCache {
  private readonly loader = new GLTFLoader()
  private readonly entries = new Map<string, Entry>()
  private destroyed = false

  async acquire(url: string): Promise<{ asset: GLTF; release: () => void }> {
    if (this.destroyed) throw new Error('Actor asset cache is disposed')
    if (!url.startsWith('/assets/game/characters/')) throw new Error(`Invalid actor asset URL: ${url}`)
    let entry = this.entries.get(url)
    if (!entry) {
      const created: Entry = { promise: Promise.resolve(null as unknown as GLTF), references: 0, bytes: 0, lastUsed: performance.now() }
      created.promise = this.loader.loadAsync(url).then((asset) => {
        if (this.destroyed) {
          disposeAsset(asset)
          throw new Error('Actor asset cache disposed during load')
        }
        created.asset = asset
        created.bytes = bytesUsed(asset)
        this.evictUnused()
        if (this.residentBytes > ACTOR_RENDER.maxResidentBytes) {
          disposeAsset(asset)
          created.asset = undefined
          created.bytes = 0
          throw new Error(`Actor asset memory budget exceeded by ${url}`)
        }
        return asset
      }).catch((error: unknown) => {
        this.entries.delete(url)
        throw new Error(`Unable to load actor asset ${url}`, { cause: error })
      })
      entry = created
      this.entries.set(url, entry)
    }
    entry.references++
    try {
      const asset = await entry.promise
      let released = false
      return { asset, release: () => {
        if (released) return
        released = true
        entry.references--
        entry.lastUsed = performance.now()
        this.evictUnused()
      } }
    } catch (error) {
      entry.references--
      throw error
    }
  }

  private evictUnused(): void {
    const unused = [...this.entries].filter(([, entry]) => entry.references === 0 && entry.asset)
      .sort((first, second) => first[1].lastUsed - second[1].lastUsed)
    for (const [url, entry] of unused) {
      if (this.residentBytes <= ACTOR_RENDER.maxResidentBytes) break
      disposeAsset(entry.asset!)
      this.entries.delete(url)
    }
  }

  get residentBytes(): number { return [...this.entries.values()].reduce((sum, entry) => sum + entry.bytes, 0) }
  get loadedCount(): number { return [...this.entries.values()].filter((entry) => entry.asset).length }

  destroy(): void {
    this.destroyed = true
    for (const entry of this.entries.values()) if (entry.asset) disposeAsset(entry.asset)
    this.entries.clear()
  }
}
