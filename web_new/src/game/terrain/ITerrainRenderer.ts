import type { TerrainDrawCmd, TerrainRenderContext } from './types'

export interface ITerrainRenderer {
  setCurrentChunk(chunkKey: string): void
  addTile(cmds: TerrainDrawCmd[], context: TerrainRenderContext): void
  finalize(): void
  destroy(): void
  getTerrainSpritesForChunk(chunkKey: string): unknown[]
  clearChunk(chunkKey: string): void
}
