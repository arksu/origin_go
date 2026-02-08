export interface TerrainLayer {
  img: string
  offset: number[]
  p: number
  z?: number
}

export interface TerrainVariant {
  chance: number
  offset: number[]
  layers: TerrainLayer[]
}

export type TerrainConfig = TerrainVariant[]

export interface TerrainDrawCmd {
  textureFrameId: string
  x: number
  y: number
  zOffset: number
}

export interface TerrainFileEntry {
  fileName: string
  config: TerrainConfig
}
