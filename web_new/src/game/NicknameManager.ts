import { Container, Text } from 'pixi.js'
import { cameraController } from './CameraController'
import type { ObjectManager } from './ObjectManager'
import {
  NICKNAME_DEFAULT_COLOR,
  NICKNAME_FONT_SIZE,
  NICKNAME_OUTLINE_COLOR,
  NICKNAME_OUTLINE_WIDTH,
  NICKNAME_PALETTE,
  NICKNAME_Y_OFFSET_PX,
  NICKNAME_Z_INDEX,
} from '@/constants/nickname'
import { WORLD_TEXT_FONT_FAMILY } from '@/constants/fonts'

interface ActiveNickname {
  container: Container
  text: Text
  entityId: number
  name: string
  color: number
}

/**
 * Renders persistent nickname labels over named world entities (players, later
 * NPCs). Labels live in objectsContainer (so they follow camera and zoom) with
 * a very high zIndex below chat balloons, and are counter-scaled each frame to
 * keep a constant on-screen size across zoom levels. Unlike chat balloons they
 * never expire: they follow the entity until it despawns or the world resets.
 */
export class NicknameManager {
  private readonly parent: Container
  private labels: Map<number, ActiveNickname> = new Map()

  constructor(parent: Container) {
    this.parent = parent
  }

  show(entityId: number, name: string, color: number): void {
    // Empty name = unnamed entity (objects, items): never labeled.
    if (!name) {
      this.remove(entityId)
      return
    }
    const existing = this.labels.get(entityId)
    if (existing && existing.name === name && existing.color === color) {
      return
    }

    this.remove(entityId)

    const text = new Text({
      text: name,
      style: {
        fontFamily: WORLD_TEXT_FONT_FAMILY,
        fontSize: NICKNAME_FONT_SIZE,
        fill: this.resolveColor(color),
        stroke: { color: NICKNAME_OUTLINE_COLOR, width: NICKNAME_OUTLINE_WIDTH },
      },
    })
    // Bottom-center sits on the object's visual bounds top.
    text.anchor.set(0.5, 1)

    const container = new Container()
    container.zIndex = NICKNAME_Z_INDEX
    container.eventMode = 'none'
    container.addChild(text)
    this.parent.addChild(container)

    this.labels.set(entityId, { container, text, entityId, name, color })
  }

  has(entityId: number): boolean {
    return this.labels.has(entityId)
  }

  update(objectManager: ObjectManager): void {
    if (this.labels.size === 0) {
      return
    }

    const zoom = cameraController.getZoom()
    const inverseScale = zoom > 0 ? 1 / zoom : 1
    const removedEntityIds: number[] = []

    for (const [entityId, label] of this.labels) {
      const objectView = objectManager.getObject(entityId)
      if (!objectView) {
        removedEntityIds.push(entityId)
        continue
      }

      const objectContainer = objectView.getContainer()
      const boundsTop = objectContainer.getLocalBounds().top
      label.container.position.set(objectContainer.x, objectContainer.y + boundsTop + NICKNAME_Y_OFFSET_PX)
      label.container.scale.set(inverseScale)
      // Mirror culling: a hidden object must not leave a floating label.
      label.container.visible = objectContainer.visible
    }

    for (const entityId of removedEntityIds) {
      this.remove(entityId)
    }
  }

  clear(): void {
    for (const entityId of Array.from(this.labels.keys())) {
      this.remove(entityId)
    }
  }

  destroy(): void {
    this.clear()
  }

  private resolveColor(color: number): number {
    return NICKNAME_PALETTE[color] ?? NICKNAME_DEFAULT_COLOR
  }

  remove(entityId: number): void {
    const label = this.labels.get(entityId)
    if (!label) {
      return
    }
    this.parent.removeChild(label.container)
    label.container.destroy({ children: true })
    this.labels.delete(entityId)
  }
}
