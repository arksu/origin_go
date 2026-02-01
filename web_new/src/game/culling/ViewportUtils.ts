/**
 * Viewport utilities for culling calculations.
 * Provides functions to compute viewport rect in local container coordinates
 * and convert tile-based margins to local units.
 */

import { Container, Point } from 'pixi.js'
import { type AABB, fromPoints, expand } from './AABB'
import { TILE_WIDTH_HALF, TILE_HEIGHT_HALF } from '../tiles/Tile'

/**
 * Get viewport rectangle in local coordinates of a container.
 * Transforms screen corners (0,0), (w,0), (w,h), (0,h) to container local space.
 */
export function getViewportRectLocal(
  container: Container,
  screenWidth: number,
  screenHeight: number,
): AABB {
  const corners = [
    new Point(0, 0),
    new Point(screenWidth, 0),
    new Point(screenWidth, screenHeight),
    new Point(0, screenHeight),
  ]

  const localCorners = corners.map((corner) => {
    const local = container.toLocal(corner)
    return { x: local.x, y: local.y }
  })

  return fromPoints(localCorners)
}

/**
 * Convert tile margin to local coordinate margin.
 * For isometric projection, we use conservative expansion based on tile dimensions.
 * 
 * @param tiles - Number of tiles for margin
 * @returns dx, dy expansion values in local units
 */
export function tilesToLocalMargin(tiles: number): { dx: number; dy: number } {
  // Conservative expansion: full tile width/height per tile of margin
  // This ensures we don't accidentally cull visible content
  const dx = tiles * TILE_WIDTH_HALF * 2 // Full tile width
  const dy = tiles * TILE_HEIGHT_HALF * 2 // Full tile height
  return { dx, dy }
}

/**
 * Compute cull rect from viewport rect with tile-based margin.
 */
export function getCullRect(viewportRect: AABB, marginTiles: number): AABB {
  const { dx, dy } = tilesToLocalMargin(marginTiles)
  return expand(viewportRect, dx, dy)
}

/**
 * Compute enter/exit rects for hysteresis culling.
 * Enter rect is larger (objects become visible when entering).
 * Exit rect is smaller (objects become invisible only when fully outside).
 */
export function getHysteresisRects(
  viewportRect: AABB,
  enterMarginTiles: number,
  exitMarginTiles: number,
): { enterRect: AABB; exitRect: AABB } {
  const enterMargin = tilesToLocalMargin(enterMarginTiles)
  const exitMargin = tilesToLocalMargin(exitMarginTiles)

  return {
    enterRect: expand(viewportRect, enterMargin.dx, enterMargin.dy),
    exitRect: expand(viewportRect, exitMargin.dx, exitMargin.dy),
  }
}
