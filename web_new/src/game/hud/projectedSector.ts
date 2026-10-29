import { directionSectorPoints, type DirectionSector } from './directionAim'
import { coordGame2Screen } from '../utils/coordConvert'

/** Build once at the local origin; following the actor only changes graphics position. */
export function projectSectorPoints(points: number[], direction: number, sector: DirectionSector): void {
  points.length = 0
  for (const point of directionSectorPoints({ x: 0, y: 0 }, direction, sector)) {
    const projected = coordGame2Screen(point.x, point.y)
    points.push(projected.x, projected.y)
  }
}
