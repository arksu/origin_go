import { Point, Sprite, Texture, groupD8 } from 'pixi.js'
import { RMB_ALPHA_MASK_CACHE_MAX_BYTES } from '@/constants/render'

type AlphaMaskEntry = {
  width: number
  height: number
  resolution: number
  bits: Uint8Array
  bytes: number
}

export type SpriteAlphaMask = Readonly<AlphaMaskEntry>

type AlphaMaskStats = {
  entries: number
  bytes: number
  hits: number
  misses: number
  evictions: number
}

const alphaMaskCache = new Map<string, AlphaMaskEntry>()
const failedMaskKeys = new Set<string>()

let cacheBytes = 0
let cacheHits = 0
let cacheMisses = 0
let cacheEvictions = 0

let extractCanvas: HTMLCanvasElement | null = null
let extractContext: CanvasRenderingContext2D | null = null

const reusablePoint = new Point()

function getResolution(texture: Texture): number {
  const source = texture.source as { _resolution?: number; resolution?: number }
  return source._resolution ?? source.resolution ?? 1
}

function getTextureCacheKey(texture: Texture): string {
  const frame = texture.frame
  const orig = texture.orig
  const trim = texture.trim
  const resolution = getResolution(texture)

  const trimKey = trim
    ? `${trim.x},${trim.y},${trim.width},${trim.height}`
    : 'none'

  return [
    texture.source.uid,
    texture.rotate,
    frame.x, frame.y, frame.width, frame.height,
    orig.x, orig.y, orig.width, orig.height,
    trimKey,
    resolution,
  ].join('|')
}

function getExtractContext(): CanvasRenderingContext2D | null {
  if (!extractCanvas) {
    extractCanvas = document.createElement('canvas')
    extractContext = extractCanvas.getContext('2d', { willReadFrequently: true })
  }
  return extractContext
}

function asCanvasImageSource(value: unknown): CanvasImageSource | null {
  if (!value) return null
  return value as CanvasImageSource
}

function applyInverseRotation(
  context: CanvasRenderingContext2D,
  rotate: number,
  srcWidth: number,
  srcHeight: number
): void {
  const inv = groupD8.inv(rotate)
  const a = groupD8.uX(inv)
  const b = groupD8.uY(inv)
  const c = groupD8.vX(inv)
  const d = groupD8.vY(inv)
  const tx = -Math.min(0, a * srcWidth, c * srcHeight, a * srcWidth + c * srcHeight)
  const ty = -Math.min(0, b * srcWidth, d * srcHeight, b * srcWidth + d * srcHeight)
  context.transform(a, b, c, d, tx, ty)
}

function setBit(bits: Uint8Array, bitIndex: number): void {
  const byteIndex = bitIndex >> 3
  bits[byteIndex] = (bits[byteIndex] ?? 0) | (1 << (bitIndex & 7))
}

function getBit(bits: Uint8Array, bitIndex: number): boolean {
  const byteIndex = bitIndex >> 3
  return (((bits[byteIndex] ?? 0) & (1 << (bitIndex & 7))) !== 0)
}

function evictIfNeeded(): void {
  while (cacheBytes > RMB_ALPHA_MASK_CACHE_MAX_BYTES && alphaMaskCache.size > 0) {
    const oldestKey = alphaMaskCache.keys().next().value as string | undefined
    if (!oldestKey) break
    const entry = alphaMaskCache.get(oldestKey)
    alphaMaskCache.delete(oldestKey)
    if (entry) {
      cacheBytes -= entry.bytes
      cacheEvictions++
    }
  }
}

function touchLru(key: string, entry: AlphaMaskEntry): void {
  alphaMaskCache.delete(key)
  alphaMaskCache.set(key, entry)
}

