import { Assets, Texture } from 'pixi.js'

/**
 * ResourceLoader manages loading and caching of game object resources.
 * 
 * Future extensions:
 * - Support for sprite sheets and animations
 * - Multi-layer objects (shadows, overlays)
 * - Resource preloading and bundling
 * - Fallback textures for missing resources
 */
export class ResourceLoader {
  private static cache = new Map<string, Texture>()
  private static loading = new Map<string, Promise<Texture>>()
  private static placeholderTexture: Texture | null = null

  /**
   * Load a texture from the given resource path.
   * Returns cached texture if already loaded.
   * Returns placeholder if path is empty or loading fails.
   */
  static async loadTexture(resourcePath: string): Promise<Texture> {
    // Empty path - return placeholder
    if (!resourcePath || resourcePath.trim() === '') {
      return this.getPlaceholder()
    }

    // Check cache
    const cached = this.cache.get(resourcePath)
    if (cached) {
      return cached
    }

    // Check if already loading
    const loading = this.loading.get(resourcePath)
    if (loading) {
      return loading
    }

    // Start loading
    const loadPromise = this.loadTextureInternal(resourcePath)
    this.loading.set(resourcePath, loadPromise)

    try {
      const texture = await loadPromise
      this.cache.set(resourcePath, texture)
      return texture
    } catch (error) {
      console.warn(`[ResourceLoader] Failed to load texture: ${resourcePath}`, error)
      return this.getPlaceholder()
    } finally {
      this.loading.delete(resourcePath)
    }
  }

  private static async loadTextureInternal(resourcePath: string): Promise<Texture> {
    // Future: support different resource types (sprite sheets, animations, etc.)
    // For now, just load as a simple texture
    const texture = await Assets.load(resourcePath)
    return texture
  }

  /**
   * Get or create a placeholder texture for objects without resources.
   */
  private static getPlaceholder(): Texture {
    if (!this.placeholderTexture) {
      // Create a simple colored rectangle as placeholder
      // Future: load from a dedicated placeholder asset
      this.placeholderTexture = Texture.WHITE
    }
    return this.placeholderTexture
  }

  /**
   * Preload multiple resources at once.
   * Useful for loading resources for visible chunks.
   */
  static async preloadResources(resourcePaths: string[]): Promise<void> {
    const validPaths = resourcePaths.filter(path => path && path.trim() !== '')
    if (validPaths.length === 0) return

    await Promise.all(validPaths.map(path => this.loadTexture(path)))
  }

  /**
   * Clear cached resources to free memory.
   * Can specify paths to clear, or clear all if not specified.
   */
  static clearCache(resourcePaths?: string[]): void {
    if (resourcePaths) {
      resourcePaths.forEach(path => this.cache.delete(path))
    } else {
      this.cache.clear()
    }
  }

  /**
   * Get cache statistics for debugging.
   */
  static getCacheStats(): { cached: number; loading: number } {
    return {
      cached: this.cache.size,
      loading: this.loading.size,
    }
  }
}
