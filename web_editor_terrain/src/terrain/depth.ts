/**
 * Runtime terrain sprites use the lower half of their tile as the default
 * ground-contact depth. Keep this in step with web_new/src/game/tiles/Tile.ts.
 */
export const TERRAIN_DEFAULT_DEPTH_Y = 16

export function deriveTerrainLayerZ(
  variantOffsetY: number | undefined,
  layerOffsetY: number | undefined,
  editorOffsetY: number,
  textureHeight: number,
): number {
  if (!Number.isFinite(textureHeight) || textureHeight <= 0) {
    throw new Error(`Terrain layer texture height must be a positive number, got ${textureHeight}`)
  }

  const visualTopY = -(variantOffsetY ?? 0) + (layerOffsetY ?? 0) + editorOffsetY
  const spriteBottomZ = Math.round(visualTopY + textureHeight - TERRAIN_DEFAULT_DEPTH_Y)

  // Terrain decals do not occlude actors on their own tile. World tile depth
  // still controls which decals are in front of actors on other tiles.
  return Math.min(0, spriteBottomZ)
}