function buildAlphaMask(texture: Texture): AlphaMaskEntry | null {
  const context = getExtractContext()
  if (!context) return null

  const source = asCanvasImageSource((texture.source as { resource?: unknown }).resource)
  if (!source) return null

  const resolution = getResolution(texture)
  const frame = texture.frame
  const orig = texture.orig
  const trim = texture.trim

  const cropX = Math.round(frame.x * resolution)
  const cropY = Math.round(frame.y * resolution)
  const cropWidth = Math.max(1, Math.round(frame.width * resolution))
  const cropHeight = Math.max(1, Math.round(frame.height * resolution))

  const rotate = texture.rotate
  const isVertical = rotate !== 0 && groupD8.isVertical(rotate)
  const outWidth = isVertical ? cropHeight : cropWidth
  const outHeight = isVertical ? cropWidth : cropHeight

  extractCanvas!.width = outWidth
  extractCanvas!.height = outHeight
  context.clearRect(0, 0, outWidth, outHeight)

  context.save()
  if (rotate) {
    applyInverseRotation(context, rotate, cropWidth, cropHeight)
  }
  try {
    context.drawImage(source, cropX, cropY, cropWidth, cropHeight, 0, 0, cropWidth, cropHeight)
  } catch {
    context.restore()
    return null
  }
  context.restore()

  let imageData: ImageData
  try {
    imageData = context.getImageData(0, 0, outWidth, outHeight)
  } catch {
    return null
  }

  const width = Math.max(1, Math.round(orig.width * resolution))
  const height = Math.max(1, Math.round(orig.height * resolution))
  const totalPixels = width * height
  const bits = new Uint8Array(Math.ceil(totalPixels / 8))

  const trimX = trim ? Math.round(trim.x * resolution) : 0
  const trimY = trim ? Math.round(trim.y * resolution) : 0
  const trimW = trim ? Math.round(trim.width * resolution) : outWidth
  const trimH = trim ? Math.round(trim.height * resolution) : outHeight
  const copyWidth = Math.min(outWidth, trimW)
  const copyHeight = Math.min(outHeight, trimH)

  const rgba = imageData.data
  for (let y = 0; y < copyHeight; y++) {
    const targetY = trimY + y
    if (targetY < 0 || targetY >= height) continue
    for (let x = 0; x < copyWidth; x++) {
      const targetX = trimX + x
      if (targetX < 0 || targetX >= width) continue
      const alpha = rgba[(y * outWidth + x) * 4 + 3] ?? 0
      if (alpha > 0) {
        setBit(bits, targetY * width + targetX)
      }
    }
  }

  return {
    width,
    height,
    resolution,
    bits,
    bytes: bits.byteLength,
  }
}

function getMask(texture: Texture): AlphaMaskEntry | null {
  const key = getTextureCacheKey(texture)
  const cached = alphaMaskCache.get(key)
  if (cached) {
    cacheHits++
    touchLru(key, cached)
    return cached
  }

  if (failedMaskKeys.has(key)) {
    cacheMisses++
    return null
  }

  cacheMisses++
  const built = buildAlphaMask(texture)
  if (!built) {
    failedMaskKeys.add(key)
    return null
  }

  alphaMaskCache.set(key, built)
  cacheBytes += built.bytes
  evictIfNeeded()
  return built
}

export function getSpriteAlphaMask(sprite: Sprite): SpriteAlphaMask | null {
  const texture = sprite.texture
  if (!texture || texture === Texture.EMPTY) {
    return null
  }
  return getMask(texture)
}

export function hitTestSpritePixel(
  sprite: Sprite,
  screenX: number,
  screenY: number,
  // Bitmask is binary alpha (> 0), threshold is kept for API compatibility.
  _alphaThreshold: number,
): boolean {
  const bounds = sprite.getBounds()
  if (
    screenX < bounds.minX ||
    screenX > bounds.maxX ||
    screenY < bounds.minY ||
    screenY > bounds.maxY
  ) {
    return false
  }

  const texture = sprite.texture
  if (!texture || texture === Texture.EMPTY) {
    return false
  }

  const mask = getMask(texture)
  if (!mask) {
    return false
  }

  const local = sprite.toLocal({ x: screenX, y: screenY }, undefined, reusablePoint, true)
  const orig = texture.orig

  const localOrigX = local.x + sprite.anchor.x * orig.width
  const localOrigY = local.y + sprite.anchor.y * orig.height

  if (
    localOrigX < 0 ||
    localOrigY < 0 ||
    localOrigX >= orig.width ||
    localOrigY >= orig.height
  ) {
    return false
  }

  const px = Math.floor(localOrigX * mask.resolution)
  const py = Math.floor(localOrigY * mask.resolution)

  if (px < 0 || py < 0 || px >= mask.width || py >= mask.height) {
    return false
  }

  return getBit(mask.bits, py * mask.width + px)
}

export function clearAlphaMaskCache(): void {
  alphaMaskCache.clear()
  failedMaskKeys.clear()
  cacheBytes = 0
  cacheHits = 0
  cacheMisses = 0
  cacheEvictions = 0
}

export function getAlphaMaskCacheStats(): AlphaMaskStats {
  return {
    entries: alphaMaskCache.size,
    bytes: cacheBytes,
    hits: cacheHits,
    misses: cacheMisses,
    evictions: cacheEvictions,
  }
}
