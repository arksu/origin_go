/**
 * CameraController - manages camera behavior (follow, pan, zoom).
 * 
 * Features:
 * - Follow player with smooth lerp or hard follow
 * - Pan offset from player position (middle mouse drag)
 * - Zoom with limits and zoom-to-cursor
 */

import { config } from '@/config'
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
    this.zoom = Math.max(config.ZOOM_MIN, Math.min(config.ZOOM_MAX, zoom))
  }

  getZoom(): number {
    return this.zoom
  }

  adjustZoom(delta: number): void {
    const newZoom = this.zoom - delta * config.ZOOM_SPEED
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
    if (this.targetEntityId !== null) {
      const renderPos = moveController.getRenderPosition(this.targetEntityId)

      if (renderPos) {
        const targetX = renderPos.x + this.panOffsetX
        const targetY = renderPos.y + this.panOffsetY

        if (config.CAMERA_FOLLOW_HARD) {
          this.x = targetX
          this.y = targetY
        } else {
          this.x += (targetX - this.x) * config.CAMERA_FOLLOW_LERP
          this.y += (targetY - this.y) * config.CAMERA_FOLLOW_LERP
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
