import { BitmapFont, BitmapText, Container } from 'pixi.js'
import {
  CACHE_CAPACITY,
  CACHE_TTL_MS,
  CAPACITY,
  FILL_COLOR,
  FONT_SIZE_PX,
  STROKE_COLOR,
  STROKE_WIDTH_PX,
  DamageNumbersPresentation,
  safeDamageTargetId,
  type DamageNumberHit,
} from './hud/damageNumbers'

const FONT_NAME = 'OriginDamageNumbers'
// Below nicknames (900_000) and chat balloons (1_000_000).
const Z_INDEX = 800_000

interface ObjectViews {
  getObject(entityId: number): { getContainer(): Container } | undefined
}

interface Anchor {
  x: number
  y: number
}

interface RetiredAnchor extends Anchor {
  expiresAtMs: number
}

/** Owns a bounded bitmap-text pool; gameplay and packet validation stay outside Pixi. */
export class DamageNumberManager {
  readonly presentation = new DamageNumbersPresentation()
  private readonly container = new Container()
  private readonly views: BitmapText[] = []
  private readonly retired = new Map<number, RetiredAnchor>()
  private readonly liveAnchor: Anchor = { x: 0, y: 0 }
  private displayedCount = 0
  private fontInstalled = false

  constructor(parent: Container) {
    this.container.zIndex = Z_INDEX
    this.container.eventMode = 'none'
    this.container.interactiveChildren = false
    for (let index = 0; index < CAPACITY; index++) {
      const view = new BitmapText({ text: '', style: { fontFamily: FONT_NAME, fontSize: FONT_SIZE_PX } })
      view.anchor.set(0.5, 1)
      view.visible = false
      view.eventMode = 'none'
      this.views.push(view)
      this.container.addChild(view)
    }
    parent.addChild(this.container)
  }

  /** Browser-only atlas creation, before the render loop can receive hits. */
  installFont(): void {
    if (this.fontInstalled) return
    BitmapFont.install({
      name: FONT_NAME,
      chars: '0123456789.<e+-',
      style: {
        fontFamily: 'Arial', fontWeight: 'bold', fontSize: FONT_SIZE_PX,
        fill: FILL_COLOR, stroke: { color: STROKE_COLOR, width: STROKE_WIDTH_PX },
      },
      resolution: 2,
      padding: 4,
      skipKerning: true,
      dynamicFill: false,
      textureStyle: { scaleMode: 'linear' },
    })
    this.fontInstalled = true
  }

  getContainer(): Container {
    return this.container
  }

  show(hits: readonly DamageNumberHit[], objects: ObjectViews, now: number, zoom: number): void {
    for (const hit of hits) {
      const entityId = safeDamageTargetId(hit.targetId)
      if (entityId === null) continue
      const object = objects.getObject(entityId)
      const anchor = object ? this.readAnchor(object.getContainer()) : this.readRetiredAnchor(entityId, now)
      if (anchor) this.presentation.emit(entityId, hit.damage, anchor.x, anchor.y, now)
    }
    this.update(now, zoom)
  }

  /** Quarantine/despawn may arrive before the fatal AttackResult. Never retain a view. */
  rememberDespawn(entityId: number, objects: ObjectViews, now: number): void {
    const object = objects.getObject(entityId)
    const anchor = object ? this.readAnchor(object.getContainer()) : null
    if (!anchor) return
    this.retired.delete(entityId)
    if (this.retired.size === CACHE_CAPACITY) {
      const oldest = this.retired.keys().next().value
      if (oldest !== undefined) this.retired.delete(oldest)
    }
    this.retired.set(entityId, { x: anchor.x, y: anchor.y, expiresAtMs: now + CACHE_TTL_MS })
  }

  forgetSpawn(entityId: number): void {
    this.retired.delete(entityId)
  }

  update(now: number, zoom: number): void {
    this.presentation.update(now, zoom)
    if (this.presentation.activeCount === 0 && this.displayedCount === 0) return
    for (let index = 0; index < CAPACITY; index++) {
      const entry = this.presentation.entries[index]!
      const view = this.views[index]!
      view.visible = entry.active
      if (!entry.active) continue
      if (view.text !== entry.text) view.text = entry.text
      view.position.set(entry.x, entry.y)
      view.scale.set(entry.scale)
      view.alpha = entry.alpha
    }
    this.displayedCount = this.presentation.activeCount
  }

  clear(): void {
    this.presentation.clear()
    this.retired.clear()
    for (const view of this.views) view.visible = false
    this.displayedCount = 0
  }

  destroy(): void {
    this.clear()
    this.container.destroy({ children: true })
    if (this.fontInstalled) {
      BitmapFont.uninstall(FONT_NAME)
      this.fontInstalled = false
    }
  }

  private readAnchor(container: Container): Anchor | null {
    const x = container.x
    const y = container.y + container.getLocalBounds().top
    if (!Number.isFinite(x) || !Number.isFinite(y)) return null
    this.liveAnchor.x = x
    this.liveAnchor.y = y
    return this.liveAnchor
  }

  private readRetiredAnchor(entityId: number, now: number): Anchor | null {
    const anchor = this.retired.get(entityId)
    if (!anchor) return null
    if (now >= anchor.expiresAtMs) {
      this.retired.delete(entityId)
      return null
    }
    return anchor
  }
}
