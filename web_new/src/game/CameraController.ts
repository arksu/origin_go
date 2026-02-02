/**
 * CameraController - manages camera behavior (follow, pan, zoom).
 * 
 * Features:
 * - Follow player with smooth lerp or hard follow
 * - Pan offset from player position (middle mouse drag)
 * - Zoom with limits and zoom-to-cursor
 */

import {
  ZOOM_MIN,
  ZOOM_MAX,
  ZOOM_SPEED,
  CAMERA_FOLLOW_LERP,
  CAMERA_FOLLOW_HARD,
  DEBUG_MOVEMENT,
} from '@/constants/game'
import { moveController } from './MoveController'

export interface CameraState {
  x: number
  y: number
  zoom: number
  panOffsetX: number
  panOffsetY: number
}

export class CameraController {
  private targetEntityId: number | null = null

  private x: number = 0
  private y: number = 0
  private zoom: number = 1

  private panOffsetX: number = 0
  private panOffsetY: number = 0

  setTargetEntity(entityId: number | null): void {
    this.targetEntityId = entityId
    this.panOffsetX = 0
    this.panOffsetY = 0
  }

  getTargetEntityId(): number | null {
    return this.targetEntityId
  }

  setPosition(x: number, y: number): void {
    this.x = x
    this.y = y
  }

  getPosition(): { x: number; y: number } {
    return { x: this.x, y: this.y }
  }

  setZoom(zoom: number): void {
    this.zoom = Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, zoom))
  }

  getZoom(): number {
    return this.zoom
  }

  adjustZoom(delta: number): void {
    // Use logarithmic scale for linear perception
    // Convert current zoom to log scale, adjust linearly, then convert back
    const logZoom = Math.log(this.zoom)
    const newLogZoom = logZoom - delta * ZOOM_SPEED
    const newZoom = Math.exp(newLogZoom)
    this.setZoom(newZoom)
  }

  startPan(): void {
    // Pan started - offset will be accumulated via pan() calls
  }

  pan(deltaX: number, deltaY: number): void {
    this.panOffsetX -= deltaX / this.zoom
    this.panOffsetY -= deltaY / this.zoom
  }

  endPan(): void {
    // Pan ended - offset is preserved until reset
  }

  resetPanOffset(): void {
    this.panOffsetX = 0
    this.panOffsetY = 0
  }

  update(): CameraState {
    const prevX = this.x
    const prevY = this.y

    if (this.targetEntityId !== null) {
      const renderPos = moveController.getRenderPosition(this.targetEntityId)

      if (renderPos) {
        const targetX = renderPos.x + this.panOffsetX
        const targetY = renderPos.y + this.panOffsetY

        if (CAMERA_FOLLOW_HARD) {
          this.x = targetX
          this.y = targetY
        } else {
          this.x += (targetX - this.x) * CAMERA_FOLLOW_LERP
          this.y += (targetY - this.y) * CAMERA_FOLLOW_LERP
        }

        // Log camera movement for debugging
        if (DEBUG_MOVEMENT) {
          const dx = this.x - prevX
          const dy = this.y - prevY
          const distance = Math.sqrt(dx * dx + dy * dy)

          if (distance > 0.1) {
            // console.log(`[CameraController] Following entity ${this.targetEntityId}:`, {
            //   entityPos: `(${renderPos.x.toFixed(2)}, ${renderPos.y.toFixed(2)})`,
            //   targetPos: `(${targetX.toFixed(2)}, ${targetY.toFixed(2)})`,
            //   currentPos: `(${this.x.toFixed(2)}, ${this.y.toFixed(2)})`,
            //   delta: `(${dx.toFixed(2)}, ${dy.toFixed(2)})`,
            //   distance: distance.toFixed(2),
            //   panOffset: `(${this.panOffsetX.toFixed(2)}, ${this.panOffsetY.toFixed(2)})`,
            //   isMoving: renderPos.isMoving,
            // })
          }
        }
      }
    }

    return {
      x: this.x,
      y: this.y,
      zoom: this.zoom,
      panOffsetX: this.panOffsetX,
      panOffsetY: this.panOffsetY,
    }
  }

  getState(): CameraState {
    return {
      x: this.x,
      y: this.y,
      zoom: this.zoom,
      panOffsetX: this.panOffsetX,
      panOffsetY: this.panOffsetY,
    }
  }

  reset(): void {
    this.targetEntityId = null
    this.x = 0
    this.y = 0
    this.zoom = 1
    this.panOffsetX = 0
    this.panOffsetY = 0
  }
}

export const cameraController = new CameraController()
