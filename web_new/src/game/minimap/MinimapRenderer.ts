import { MINIMAP_DEFAULT_ZOOM, MINIMAP_SIZE, MINIMAP_ZOOM_LEVELS } from '@/constants/minimap'
import { loadMinimapAtlas } from './atlas'
import { rasterizeMinimap } from './raster'
import type { MinimapAtlas, MinimapFrame } from './types'

export class MinimapRenderer {
  private readonly context: CanvasRenderingContext2D | null
  private readonly terrainCanvas = document.createElement('canvas')
  private readonly terrainContext: CanvasRenderingContext2D | null
  private readonly assetAbort = new AbortController()
  private terrainPixels: ImageData | null = null
  private atlas: MinimapAtlas | null = null
  private zoom: number = MINIMAP_DEFAULT_ZOOM
  private destroyed = false
  private warnedUnknownTile = false

  constructor(private readonly canvas: HTMLCanvasElement) {
    this.context = canvas.getContext('2d')
    this.terrainContext = this.terrainCanvas.getContext('2d')
    if (!this.context || !this.terrainContext) {
      console.warn('[Minimap] Canvas 2D is unavailable; minimap rendering is disabled.')
      return
    }
    void loadMinimapAtlas(this.assetAbort.signal).then(({ atlas, invalidTileIds }) => {
      if (this.destroyed) return
      this.atlas = atlas
      if (invalidTileIds.length) {
        console.warn(`[Minimap] Invalid or missing minimap_base.json regions for tile IDs ${invalidTileIds.join(', ')}; using fallback colors. Check the atlas dimensions, region overlap, and opacity.`)
      }
      // Static art may finish across a world reset. Only the next fresh frame can paint it.
    }).catch((error: unknown) => {
      if (this.destroyed) return
      console.warn('[Minimap] Cannot load base textures; using fallback colors. Check /assets/game/minimap_base.png and minimap_base.json.', error)
    })
  }

  setZoom(zoom: number): void {
    if (!(MINIMAP_ZOOM_LEVELS as readonly number[]).includes(zoom)) {
      throw new Error(`Unsupported minimap zoom: ${zoom}`)
    }
    this.zoom = zoom
  }

  render(frame: MinimapFrame): void {
    if (this.destroyed || !this.context || !this.terrainContext) return
    const { player, coordPerTile, chunkSize } = frame
    if (!player || !Number.isFinite(player.x) || !Number.isFinite(player.y) || !Number.isFinite(player.heading)
      || !Number.isFinite(coordPerTile) || coordPerTile <= 0 || !Number.isSafeInteger(chunkSize) || chunkSize <= 0) {
      this.clear()
      return
    }
    const width = this.canvas.clientWidth || MINIMAP_SIZE
    const height = this.canvas.clientHeight || MINIMAP_SIZE
    const deviceScale = window.devicePixelRatio || 1
    const backingWidth = Math.round(width * deviceScale)
    const backingHeight = Math.round(height * deviceScale)
    if (this.canvas.width !== backingWidth || this.canvas.height !== backingHeight) {
      this.canvas.width = backingWidth
      this.canvas.height = backingHeight
    }

    const rasterWidth = Math.ceil(width / this.zoom) + 1
    const rasterHeight = Math.ceil(height / this.zoom) + 1
    if (!this.terrainPixels || this.terrainPixels.width !== rasterWidth || this.terrainPixels.height !== rasterHeight) {
      this.terrainCanvas.width = rasterWidth
      this.terrainCanvas.height = rasterHeight
      this.terrainPixels = this.terrainContext.createImageData(rasterWidth, rasterHeight)
    }
    const leftTile = player.x / coordPerTile - width / (2 * this.zoom)
    const topTile = player.y / coordPerTile - height / (2 * this.zoom)
    const originTileX = Math.floor(leftTile)
    const originTileY = Math.floor(topTile)
    rasterizeMinimap(this.terrainPixels, frame, originTileX, originTileY, this.atlas, this.reportUnknownTile)
    this.terrainContext.putImageData(this.terrainPixels, 0, 0)
    this.context.setTransform(deviceScale, 0, 0, deviceScale, 0, 0)
    this.context.clearRect(0, 0, width, height)
    this.context.imageSmoothingEnabled = false
    this.context.drawImage(
      this.terrainCanvas,
      (originTileX - leftTile) * this.zoom,
      (originTileY - topTile) * this.zoom,
      rasterWidth * this.zoom,
      rasterHeight * this.zoom,
    )
    this.drawPlayer(width / 2, height / 2, player.heading)
  }

  clear(): void {
    this.terrainPixels?.data.fill(0)
    this.terrainContext?.clearRect(0, 0, this.terrainCanvas.width, this.terrainCanvas.height)
    this.context?.setTransform(1, 0, 0, 1, 0, 0)
    this.context?.clearRect(0, 0, this.canvas.width, this.canvas.height)
  }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    this.assetAbort.abort()
    this.clear()
    this.atlas = null
    this.terrainPixels = null
    this.terrainCanvas.width = this.terrainCanvas.height = 0
  }

  private readonly reportUnknownTile = (tileId: number): void => {
    if (this.warnedUnknownTile) return
    this.warnedUnknownTile = true
    console.warn(`[Minimap] Unknown terrain tile ID ${tileId}; showing diagnostic magenta. Update the tile palette and minimap atlas.`)
  }

  private drawPlayer(centerX: number, centerY: number, heading: number): void {
    const context = this.context!
    context.save()
    context.translate(centerX, centerY)
    context.rotate(heading)
    context.beginPath()
    context.moveTo(7, 0)
    context.lineTo(-5, -4)
    context.lineTo(-3, 0)
    context.lineTo(-5, 4)
    context.closePath()
    context.fillStyle = '#fff4c2'
    context.strokeStyle = '#242319'
    context.lineWidth = 1.5
    context.lineJoin = 'round'
    context.stroke()
    context.fill()
    context.restore()
  }
}
