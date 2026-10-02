import { MINIMAP_ATLAS_URL, MINIMAP_MANIFEST_URL, MINIMAP_TEXTURE_SIZE } from '@/constants/minimap'
import { RENDERABLE_TILE_IDS } from '../tiles/tileIds'
import type { MinimapAtlas, MinimapPixels, MinimapTextureRegion } from './types'

export interface MinimapAtlasValidation {
  readonly atlas: MinimapAtlas
  readonly invalidTileIds: readonly number[]
}

function isRegion(value: unknown, pixels: MinimapPixels): value is MinimapTextureRegion {
  if (!value || typeof value !== 'object') return false
  const region = value as Partial<MinimapTextureRegion>
  return Number.isInteger(region.x) && Number.isInteger(region.y)
    && region.width === MINIMAP_TEXTURE_SIZE && region.height === MINIMAP_TEXTURE_SIZE
    && region.x! >= 0 && region.y! >= 0
    && region.x! + region.width <= pixels.width && region.y! + region.height <= pixels.height
}

function overlaps(first: MinimapTextureRegion, second: MinimapTextureRegion): boolean {
  return first.x < second.x + second.width && first.x + first.width > second.x
    && first.y < second.y + second.height && first.y + first.height > second.y
}

/** Invalid materials fall back independently, so a bad region cannot hide valid terrain. */
export function validateMinimapAtlas(manifest: unknown, pixels: MinimapPixels): MinimapAtlasValidation {
  if (!Number.isSafeInteger(pixels.width) || !Number.isSafeInteger(pixels.height)
    || pixels.width < 1 || pixels.height < 1 || pixels.data.length !== pixels.width * pixels.height * 4) {
    throw new Error('Minimap atlas has invalid pixel dimensions')
  }
  const mappings = manifest && typeof manifest === 'object' ? manifest as Record<string, unknown> : {}
  const regions = new Map<number, MinimapTextureRegion>()
  const invalid = new Set<number>()
  for (const tileId of RENDERABLE_TILE_IDS) {
    const region = mappings[String(tileId)]
    if (!isRegion(region, pixels)) {
      invalid.add(tileId)
      continue
    }
    let opaque = true
    for (let row = region.y; row < region.y + region.height && opaque; row++) {
      for (let column = region.x; column < region.x + region.width; column++) {
        if (pixels.data[(row * pixels.width + column) * 4 + 3] !== 255) {
          opaque = false
          break
        }
      }
    }
    if (opaque) regions.set(tileId, { ...region })
    else invalid.add(tileId)
  }
  for (const [firstId, firstRegion] of regions) {
    for (const [secondId, secondRegion] of regions) {
      if (firstId < secondId && overlaps(firstRegion, secondRegion)) {
        invalid.add(firstId)
        invalid.add(secondId)
      }
    }
  }
  for (const tileId of invalid) regions.delete(tileId)
  return {
    atlas: { width: pixels.width, height: pixels.height, data: pixels.data, regions },
    invalidTileIds: [...invalid],
  }
}

function loadImage(signal: AbortSignal): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    const cleanup = () => {
      image.onload = null
      image.onerror = null
      signal.removeEventListener('abort', abort)
    }
    const abort = () => {
      cleanup()
      image.src = ''
      reject(new DOMException('Minimap loading cancelled', 'AbortError'))
    }
    if (signal.aborted) {
      abort()
      return
    }
    signal.addEventListener('abort', abort, { once: true })
    image.onload = () => { cleanup(); resolve(image) }
    image.onerror = () => { cleanup(); reject(new Error(`Cannot load ${MINIMAP_ATLAS_URL}`)) }
    image.src = MINIMAP_ATLAS_URL
  })
}

export async function loadMinimapAtlas(signal: AbortSignal): Promise<MinimapAtlasValidation> {
  const [manifest, image] = await Promise.all([
    fetch(MINIMAP_MANIFEST_URL, { signal }).then(response => {
      if (!response.ok) throw new Error(`Cannot load ${MINIMAP_MANIFEST_URL}: HTTP ${response.status}`)
      return response.json() as Promise<unknown>
    }),
    loadImage(signal),
  ])
  signal.throwIfAborted()
  // Validate dimensions before allocating a decoded pixel buffer.
  if (image.naturalWidth < 1 || image.naturalHeight < 1
    || image.naturalWidth > 4096 || image.naturalHeight > 4096) {
    throw new Error('Minimap atlas dimensions must be between 1 and 4096 pixels')
  }
  const canvas = document.createElement('canvas')
  canvas.width = image.naturalWidth
  canvas.height = image.naturalHeight
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) throw new Error('Cannot decode minimap atlas: Canvas 2D is unavailable')
  context.drawImage(image, 0, 0)
  const pixels = context.getImageData(0, 0, canvas.width, canvas.height)
  canvas.width = canvas.height = 0
  return validateMinimapAtlas(manifest, pixels)
}
