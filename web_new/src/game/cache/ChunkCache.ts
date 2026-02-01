import type { CachedChunk, CacheMetrics, SubchunkGpuResources } from './types'
import { CACHE_MAX_ENTRIES, CACHE_TTL_MS, CACHE_SWEEP_INTERVAL_MS } from './constants'

/**
 * LRU cache for chunk data with TTL support.
 * Stores tiles, CPU geometry, and optionally GPU resources.
 */
export class ChunkCache {
  private cache: Map<string, CachedChunk> = new Map()
  private sweepIntervalId: ReturnType<typeof setInterval> | null = null

  // Metrics
  private _hits = 0
  private _misses = 0
  private _evictionsLru = 0
  private _evictionsTtl = 0
  private _evictionsVersionMismatch = 0

  constructor() {
    this.startSweep()
  }

  /**
   * Get a cached chunk by key. Updates lastUsedAt on hit.
   */
  get(key: string): CachedChunk | undefined {
    const entry = this.cache.get(key)
    if (entry) {
      entry.lastUsedAt = performance.now()
      this._hits++
      // Move to end for LRU (Map maintains insertion order)
      this.cache.delete(key)
      this.cache.set(key, entry)
      return entry
    }
    this._misses++
    return undefined
  }

  /**
   * Check if cache has entry with matching version.
   */
  hasValidEntry(key: string, version: number): boolean {
    const entry = this.cache.get(key)
    if (!entry) return false
    if (entry.version !== version) {
      this._evictionsVersionMismatch++
      this.evict(key)
      return false
    }
    return true
  }

  /**
   * Store a chunk in cache. Evicts LRU entries if at capacity.
   */
  set(chunk: CachedChunk): void {
    // If already exists, remove first to update position
    if (this.cache.has(chunk.key)) {
      this.cache.delete(chunk.key)
    }

    // Evict LRU entries if at capacity
    while (this.cache.size >= CACHE_MAX_ENTRIES) {
      const oldestKey = this.cache.keys().next().value
      if (oldestKey) {
        this.evict(oldestKey)
        this._evictionsLru++
      }
    }

    this.cache.set(chunk.key, chunk)
  }

  /**
   * Remove a chunk from cache and destroy its GPU resources.
   */
  evict(key: string): void {
    const entry = this.cache.get(key)
    if (!entry) return

    // Destroy GPU resources if present
    if (entry.gpu) {
      this.destroyGpuResources(entry.gpu)
      entry.gpu = undefined
    }

    this.cache.delete(key)
  }

  /**
   * Clear all entries and destroy all GPU resources.
   */
  clear(): void {
    for (const entry of this.cache.values()) {
      if (entry.gpu) {
        this.destroyGpuResources(entry.gpu)
      }
    }
    this.cache.clear()
  }

  /**
   * Destroy GPU resources for a chunk.
   */
  private destroyGpuResources(gpu: Map<string, SubchunkGpuResources>): void {
    for (const resources of gpu.values()) {
      resources.geometry.destroy()
      resources.container.destroy({ children: true })
    }
    gpu.clear()
  }

  /**
   * Start periodic TTL sweep.
   */
  private startSweep(): void {
    if (this.sweepIntervalId) return
    this.sweepIntervalId = setInterval(() => this.sweep(), CACHE_SWEEP_INTERVAL_MS)
  }

  /**
   * Stop periodic TTL sweep.
   */
  stopSweep(): void {
    if (this.sweepIntervalId) {
      clearInterval(this.sweepIntervalId)
      this.sweepIntervalId = null
    }
  }

  /**
   * Remove entries older than TTL.
   */
  private sweep(): void {
    const now = performance.now()
    const keysToEvict: string[] = []

    for (const [key, entry] of this.cache) {
      if (now - entry.lastUsedAt > CACHE_TTL_MS) {
        keysToEvict.push(key)
      }
    }

    for (const key of keysToEvict) {
      this.evict(key)
      this._evictionsTtl++
    }
  }

  /**
   * Get total bytes used by cache.
   */
  getTotalBytes(): number {
    let total = 0
    for (const entry of this.cache.values()) {
      total += entry.tilesBytes + entry.cpuBytes + entry.gpuBytes
    }
    return total
  }

  /**
   * Get cache metrics for debugging.
   */
  getMetrics(): Partial<CacheMetrics> {
    const total = this._hits + this._misses
    let bytesTiles = 0
    let bytesCpu = 0
    let bytesGpu = 0

    for (const entry of this.cache.values()) {
      bytesTiles += entry.tilesBytes
      bytesCpu += entry.cpuBytes
      bytesGpu += entry.gpuBytes
    }

    return {
      entries: this.cache.size,
      hits: this._hits,
      misses: this._misses,
      hitRate: total > 0 ? this._hits / total : 0,
      bytesTotal: bytesTiles + bytesCpu + bytesGpu,
      bytesTiles,
      bytesCpu,
      bytesGpu,
      evictionsLru: this._evictionsLru,
      evictionsTtl: this._evictionsTtl,
      evictionsVersionMismatch: this._evictionsVersionMismatch,
    }
  }

  /**
   * Reset metrics counters.
   */
  resetMetrics(): void {
    this._hits = 0
    this._misses = 0
    this._evictionsLru = 0
    this._evictionsTtl = 0
    this._evictionsVersionMismatch = 0
  }

  /**
   * Get all cached chunk keys.
   */
  keys(): IterableIterator<string> {
    return this.cache.keys()
  }

  /**
   * Get cache size.
   */
  get size(): number {
    return this.cache.size
  }

  /**
   * Destroy cache and cleanup.
   */
  destroy(): void {
    this.stopSweep()
    this.clear()
  }
}

export const chunkCache = new ChunkCache()
