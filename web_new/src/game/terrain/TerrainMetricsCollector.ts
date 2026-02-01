import type { TerrainMetrics } from './TerrainSubchunkTypes'
import { terrainSpritePool } from './TerrainSpritePool'
import { terrainBuildQueue } from './TerrainBuildQueue'

/**
 * Collects terrain-specific metrics for debugging.
 */
export class TerrainMetricsCollector {
  private _spritesActive = 0
  private _subchunksDone = 0
  private _subchunksCanceled = 0
  private _clearTimes: number[] = []

  recordSubchunkBuilt(): void {
    this._subchunksDone++
  }

  recordSubchunkCanceled(): void {
    this._subchunksCanceled++
  }

  recordClearTime(ms: number): void {
    this._clearTimes.push(ms)
    if (this._clearTimes.length > 50) {
      this._clearTimes.shift()
    }
  }

  setActiveSprites(count: number): void {
    this._spritesActive = count
  }

  private getAvgClearTime(): number {
    if (this._clearTimes.length === 0) return 0
    const sum = this._clearTimes.reduce((a, b) => a + b, 0)
    return sum / this._clearTimes.length
  }

  private getP95ClearTime(): number {
    if (this._clearTimes.length === 0) return 0
    const sorted = [...this._clearTimes].sort((a, b) => a - b)
    const idx = Math.floor(sorted.length * 0.95)
    return sorted[idx] ?? sorted[sorted.length - 1] ?? 0
  }

  getMetrics(): TerrainMetrics {
    const poolMetrics = terrainSpritePool.getMetrics()
    const queueMetrics = terrainBuildQueue.getMetrics()

    return {
      spritesActive: this._spritesActive,
      spritesPooled: poolMetrics.pooled,
      spritesCreatedTotal: poolMetrics.createdTotal,
      subchunksQueued: queueMetrics.queueLength,
      subchunksDone: this._subchunksDone,
      subchunksCanceled: this._subchunksCanceled,
      buildMsAvg: queueMetrics.avgBuildTimeMs,
      buildMsP95: queueMetrics.p95BuildTimeMs,
      clearMsAvg: this.getAvgClearTime(),
      clearMsP95: this.getP95ClearTime(),
    }
  }

  formatForOverlay(): string[] {
    const m = this.getMetrics()
    return [
      `Terrain sprites: ${m.spritesActive} active, ${m.spritesPooled} pooled, ${m.spritesCreatedTotal} total`,
      `Terrain build: ${m.subchunksQueued} queued, ${m.subchunksDone} done, ${m.subchunksCanceled} canceled`,
      `Terrain build time: avg ${m.buildMsAvg.toFixed(2)}ms, p95 ${m.buildMsP95.toFixed(2)}ms`,
      `Terrain clear time: avg ${m.clearMsAvg.toFixed(2)}ms, p95 ${m.clearMsP95.toFixed(2)}ms`,
    ]
  }

  resetMetrics(): void {
    this._subchunksDone = 0
    this._subchunksCanceled = 0
    this._clearTimes = []
    terrainSpritePool.resetMetrics()
    terrainBuildQueue.resetMetrics()
  }
}

export const terrainMetrics = new TerrainMetricsCollector()
