/**
 * MoveController - manages smooth movement interpolation for game entities.
 * 
 * Features:
 * - Keyframe buffer per entity for time-based interpolation
 * - Handles out-of-order packets via move_seq
 * - Stream epoch validation to prevent race conditions
 * - Bounded extrapolation for brief network gaps
 * - Snap/teleport handling
 * - Error correction with smooth damping
 */

import { timeSync } from '@/network/TimeSync'

// Constants
const MAX_KEYFRAMES = 32
const MAX_EXTRAPOLATION_MS = 180
const SNAP_DISTANCE_SQUARED = 2400 * 2400 // ~0.75 tiles at 32 coord/tile -> (0.75 * 32 * 100)^2
const ERROR_CORRECTION_SPEED = 0.15 // per-frame lerp factor for small corrections
const VELOCITY_DECAY_RATE = 0.9 // decay rate when extrapolating past max time

export interface MoveKeyframe {
  tServerMs: number
  x: number
  y: number
  vx: number
  vy: number
  isMoving: boolean
  moveMode: number
  heading: number
  moveSeq: number
}

export interface RenderPosition {
  x: number
  y: number
  heading: number
  isMoving: boolean
  moveMode: number
}

export interface EntityMoveState {
  entityId: number
  streamEpoch: number
  keyframes: MoveKeyframe[]
  lastMoveSeq: number

  // Current visual position (smoothed)
  visualX: number
  visualY: number
  visualHeading: number

  // Interpolation state
  isExtrapolating: boolean
  extrapolationStartMs: number

  // Debug metrics
  ignoredOutOfOrder: number
  snapCount: number
  bufferUnderrunCount: number
}

export interface MoveDebugMetrics {
  entityId: number
  bufferSize: number
  lastMoveSeq: number
  isExtrapolating: boolean
  ignoredOutOfOrder: number
  snapCount: number
  bufferUnderrunCount: number
  visualX: number
  visualY: number
}

class MoveController {
  private entities: Map<number, EntityMoveState> = new Map()
  private globalStreamEpoch = 0

  /**
   * Set the global stream epoch (from S2C_PlayerEnterWorld).
   */
  setStreamEpoch(epoch: number): void {
    this.globalStreamEpoch = epoch
  }

  /**
   * Initialize or reset an entity's movement state.
   */
  initEntity(entityId: number, x: number, y: number, heading: number = 0): void {
    this.entities.set(entityId, {
      entityId,
      streamEpoch: this.globalStreamEpoch,
      keyframes: [],
      lastMoveSeq: 0,
      visualX: x,
      visualY: y,
      visualHeading: heading,
      isExtrapolating: false,
      extrapolationStartMs: 0,
      ignoredOutOfOrder: 0,
      snapCount: 0,
      bufferUnderrunCount: 0,
    })
  }

  /**
   * Remove an entity from tracking.
   */
  removeEntity(entityId: number): void {
    this.entities.delete(entityId)
  }

  /**
   * Process incoming S2C_ObjectMove message.
   */
  onObjectMove(
    entityId: number,
    serverTimeMs: number,
    moveSeq: number,
    streamEpoch: number,
    isTeleport: boolean,
    x: number,
    y: number,
    vx: number,
    vy: number,
    isMoving: boolean,
    moveMode: number,
    heading: number,
  ): void {
    let state = this.entities.get(entityId)

    // If entity not tracked, initialize it
    if (!state) {
      this.initEntity(entityId, x, y, heading)
      state = this.entities.get(entityId)!
    }

    // Validate stream epoch
    if (streamEpoch !== this.globalStreamEpoch) {
      // Epoch mismatch - ignore this packet
      return
    }

    // Handle teleport - snap and reset buffer
    if (isTeleport) {
      state.keyframes = []
      state.visualX = x
      state.visualY = y
      state.visualHeading = heading
      state.lastMoveSeq = moveSeq
      state.isExtrapolating = false
      state.snapCount++

      // Add initial keyframe
      state.keyframes.push({
        tServerMs: serverTimeMs,
        x, y, vx, vy,
        isMoving, moveMode, heading, moveSeq,
      })
      return
    }

    // Check move_seq for out-of-order detection
    // Simple comparison - assumes seq doesn't wrap frequently
    if (moveSeq <= state.lastMoveSeq && state.lastMoveSeq - moveSeq < 1000) {
      state.ignoredOutOfOrder++
      return
    }

    state.lastMoveSeq = moveSeq
    state.isExtrapolating = false

    // Add keyframe to buffer
    const keyframe: MoveKeyframe = {
      tServerMs: serverTimeMs,
      x, y, vx, vy,
      isMoving, moveMode, heading, moveSeq,
    }

    state.keyframes.push(keyframe)

    // Trim buffer if too large
    while (state.keyframes.length > MAX_KEYFRAMES) {
      state.keyframes.shift()
    }

    // Sort by server time (should already be sorted, but safety)
    state.keyframes.sort((a, b) => a.tServerMs - b.tServerMs)
  }

