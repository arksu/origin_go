import type { TileSet } from '../src/game/tiles/TileSet'

export type PieceKind = 'border' | 'corner'
export interface TransitionPiece {
  kind: PieceKind
  mask: number
  variant: number
  texture: string
}

// Same bit order as BX/BY and CX/CY in Chunk. Coordinates are world tile axes.
export const PIECE_OFFSETS = {
  border: [[-1, 0], [0, -1], [1, 0], [0, 1]],
  corner: [[-1, -1], [-1, 1], [1, 1], [1, -1]],
} as const

export function transitionPieces(set: TileSet): TransitionPiece[] {
  const result: TransitionPiece[] = []
  for (const kind of ['border', 'corner'] as const) {
    const arrays = kind === 'border' ? set.borders : set.corners
    arrays.forEach((array, index) => {
      array.getTextureNames().forEach((texture, variant) => {
        result.push({ kind, mask: index + 1, variant: variant + 1, texture })
      })
    })
  }
  return result
}

/** An isolated 3x3 neighborhood that requests exactly this mask at its center. */
export function pieceNeighborhood(kind: PieceKind, mask: number, overlay: number, base: number, size: number): Uint8Array {
  const tiles = new Uint8Array(size * size).fill(base)
  const center = Math.floor(size / 2)
  PIECE_OFFSETS[kind].forEach(([dx, dy], bit) => {
    if (mask & (1 << bit)) tiles[(center + dy) * size + center + dx] = overlay
  })
  return tiles
}
