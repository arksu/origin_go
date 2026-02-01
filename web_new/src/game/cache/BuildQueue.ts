import type { BuildTask, CacheMetrics } from './types'
import {
  BUILD_TIME_BUDGET_MS,
  BUILD_QUEUE_MAX_LENGTH,
  MAX_IN_FLIGHT_BUILDS,
  BuildPriority,
} from './constants'

/**
 * Priority queue for chunk build tasks with time budget per frame.
 * Supports task cancellation via buildToken and deduplication.
 */
export class BuildQueue {
  private queue: BuildTask[] = []
  private tasksByKey: Map<string, BuildTask> = new Map()
  private currentBuildToken = 0
  private inFlightCount = 0

  // Metrics
  private _canceledCount = 0
  private _cpuBuildTimes: number[] = []
  private _gpuUploadTimes: number[] = []

  /**
   * Generate a new unique build token.
   */
  nextBuildToken(): number {
    return ++this.currentBuildToken
  }

  /**
   * Check if a build token is still valid (not superseded).
   */
  isTokenValid(token: number): boolean {
    return token === this.currentBuildToken
  }

  /**
   * Enqueue a build task. Replaces existing task for same chunk.
   */
  enqueue(task: BuildTask): void {
    // Remove existing task for this chunk
    const existing = this.tasksByKey.get(task.chunkKey)
    if (existing) {
      this.remove(existing.chunkKey)
      this._canceledCount++
    }

    // Drop if queue is full and this is low priority
    if (this.queue.length >= BUILD_QUEUE_MAX_LENGTH) {
      if (task.priority >= BuildPriority.P2_DISTANT) {
        return // Drop distant chunks when queue is full
      }
      // Remove lowest priority task
      const lowestPriority = this.queue[this.queue.length - 1]
      if (lowestPriority && lowestPriority.priority > task.priority) {
        this.remove(lowestPriority.chunkKey)
      } else {
        return // Can't fit this task
      }
    }

    this.tasksByKey.set(task.chunkKey, task)
    this.insertSorted(task)
  }

  /**
   * Insert task in sorted position (by priority, then distance).
   */
  private insertSorted(task: BuildTask): void {
    let insertIdx = this.queue.length
    for (let i = 0; i < this.queue.length; i++) {
      const other = this.queue[i]!
      if (task.priority < other.priority ||
          (task.priority === other.priority && task.distanceToCamera < other.distanceToCamera)) {
        insertIdx = i
        break
      }
    }
    this.queue.splice(insertIdx, 0, task)
  }

  /**
   * Remove a task by chunk key.
   */
  remove(chunkKey: string): boolean {
    const task = this.tasksByKey.get(chunkKey)
    if (!task) return false

    this.tasksByKey.delete(chunkKey)
    const idx = this.queue.indexOf(task)
    if (idx >= 0) {
      this.queue.splice(idx, 1)
    }
    return true
  }

  /**
   * Cancel all tasks for a chunk (e.g., on unload).
   */
  cancel(chunkKey: string): void {
    if (this.remove(chunkKey)) {
      this._canceledCount++
    }
  }

  /**
   * Get next task to process, respecting in-flight limit.
   */
  peek(): BuildTask | undefined {
    if (this.inFlightCount >= MAX_IN_FLIGHT_BUILDS) {
      return undefined
    }
    return this.queue[0]
  }

  /**
   * Dequeue the next task for processing.
   */
  dequeue(): BuildTask | undefined {
    if (this.inFlightCount >= MAX_IN_FLIGHT_BUILDS) {
      return undefined
    }
    const task = this.queue.shift()
    if (task) {
      this.tasksByKey.delete(task.chunkKey)
      this.inFlightCount++
    }
    return task
  }

  /**
   * Mark a build as complete (decrements in-flight count).
   */
  buildComplete(): void {
    if (this.inFlightCount > 0) {
      this.inFlightCount--
    }
  }

  /**
   * Record CPU build time for metrics.
   */
  recordCpuBuildTime(ms: number): void {
    this._cpuBuildTimes.push(ms)
    if (this._cpuBuildTimes.length > 100) {
      this._cpuBuildTimes.shift()
    }
  }

  /**
   * Record GPU upload time for metrics.
   */
  recordGpuUploadTime(ms: number): void {
    this._gpuUploadTimes.push(ms)
    if (this._gpuUploadTimes.length > 100) {
      this._gpuUploadTimes.shift()
    }
  }

  /**
   * Process tasks within time budget. Returns tasks to execute this frame.
   * @param budgetMs Time budget in milliseconds (default from constants)
   */
  getTasksForFrame(budgetMs: number = BUILD_TIME_BUDGET_MS): BuildTask[] {
    const tasks: BuildTask[] = []
    let estimatedTime = 0
    const avgBuildTime = this.getAvgCpuBuildTime()

    while (this.queue.length > 0 && this.inFlightCount < MAX_IN_FLIGHT_BUILDS) {
      const nextTask = this.queue[0]
      if (!nextTask) break

      // P0 tasks always get processed
      if (nextTask.priority === BuildPriority.P0_VISIBLE) {
        tasks.push(this.dequeue()!)
        continue
      }

      // For other priorities, check budget
      if (estimatedTime + avgBuildTime > budgetMs) {
        break
      }

      tasks.push(this.dequeue()!)
      estimatedTime += avgBuildTime
    }

    return tasks
  }

  /**
   * Check if there are pending tasks.
   */
  hasPendingTasks(): boolean {
    return this.queue.length > 0
  }

  /**
   * Get queue length.
   */
  get length(): number {
    return this.queue.length
  }

  /**
   * Get average CPU build time.
   */
  private getAvgCpuBuildTime(): number {
    if (this._cpuBuildTimes.length === 0) return 1 // Default estimate
    const sum = this._cpuBuildTimes.reduce((a, b) => a + b, 0)
    return sum / this._cpuBuildTimes.length
  }

  /**
   * Get average GPU upload time.
   */
  private getAvgGpuUploadTime(): number {
    if (this._gpuUploadTimes.length === 0) return 0.5 // Default estimate
    const sum = this._gpuUploadTimes.reduce((a, b) => a + b, 0)
    return sum / this._gpuUploadTimes.length
  }

  /**
   * Get metrics for debugging.
   */
  getMetrics(): Partial<CacheMetrics> {
    return {
      buildQueueLength: this.queue.length,
      canceledBuilds: this._canceledCount,
      cpuBuildMsAvg: this.getAvgCpuBuildTime(),
      gpuUploadMsAvg: this.getAvgGpuUploadTime(),
    }
  }

  /**
   * Clear all tasks.
   */
  clear(): void {
    this.queue = []
    this.tasksByKey.clear()
    this.inFlightCount = 0
  }

  /**
   * Reset metrics.
   */
  resetMetrics(): void {
    this._canceledCount = 0
    this._cpuBuildTimes = []
    this._gpuUploadTimes = []
  }
}

export const buildQueue = new BuildQueue()
