import { getRandomByCoord } from '../utils/random'
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

    const baseSeed = getRandomByCoord(tileX, tileY)

    for (const variant of this.variants) {
      if (baseSeed % variant.chance === 0) {
        return this.generateVariant(variant, tileX, tileY, anchorScreenX, anchorScreenY)
      }
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
    let hasShadow = false
    let attempts = 0
    const maxAttempts = 10

    do {
      cmds.length = 0

      for (let i = 0; i < layers.length; i++) {
        const layer = layers[i]!
        if (layer.p === 0) {
          hasShadow = true
        }

        seed = getRandomByCoord(tileX, tileY, i, seed)

        const shouldGenerate = (layer.p === 0 && cmds.length === 0) || (layer.p > 0 && seed % layer.p === 0)

        if (shouldGenerate) {
          const cmd = this.createDrawCmd(layer, variant, anchorScreenX, anchorScreenY)
          cmds.push(cmd)
        }
      }

      attempts++
    } while (hasShadow && cmds.length < 2 && layers.length > 1 && attempts < maxAttempts)

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
