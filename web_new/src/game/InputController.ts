/**
 * InputController - handles mouse and keyboard input for the game.
 * 
 * Responsibilities:
 * - Normalize pointer events (click vs drag detection)
 * - Track modifier keys (Shift/Ctrl/Alt)
 * - Handle blur/visibility changes to prevent stuck states
 * - Emit normalized input events to other controllers
 */

import { CLICK_DRAG_THRESHOLD_PX } from '@/constants/game'
import type { ScreenPoint } from './types'

export const enum Modifiers {
  NONE = 0,
  SHIFT = 1,
  CTRL = 2,
  ALT = 4,
}

export interface PointerClickEvent {
  screenX: number
  screenY: number
  button: number
  modifiers: number
}

export interface PointerDragEvent {
  deltaX: number
  deltaY: number
  button: number
}

export interface WheelEvent {
  deltaY: number
  screenX: number
  screenY: number
}

type ClickHandler = (event: PointerClickEvent) => void
type DragStartHandler = (button: number) => void
type DragMoveHandler = (event: PointerDragEvent) => void
type DragEndHandler = (button: number) => void
type WheelHandler = (event: WheelEvent) => void
type PointerMoveHandler = (screenX: number, screenY: number) => void

export class InputController {
  private canvas: HTMLCanvasElement | null = null

  private modifiers: number = Modifiers.NONE

  private pointerDownPos: ScreenPoint | null = null
  private pointerDownButton: number = -1
  private isDragging: boolean = false

  private onClickHandler: ClickHandler | null = null
  private onDragStartHandler: DragStartHandler | null = null
  private onDragMoveHandler: DragMoveHandler | null = null
  private onDragEndHandler: DragEndHandler | null = null
  private onWheelHandler: WheelHandler | null = null
  private onPointerMoveHandler: PointerMoveHandler | null = null

  private boundPointerDown: (e: globalThis.PointerEvent) => void
  private boundPointerMove: (e: globalThis.PointerEvent) => void
  private boundPointerUp: (e: globalThis.PointerEvent) => void
  private boundWheel: (e: globalThis.WheelEvent) => void
  private boundKeyDown: (e: KeyboardEvent) => void
  private boundKeyUp: (e: KeyboardEvent) => void
  private boundBlur: () => void
  private boundVisibilityChange: () => void
  private boundContextMenu: (e: Event) => void

  constructor() {
    this.boundPointerDown = this.handlePointerDown.bind(this)
    this.boundPointerMove = this.handlePointerMove.bind(this)
    this.boundPointerUp = this.handlePointerUp.bind(this)
    this.boundWheel = this.handleWheel.bind(this)
    this.boundKeyDown = this.handleKeyDown.bind(this)
    this.boundKeyUp = this.handleKeyUp.bind(this)
    this.boundBlur = this.handleBlur.bind(this)
    this.boundVisibilityChange = this.handleVisibilityChange.bind(this)
    this.boundContextMenu = this.handleContextMenu.bind(this)
  }

  init(canvas: HTMLCanvasElement): void {
    this.canvas = canvas

    canvas.addEventListener('pointerdown', this.boundPointerDown)
    canvas.addEventListener('pointermove', this.boundPointerMove)
    canvas.addEventListener('pointerup', this.boundPointerUp)
    canvas.addEventListener('pointercancel', this.boundPointerUp)
    canvas.addEventListener('wheel', this.boundWheel, { passive: false })
    canvas.addEventListener('contextmenu', this.boundContextMenu)

    window.addEventListener('keydown', this.boundKeyDown)
    window.addEventListener('keyup', this.boundKeyUp)
    window.addEventListener('blur', this.boundBlur)
    document.addEventListener('visibilitychange', this.boundVisibilityChange)
  }

