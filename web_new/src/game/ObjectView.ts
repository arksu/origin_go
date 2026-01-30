import { Container, Sprite, Graphics, Text, Rectangle } from 'pixi.js'
import { ResourceLoader } from './ResourceLoader'
import { coordGame2Screen } from './utils/coordConvert'

export interface ObjectViewOptions {
  entityId: number
  objectType: number
  resourcePath: string
  position: { x: number; y: number }
  size: { x: number; y: number }
}

/**
 * ObjectView represents a visual game object (character, resource, building, etc.)
 * 
 * Architecture for future extensions:
 * - Multi-layer rendering (shadow, base, overlay, effects)
 * - Animation support (sprite sheets, frame sequences)
 * - Interactive states (hover, selected, targeted)
 * - Health bars, labels, status indicators
 * - Equipment and appearance customization
 */
export class ObjectView {
  readonly entityId: number
  readonly objectType: number

  private container: Container
  private sprite: Sprite
  private debugText: Text | null = null

  private resourcePath: string
  private position: { x: number; y: number }
  private size: { x: number; y: number }

  private isResourceLoaded = false

  constructor(options: ObjectViewOptions) {
    this.entityId = options.entityId
    this.objectType = options.objectType
    this.resourcePath = options.resourcePath
    this.position = options.position
    this.size = options.size

    this.container = new Container()
    this.container.sortableChildren = true

    // Create placeholder sprite
    this.sprite = new Sprite()
    this.sprite.anchor.set(0.5, 1) // Bottom-center anchor for isometric
    this.container.addChild(this.sprite)

    // Initialize visual representation
    this.createPlaceholder()
    this.updateScreenPosition()

    // Load actual resource asynchronously
    this.loadResource()
  }

  getContainer(): Container {
    return this.container
  }

  /**
   * Create a temporary placeholder visual until resource loads.
   * Temporary: type 0 = blue cross, type 6 = red cross
   */
  private createPlaceholder(): void {
    const placeholder = new Graphics()

    // Choose color based on object type
    let color = 0x00ff00 // default green
    if (this.objectType === 1) {
      color = 0x0000ff // blue for type 0
    } else if (this.objectType === 6) {
      color = 0xff0000 // red for type 6
    }

    // Draw cross (X shape)
    const size = 10
    placeholder.moveTo(-size, -size)
    placeholder.lineTo(size, size)
    placeholder.moveTo(size, -size)
    placeholder.lineTo(-size, size)
    placeholder.stroke({ color, width: 3 })

    // Set hit area for interaction (larger than visual for easier clicking)
    const hitSize = size + 5
    placeholder.hitArea = new Rectangle(-hitSize, -hitSize, hitSize * 2, hitSize * 2)

    // Make interactive
    placeholder.eventMode = 'static'
    placeholder.cursor = 'pointer'

    // Add event handlers
    placeholder.on('pointerdown', () => {
      console.log(`[ObjectView] Pointer down on entity ${this.entityId}`)
      this.onClick()
    })

    placeholder.on('pointerover', () => {
      console.log(`[ObjectView] Hover over entity ${this.entityId}`)
      this.setHovered(true)
    })

    placeholder.on('pointerout', () => {
      console.log(`[ObjectView] Hover out from entity ${this.entityId}`)
      this.setHovered(false)
    })

    // Add Graphics directly to container, not to sprite
    this.container.addChild(placeholder)
  }

  /**
   * Load the actual resource for this object.
   * Future: support animations, multi-layer objects, etc.
   */
  private async loadResource(): Promise<void> {
    if (this.isResourceLoaded) return

    try {
      const texture = await ResourceLoader.loadTexture(this.resourcePath)

      // Remove placeholder
      this.sprite.removeChildren()

      // Set loaded texture
      this.sprite.texture = texture

      // Future: handle multi-layer objects
      // - Load shadow texture
      // - Load overlay textures
      // - Setup animation frames

      this.isResourceLoaded = true

      if (this.resourcePath) {
        console.log(`[ObjectView] Resource loaded for entity ${this.entityId}: ${this.resourcePath}`)
      }
    } catch (error) {
      console.warn(`[ObjectView] Failed to load resource for entity ${this.entityId}:`, error)
    }
  }

  /**
   * Update game position and convert to screen coordinates.
   */
  updatePosition(x: number, y: number): void {
    this.position.x = x
    this.position.y = y
    this.updateScreenPosition()
  }

  private updateScreenPosition(): void {
    const screenPos = coordGame2Screen(this.position.x, this.position.y)
    this.container.x = screenPos.x
    this.container.y = screenPos.y
  }

  /**
   * Get Y coordinate for depth sorting.
   * Objects with higher Y should be drawn on top.
   */
  getDepthY(): number {
    return this.position.y + this.size.y
  }

  /**
   * Enable/disable debug visualization.
   */
  setDebugMode(enabled: boolean): void {
    if (enabled && !this.debugText) {
      this.debugText = new Text({
        text: `E:${this.entityId}\nT:${this.objectType}`,
        style: {
          fontSize: 10,
          fill: 0xffffff,
          stroke: { color: 0x000000, width: 2 },
        },
      })
      this.debugText.anchor.set(0.5, 1)
      this.debugText.y = -40
      this.container.addChild(this.debugText)
    } else if (!enabled && this.debugText) {
      this.container.removeChild(this.debugText)
      this.debugText.destroy()
      this.debugText = null
    }
  }

  /**
   * Handle click interaction.
   * Future: emit events for game logic to handle.
   */
  onClick(): void {
    console.log(`[ObjectView] Clicked entity ${this.entityId}`)
    // Future: emit event to game logic
  }

  /**
   * Handle hover state.
   * Future: visual feedback, tooltips, etc.
   */
  setHovered(hovered: boolean): void {
    // Future: change tint, show outline, etc.
    if (hovered) {
      this.sprite.tint = 0xcccccc
    } else {
      this.sprite.tint = 0xffffff
    }
  }

  destroy(): void {
    this.sprite.destroy({ children: true })
    this.container.destroy({ children: true })
  }
}
