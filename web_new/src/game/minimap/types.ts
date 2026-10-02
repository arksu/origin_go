/** Borrowed only during the synchronous minimap frame. */
export interface MinimapChunk {
  readonly x: number
  readonly y: number
  readonly tiles: Uint8Array
  readonly version: number
}

export interface MinimapPose {
  readonly x: number
  readonly y: number
  readonly heading: number
}

export interface MinimapFrame {
  readonly player: MinimapPose | null
  readonly coordPerTile: number
  readonly chunkSize: number
  readonly getChunk: (x: number, y: number) => MinimapChunk | undefined
}

export interface MinimapPixels {
  readonly width: number
  readonly height: number
  readonly data: Uint8ClampedArray
}

export interface MinimapTextureRegion {
  readonly x: number
  readonly y: number
  readonly width: number
  readonly height: number
}

export interface MinimapAtlas extends MinimapPixels {
  readonly regions: ReadonlyMap<number, MinimapTextureRegion>
}
