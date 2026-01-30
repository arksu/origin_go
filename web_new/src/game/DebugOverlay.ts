import { Text, Container, TextStyle } from 'pixi.js'
import type { DebugInfo } from './types'

export class DebugOverlay {
  private container: Container
  private text: Text
  private visible: boolean = true

  constructor() {
    this.container = new Container()
    this.container.zIndex = 1000

    const style = new TextStyle({
      fontFamily: 'monospace',
      fontSize: 12,
      fill: '#00ff00',
      stroke: { color: '#000000', width: 2 },
    })

    this.text = new Text({ text: '', style })
    this.text.x = 10
    this.text.y = 10
    this.container.addChild(this.text)
  }

  getContainer(): Container {
    return this.container
  }

  update(info: DebugInfo): void {
    if (!this.visible) return

    this.text.text = [
      `FPS: ${info.fps.toFixed(0)}`,
      `Camera: ${info.cameraX.toFixed(0)}, ${info.cameraY.toFixed(0)}`,
      `Zoom: ${info.zoom.toFixed(2)}`,
      `Viewport: ${info.viewportWidth}x${info.viewportHeight}`,
      `Click (screen): ${info.lastClickScreenX}, ${info.lastClickScreenY}`,
      `Click (world): ${info.lastClickWorldX.toFixed(0)}, ${info.lastClickWorldY.toFixed(0)}`,
      `Objects: ${info.objectsCount}`,
      `Chunks: ${info.chunksLoaded}`,
    ].join('\n')
  }

  toggle(): void {
    this.visible = !this.visible
    this.container.visible = this.visible
  }

  setVisible(visible: boolean): void {
    this.visible = visible
    this.container.visible = visible
  }

  isVisible(): boolean {
    return this.visible
  }

  destroy(): void {
    this.container.destroy({ children: true })
  }
}
