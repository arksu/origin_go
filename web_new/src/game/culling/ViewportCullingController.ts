/**
 * ViewportCullingController - единый контроллер для culling subchunks, terrain и objects.
 * 
 * Responsibilities:
 * - Compute viewportRectLocal and cullRectLocal for map-layer and objects-layer
 * - Cull subchunks (map-layer)
 * - Cull terrain sprites (objects-layer)
 * - Cull objects (objects-layer) + toggle interactivity
 * - Collect metrics for debug overlay
 */

import { Container, Application } from 'pixi.js'
import { type AABB, intersects } from './AABB'
import { getViewportRectLocal, getCullRect, getHysteresisRects } from './ViewportUtils'

export interface CullingMetrics {
  subchunksTotal: number
  subchunksVisible: number
  subchunksCulled: number
  terrainTotal: number
  terrainVisible: number
  terrainCulled: number
  objectsTotal: number
  objectsVisible: number
  objectsCulled: number
  cullingTimeMs: number
  marginTiles: number
}

export interface SubchunkCullData {
  container: Container
  bounds: AABB
  wasVisible: boolean
}

export interface TerrainCullData {
  container: Container
  bounds: AABB
  wasVisible: boolean
}

export interface ObjectCullData {
  container: Container
  bounds: AABB
  wasVisible: boolean
  originalEventMode: string
}

export class ViewportCullingController {
  private marginTiles: number = 4
  private enterMarginTiles: number = 4
  private exitMarginTiles: number = 2
  private useHysteresis: boolean = true

  private subchunks: Map<string, SubchunkCullData> = new Map()
  private terrain: Map<string, TerrainCullData> = new Map()
  private objects: Map<number, ObjectCullData> = new Map()

  private lastMetrics: CullingMetrics = {
    subchunksTotal: 0,
    subchunksVisible: 0,
    subchunksCulled: 0,
    terrainTotal: 0,
    terrainVisible: 0,
    terrainCulled: 0,
    objectsTotal: 0,
    objectsVisible: 0,
    objectsCulled: 0,
    cullingTimeMs: 0,
    marginTiles: 4,
  }

  setMarginTiles(tiles: number): void {
    this.marginTiles = tiles
    this.enterMarginTiles = tiles
    this.exitMarginTiles = Math.max(0, tiles - 2)
  }

  setHysteresis(enabled: boolean, enterTiles?: number, exitTiles?: number): void {
    this.useHysteresis = enabled
    if (enterTiles !== undefined) this.enterMarginTiles = enterTiles
    if (exitTiles !== undefined) this.exitMarginTiles = exitTiles
  }

  /**
   * Register a subchunk for culling.
   */
  registerSubchunk(key: string, container: Container, bounds: AABB): void {
    this.subchunks.set(key, {
      container,
      bounds,
      wasVisible: true,
    })
  }

  /**
   * Unregister a subchunk.
   */
  unregisterSubchunk(key: string): void {
    this.subchunks.delete(key)
  }

  /**
   * Register terrain sprite for culling.
   */
  registerTerrain(key: string, container: Container, bounds: AABB): void {
    this.terrain.set(key, {
      container,
      bounds,
      wasVisible: true,
    })
  }

  /**
   * Unregister terrain.
   */
  unregisterTerrain(key: string): void {
    this.terrain.delete(key)
  }

  /**
   * Clear all terrain for a chunk.
   */
  clearTerrainForChunk(chunkKey: string): void {
    const keysToDelete: string[] = []
    for (const key of this.terrain.keys()) {
      if (key.startsWith(chunkKey + ':')) {
        keysToDelete.push(key)
      }
    }
    for (const key of keysToDelete) {
      this.terrain.delete(key)
    }
  }

  /**
   * Register an object for culling.
   */
  registerObject(entityId: number, container: Container, bounds: AABB): void {
    this.objects.set(entityId, {
      container,
      bounds,
      wasVisible: true,
      originalEventMode: (container.eventMode as string) || 'auto',
    })
  }

  /**
   * Update object bounds (when position changes).
   */
  updateObjectBounds(entityId: number, bounds: AABB): void {
    const data = this.objects.get(entityId)
    if (data) {
      data.bounds = bounds
    }
  }

  /**
   * Unregister an object.
   */
  unregisterObject(entityId: number): void {
    this.objects.delete(entityId)
  }