  /**
   * Update all entities for the current frame.
   * Returns map of entityId -> RenderPosition.
   */
  update(): Map<number, RenderPosition> {
    const result = new Map<number, RenderPosition>()
    const clientNowMs = Date.now()
    const serverNowMs = timeSync.estimateServerNowMs(clientNowMs)
    const interpolationDelayMs = timeSync.getInterpolationDelayMs()
    const renderTimeMs = serverNowMs - interpolationDelayMs

    for (const [entityId, state] of this.entities) {
      const pos = this.interpolateEntity(state, renderTimeMs, clientNowMs)
      result.set(entityId, pos)
    }

    return result
  }

  /**
   * Get render position for a specific entity.
   */
  getRenderPosition(entityId: number): RenderPosition | null {
    const state = this.entities.get(entityId)
    if (!state) return null

    const clientNowMs = Date.now()
    const serverNowMs = timeSync.estimateServerNowMs(clientNowMs)
    const interpolationDelayMs = timeSync.getInterpolationDelayMs()
    const renderTimeMs = serverNowMs - interpolationDelayMs

    return this.interpolateEntity(state, renderTimeMs, clientNowMs)
  }

  private interpolateEntity(state: EntityMoveState, renderTimeMs: number, clientNowMs: number): RenderPosition {
    const keyframes = state.keyframes

    // No keyframes - return current visual position
    if (keyframes.length === 0) {
      return {
        x: state.visualX,
        y: state.visualY,
        heading: state.visualHeading,
        isMoving: false,
        moveMode: 0,
      }
    }

    // Find keyframes A and B for interpolation
    let frameA: MoveKeyframe | undefined = undefined
    let frameB: MoveKeyframe | undefined = undefined

    for (let i = 0; i < keyframes.length - 1; i++) {
      const kfA = keyframes[i]
      const kfB = keyframes[i + 1]
      if (kfA && kfB && kfA.tServerMs <= renderTimeMs && kfB.tServerMs >= renderTimeMs) {
        frameA = kfA
        frameB = kfB
        break
      }
    }

    let targetX: number
    let targetY: number
    let heading: number
    let isMoving: boolean
    let moveMode: number

    if (frameA && frameB) {
      // Normal interpolation between two keyframes
      const duration = frameB.tServerMs - frameA.tServerMs
      const alpha = duration > 0 ? (renderTimeMs - frameA.tServerMs) / duration : 0
      const clampedAlpha = Math.max(0, Math.min(1, alpha))

      targetX = frameA.x + (frameB.x - frameA.x) * clampedAlpha
      targetY = frameA.y + (frameB.y - frameA.y) * clampedAlpha
      heading = frameB.heading
      isMoving = frameB.isMoving
      moveMode = frameB.moveMode
      state.isExtrapolating = false
    } else if (keyframes[0] && renderTimeMs < keyframes[0].tServerMs) {
      // Before first keyframe - use first keyframe position
      const first = keyframes[0]
      targetX = first.x
      targetY = first.y
      heading = first.heading
      isMoving = first.isMoving
      moveMode = first.moveMode
      state.isExtrapolating = false
    } else if (keyframes.length > 0) {
      // After last keyframe - extrapolation
      const last = keyframes[keyframes.length - 1]!
      const timePastLast = renderTimeMs - last.tServerMs

      if (!state.isExtrapolating) {
        state.isExtrapolating = true
        state.extrapolationStartMs = clientNowMs
        state.bufferUnderrunCount++
      }

      if (timePastLast <= MAX_EXTRAPOLATION_MS && last.isMoving) {
        // Bounded extrapolation using velocity
        const extrapolationTime = timePastLast / 1000 // convert to seconds
        targetX = last.x + last.vx * extrapolationTime
        targetY = last.y + last.vy * extrapolationTime
      } else {
        // Beyond max extrapolation or stopped - decay velocity
        const extrapolationTime = Math.min(timePastLast, MAX_EXTRAPOLATION_MS) / 1000
        const decayFactor = Math.pow(VELOCITY_DECAY_RATE, (timePastLast - MAX_EXTRAPOLATION_MS) / 100)
        targetX = last.x + last.vx * extrapolationTime * (last.isMoving ? decayFactor : 0)
        targetY = last.y + last.vy * extrapolationTime * (last.isMoving ? decayFactor : 0)
      }

      heading = last.heading
      isMoving = last.isMoving && timePastLast <= MAX_EXTRAPOLATION_MS
      moveMode = last.moveMode
    } else {
      // No keyframes at all - use current visual position
      return {
        x: state.visualX,
        y: state.visualY,
        heading: state.visualHeading,
        isMoving: false,
        moveMode: 0,
      }
    }

    // Apply error correction (smooth damping)
    const errorX = targetX - state.visualX
    const errorY = targetY - state.visualY
    const errorDistSq = errorX * errorX + errorY * errorY

    if (errorDistSq > SNAP_DISTANCE_SQUARED) {
      // Large error - snap immediately
      state.visualX = targetX
      state.visualY = targetY
      state.snapCount++
    } else {
      // Small error - smooth correction
      state.visualX += errorX * ERROR_CORRECTION_SPEED
      state.visualY += errorY * ERROR_CORRECTION_SPEED
    }

    state.visualHeading = heading

    // Prune old keyframes (keep only those potentially needed)
    const pruneThreshold = renderTimeMs - 500 // keep 500ms of history
    while (keyframes.length > 2 && keyframes[0] && keyframes[0].tServerMs < pruneThreshold) {
      keyframes.shift()
    }

    return {
      x: state.visualX,
      y: state.visualY,
      heading: state.visualHeading,
      isMoving,
      moveMode,
    }
  }

