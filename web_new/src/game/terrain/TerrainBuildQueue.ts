import type { TerrainBuildTask } from './TerrainSubchunkTypes'
import { TERRAIN_BUILD_BUDGET_MS, MAX_TERRAIN_SUBCHUNKS_PER_FRAME } from './constants'

/**
 * Priority queue for terrain subchunk build tasks with frame budget.
 */
export class TerrainBuildQueue {
  private queue: TerrainBuildTask[] = []
  private buildTimes: number[] = []

  // Metrics
  private _canceledBuilds = 0

  /**
   * Enqueue a build task. Sorted by distance to camera (nearest first).
   */
  enqueue(task: TerrainBuildTask): void {
    // Remove existing task for same subchunk
    this.cancel(task.subchunkKey)

    // Insert sorted by distance
    let inserted = false
    for (let i = 0; i < this.queue.length; i++) {
      if (task.distanceToCamera < this.queue[i]!.distanceToCamera) {
        this.queue.splice(i, 0, task)
        inserted = true
        break
      }
    }
    if (!inserted) {
      this.queue.push(task)
    }
  }

  /**
   * Cancel a pending build task.
   */
  cancel(subchunkKey: string): boolean {
    const idx = this.queue.findIndex(t => t.subchunkKey === subchunkKey)
    if (idx >= 0) {
      this.queue.splice(idx, 1)
      this._canceledBuilds++
      return true
    }
    return false
  }

  /**
   * Cancel all tasks for a chunk.
   */
  cancelChunk(chunkKey: string): number {
    const prefix = chunkKey + ':'
    let canceled = 0
    this.queue = this.queue.filter(t => {
      if (t.subchunkKey.startsWith(prefix)) {
        canceled++
        this._canceledBuilds++
        return false
      }
      return true
    })
    return canceled
  }

  /**
   * Get tasks to process this frame within budget.
   */
  getTasksForFrame(): TerrainBuildTask[] {
    const tasks: TerrainBuildTask[] = []
    const avgBuildTime = this.getAvgBuildTime()
    const budget = TERRAIN_BUILD_BUDGET_MS
    let estimatedTime = 0

    while (
      this.queue.length > 0 &&
      tasks.length < MAX_TERRAIN_SUBCHUNKS_PER_FRAME &&
      estimatedTime + avgBuildTime <= budget
    ) {
      const task = this.queue.shift()!
      tasks.push(task)
      estimatedTime += avgBuildTime
    }

    return tasks
  }

  /**
   * Record build time for adaptive budget.
   */
  recordBuildTime(ms: number): void {
    this.buildTimes.push(ms)
    if (this.buildTimes.length > 50) {
      this.buildTimes.shift()
    }
  }

  private getAvgBuildTime(): number {
    if (this.buildTimes.length === 0) return 0.5 // Default estimate
    const sum = this.buildTimes.reduce((a, b) => a + b, 0)
    return sum / this.buildTimes.length
  }

  getP95BuildTime(): number {
    if (this.buildTimes.length === 0) return 0
    const sorted = [...this.buildTimes].sort((a, b) => a - b)
    const idx = Math.floor(sorted.length * 0.95)
    return sorted[idx] ?? sorted[sorted.length - 1] ?? 0
  }

  /**
   * Get queue length.
   */
  get length(): number {
    return this.queue.length
  }

  /**
   * Clear all pending tasks.
   */
  clear(): void {
    this._canceledBuilds += this.queue.length
    this.queue = []
  }

  getMetrics() {
    return {
      queueLength: this.queue.length,
      canceledBuilds: this._canceledBuilds,
      avgBuildTimeMs: this.getAvgBuildTime(),
      p95BuildTimeMs: this.getP95BuildTime(),
    }
  }

  resetMetrics(): void {
    this._canceledBuilds = 0
    this.buildTimes = []
  }
}

export const terrainBuildQueue = new TerrainBuildQueue()
