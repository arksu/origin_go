import { TerrainGenerator } from './TerrainGenerator'
import type { TerrainConfig } from './types'

const terrainGenerators: Map<number, TerrainGenerator> = new Map()

export function registerTerrain(tileType: number, config: TerrainConfig): void {
  terrainGenerators.set(tileType, new TerrainGenerator(config))
}

export function getTerrainGenerator(tileType: number): TerrainGenerator | undefined {
  return terrainGenerators.get(tileType)
}

export function hasTerrainGenerator(tileType: number): boolean {
  return terrainGenerators.has(tileType)
}

export function clearTerrainRegistry(): void {
  terrainGenerators.clear()
}
