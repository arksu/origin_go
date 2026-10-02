import { TILE_VOID } from '../tiles/tileIds'
import { MINIMAP_FALLBACK_COLORS, MINIMAP_UNKNOWN_COLOR } from './palette'
import type { MinimapAtlas, MinimapFrame, MinimapPixels } from './types'

export function nonnegativeModulo(coordinate: number, size: number): number {
  return ((coordinate % size) + size) % size
}

/** All chunk references stay on this synchronous stack and are never cached. */
export function rasterizeMinimap(
  output: MinimapPixels,
  frame: MinimapFrame,
  originTileX: number,
  originTileY: number,
  atlas: MinimapAtlas | null,
  reportUnknownTile: (tileId: number) => void,
): void {
  output.data.fill(0)
  const { chunkSize } = frame
  const endTileX = originTileX + output.width
  const endTileY = originTileY + output.height
  for (let chunkY = Math.floor(originTileY / chunkSize); chunkY < Math.ceil(endTileY / chunkSize); chunkY++) {
    for (let chunkX = Math.floor(originTileX / chunkSize); chunkX < Math.ceil(endTileX / chunkSize); chunkX++) {
      const chunk = frame.getChunk(chunkX, chunkY)
      if (!chunk) continue
      const chunkTileX = chunkX * chunkSize
      const chunkTileY = chunkY * chunkSize
      for (let tileY = Math.max(originTileY, chunkTileY); tileY < Math.min(endTileY, chunkTileY + chunkSize); tileY++) {
        for (let tileX = Math.max(originTileX, chunkTileX); tileX < Math.min(endTileX, chunkTileX + chunkSize); tileX++) {
          const tileId = chunk.tiles[(tileY - chunkTileY) * chunkSize + tileX - chunkTileX]
          if (tileId === undefined || tileId === TILE_VOID) continue
          const destination = ((tileY - originTileY) * output.width + tileX - originTileX) * 4
          const region = atlas?.regions.get(tileId)
          if (region && atlas) {
            const sampleX = region.x + nonnegativeModulo(tileX, region.width)
            const sampleY = region.y + nonnegativeModulo(tileY, region.height)
            const source = (sampleY * atlas.width + sampleX) * 4
            output.data[destination] = atlas.data[source]!
            output.data[destination + 1] = atlas.data[source + 1]!
            output.data[destination + 2] = atlas.data[source + 2]!
          } else {
            const color = MINIMAP_FALLBACK_COLORS[tileId] ?? MINIMAP_UNKNOWN_COLOR
            if (color === MINIMAP_UNKNOWN_COLOR) reportUnknownTile(tileId)
            output.data[destination] = color[0]
            output.data[destination + 1] = color[1]
            output.data[destination + 2] = color[2]
          }
          output.data[destination + 3] = 255
        }
      }
    }
  }
}