  /**
   * Main update method - called every frame after camera update.
   */
  update(
    app: Application,
    mapContainer: Container,
    objectsContainer: Container,
  ): void {
    const startTime = performance.now()

    const screenWidth = app.screen.width
    const screenHeight = app.screen.height

    // Compute viewport rects for both containers
    const mapViewportRect = getViewportRectLocal(mapContainer, screenWidth, screenHeight)
    const objectsViewportRect = getViewportRectLocal(objectsContainer, screenWidth, screenHeight)

    // Compute cull rects (with margin or hysteresis)
    let mapCullRect: AABB
    let objectsCullRect: AABB
    let mapEnterRect: AABB | null = null
    let mapExitRect: AABB | null = null
    let objectsEnterRect: AABB | null = null
    let objectsExitRect: AABB | null = null

    if (this.useHysteresis) {
      const mapRects = getHysteresisRects(mapViewportRect, this.enterMarginTiles, this.exitMarginTiles)
      const objectsRects = getHysteresisRects(objectsViewportRect, this.enterMarginTiles, this.exitMarginTiles)
      mapEnterRect = mapRects.enterRect
      mapExitRect = mapRects.exitRect
      objectsEnterRect = objectsRects.enterRect
      objectsExitRect = objectsRects.exitRect
      mapCullRect = mapEnterRect // Use enter rect for initial visibility check
      objectsCullRect = objectsEnterRect
    } else {
      mapCullRect = getCullRect(mapViewportRect, this.marginTiles)
      objectsCullRect = getCullRect(objectsViewportRect, this.marginTiles)
    }

    // Cull subchunks
    let subchunksVisible = 0
    let subchunksCulled = 0

    for (const [, data] of this.subchunks) {
      const shouldBeVisible = this.checkVisibility(
        data.bounds,
        data.wasVisible,
        mapCullRect,
        mapEnterRect,
        mapExitRect,
      )

      if (shouldBeVisible !== data.wasVisible) {
        data.container.visible = shouldBeVisible
        data.wasVisible = shouldBeVisible
      }

      if (shouldBeVisible) {
        subchunksVisible++
      } else {
        subchunksCulled++
      }
    }

    // Cull terrain
    let terrainVisible = 0
    let terrainCulled = 0

    for (const [, data] of this.terrain) {
      const shouldBeVisible = this.checkVisibility(
        data.bounds,
        data.wasVisible,
        objectsCullRect,
        objectsEnterRect,
        objectsExitRect,
      )

      if (shouldBeVisible !== data.wasVisible) {
        data.container.visible = shouldBeVisible
        data.wasVisible = shouldBeVisible
      }

      if (shouldBeVisible) {
        terrainVisible++
      } else {
        terrainCulled++
      }
    }

    // Cull objects
    let objectsVisibleCount = 0
    let objectsCulledCount = 0

    for (const [, data] of this.objects) {
      const shouldBeVisible = this.checkVisibility(
        data.bounds,
        data.wasVisible,
        objectsCullRect,
        objectsEnterRect,
        objectsExitRect,
      )

      if (shouldBeVisible !== data.wasVisible) {
        data.container.visible = shouldBeVisible
        // Toggle interactivity
        if (shouldBeVisible) {
          data.container.eventMode = data.originalEventMode as 'auto' | 'none' | 'passive' | 'static' | 'dynamic'
        } else {
          data.container.eventMode = 'none'
        }
        data.wasVisible = shouldBeVisible
      }

      if (shouldBeVisible) {
        objectsVisibleCount++
      } else {
        objectsCulledCount++
      }
    }

    const endTime = performance.now()

    this.lastMetrics = {
      subchunksTotal: this.subchunks.size,
      subchunksVisible,
      subchunksCulled,
      terrainTotal: this.terrain.size,
      terrainVisible,
      terrainCulled,
      objectsTotal: this.objects.size,
      objectsVisible: objectsVisibleCount,
      objectsCulled: objectsCulledCount,
      cullingTimeMs: endTime - startTime,
      marginTiles: this.marginTiles,
    }
  }

  /**
   * Check visibility with optional hysteresis.
   */
  private checkVisibility(
    bounds: AABB,
    wasVisible: boolean,
    cullRect: AABB,
    enterRect: AABB | null,
    exitRect: AABB | null,
  ): boolean {
    if (this.useHysteresis && enterRect && exitRect) {
      if (wasVisible) {
        // Was visible -> hide only when outside exit rect
        return intersects(bounds, exitRect)
      } else {
        // Was hidden -> show when inside enter rect
        return intersects(bounds, enterRect)
      }
    } else {
      return intersects(bounds, cullRect)
    }
  }

  getMetrics(): CullingMetrics {
    return this.lastMetrics
  }

  /**
   * Check if an object is currently visible (not culled).
   */
  isObjectVisible(entityId: number): boolean {
    const data = this.objects.get(entityId)
    return data ? data.wasVisible : true // Default to visible if not registered
  }

  /**
   * Get list of visible object entity IDs.
   */
  getVisibleObjectIds(): number[] {
    const result: number[] = []
    for (const [entityId, data] of this.objects) {
      if (data.wasVisible) {
        result.push(entityId)
      }
    }
    return result
  }

  clear(): void {
    this.subchunks.clear()
    this.terrain.clear()
    this.objects.clear()
  }

  destroy(): void {
    this.clear()
  }
}

export const cullingController = new ViewportCullingController()
