import { getRandomByCoord } from '../utils/random'
import { TILE_HEIGHT_HALF, TILE_WIDTH_HALF } from '../tiles/Tile'
import type { TerrainConfig, TerrainDrawCmd, TerrainVariant, TerrainLayer } from './types'

export class TerrainGenerator {
  private variants: TerrainVariant[]

  constructor(config: TerrainConfig) {
    this.variants = config
  }

  generate(tileX: number, tileY: number, anchorScreenX: number, anchorScreenY: number): TerrainDrawCmd[] | null {
    if (this.variants.length === 0) {
      return null
    }

    let baseSeed = getRandomByCoord(tileX, tileY)

    for (const variant of this.variants) {
      if (baseSeed % variant.chance === 0) {
        const cmds = this.generateVariant(variant, tileX, tileY, anchorScreenX, anchorScreenY)
        if (cmds) {
          return cmds
        }
      }
      baseSeed = getRandomByCoord(tileX, tileY, 1000, baseSeed)
    }

    return null
  }

  private generateVariant(
    variant: TerrainVariant,
    tileX: number,
    tileY: number,
    anchorScreenX: number,
    anchorScreenY: number,
  ): TerrainDrawCmd[] | null {
    const layers = variant.layers
    if (layers.length === 0) {
      return null
    }

    const cmds: TerrainDrawCmd[] = []
    let seed = getRandomByCoord(tileX, tileY)

    const jitterSeedX = getRandomByCoord(tileX, tileY, 1001, seed)
    const jitterSeedY = getRandomByCoord(tileX, tileY, 1002, jitterSeedX)
    const jitterX = ((jitterSeedX % 1000) / 1000 - 0.5) * TILE_WIDTH_HALF
    const jitterY = ((jitterSeedY % 1000) / 1000 - 0.5) * TILE_HEIGHT_HALF

    const jitteredAnchorX = anchorScreenX + jitterX
    const jitteredAnchorY = anchorScreenY + jitterY
    let hasShadow = false
    let attempts = 0
    const maxAttempts = 10

    let shadowCmd: TerrainDrawCmd | null = null

    do {
      cmds.length = 0

      for (let i = 0; i < layers.length; i++) {
        const layer = layers[i]!
        if (layer.p === 0) {
          if (layers.length > 1) {
            hasShadow = true
            shadowCmd = this.createDrawCmd(layer, variant, jitteredAnchorX, jitteredAnchorY)
          } else if (layers.length === 1) {
            cmds.push(this.createDrawCmd(layer, variant, jitteredAnchorX, jitteredAnchorY))
          }
          continue
        }

        seed = getRandomByCoord(tileX, tileY, i, seed)

        const shouldGenerate = (seed % layer.p) === 0

        if (shouldGenerate) {
          const cmd = this.createDrawCmd(layer, variant, jitteredAnchorX, jitteredAnchorY)
          cmds.push(cmd)
        }
      }

      attempts++
    } while (cmds.length < 1 && attempts < maxAttempts)

    if (hasShadow && cmds.length > 0) {
      // set shadowCmd at start of cmds
      cmds.unshift(shadowCmd!)
    }

    return cmds.length > 0 ? cmds : null
  }

  private createDrawCmd(
    layer: TerrainLayer,
    variant: TerrainVariant,
    anchorScreenX: number,
    anchorScreenY: number,
  ): TerrainDrawCmd {
    const dx = -(variant.offset[0] ?? 0) + (layer.offset[0] ?? 0)
    const dy = -(variant.offset[1] ?? 0) + (layer.offset[1] ?? 0)

    return {
      textureFrameId: layer.img,
      x: anchorScreenX + dx,
      y: anchorScreenY + dy,
      zOffset: layer.z ?? 0,
    }
  }
}
