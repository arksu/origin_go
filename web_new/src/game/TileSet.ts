import { getRandomByCoord } from './utils/random'

interface TileVariant {
  img: string
  w: number
}

export class TileArray {
  private tiles: TileVariant[]
  private totalWeight: number

  constructor(list: TileVariant[]) {
    this.tiles = list
    this.totalWeight = 0
    for (const tile of list) {
      this.totalWeight += tile.w
    }
  }

  get(seed: number): string | null {
    if (this.totalWeight === 0) return null

    let w = seed % this.totalWeight
    for (const tile of this.tiles) {
      w -= tile.w
      if (w < 0) {
        return tile.img
      }
    }
    return this.tiles[0]?.img ?? null
  }
}

export class TileSet {
  readonly ground: TileArray
  readonly borders: TileArray[]
  readonly corners: TileArray[]

  constructor(data: { ground: TileVariant[]; borders: TileVariant[][]; corners: TileVariant[][] }) {
    this.ground = new TileArray(data.ground)
    this.borders = data.borders.map((b) => new TileArray(b))
    this.corners = data.corners.map((c) => new TileArray(c))
  }

  getGroundTexture(x: number, y: number): string | null {
    return this.ground.get(getRandomByCoord(x, y))
  }

  getBorderTexture(borderMask: number, x: number, y: number): string | null {
    if (borderMask <= 0 || borderMask > this.borders.length) return null
    return this.borders[borderMask - 1]?.get(getRandomByCoord(x, y)) ?? null
  }

  getCornerTexture(cornerMask: number, x: number, y: number): string | null {
    if (cornerMask <= 0 || cornerMask > this.corners.length) return null
    return this.corners[cornerMask - 1]?.get(getRandomByCoord(x, y)) ?? null
  }
}

const tileSets: Map<number, TileSet> = new Map()

export function registerTileSet(tileType: number, data: { ground: TileVariant[]; borders: TileVariant[][]; corners: TileVariant[][] }): void {
  tileSets.set(tileType, new TileSet(data))
}

export function getTileSet(tileType: number): TileSet | undefined {
  return tileSets.get(tileType)
}

export function getGroundTextureName(tileType: number, x: number, y: number): string | null {
  const set = tileSets.get(tileType)
  return set?.getGroundTexture(x, y) ?? null
}

export function hasTileSet(tileType: number): boolean {
  return tileSets.has(tileType)
}
