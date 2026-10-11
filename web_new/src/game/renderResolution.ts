/** Canvas density is a presentation setting; logical coordinates stay unchanged. */
export function resolveRenderResolution(economy: boolean, devicePixelRatio: number): number {
  if (economy || !Number.isFinite(devicePixelRatio) || devicePixelRatio <= 0) return 1
  return Math.min(devicePixelRatio, 2)
}
