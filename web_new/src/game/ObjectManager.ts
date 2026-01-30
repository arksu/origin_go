import { Container } from 'pixi.js'
import { ObjectView, type ObjectViewOptions } from './ObjectView'

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
   */
  private sortByDepth(): void {
    const sorted = Array.from(this.objects.values()).sort((a, b) => {
      return a.getDepthY() - b.getDepthY()
    })

    sorted.forEach((obj, index) => {
      obj.getContainer().zIndex = index
    })
  }

  /**
   * Clear all objects.
   */
  clear(): void {
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
