import type { proto } from '@/network/proto/packets.js'
import type { ScreenPoint } from '../types'

const FULL_TURN = 2 * Math.PI
const ARC_SEGMENTS = 32

export interface DirectionAimState {
  actionId: string
  streamEpoch: number
}

export interface DirectionActionInput {
  aimAngle: number
  streamEpoch: number
}

export interface DirectionSector {
  range: number
  angle: number
}

export function getDirectionSector(action: proto.IActionDefinition | undefined): DirectionSector | null {
  if (action?.targetKind !== 'direction') return null
  const range = action.sector?.range
  const angle = action.sector?.sectorAngle
  if (typeof range !== 'number' || !Number.isFinite(range) || range <= 0 ||
      typeof angle !== 'number' || !Number.isFinite(angle) || angle <= 0 || angle > Math.fround(FULL_TURN)) return null
  // A full circle can round slightly above 2*pi in the protocol float.
  return { range, angle: Math.min(angle, FULL_TURN) }
}

export function normalizeAimAngle(angle: number): number {
  if (!Number.isFinite(angle)) return 0
  const normalized = ((angle % FULL_TURN) + FULL_TURN) % FULL_TURN
  return normalized === 0 ? 0 : normalized
}

export function resolveDirectionAimAngle(origin: ScreenPoint, pointer: ScreenPoint | null, fallback: number): number {
  if (!pointer || !Number.isFinite(pointer.x) || !Number.isFinite(pointer.y)) return normalizeAimAngle(fallback)
  const deltaX = pointer.x - origin.x
  const deltaY = pointer.y - origin.y
  if (deltaX === 0 && deltaY === 0) return normalizeAimAngle(fallback)
  return normalizeAimAngle(Math.atan2(deltaY, deltaX))
}

export function directionSectorPoints(origin: ScreenPoint, direction: number, sector: DirectionSector): ScreenPoint[] {
  const points = [{ x: origin.x, y: origin.y }]
  const startAngle = direction - sector.angle / 2
  for (let index = 0; index <= ARC_SEGMENTS; index++) {
    const angle = startAngle + sector.angle * index / ARC_SEGMENTS
    points.push({ x: origin.x + Math.cos(angle) * sector.range, y: origin.y + Math.sin(angle) * sector.range })
  }
  return points
}
