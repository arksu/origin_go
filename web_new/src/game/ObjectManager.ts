import { Container } from 'pixi.js'
import { ObjectView, type ObjectViewOptions } from './ObjectView'
import { cullingController } from './culling'

/**
 * ObjectManager manages all game objects (characters, resources, buildings, etc.)
 * Handles spawning, despawning, updates, and Z-sorting.
 */
export class ObjectManager {
  private container: Container
  private objects: Map<number, ObjectView> = new Map()
  private needsSort = false

  constructor() {
    this.container = new Container()
    this.container.sortableChildren = true
  }

  getContainer(): Container {
    return this.container
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
    this.container.addChild(objectView.getContainer())

    // Register with culling controller
    cullingController.registerObject(
      options.entityId,
      objectView.getContainer(),
      objectView.computeScreenBounds(),
    )

    this.needsSort = true

    console.log(`[ObjectManager] Spawned object ${options.entityId}, type=${options.objectType}, total=${this.objects.size}`)
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

    this.container.removeChild(objectView.getContainer())
    objectView.destroy()
    this.objects.delete(entityId)

    console.log(`[ObjectManager] Despawned object ${entityId}, remaining=${this.objects.size}`)
  }

  /**
   * Update object position (for movement).
   */
  updateObjectPosition(entityId: number, x: number, y: number): void {
    const objectView = this.objects.get(entityId)
    if (!objectView) {
      console.warn(`[ObjectManager] Cannot update position for object ${entityId}: not found`)
      return
    }

    objectView.updatePosition(x, y)

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
   * Only sorts visible objects for performance optimization.
   */
  private sortByDepth(): void {
    // Get only visible objects for sorting
    const visibleIds = cullingController.getVisibleObjectIds()
    const visibleObjects: ObjectView[] = []

    for (const entityId of visibleIds) {
      const obj = this.objects.get(entityId)
      if (obj) {
        visibleObjects.push(obj)
      }
    }

    // Sort visible objects by depth
    visibleObjects.sort((a, b) => a.getDepthY() - b.getDepthY())

    // Assign zIndex only to visible objects
    visibleObjects.forEach((obj, index) => {
      obj.getContainer().zIndex = index
    })
  }

  /**
   * Clear all objects.
   */
  clear(): void {
    // Unregister all objects from culling
    for (const entityId of this.objects.keys()) {
      cullingController.unregisterObject(entityId)
    }
    this.objects.forEach(obj => obj.destroy())
    this.objects.clear()
    this.container.removeChildren()
  }

  /**
   * Destroy the manager and all objects.
   */
  destroy(): void {
    this.clear()
    this.container.destroy({ children: true })
  }
}
