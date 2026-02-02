import { Container, Sprite, Graphics, Text, Rectangle } from 'pixi.js'
import { ResourceLoader } from './ResourceLoader'
import { coordGame2Screen } from './utils/coordConvert'
import { type AABB, fromMinMax } from './culling/AABB'
import { OBJECT_BOUNDS_COLOR, OBJECT_BOUNDS_WIDTH, OBJECT_BOUNDS_ALPHA } from '@/constants/render'

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
  private boundsGraphics: Graphics | null = null

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
      // console.log(`[ObjectView] Hover over entity ${this.entityId}`)
      this.setHovered(true)
    })

    placeholder.on('pointerout', () => {
      // console.log(`[ObjectView] Hover out from entity ${this.entityId}`)
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
    this.updateBoundsGraphics()
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
   * Compute AABB bounds in screen/local coordinates for culling.
   * Uses conservative bounds based on object size.
   */
  computeScreenBounds(): AABB {
    // Container position is already in screen coordinates
    const cx = this.container.x
    const cy = this.container.y

    // Estimate screen-space size based on game size
    // For isometric, we use a conservative estimate
    const halfWidth = Math.max(this.size.x, this.size.y) * 2 + 32 // Extra padding
    const halfHeight = Math.max(this.size.x, this.size.y) + 64 // Extra padding for height

    return fromMinMax(
      cx - halfWidth,
      cy - halfHeight,
      cx + halfWidth,
      cy,
    )
  }

  /**
   * Get current game position.
   */
  getPosition(): { x: number; y: number } {
    return { x: this.position.x, y: this.position.y }
  }

  /**
   * Check if a world point is within this object's bounds.
   */
  containsWorldPoint(worldX: number, worldY: number): boolean {
    const halfSizeX = this.size.x / 2
    const halfSizeY = this.size.y / 2

    return (
      worldX >= this.position.x - halfSizeX &&
      worldX <= this.position.x + halfSizeX &&
      worldY >= this.position.y - halfSizeY &&
      worldY <= this.position.y + halfSizeY
    )
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

  /**
   * Enable/disable bounds visualization.
   */
  setBoundsVisible(visible: boolean): void {
    if (visible && !this.boundsGraphics) {
      this.createBoundsGraphics()
    } else if (!visible && this.boundsGraphics) {
      this.removeBoundsGraphics()
    }
  }

  /**
   * Check if bounds are currently visible.
   */
  isBoundsVisible(): boolean {
    return this.boundsGraphics !== null
  }

  /**
   * Create bounds graphics for the object.
   */
  private createBoundsGraphics(): void {
    if (this.boundsGraphics) return

    this.boundsGraphics = new Graphics()
    this.updateBoundsGraphics()
    this.container.addChild(this.boundsGraphics)
  }

  /**
   * Remove bounds graphics.
   */
  private removeBoundsGraphics(): void {
    if (this.boundsGraphics) {
      this.container.removeChild(this.boundsGraphics)
      this.boundsGraphics.destroy()
      this.boundsGraphics = null
    }
  }

  /**
   * Update bounds graphics to match current object size and position.
   * Draws bounds in isometric projection using game coordinates.
   */
  private updateBoundsGraphics(): void {
    if (!this.boundsGraphics) return

    this.boundsGraphics.clear()

    // Skip if size is zero
    if (this.size.x === 0 || this.size.y === 0) {
      return
    }

    // Calculate bounds in game coordinates
    const halfWidthX = this.size.x / 2
    const halfHeightY = this.size.y / 2

    // Four corners of the bounding box in game coordinates
    const corners = [
      { x: this.position.x - halfWidthX, y: this.position.y - halfHeightY }, // Top-left
      { x: this.position.x + halfWidthX, y: this.position.y - halfHeightY }, // Top-right
      { x: this.position.x + halfWidthX, y: this.position.y + halfHeightY }, // Bottom-right
      { x: this.position.x - halfWidthX, y: this.position.y + halfHeightY }, // Bottom-left
    ]

    // Transform corners to screen coordinates
    const screenCorners = corners.map(corner => coordGame2Screen(corner.x, corner.y))

    // Transform to local coordinates relative to container position
    const containerScreenPos = coordGame2Screen(this.position.x, this.position.y)
    const localCorners = screenCorners.map(screen => ({
      x: screen.x - containerScreenPos.x,
      y: screen.y - containerScreenPos.y
    }))

    // Draw isometric rectangle
    this.boundsGraphics.setStrokeStyle({
      width: OBJECT_BOUNDS_WIDTH,
      color: OBJECT_BOUNDS_COLOR,
      alpha: OBJECT_BOUNDS_ALPHA
    })

    this.boundsGraphics.moveTo(localCorners[0]?.x || 0, localCorners[0]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[1]?.x || 0, localCorners[1]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[2]?.x || 0, localCorners[2]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[3]?.x || 0, localCorners[3]?.y || 0)
    this.boundsGraphics.lineTo(localCorners[0]?.x || 0, localCorners[0]?.y || 0)
    this.boundsGraphics.stroke()
  }

  destroy(): void {
    this.removeBoundsGraphics()
    this.sprite.destroy({ children: true })
    this.container.destroy({ children: true })
  }
}
