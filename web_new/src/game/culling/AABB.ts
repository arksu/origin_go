/**
 * Axis-Aligned Bounding Box for viewport culling.
 * All coordinates are in local container space (screen-like coordinates after isometric projection).
 */
export interface AABB {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

/**
 * Check if two AABBs intersect.
 */
export function intersects(a: AABB, b: AABB): boolean {
  return a.minX <= b.maxX && a.maxX >= b.minX && a.minY <= b.maxY && a.maxY >= b.minY
}

/**
 * Expand AABB by dx/dy in all directions.
 */
export function expand(rect: AABB, dx: number, dy: number): AABB {
  return {
    minX: rect.minX - dx,
    minY: rect.minY - dy,
    maxX: rect.maxX + dx,
    maxY: rect.maxY + dy,
  }
}

/**
 * Create AABB from center point and half-sizes.
 */
export function fromCenter(cx: number, cy: number, halfWidth: number, halfHeight: number): AABB {
  return {
    minX: cx - halfWidth,
    minY: cy - halfHeight,
    maxX: cx + halfWidth,
    maxY: cy + halfHeight,
  }
}

/**
 * Create AABB from min/max points.
 */
export function fromMinMax(minX: number, minY: number, maxX: number, maxY: number): AABB {
  return { minX, minY, maxX, maxY }
}

/**
 * Create AABB from an array of points (computes bounding box).
 */
export function fromPoints(points: { x: number; y: number }[]): AABB {
  if (points.length === 0) {
    return { minX: 0, minY: 0, maxX: 0, maxY: 0 }
  }

  let minX = points[0]!.x
  let minY = points[0]!.y
  let maxX = points[0]!.x
  let maxY = points[0]!.y

  for (let i = 1; i < points.length; i++) {
    const p = points[i]!
    if (p.x < minX) minX = p.x
    if (p.y < minY) minY = p.y
    if (p.x > maxX) maxX = p.x
    if (p.y > maxY) maxY = p.y
  }

  return { minX, minY, maxX, maxY }
}

/**
 * Get width of AABB.
 */
export function width(rect: AABB): number {
  return rect.maxX - rect.minX
}

/**
 * Get height of AABB.
 */
export function height(rect: AABB): number {
  return rect.maxY - rect.minY
}

/**
 * Check if point is inside AABB.
 */
export function containsPoint(rect: AABB, x: number, y: number): boolean {
  return x >= rect.minX && x <= rect.maxX && y >= rect.minY && y <= rect.maxY
}
