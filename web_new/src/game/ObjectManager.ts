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
  private animatedObjectIds: Set<number> = new Set()
  private carriedByByObject: Map<number, number> = new Map()
  private carriedObjectsByCarrier: Map<number, Set<number>> = new Map()
  private activeCarriedObjects: Set<number> = new Set()
  private needsSort = false
  private boundsVisible: boolean = false
  private hoveredEntityId: number | null = null

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
    if (objectView.hasAnimatedFrames()) {
      this.animatedObjectIds.add(options.entityId)
    }
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

    // console.log(`[ObjectManager] Spawned object ${options.entityId}, type=${options.typeId}, total=${this.objects.size}`)
  }

  /**
   * Despawn an object.
   */
  despawnObject(entityId: number): void {
    const carriedSet = this.carriedObjectsByCarrier.get(entityId)
    if (carriedSet && carriedSet.size > 0) {
      for (const carriedObjectId of Array.from(carriedSet)) {
        this.clearCarryVisualRelation(carriedObjectId)
      }
    }
    this.clearCarryVisualRelation(entityId)

    const objectView = this.objects.get(entityId)
    if (!objectView) {
      console.warn(`[ObjectManager] Cannot despawn object ${entityId}: not found`)
      return
    }
    this.animatedObjectIds.delete(entityId)

    // Unregister from culling controller
    cullingController.unregisterObject(entityId)

    this.parentContainer?.removeChild(objectView.getContainer())
    objectView.destroy()
    this.objects.delete(entityId)
    if (this.hoveredEntityId === entityId) {
      this.hoveredEntityId = null
    }

    // console.log(`[ObjectManager] Despawned object ${entityId}, remaining=${this.objects.size}`)
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

  setCarryVisualRelation(objectId: number, carrierId: number | null): void {
    const nextCarrierId = carrierId != null && carrierId > 0 ? carrierId : null
    const prevCarrierId = this.carriedByByObject.get(objectId)
    if ((prevCarrierId ?? null) === nextCarrierId) {
      return
    }

    if (prevCarrierId != null) {
      const prevSet = this.carriedObjectsByCarrier.get(prevCarrierId)
      if (prevSet) {
        prevSet.delete(objectId)
        if (prevSet.size === 0) {
          this.carriedObjectsByCarrier.delete(prevCarrierId)
        }
      }
      this.carriedByByObject.delete(objectId)
      this.activeCarriedObjects.delete(objectId)
    }

    const objectView = this.objects.get(objectId)

    if (nextCarrierId == null) {
      if (objectView) {
        objectView.clearScreenPositionOverride()
        objectView.clearVisualScreenOffset()
        objectView.setZIndexOverride(null)
        objectView.setInteractionSuppressed(false)
        objectView.setShadowSuppressed(false)
        cullingController.updateObjectBounds(objectId, objectView.computeScreenBounds())
      }
      this.needsSort = true
      return
    }

    this.carriedByByObject.set(objectId, nextCarrierId)
    let carriedSet = this.carriedObjectsByCarrier.get(nextCarrierId)
    if (!carriedSet) {
      carriedSet = new Set<number>()
      this.carriedObjectsByCarrier.set(nextCarrierId, carriedSet)
    }
    carriedSet.add(objectId)
    this.activeCarriedObjects.add(objectId)

    if (objectView) {
      objectView.setInteractionSuppressed(true)
      objectView.setShadowSuppressed(true)
    }
    this.needsSort = true
  }

  clearCarryVisualRelation(objectId: number): void {
    this.setCarryVisualRelation(objectId, null)
  }

  syncActiveCarryVisuals(offsetPx: number): void {
    if (this.activeCarriedObjects.size === 0) {
      return
    }

    const staleObjectIds: number[] = []
    for (const objectId of this.activeCarriedObjects) {
      const carrierId = this.carriedByByObject.get(objectId)
      if (carrierId == null) {
        staleObjectIds.push(objectId)
        continue
      }

      const objectView = this.objects.get(objectId)
      if (!objectView) {
        staleObjectIds.push(objectId)
        continue
      }

      objectView.setInteractionSuppressed(true)
      objectView.setShadowSuppressed(true)

      const carrierView = this.objects.get(carrierId)
      if (!carrierView) {
        const hadOverride = objectView.getZIndexOverride() != null
        objectView.clearScreenPositionOverride()
        objectView.clearVisualScreenOffset()
        objectView.setZIndexOverride(null)
        cullingController.updateObjectBounds(objectId, objectView.computeScreenBounds())
        if (hadOverride) {
          this.needsSort = true
        }
        continue
      }

      const targetZ = TERRAIN_BASE_Z_INDEX + carrierView.getContainer().y + 1
      const zChanged = objectView.getZIndexOverride() !== targetZ
      objectView.setScreenPositionOverride(
        carrierView.getContainer().x,
        carrierView.getContainer().y - offsetPx,
      )
      objectView.setZIndexOverride(targetZ)
      cullingController.updateObjectBounds(objectId, objectView.computeScreenBounds())
      if (zChanged) {
        this.needsSort = true
      }
    }

    for (const staleObjectId of staleObjectIds) {
      this.clearCarryVisualRelation(staleObjectId)
    }
  }

  /**
   * Get an object by entity ID.
   */
  getObject(entityId: number): ObjectView | undefined {
    return this.objects.get(entityId)
  }

  getCarryVisualCarrierId(objectId: number): number | null {
    return this.carriedByByObject.get(objectId) ?? null
  }

  /**
   * Find entity at screen coordinates.
   * Returns { entityId, typeId } if found, null otherwise.
   */
  getEntityAtScreen(
    screenX: number,
    screenY: number,
    screenToWorld: (x: number, y: number) => { x: number; y: number }
  ): { entityId: number; typeId: number } | null {
    return this.pickEntityAtScreen(screenX, screenY, screenToWorld)
  }

  getHoverEntityAtScreen(
    screenX: number,
    screenY: number,
    screenToWorld: (x: number, y: number) => { x: number; y: number }
  ): { entityId: number; typeId: number } | null {
    return this.pickEntityAtScreen(screenX, screenY, screenToWorld)
  }

  updateHover(
    screenX: number,
    screenY: number,
    screenToWorld: (x: number, y: number) => { x: number; y: number }
  ): void {
    const hovered = this.getHoverEntityAtScreen(screenX, screenY, screenToWorld)
    const nextHoveredId = hovered?.entityId ?? null
    if (nextHoveredId === this.hoveredEntityId) {
      if (nextHoveredId !== null) {
        this.objects.get(nextHoveredId)?.setHovered(true)
      }
      return
    }

    if (this.hoveredEntityId !== null) {
      this.objects.get(this.hoveredEntityId)?.setHovered(false)
    }

    this.hoveredEntityId = nextHoveredId

    if (this.hoveredEntityId !== null) {
      this.objects.get(this.hoveredEntityId)?.setHovered(true)
    }
  }

  clearHover(): void {
    if (this.hoveredEntityId !== null) {
      this.objects.get(this.hoveredEntityId)?.setHovered(false)
    }
    this.hoveredEntityId = null
  }

  private pickEntityAtScreen(
    screenX: number,
    screenY: number,
    screenToWorld: (x: number, y: number) => { x: number; y: number }
  ): { entityId: number; typeId: number } | null {
    const visibleIds = cullingController.getVisibleObjectIds()

    const candidates: Array<{ entityId: number; objectView: ObjectView }> = []

    if (visibleIds.length > 0) {
      for (const entityId of visibleIds) {
        const objectView = this.objects.get(entityId)
        if (!objectView) continue
        candidates.push({ entityId, objectView })
      }
    } else {
      for (const [entityId, objectView] of this.objects) {
        candidates.push({ entityId, objectView })
      }
    }

    candidates.sort((left, right) => {
      const zDiff = right.objectView.getContainer().zIndex - left.objectView.getContainer().zIndex
      if (zDiff !== 0) return zDiff
      return right.entityId - left.entityId
    })

    for (const candidate of candidates) {
      if (candidate.objectView.hitTestRmbScreenPoint(screenX, screenY, screenToWorld)) {
        return { entityId: candidate.entityId, typeId: candidate.objectView.typeId }
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
    if (this.animatedObjectIds.size > 0) {
      const nowMs = typeof performance !== 'undefined' ? performance.now() : Date.now()
      const staleAnimatedIds: number[] = []
      for (const entityId of this.animatedObjectIds) {
        const objectView = this.objects.get(entityId)
        if (!objectView) {
          staleAnimatedIds.push(entityId)
          continue
        }
        objectView.updateAnimation(nowMs)
      }
      for (const entityId of staleAnimatedIds) {
        this.animatedObjectIds.delete(entityId)
      }
    }

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
        const zIndexOverride = obj.getZIndexOverride()
        obj.getContainer().zIndex = zIndexOverride ?? (TERRAIN_BASE_Z_INDEX + obj.getContainer().y)
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
    this.clearHover()

    // Unregister all objects from culling
    for (const entityId of this.objects.keys()) {
      cullingController.unregisterObject(entityId)
    }
    for (const obj of this.objects.values()) {
      this.parentContainer?.removeChild(obj.getContainer())
      obj.destroy()
    }
    this.objects.clear()
    this.animatedObjectIds.clear()
    this.carriedByByObject.clear()
    this.carriedObjectsByCarrier.clear()
    this.activeCarriedObjects.clear()
  }

  /**
   * Destroy the manager and all objects.
   */
  destroy(): void {
    this.clear()
    this.parentContainer = null
  }
}
