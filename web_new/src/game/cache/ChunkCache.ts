import type { CachedChunk, CacheMetrics } from './types'
import { CACHE_MAX_ENTRIES, CACHE_TTL_MS, CACHE_SWEEP_INTERVAL_MS } from '@/constants/cache'

/** Metadata for ready Chunk instances; ChunkManager owns and disposes their graphics. */
export class ChunkCache {
  private cache = new Map<string, CachedChunk>()
  private hidden = new Map<string, number>()
  private sweepIntervalId: ReturnType<typeof setInterval> | null = null
  private dispose: (key: string) => void = () => {}
  private hits = 0
  private misses = 0
  private evictionsLru = 0
  private evictionsTtl = 0

  setDisposer(dispose: (key: string) => void): void {
    this.dispose = dispose
    if (!this.sweepIntervalId) {
      this.sweepIntervalId = setInterval(() => this.sweep(), CACHE_SWEEP_INTERVAL_MS)
    }
  }

  get(key: string): CachedChunk | undefined {
    const entry = this.cache.get(key)
    if (entry) this.hits++
    else this.misses++
    return entry
  }

  // Neighbor checks must not extend hidden retention or change LRU order.
  peek(key: string): CachedChunk | undefined { return this.cache.get(key) }

  set(chunk: CachedChunk): void {
    this.cache.set(chunk.key, chunk)
    this.markActive(chunk.key)
  }

  markActive(key: string): void {
    const entry = this.cache.get(key)
    if (entry) entry.retainedAt = null
    this.hidden.delete(key)
  }

  retain(key: string): void {
    const entry = this.cache.get(key)
    if (!entry || this.hidden.has(key)) return
    entry.retainedAt = performance.now()
    this.hidden.set(key, entry.retainedAt)
    while (this.hidden.size > CACHE_MAX_ENTRIES) {
      const oldest = this.hidden.keys().next().value
      if (oldest == null) break
      this.evictionsLru++
      this.evict(oldest)
    }
  }

  // Used by the owner's single disposal path and before replacing a built version.
  forget(key: string): void {
    this.cache.delete(key)
    this.hidden.delete(key)
  }

  evict(key: string): void {
    if (!this.cache.has(key)) return
    this.forget(key)
    this.dispose(key)
  }

  clear(): void {
    for (const key of [...this.cache.keys()]) this.evict(key)
  }

  sweep(now = performance.now()): void {
    for (const [key, retainedAt] of [...this.hidden]) {
      if (now - retainedAt >= CACHE_TTL_MS) {
        this.evictionsTtl++
        this.evict(key)
      }
    }
  }

  getMetrics(): Partial<CacheMetrics> {
    let bytesTiles = 0, bytesCpu = 0, bytesGpu = 0
    for (const entry of this.cache.values()) {
      bytesTiles += entry.tilesBytes
      bytesCpu += entry.cpuBytes
      bytesGpu += entry.gpuBytes
    }
    const total = this.hits + this.misses
    return {
      entries: this.cache.size, hits: this.hits, misses: this.misses,
      hitRate: total ? this.hits / total : 0,
      bytesTotal: bytesTiles + bytesCpu + bytesGpu, bytesTiles, bytesCpu, bytesGpu,
      evictionsLru: this.evictionsLru, evictionsTtl: this.evictionsTtl, evictionsVersionMismatch: 0,
    }
  }

  getTotalBytes(): number { return this.getMetrics().bytesTotal ?? 0 }
  keys(): IterableIterator<string> { return this.cache.keys() }
  get size(): number { return this.cache.size }

  resetMetrics(): void {
    this.hits = this.misses = this.evictionsLru = this.evictionsTtl = 0
  }

  stopSweep(): void {
    if (this.sweepIntervalId) clearInterval(this.sweepIntervalId)
    this.sweepIntervalId = null
  }

  destroy(): void {
    this.stopSweep()
    this.clear()
    this.dispose = () => {}
  }
}

export const chunkCache = new ChunkCache()
