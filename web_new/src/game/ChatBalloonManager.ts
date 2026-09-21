import { Container, Graphics, Text } from 'pixi.js'
import { cameraController } from './CameraController'
import type { ObjectManager } from './ObjectManager'
import type { NicknameManager } from './NicknameManager'
import {
  CHAT_BALLOON_MAX_CHARS,
  CHAT_BALLOON_LIFETIME_MS,
  CHAT_BALLOON_FADEOUT_MS,
} from '@/constants/chat'
import { NICKNAME_LABEL_HEIGHT, NICKNAME_Y_OFFSET_PX } from '@/constants/nickname'
import { WORLD_TEXT_FONT_FAMILY } from '@/constants/fonts'

// Above every object and FX: object zIndex is TERRAIN_BASE_Z_INDEX + screen y,
// which grows with the map size, so a fixed very high value is used (see FxManager).
const BALLOON_Z_INDEX = 1_000_000

const BALLOON_FONT_SIZE = 11
const BALLOON_MAX_TEXT_WIDTH = 150
const BALLOON_PADDING_X = 8
const BALLOON_PADDING_Y = 5
const BALLOON_TAIL_WIDTH = 10
const BALLOON_TAIL_HEIGHT = 6
const BALLOON_RADIUS = 4
const BALLOON_TEXT_COLOR = 0x2b2b2b
const BALLOON_BORDER_COLOR = 0x333333
const BALLOON_BORDER_ALPHA = 0.5
const BALLOON_BACKGROUND_ALPHA = 0.95

interface ActiveBalloon {
  container: Container
  bubble: Graphics
  text: Text
  entityId: number
  expiresAtMs: number
}

