import { Assets, Texture, Sprite } from 'pixi.js'
import { Spine } from '@esotericsoftware/spine-pixi-v8'
import objects from './objects'
import { clearAlphaMaskCache } from './PixelHitTest'

// --- Types matching objects.json structure ---

export interface LayerDef {
  img?: string
  spine?: SpineDef
  interactive?: boolean
  offset?: [number, number]
  z?: number
  shadow?: boolean
  frames?: FrameDef[]
  fps?: number
  loop?: boolean
}

export interface FrameDef {
  img: string
  offset?: [number, number]
}

export interface SpineDef {
  file: string
  scale?: number
  skin?: string
  dirs?: Directions
}

export type Directions = {
  [key: string]: string[]
}

export interface ResourceDef {
  layers: LayerDef[]
  size?: [number, number]
  offset?: [number, number]
}

/**
 * ResourceLoader manages loading and caching of game assets.
 * Supports multi-layer sprites, Spine animations, and resource definitions from objects.json.
 */
export class ResourceLoader {
  private static textureCache = new Map<string, Texture>()
  private static textureLoading = new Map<string, Promise<Texture>>()

  /**
   * Resolve a resource path (e.g. "trees/oak/6") to its ResourceDef from objects.json.
   */
  static getResourceDef(resourcePath: string): ResourceDef | undefined {
    if (!resourcePath) return undefined

    const parts = resourcePath.split('/')
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let node: any = objects
    for (const part of parts) {
      if (node == null || typeof node !== 'object') return undefined
      node = node[part]
    }

    if (node && node.layers) {
      return node as ResourceDef
    }
    return undefined
  }

  /**
   * Load a texture by path. Prepends /assets/game/ for file paths.
   * Returns cached texture if available.
   */
  static async loadTexture(imgPath: string): Promise<Texture> {
    const fullPath = imgPath.includes('.') ? '/assets/game/' + imgPath : imgPath

    const cached = this.textureCache.get(fullPath)
    if (cached) return cached

    const loading = this.textureLoading.get(fullPath)
    if (loading) return loading

    const promise = Assets.load(fullPath).then((tex: Texture) => {
      this.textureCache.set(fullPath, tex)
      this.textureLoading.delete(fullPath)
      return tex
    }).catch((err: unknown) => {
      console.warn(`[ResourceLoader] Failed to load texture: ${fullPath}`, err)
      this.textureLoading.delete(fullPath)
      return Texture.WHITE
    })

    this.textureLoading.set(fullPath, promise)
    return promise
  }

  static resolveLayerZ(layer: LayerDef): number {
    return layer.shadow ? -1 : (layer.z ?? 0)
  }

  static resolveLayerPosition(
    layer: LayerDef,
    resDef: ResourceDef,
    frameOffset?: readonly [number, number] | number[],
  ): { x: number; y: number } {
    const layerOffsetX = Number(layer.offset?.[0] ?? 0)
    const layerOffsetY = Number(layer.offset?.[1] ?? 0)
    const rootOffsetX = Number(resDef.offset?.[0] ?? 0)
    const rootOffsetY = Number(resDef.offset?.[1] ?? 0)
    const frameOffsetX = Number(frameOffset?.[0] ?? 0)
    const frameOffsetY = Number(frameOffset?.[1] ?? 0)
    return {
      x: layerOffsetX - rootOffsetX + frameOffsetX,
      y: layerOffsetY - rootOffsetY + frameOffsetY,
    }
  }

  /**
   * Create a Sprite from a loaded texture with layer offsets applied.
   */
  static async createSprite(layer: LayerDef, resDef: ResourceDef): Promise<Sprite> {
    const tex = await this.loadTexture(layer.img!)
    const spr = new Sprite(tex)
    const pos = this.resolveLayerPosition(layer, resDef)
    spr.x = pos.x
    spr.y = pos.y
    spr.zIndex = this.resolveLayerZ(layer)
    return spr
  }

  static async createFrameSprite(layer: LayerDef, resDef: ResourceDef, frame: FrameDef): Promise<Sprite> {
    const tex = await this.loadTexture(frame.img)
    const spr = new Sprite(tex)
    const pos = this.resolveLayerPosition(layer, resDef, frame.offset)
    spr.x = pos.x
    spr.y = pos.y
    spr.zIndex = this.resolveLayerZ(layer)
    return spr
  }

  private static spineLoading = new Map<string, Promise<void>>()

  /**
   * Load and create a Spine animation from a layer definition.
   * Assets must be preloaded via PIXI.Assets before Spine.from() can be called.
   */
  static async loadSpine(layer: LayerDef, resDef: ResourceDef): Promise<Spine> {
    const spineDef = layer.spine!
    const basePath = '/assets/game/' + spineDef.file
    const dataAlias = spineDef.file + '-data'
    const atlasAlias = spineDef.file + '-atlas'

    // Deduplicate concurrent loads for the same spine file
    const cacheKey = spineDef.file
    if (!Assets.cache.has(basePath)) {
      let loadPromise = this.spineLoading.get(cacheKey)
      if (!loadPromise) {
        if (!Assets.resolver.hasKey(dataAlias)) {
          Assets.add({ alias: dataAlias, src: basePath + '.json' })
          Assets.add({ alias: atlasAlias, src: basePath + '.atlas' })
        }
        loadPromise = Assets.load([dataAlias, atlasAlias]).then(() => {
          this.spineLoading.delete(cacheKey)
        })
        this.spineLoading.set(cacheKey, loadPromise)
      }
      await loadPromise
    }

    const spineAnim = Spine.from({ skeleton: dataAlias, atlas: atlasAlias, autoUpdate: true })

    if (layer.offset) {
      spineAnim.x = layer.offset[0]
      spineAnim.y = layer.offset[1]
    }
    if (resDef.offset) {
      spineAnim.x -= resDef.offset[0]
      spineAnim.y -= resDef.offset[1]
    }
    if (spineDef.scale != null) {
      spineAnim.scale = spineDef.scale
    }
    if (spineDef.skin) {
      spineAnim.skeleton.setSkinByName(spineDef.skin)
    }
    spineAnim.state.data.defaultMix = 0.25

    return spineAnim
  }

  /**
   * Reload a texture (cache-bust for hot-reload).
   */
  static async reloadTexture(imgPath: string): Promise<Texture> {
    const fullPath = '/assets/game/' + imgPath + '?' + Date.now()
    const tex = await Assets.load(fullPath)
    return tex
  }

  static clearCache(): void {
    this.textureCache.clear()
    clearAlphaMaskCache()
  }

  static getCacheStats(): { cached: number; loading: number } {
    return {
      cached: this.textureCache.size,
      loading: this.textureLoading.size,
    }
  }
}
