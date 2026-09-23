import { actionCursorCss } from './cursorCatalog'
import type { EventSystem } from 'pixi.js'

export class CursorManager {
  private canvas: HTMLCanvasElement | null = null
  private events: EventSystem | null = null

  attach(canvas: HTMLCanvasElement, events: EventSystem): void {
    this.canvas = canvas
    this.events = events
    this.set('')
  }

  set(cursorId: string): void {
    if (!this.canvas || !this.events) return
    const css = actionCursorCss(cursorId)
    // Pixi updates the canvas cursor on hover, so both map and object modes
    // must reflect the armed action until the server resets it.
    this.events.cursorStyles.default = cursorId ? css : 'inherit'
    this.events.cursorStyles.pointer = cursorId ? css : 'pointer'
    this.canvas.style.cursor = cursorId ? css : (this.events.rootBoundary.cursor || 'default')
  }

  detach(): void {
    this.set('')
    this.canvas = null
    this.events = null
  }
}
