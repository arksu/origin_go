import type { CacheMetrics } from './types'
import { chunkCache } from './ChunkCache'
import { buildQueue } from './BuildQueue'

/**
 * Collects and aggregates metrics from cache and build queue.
 */
export class CacheMetricsCollector {
  // Border refresh metrics
  private _borderRefreshCount = 0
  private _borderRefreshTimes: number[] = []

  /**
   * Record a border refresh operation.
   */
  recordBorderRefresh(ms: number): void {
    this._borderRefreshCount++
    this._borderRefreshTimes.push(ms)
    if (this._borderRefreshTimes.length > 100) {
      this._borderRefreshTimes.shift()
    }
  }

  /**
   * Get average border refresh time.
   */
  private getAvgBorderRefreshTime(): number {
    if (this._borderRefreshTimes.length === 0) return 0
    const sum = this._borderRefreshTimes.reduce((a, b) => a + b, 0)
    return sum / this._borderRefreshTimes.length
  }

  /**
   * Get combined metrics from all sources.
   */
  getMetrics(): CacheMetrics {
    const cacheMetrics = chunkCache.getMetrics()
    const buildMetrics = buildQueue.getMetrics()

    return {
      // Cache stats
      entries: cacheMetrics.entries ?? 0,
      hits: cacheMetrics.hits ?? 0,
      misses: cacheMetrics.misses ?? 0,
      hitRate: cacheMetrics.hitRate ?? 0,
      bytesTotal: cacheMetrics.bytesTotal ?? 0,
      bytesCpu: cacheMetrics.bytesCpu ?? 0,
      bytesGpu: cacheMetrics.bytesGpu ?? 0,
      bytesTiles: cacheMetrics.bytesTiles ?? 0,

      // Eviction stats
      evictionsLru: cacheMetrics.evictionsLru ?? 0,
      evictionsTtl: cacheMetrics.evictionsTtl ?? 0,
      evictionsVersionMismatch: cacheMetrics.evictionsVersionMismatch ?? 0,

      // Build stats
      buildQueueLength: buildMetrics.buildQueueLength ?? 0,
      canceledBuilds: buildMetrics.canceledBuilds ?? 0,
      cpuBuildMsAvg: buildMetrics.cpuBuildMsAvg ?? 0,
      gpuUploadMsAvg: buildMetrics.gpuUploadMsAvg ?? 0,

      // Border refresh stats
      borderRefreshCount: this._borderRefreshCount,
      borderRefreshMsAvg: this.getAvgBorderRefreshTime(),
    }
  }

  /**
   * Reset all metrics.
   */
  resetMetrics(): void {
    chunkCache.resetMetrics()
    buildQueue.resetMetrics()
    this._borderRefreshCount = 0
    this._borderRefreshTimes = []
  }

  /**
   * Format metrics for debug overlay.
   */
  formatForOverlay(): string[] {
    const m = this.getMetrics()
    const bytesKb = (m.bytesTotal / 1024).toFixed(1)
    const hitRatePct = (m.hitRate * 100).toFixed(1)

    return [
      `Cache: ${m.entries} entries, ${bytesKb}KB`,
      `Hit rate: ${hitRatePct}% (${m.hits}/${m.hits + m.misses})`,
      `Build queue: ${m.buildQueueLength}, canceled: ${m.canceledBuilds}`,
      `Build avg: ${m.cpuBuildMsAvg.toFixed(2)}ms CPU, ${m.gpuUploadMsAvg.toFixed(2)}ms GPU`,
      `Border refresh: ${m.borderRefreshCount}, avg ${m.borderRefreshMsAvg.toFixed(2)}ms`,
    ]
  }
}

export const cacheMetrics = new CacheMetricsCollector()