function nowMs(): number {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

/**
 * Renders timed speech balloons over world objects for local chat messages.
 * Balloons live in objectsContainer (so they follow camera and zoom) with a
 * very high zIndex, and are counter-scaled each frame to keep a constant
 * on-screen size across zoom levels.
 */
export class ChatBalloonManager {
  private readonly parent: Container
  private readonly nicknameManager: NicknameManager | null
  private balloons: Map<number, ActiveBalloon> = new Map()

  constructor(parent: Container, nicknameManager: NicknameManager | null = null) {
    this.parent = parent
    // When a nickname label is shown for an entity, the balloon floats above
    // it instead of overlapping (the nickname owns the object's bounds top).
    this.nicknameManager = nicknameManager
  }

  show(entityId: number, text: string): void {
    const displayText = this.truncate(text)
    if (!displayText) {
      return
    }

    // A new message from the same sender replaces the previous balloon.
    this.remove(entityId)

    const textEl = new Text({
      text: displayText,
      style: {
        fontFamily: WORLD_TEXT_FONT_FAMILY,
        fontSize: BALLOON_FONT_SIZE,
        fill: BALLOON_TEXT_COLOR,
        wordWrap: true,
        wordWrapWidth: BALLOON_MAX_TEXT_WIDTH,
        breakWords: true,
      },
    })
    textEl.anchor.set(0.5, 1)
    // Text bottom-center sits just above the bubble bottom, tail tip at (0, 0).
    textEl.position.set(0, -(BALLOON_TAIL_HEIGHT + BALLOON_PADDING_Y))

    const bubble = this.buildBubble(textEl.width, textEl.height)

    const container = new Container()
    container.zIndex = BALLOON_Z_INDEX
    container.eventMode = 'none'
    container.addChild(bubble, textEl)
    this.parent.addChild(container)

    this.balloons.set(entityId, {
      container,
      bubble,
      text: textEl,
      entityId,
      expiresAtMs: nowMs() + CHAT_BALLOON_LIFETIME_MS,
    })
  }

  update(objectManager: ObjectManager): void {
    if (this.balloons.size === 0) {
      return
    }

    const currentTimeMs = nowMs()
    const zoom = cameraController.getZoom()
    const inverseScale = zoom > 0 ? 1 / zoom : 1
    const expiredEntityIds: number[] = []

    for (const [entityId, balloon] of this.balloons) {
      const objectView = objectManager.getObject(entityId)
      if (!objectView) {
        expiredEntityIds.push(entityId)
        continue
      }

      const remainingMs = balloon.expiresAtMs - currentTimeMs
      if (remainingMs <= 0) {
        expiredEntityIds.push(entityId)
        continue
      }

      const objectContainer = objectView.getContainer()
      // Anchor the tail tip to the top of the object's visual bounds so the
      // bubble adapts to tall characters and small props alike, then lift it
      // above the nickname label when one is shown for this entity. Both the
      // balloon and the label share the same 15px drop toward the head.
      const boundsTop = objectContainer.getLocalBounds().top
      const nicknameOffset = this.nicknameManager?.has(entityId) ? NICKNAME_LABEL_HEIGHT : 0
      balloon.container.position.set(objectContainer.x, objectContainer.y + boundsTop + NICKNAME_Y_OFFSET_PX - nicknameOffset)
      balloon.container.scale.set(inverseScale)
      // Mirror culling: a hidden object must not leave a floating balloon.
      balloon.container.visible = objectContainer.visible
      balloon.container.alpha = remainingMs < CHAT_BALLOON_FADEOUT_MS
        ? remainingMs / CHAT_BALLOON_FADEOUT_MS
        : 1
    }

    for (const entityId of expiredEntityIds) {
      this.remove(entityId)
    }
  }

  clear(): void {
    for (const entityId of Array.from(this.balloons.keys())) {
      this.remove(entityId)
    }
  }

  destroy(): void {
    this.clear()
  }

  private truncate(text: string): string {
    const trimmed = text.trim()
    if (trimmed.length <= CHAT_BALLOON_MAX_CHARS) {
      return trimmed
    }
    return trimmed.slice(0, CHAT_BALLOON_MAX_CHARS) + '...'
  }

  private buildBubble(textWidth: number, textHeight: number): Graphics {
    const bubble = new Graphics()
    const width = textWidth + BALLOON_PADDING_X * 2
    const height = textHeight + BALLOON_PADDING_Y * 2
    const bottom = -BALLOON_TAIL_HEIGHT

    // Bubble body: fill first, then border.
    bubble.roundRect(-width / 2, bottom - height, width, height, BALLOON_RADIUS)
    bubble.fill({ color: 0xffffff, alpha: BALLOON_BACKGROUND_ALPHA })
    bubble.roundRect(-width / 2, bottom - height, width, height, BALLOON_RADIUS)
    bubble.stroke({ color: BALLOON_BORDER_COLOR, width: 1, alpha: BALLOON_BORDER_ALPHA })

    // Tail: its base sits 1px inside the bubble so the fill covers the border
    // segment under the tail base, then the two tail sides are stroked.
    bubble.moveTo(-BALLOON_TAIL_WIDTH / 2, bottom - 1)
    bubble.lineTo(BALLOON_TAIL_WIDTH / 2, bottom - 1)
    bubble.lineTo(0, 0)
    bubble.closePath()
    bubble.fill({ color: 0xffffff, alpha: BALLOON_BACKGROUND_ALPHA })

    bubble.moveTo(-BALLOON_TAIL_WIDTH / 2, bottom)
    bubble.lineTo(0, 0)
    bubble.lineTo(BALLOON_TAIL_WIDTH / 2, bottom)
    bubble.stroke({ color: BALLOON_BORDER_COLOR, width: 1, alpha: BALLOON_BORDER_ALPHA })

    return bubble
  }

  private remove(entityId: number): void {
    const balloon = this.balloons.get(entityId)
    if (!balloon) {
      return
    }
    this.parent.removeChild(balloon.container)
    balloon.container.destroy({ children: true })
    this.balloons.delete(entityId)
  }
}
