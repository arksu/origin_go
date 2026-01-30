export const TEXTURE_WIDTH = 64
export const TEXTURE_HEIGHT = 32

export const TILE_WIDTH_HALF = TEXTURE_WIDTH / 2
export const TILE_HEIGHT_HALF = TEXTURE_HEIGHT / 2

let coordPerTile = 32
let chunkSize = 128

export function setWorldParams(newCoordPerTile: number, newChunkSize: number): void {
  coordPerTile = newCoordPerTile
  chunkSize = newChunkSize
}

export function getCoordPerTile(): number {
  return coordPerTile
}

export function getChunkSize(): number {
  return chunkSize
}

export function getFullChunkSize(): number {
  return chunkSize * coordPerTile
}