  /**
   * Check if an entity is being tracked.
   */
  hasEntity(entityId: number): boolean {
    return this.entities.has(entityId)
  }

  /**
   * Get debug metrics for an entity.
   */
  getEntityDebugMetrics(entityId: number): MoveDebugMetrics | null {
    const state = this.entities.get(entityId)
    if (!state) return null

    return {
      entityId,
      bufferSize: state.keyframes.length,
      lastMoveSeq: state.lastMoveSeq,
      isExtrapolating: state.isExtrapolating,
      ignoredOutOfOrder: state.ignoredOutOfOrder,
      snapCount: state.snapCount,
      bufferUnderrunCount: state.bufferUnderrunCount,
      visualX: state.visualX,
      visualY: state.visualY,
    }
  }

  /**
   * Get all tracked entity IDs.
   */
  getTrackedEntityIds(): number[] {
    return Array.from(this.entities.keys())
  }

  /**
   * Get global debug metrics.
   */
  getGlobalDebugMetrics(): {
    entityCount: number
    totalSnapCount: number
    totalIgnoredOutOfOrder: number
    totalBufferUnderrun: number
  } {
    let totalSnapCount = 0
    let totalIgnoredOutOfOrder = 0
    let totalBufferUnderrun = 0

    for (const state of this.entities.values()) {
      totalSnapCount += state.snapCount
      totalIgnoredOutOfOrder += state.ignoredOutOfOrder
      totalBufferUnderrun += state.bufferUnderrunCount
    }

    return {
      entityCount: this.entities.size,
      totalSnapCount,
      totalIgnoredOutOfOrder,
      totalBufferUnderrun,
    }
  }

  /**
   * Clear all entities.
   */
  clear(): void {
    this.entities.clear()
  }

  /**
   * Reset controller state.
   */
  reset(): void {
    this.entities.clear()
    this.globalStreamEpoch = 0
  }
}

export const moveController = new MoveController()
