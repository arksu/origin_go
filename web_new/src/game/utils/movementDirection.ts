import { coordScreen2Game } from './coordConvert'
import type { Coord } from './Coord'

export function screenMovementDirection(screenX: number, screenY: number): Coord {
  const world = coordScreen2Game(screenX, screenY)
  const length = Math.hypot(world.x, world.y)
  if (length === 0) return { x: 0, y: 0 }
  // Store the wire values so every refresh is identical after float serialization.
  return { x: Math.fround(world.x / length), y: Math.fround(world.y / length) }
}
