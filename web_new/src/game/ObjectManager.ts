import { Container } from 'pixi.js'
import { ObjectView, type ObjectViewOptions } from './ObjectView'
import { cullingController } from './culling'
import { TERRAIN_BASE_Z_INDEX } from '@/constants/terrain'

/**
 * ObjectManager manages all game objects (characters, resources, buildings, etc.)
 * Handles spawning, despawning, updates, and Z-sorting.
 */
export class ObjectManager {
  private parentContainer: Container | null = null
  private objects: Map<number, ObjectView> = new Map()
  private needsSort = false
  private boundsVisible: boolean = false

  /**
   * Set the shared parent container (objectsContainer) where object views
   * are added directly alongside terrain sprites for correct z-sorting.
   */
  setParentContainer(container: Container): void {
    this.parentContainer = container
  }

  /**
   * Spawn a new object.
   */
  spawnObject(options: ObjectViewOptions): void {
    // Remove existing object if it exists
    if (this.objects.has(options.entityId)) {
      console.warn(`[ObjectManager] Object ${options.entityId} already exists, removing old one`)
      this.despawnObject(options.entityId)
    }

    const objectView = new ObjectView(options)
    this.objects.set(options.entityId, objectView)
    this.parentContainer!.addChild(objectView.getContainer())

    // Register with culling controller
    cullingController.registerObject(
      options.entityId,
      objectView.getContainer(),
      objectView.computeScreenBounds(),
    )

    // Set bounds visibility if currently enabled
    if (this.areBoundsVisible()) {
      objectView.setBoundsVisible(true)
    }

    this.needsSort = true

    console.log(`[ObjectManager] Spawned object ${options.entityId}, type=${options.typeId}, total=${this.objects.size}`)
  }

  /**
   * Despawn an object.
   */
  despawnObject(entityId: number): void {
    const objectView = this.objects.get(entityId)
    if (!objectView) {
      console.warn(`[ObjectManager] Cannot despawn object ${entityId}: not found`)
      return
    }

    // Unregister from culling controller
    cullingController.unregisterObject(entityId)

    this.parentContainer?.removeChild(objectView.getContainer())
    objectView.destroy()
    this.objects.delete(entityId)

    console.log(`[ObjectManager] Despawned object ${entityId}, remaining=${this.objects.size}`)
  }

  /**
   * Update object position and movement state.
   */
  updateObjectPosition(entityId: number, x: number, y: number, isMoving?: boolean, direction?: number): void {
    const objectView = this.objects.get(entityId)
    if (!objectView) {
      return
    }

    objectView.updatePosition(x, y)

    if (isMoving !== undefined && direction !== undefined) {
      if (isMoving) {
        objectView.onMoved(direction)
      } else {
        objectView.onStopped()
      }
    }

    // Update bounds in culling controller
    cullingController.updateObjectBounds(entityId, objectView.computeScreenBounds())

    this.needsSort = true
  }

  /**
   * Get an object by entity ID.
   */
  getObject(entityId: number): ObjectView | undefined {
    return this.objects.get(entityId)
  }

  /**
   * Find entity at screen coordinates.
   * Returns entityId if found, null otherwise.
   */
  getEntityAtScreen(
    screenX: number,
    screenY: number,
    screenToWorld: (x: number, y: number) => { x: number; y: number }
  ): number | null {
    const worldPos = screenToWorld(screenX, screenY)

    for (const [entityId, objectView] of this.objects) {
      if (objectView.containsWorldPoint(worldPos.x, worldPos.y)) {
        return entityId
      }
    }

    return null
  }

  /**
   * Get total number of objects.
   */
  getObjectCount(): number {
    return this.objects.size
  }

  /**
   * Update all objects (called every frame).
   * Performs Z-sorting if needed.
   */
  update(): void {
    if (this.needsSort) {
      this.sortByDepth()
      this.needsSort = false
    }
  }

  /**
   * Sort objects by depth (Y coordinate).
   * Objects with higher Y should be drawn on top.
   * Uses screen Y position for zIndex so objects and terrain sprites
   * share the same z-sorting coordinate space.
   */
  private sortByDepth(): void {
    // Get only visible objects for sorting
    const visibleIds = cullingController.getVisibleObjectIds()

    for (const entityId of visibleIds) {
      const obj = this.objects.get(entityId)
      if (obj) {
        obj.getContainer().zIndex = TERRAIN_BASE_Z_INDEX + obj.getContainer().y
      }
    }
  }

  /**
   * Set bounds visibility for all objects.
   * Used when debug mode is toggled.
   */
  setBoundsVisible(visible: boolean): void {
    this.boundsVisible = visible
    for (const objectView of this.objects.values()) {
      objectView.setBoundsVisible(visible)
    }
  }

  /**
   * Get bounds visibility state.
   */
  areBoundsVisible(): boolean {
    return this.boundsVisible
  }

  /**
   * Clear all objects.
   */
  clear(): void {
    // Unregister all objects from culling
    for (const entityId of this.objects.keys()) {
      cullingController.unregisterObject(entityId)
    }
    for (const obj of this.objects.values()) {
      this.parentContainer?.removeChild(obj.getContainer())
      obj.destroy()
    }
    this.objects.clear()
  }

  /**
   * Destroy the manager and all objects.
   */
  destroy(): void {
    this.clear()
    this.parentContainer = null
  }
}
