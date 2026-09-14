import type { Sprite } from 'pixi.js'
import type { TerrainDrawCmd } from './types'

export interface ITerrainRenderer {
  setCurrentSubchunk(subchunkKey: string): void
  addTile(cmds: TerrainDrawCmd[]): void
  finalize(): void
  destroy(): void
  clearSubchunk(subchunkKey: string): void
  clearChunk(chunkKey: string): void
  hideSubchunk(subchunkKey: string): void
  hasSubchunk(subchunkKey: string): boolean
  getSpritesForSubchunk(subchunkKey: string): Sprite[]
  getActiveSpritesCount(): number
}
