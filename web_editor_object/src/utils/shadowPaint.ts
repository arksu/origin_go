export interface PaintStrokeParams {
  width: number
  height: number
  radius: number
  alpha: number
  from: { x: number; y: number }
  to: { x: number; y: number }
  visited: Set<number>
}

function paintCircle(
  pixels: Uint8ClampedArray,
  width: number,
  height: number,
  cx: number,
  cy: number,
  radius: number,
  alpha: number,
  visited: Set<number>,
): void {
  const minX = Math.max(0, Math.floor(cx - radius))
  const maxX = Math.min(width - 1, Math.ceil(cx + radius))
  const minY = Math.max(0, Math.floor(cy - radius))
  const maxY = Math.min(height - 1, Math.ceil(cy + radius))
  const r2 = radius * radius

  for (let y = minY; y <= maxY; y++) {
    for (let x = minX; x <= maxX; x++) {
      const dx = x - cx
      const dy = y - cy
      if (dx * dx + dy * dy > r2) continue

      const idx = y * width + x
      if (visited.has(idx)) continue
      visited.add(idx)

      const p = idx * 4
      pixels[p] = 0
      pixels[p + 1] = 0
      pixels[p + 2] = 0
      pixels[p + 3] = alpha
    }
  }
}

export function paintStrokeConstantAlpha(
  pixels: Uint8ClampedArray,
  params: PaintStrokeParams,
): void {
  const { width, height, radius, alpha, from, to, visited } = params
  const dx = to.x - from.x
  const dy = to.y - from.y
  const distance = Math.hypot(dx, dy)
  const steps = Math.max(1, Math.ceil(distance / Math.max(1, radius * 0.4)))

  for (let i = 0; i <= steps; i++) {
    const t = i / steps
    const x = from.x + dx * t
    const y = from.y + dy * t
    paintCircle(pixels, width, height, x, y, radius, alpha, visited)
  }
}

export function dataUrlToPngBase64(dataUrl: string): string {
  const prefix = 'data:image/png;base64,'
  if (!dataUrl.startsWith(prefix)) {
    throw new Error('Expected PNG data URL')
  }
  return dataUrl.slice(prefix.length)
}