  destroy(): void {
    if (this.canvas) {
      this.canvas.removeEventListener('pointerdown', this.boundPointerDown)
      this.canvas.removeEventListener('pointermove', this.boundPointerMove)
      this.canvas.removeEventListener('pointerup', this.boundPointerUp)
      this.canvas.removeEventListener('pointercancel', this.boundPointerUp)
      this.canvas.removeEventListener('wheel', this.boundWheel)
      this.canvas.removeEventListener('contextmenu', this.boundContextMenu)
    }

    window.removeEventListener('keydown', this.boundKeyDown)
    window.removeEventListener('keyup', this.boundKeyUp)
    window.removeEventListener('blur', this.boundBlur)
    document.removeEventListener('visibilitychange', this.boundVisibilityChange)

    this.canvas = null
    this.resetState()
  }

  onClick(handler: ClickHandler): void {
    this.onClickHandler = handler
  }

  onDragStart(handler: DragStartHandler): void {
    this.onDragStartHandler = handler
  }

  onDragMove(handler: DragMoveHandler): void {
    this.onDragMoveHandler = handler
  }

  onDragEnd(handler: DragEndHandler): void {
    this.onDragEndHandler = handler
  }

  onWheel(handler: WheelHandler): void {
    this.onWheelHandler = handler
  }

  onPointerMove(handler: PointerMoveHandler): void {
    this.onPointerMoveHandler = handler
  }

  getModifiers(): number {
    return this.modifiers
  }

  private handlePointerDown(e: globalThis.PointerEvent): void {
    this.pointerDownPos = { x: e.clientX, y: e.clientY }
    this.pointerDownButton = e.button
    this.isDragging = false

    if (e.button === 1) {
      this.canvas?.setPointerCapture(e.pointerId)
    }
  }

  private handlePointerMove(e: globalThis.PointerEvent): void {
    this.onPointerMoveHandler?.(e.clientX, e.clientY)

    if (this.pointerDownPos === null) return

    const dx = e.clientX - this.pointerDownPos.x
    const dy = e.clientY - this.pointerDownPos.y
    const distance = Math.sqrt(dx * dx + dy * dy)

    if (!this.isDragging && distance > CLICK_DRAG_THRESHOLD_PX) {
      this.isDragging = true
      this.onDragStartHandler?.(this.pointerDownButton)
    }

    if (this.isDragging) {
      this.onDragMoveHandler?.({
        deltaX: e.movementX,
        deltaY: e.movementY,
        button: this.pointerDownButton,
      })
    }
  }

  private handlePointerUp(e: globalThis.PointerEvent): void {
    if (this.pointerDownPos === null) return

    if (e.button === 1) {
      this.canvas?.releasePointerCapture(e.pointerId)
    }

    if (this.isDragging) {
      this.onDragEndHandler?.(this.pointerDownButton)
    } else {
      this.onClickHandler?.({
        screenX: e.clientX,
        screenY: e.clientY,
        button: this.pointerDownButton,
        modifiers: this.modifiers,
      })
    }

    this.pointerDownPos = null
    this.pointerDownButton = -1
    this.isDragging = false
  }

  private handleWheel(e: globalThis.WheelEvent): void {
    e.preventDefault()
    this.onWheelHandler?.({
      deltaY: e.deltaY,
      screenX: e.clientX,
      screenY: e.clientY,
    })
  }

  private handleKeyDown(e: KeyboardEvent): void {
    this.updateModifiers(e)
  }

  private handleKeyUp(e: KeyboardEvent): void {
    this.updateModifiers(e)
  }

  private updateModifiers(e: KeyboardEvent): void {
    let mods = Modifiers.NONE
    if (e.shiftKey) mods |= Modifiers.SHIFT
    if (e.ctrlKey) mods |= Modifiers.CTRL
    if (e.altKey) mods |= Modifiers.ALT
    this.modifiers = mods
  }

  private handleBlur(): void {
    this.resetState()
  }

  private handleVisibilityChange(): void {
    if (document.hidden) {
      this.resetState()
    }
  }

  private handleContextMenu(e: Event): void {
    e.preventDefault()
  }

  private resetState(): void {
    if (this.isDragging && this.pointerDownButton !== -1) {
      this.onDragEndHandler?.(this.pointerDownButton)
    }

    this.modifiers = Modifiers.NONE
    this.pointerDownPos = null
    this.pointerDownButton = -1
    this.isDragging = false
  }
}
