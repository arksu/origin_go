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
import { DEBUG_MOVEMENT } from '@/constants/game'

// Constants
const MAX_KEYFRAMES = 32
const MAX_EXTRAPOLATION_MS = 180
const SNAP_DISTANCE_SQUARED = 2400 * 2400 // ~0.75 tiles at 32 coord/tile -> (0.75 * 32 * 100)^2
const ERROR_CORRECTION_SPEED = 0.15 // per-frame lerp factor for small corrections
const ERROR_CORRECTION_SPEED_LOW = 0.03 // reduced correction during movement start
const ERROR_CORRECTION_RAMP_MS = 200 // time to ramp up correction speed
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
  direction: number // 0-7 index for 8-direction animations (NE,E,SE,S,SW,W,NW,N)
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

  // Movement start tracking for smooth ramp-in
  movementStartClientMs: number
  wasMovingLastFrame: boolean

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
  private lastRenderPositions: Map<number, RenderPosition> = new Map()
  private globalStreamEpoch = 0
  private tickRate = 10 // Default fallback (100ms per tick)

  /**
   * Set the global stream epoch and tick rate (from S2C_PlayerEnterWorld).
   */
  setStreamEpoch(epoch: number, tickRate?: number): void {
    this.globalStreamEpoch = epoch
    if (tickRate && tickRate > 0) {
      this.tickRate = tickRate
    }
  }

  /**
   * Get the current tick rate.
   */
  getTickRate(): number {
    return this.tickRate
  }

  /**
   * Initialize or reset an entity's movement state.
   */
  initEntity(entityId: number, x: number, y: number, heading: number = 0): void {
    this.entities.set(entityId, {
      entityId,
      streamEpoch: this.globalStreamEpoch,
      keyframes: [],
      lastMoveSeq: -1,
      visualX: x,
      visualY: y,
      visualHeading: heading,
      isExtrapolating: false,
      extrapolationStartMs: 0,
      movementStartClientMs: 0,
      wasMovingLastFrame: false,
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
      if (DEBUG_MOVEMENT) {
        console.log(`[MoveController] Initializing entity ${entityId} at (${x.toFixed(2)}, ${y.toFixed(2)})`)
      }
      this.initEntity(entityId, x, y, heading)
      state = this.entities.get(entityId)!
    }

    // Handle teleport - snap and reset buffer
    if (isTeleport) {
      if (DEBUG_MOVEMENT) {
        console.log(`[MoveController] Teleport entity ${entityId} to (${x.toFixed(2)}, ${y.toFixed(2)})`)
      }
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
      if (DEBUG_MOVEMENT) {
        console.warn(`[MoveController] Out-of-order packet for entity ${entityId}: seq ${moveSeq} <= last ${state.lastMoveSeq}`)
      }
      state.ignoredOutOfOrder++
      return
    }

    state.lastMoveSeq = moveSeq
    state.isExtrapolating = false

    // Detect movement start: buffer was empty or entity wasn't moving
    const isMovementStart = state.keyframes.length === 0 || !state.wasMovingLastFrame

    // Add synthetic pre-roll keyframe for smooth movement start
    if (isMovementStart && isMoving) {
      const syntheticOffsetMs = Math.floor(1000 / this.tickRate) // One tick duration in ms
      const syntheticKeyframe: MoveKeyframe = {
        tServerMs: serverTimeMs - syntheticOffsetMs,
        x: state.visualX,
        y: state.visualY,
        vx, vy, // Use incoming velocity
        isMoving: false,
        moveMode,
        heading: state.visualHeading,
        moveSeq: moveSeq - 1,
      }
      state.keyframes.push(syntheticKeyframe)
      state.movementStartClientMs = Date.now()

      if (DEBUG_MOVEMENT) {
        console.log(`[MoveController] Synthetic pre-roll keyframe for entity ${entityId}:`, {
          pos: `(${state.visualX.toFixed(2)}, ${state.visualY.toFixed(2)})`,
          offsetMs: syntheticOffsetMs,
          tickRate: this.tickRate,
        })
      }
    }

    // Add keyframe to buffer
    const keyframe: MoveKeyframe = {
      tServerMs: serverTimeMs,
      x, y, vx, vy,
      isMoving, moveMode, heading, moveSeq,
    }

    state.keyframes.push(keyframe)
    state.wasMovingLastFrame = isMoving

    // Log keyframe addition for debugging
    if (DEBUG_MOVEMENT) {
      const prevKeyframe = state.keyframes.length > 1 ? state.keyframes[state.keyframes.length - 2] : null
      const timeDelta = prevKeyframe ? serverTimeMs - prevKeyframe.tServerMs : 0
      const posDelta = prevKeyframe ? Math.sqrt((x - prevKeyframe.x) ** 2 + (y - prevKeyframe.y) ** 2) : 0

      console.log(`[MoveController] Keyframe added for entity ${entityId}:`, {
        seq: moveSeq,
        pos: `(${x.toFixed(2)}, ${y.toFixed(2)})`,
        velocity: `(${vx.toFixed(2)}, ${vy.toFixed(2)})`,
        isMoving,
        timeDelta: `${timeDelta}ms`,
        posDelta: posDelta.toFixed(2),
        bufferSize: state.keyframes.length,
      })
    }

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
      const prevPos = { x: state.visualX, y: state.visualY }
      const pos = this.interpolateEntity(state, renderTimeMs, clientNowMs)

      // Log movement details for debugging
      const dx = pos.x - prevPos.x
      const dy = pos.y - prevPos.y
      const distance = Math.sqrt(dx * dx + dy * dy)

      if (DEBUG_MOVEMENT && (distance > 0.04)) {
        console.log(`[MoveController] Entity ${entityId}:`, {
          prevPos: `(${prevPos.x.toFixed(2)}, ${prevPos.y.toFixed(2)})`,
          newPos: `(${pos.x.toFixed(2)}, ${pos.y.toFixed(2)})`,
          distance: distance.toFixed(2),
          isMoving: pos.isMoving,
          isExtrapolating: state.isExtrapolating,
          keyframes: state.keyframes.length,
          renderTimeMs: renderTimeMs,
          lastKeyframe: state.keyframes.length > 0 ? state.keyframes[state.keyframes.length - 1]?.tServerMs : 'none',
          moveSeq: state.lastMoveSeq,
        })
      }

      result.set(entityId, pos)
    }

    this.lastRenderPositions = result
    return result
  }

  /**
   * Get render position for a specific entity.
   */
  getRenderPosition(entityId: number): RenderPosition | null {
    return this.lastRenderPositions.get(entityId) ?? null
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
        direction: 4,
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
        direction: 4,
      }
    }

    // Apply error correction (smooth damping with adaptive ramp-in)
    const errorX = targetX - state.visualX
    const errorY = targetY - state.visualY
    const errorDistSq = errorX * errorX + errorY * errorY

    if (errorDistSq > SNAP_DISTANCE_SQUARED) {
      // Large error - snap immediately
      state.visualX = targetX
      state.visualY = targetY
      state.snapCount++
    } else {
      // Small error - smooth correction with adaptive speed
      // Ramp up correction speed during first 200ms of movement
      let correctionSpeed = ERROR_CORRECTION_SPEED

      if (state.movementStartClientMs > 0) {
        const timeSinceMovementStart = clientNowMs - state.movementStartClientMs
        if (timeSinceMovementStart < ERROR_CORRECTION_RAMP_MS) {
          // Lerp from LOW to NORMAL over ramp duration
          const rampProgress = timeSinceMovementStart / ERROR_CORRECTION_RAMP_MS
          const easedProgress = rampProgress * rampProgress // ease-in quadratic
          correctionSpeed = ERROR_CORRECTION_SPEED_LOW +
            (ERROR_CORRECTION_SPEED - ERROR_CORRECTION_SPEED_LOW) * easedProgress
        }
      }

      state.visualX += errorX * correctionSpeed
      state.visualY += errorY * correctionSpeed
    }

    state.visualHeading = heading

    // Prune old keyframes (keep only those potentially needed)
    const pruneThreshold = renderTimeMs - 500 // keep 500ms of history
    while (keyframes.length > 2 && keyframes[0] && keyframes[0].tServerMs < pruneThreshold) {
      keyframes.shift()
    }

    // Calculate 8-direction index from velocity of the latest relevant keyframe
    let dir = 4 // default south
    const lastKf = keyframes[keyframes.length - 1]
    if (lastKf && (lastKf.vx !== 0 || lastKf.vy !== 0)) {
      dir = calcDirection(lastKf.vx, lastKf.vy)
    }

    const finalIsMoving = isMoving // Trust server's isMoving flag for animations

    return {
      x: state.visualX,
      y: state.visualY,
      heading: state.visualHeading,
      isMoving: finalIsMoving,
      moveMode,
      direction: dir,
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
    this.lastRenderPositions.clear()
    this.globalStreamEpoch = 0
  }
}

export const moveController = new MoveController()

/**
 * Calculate 8-direction index from velocity vector.
 * Returns 0-7: NE, E, SE, S, SW, W, NW, N
 */
function calcDirection(vx: number, vy: number): number {
  if (vx === 0 && vy === 0) return 4 // default south

  let angle = Math.atan2(vy, vx)
  angle += Math.PI / 2

  if (angle < 0) {
    angle += 2 * Math.PI
  }

  return Math.floor((angle + Math.PI / 8) / (Math.PI / 4)) % 8
}
