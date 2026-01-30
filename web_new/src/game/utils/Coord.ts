export interface Coord {
  x: number
  y: number
}

export function coord(x: number, y: number): Coord {
  return { x, y }
}
