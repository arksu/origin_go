import { Container, Mesh, MeshGeometry, Shader, GlProgram, State, Spritesheet } from 'pixi.js'
import { VertexBuffer } from './utils/VertexBuffer'
import {
  TEXTURE_WIDTH,
  TEXTURE_HEIGHT,
  TILE_WIDTH_HALF,
  TILE_HEIGHT_HALF,
  getChunkSize,
} from './tiles/Tile'
import { getGroundTextureName, getTileSet } from './tiles/TileSet'
import { type AABB, fromMinMax } from './culling/AABB'

const DIVIDER = 4

const VERTEX_SHADER = `precision mediump float;

in vec2 aPosition;
in vec2 aUV;

uniform mat3 uProjectionMatrix;
uniform mat3 uWorldTransformMatrix;
uniform mat3 uTransformMatrix;

out vec2 vUV;

void main() {
    vUV = aUV;
    mat3 mvp = uProjectionMatrix * uWorldTransformMatrix * uTransformMatrix;
    gl_Position = vec4((mvp * vec3(aPosition, 1.0)).xy, 0.0, 1.0);
}`

const FRAGMENT_SHADER = `precision mediump float;

in vec2 vUV;

uniform sampler2D uTexture;

void main() {
    gl_FragColor = texture2D(uTexture, vUV);
}`

let glProgram: GlProgram | null = null

function getGlProgram(): GlProgram {
  if (!glProgram) {
    glProgram = GlProgram.from({
      vertex: VERTEX_SHADER,
      fragment: FRAGMENT_SHADER,
    })
  }
  return glProgram
}

const BX = [0, 1, 2, 1]
const BY = [1, 0, 1, 2]
const CX = [0, 0, 2, 2]
const CY = [0, 2, 2, 0]

export interface ChunkBuildResult {
  hasBordersOrCorners: boolean[][]
}

export interface SubchunkData {
  key: string
  container: Container
  bounds: AABB
  cx: number
  cy: number
}

export class Chunk {
  readonly x: number
  readonly y: number
  readonly key: string

  private container: Container
  private subchunks: Container[] = []
  private subchunkDataList: SubchunkData[] = []
  private _visible: boolean = true
  private tiles: Uint8Array | null = null
  private lastBuildResult: ChunkBuildResult | null = null

  constructor(x: number, y: number) {
    this.x = x
    this.y = y
    this.key = `${x},${y}`

    this.container = new Container()
    this.container.sortableChildren = true
  }

  getContainer(): Container {
    return this.container
  }

  getTiles(): Uint8Array | null {
    return this.tiles
  }

  buildTiles(tiles: Uint8Array, spritesheet: Spritesheet, neighborTiles?: Map<string, Uint8Array>): ChunkBuildResult {
    const start = performance.now()
    // console.log(`[Chunk ${this.key}] buildTiles called, tiles.length=${tiles.length}`)
    this.destroySubchunks()
    this.tiles = tiles

    const chunkSize = getChunkSize()
    const subchunkSize = chunkSize / DIVIDER

    const hasBordersOrCorners: boolean[][] = []
    for (let i = 0; i < chunkSize; i++) {
      hasBordersOrCorners.push(new Array(chunkSize).fill(false))
    }

    let subchunksCreated = 0
    const subchunkTimes: number[] = []
    for (let cx = 0; cx < DIVIDER; cx++) {
      for (let cy = 0; cy < DIVIDER; cy++) {
        const subStart = performance.now()
        const result = this.buildSubchunk(cx, cy, subchunkSize, tiles, spritesheet, neighborTiles, hasBordersOrCorners)
        subchunkTimes.push(performance.now() - subStart)
        if (result) {
          this.subchunks.push(result.container)
          this.subchunkDataList.push(result)
          this.container.addChild(result.container)
          subchunksCreated++
        }
      }
    }

    const totalTime = performance.now() - start
    // const avgSubchunk = subchunkTimes.reduce((a, b) => a + b, 0) / subchunkTimes.length
    const maxSubchunk = Math.max(...subchunkTimes)

    if (totalTime > 10 || maxSubchunk > 5) {
      //console.warn(`[Chunk ${this.key}] SLOW: total=${totalTime.toFixed(2)}ms, avg subchunk=${avgSubchunk.toFixed(2)}ms, max subchunk=${maxSubchunk.toFixed(2)}ms`)
    } else {
      // console.log(`[Chunk ${this.key}] Built ${subchunksCreated} subchunks in ${totalTime.toFixed(2)}ms`)
    }

    this.lastBuildResult = { hasBordersOrCorners }
    return this.lastBuildResult
  }

  getLastBuildResult(): ChunkBuildResult | null {
    return this.lastBuildResult
  }

  private buildSubchunk(
    cx: number,
    cy: number,
    subchunkSize: number,
    tiles: Uint8Array,
    spritesheet: Spritesheet,
    neighborTiles: Map<string, Uint8Array> | undefined,
    hasBordersOrCorners: boolean[][],
  ): SubchunkData | null {
    // const start = performance.now()
    const chunkSize = getChunkSize()
    const subchunkContainer = new Container()
    subchunkContainer.sortableChildren = true

    const subchunkX = this.x + cx / DIVIDER
    const subchunkY = this.y + cy / DIVIDER

    subchunkContainer.x =
      subchunkX * TILE_WIDTH_HALF * chunkSize -
      subchunkY * TILE_WIDTH_HALF * chunkSize -
      TILE_WIDTH_HALF
    subchunkContainer.y =
      subchunkX * TILE_HEIGHT_HALF * chunkSize + subchunkY * TILE_HEIGHT_HALF * chunkSize

    const elements = subchunkSize * subchunkSize * 2
    const vertexBuffer = new VertexBuffer(elements)

    let tilesProcessed = 0
    let texturesFound = 0
    let texturesMissing = 0
    let firstMissingTexture = ''
    let borderTime = 0

    for (let tx = 0; tx < subchunkSize; tx++) {
      for (let ty = 0; ty < subchunkSize; ty++) {
        const x = cx * subchunkSize + tx
        const y = cy * subchunkSize + ty
        const idx = y * chunkSize + x

        const tileType = tiles[idx]
        if (tileType === undefined) continue
        tilesProcessed++

        const globalX = this.x * chunkSize + x
        const globalY = this.y * chunkSize + y

        const textureName = getGroundTextureName(tileType, globalX, globalY)
        if (!textureName) {
          texturesMissing++
          if (!firstMissingTexture) firstMissingTexture = `tileType=${tileType} has no texture name`
          continue
        }

        const texture = spritesheet.textures[textureName]
        if (!texture) {
          texturesMissing++
          if (!firstMissingTexture) firstMissingTexture = textureName
          continue
        }
        texturesFound++

        const sx = tx * TILE_WIDTH_HALF - ty * TILE_WIDTH_HALF
        const sy = tx * TILE_HEIGHT_HALF + ty * TILE_HEIGHT_HALF

        vertexBuffer.addVertex(sx, sy, TEXTURE_WIDTH, TEXTURE_HEIGHT, texture)

        const borderStart = performance.now()
        const hadBordersOrCorners = this.addBordersAndCorners(
          vertexBuffer,
          spritesheet,
          tiles,
          chunkSize,
          x,
          y,
          tileType,
          sx,
          sy,
          neighborTiles,
        )
        borderTime += performance.now() - borderStart

        if (hadBordersOrCorners) {
          hasBordersOrCorners[x]![y] = true
        }
      }
    }

    if (cx === 0 && cy === 0) {
      // const totalTime = performance.now() - start
      // console.log(`[Chunk ${this.key}] Subchunk(0,0) tiles=${tilesProcessed}, found=${texturesFound}, missing=${texturesMissing}, time=${totalTime.toFixed(2)}ms, borders=${borderTime.toFixed(2)}ms`)
    }

    if (vertexBuffer.count === 0) {
      // console.log(`[Chunk ${this.key}] Subchunk(${cx},${cy}) has 0 vertices, skipping`)
      return null
    }

    vertexBuffer.finish()

    const geometry = new MeshGeometry({
      positions: vertexBuffer.vertex,
      uvs: vertexBuffer.uv,
      indices: vertexBuffer.index,
    })

    const shader = new Shader({
      glProgram: getGlProgram(),
      resources: {
        uTexture: spritesheet.textureSource,
      },
    })

    const mesh = new Mesh({
      geometry,
      shader,
      state: State.for2d(),
    })

    subchunkContainer.addChild(mesh)

    // Compute bounds for this subchunk in parent (chunk container) coordinates
    // The subchunk covers tiles from (cx*subchunkSize, cy*subchunkSize) to ((cx+1)*subchunkSize-1, (cy+1)*subchunkSize-1)
    // We need to compute the screen-space bounding box
    const bounds = this.computeSubchunkBounds(cx, cy, subchunkSize)
    const subchunkKey = `${this.key}:${cx},${cy}`

    return {
      key: subchunkKey,
      container: subchunkContainer,
      bounds,
      cx,
      cy,
    }
  }

  /**
   * Compute AABB bounds for a subchunk in chunk container local coordinates.
   */
  private computeSubchunkBounds(cx: number, cy: number, subchunkSize: number): AABB {
    const chunkSize = getChunkSize()

    // Subchunk tile range within chunk
    const startTileX = cx * subchunkSize
    const startTileY = cy * subchunkSize
    const endTileX = startTileX + subchunkSize - 1
    const endTileY = startTileY + subchunkSize - 1

    // Convert to global tile coordinates
    const globalStartX = this.x * chunkSize + startTileX
    const globalStartY = this.y * chunkSize + startTileY
    const globalEndX = this.x * chunkSize + endTileX
    const globalEndY = this.y * chunkSize + endTileY

    // Compute screen positions for all 4 corners of the subchunk
    // For isometric projection, we need to find the actual bounding box
    // Top corner (min Y in screen): tile at (maxX, minY)
    // Bottom corner (max Y in screen): tile at (minX, maxY)
    // Left corner (min X in screen): tile at (minX, minY)
    // Right corner (max X in screen): tile at (maxX, maxY)

    const corners = [
      { tx: globalStartX, ty: globalStartY }, // top-left in tile space
      { tx: globalEndX, ty: globalStartY },   // top-right in tile space
      { tx: globalStartX, ty: globalEndY },   // bottom-left in tile space
      { tx: globalEndX, ty: globalEndY },     // bottom-right in tile space
    ]

    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity

    for (const corner of corners) {
      // Convert tile to screen coordinates (relative to world origin)
      const sx = corner.tx * TILE_WIDTH_HALF - corner.ty * TILE_WIDTH_HALF
      const sy = corner.tx * TILE_HEIGHT_HALF + corner.ty * TILE_HEIGHT_HALF

      if (sx < minX) minX = sx
      if (sy < minY) minY = sy
      if (sx > maxX) maxX = sx
      if (sy > maxY) maxY = sy
    }

    // Add padding for tile dimensions (tiles extend beyond their anchor point)
    minX -= TILE_WIDTH_HALF
    maxX += TILE_WIDTH_HALF
    minY -= TILE_HEIGHT_HALF // Some padding for top
    maxY += TEXTURE_HEIGHT   // Tiles extend down from anchor

    return fromMinMax(minX, minY, maxX, maxY)
  }

  private addBordersAndCorners(
    vertexBuffer: VertexBuffer,
    spritesheet: Spritesheet,
    tiles: Uint8Array,
    chunkSize: number,
    x: number,
    y: number,
    currentTileType: number,
    sx: number,
    sy: number,
    neighborTiles?: Map<string, Uint8Array>,
  ): boolean {
    let hadBordersOrCorners = false
    const tr: number[][] = [
      [0, 0, 0],
      [0, 0, 0],
      [0, 0, 0],
    ]

    for (let rx = -1; rx <= 1; rx++) {
      for (let ry = -1; ry <= 1; ry++) {
        if (rx === 0 && ry === 0) {
          tr[rx + 1]![ry + 1] = 0
          continue
        }

        const dx = x + rx
        const dy = y + ry
        let neighborType = -1

        if (dx >= 0 && dx < chunkSize && dy >= 0 && dy < chunkSize) {
          neighborType = tiles[dy * chunkSize + dx] ?? -1
        } else if (neighborTiles) {
          const ox = dx < 0 ? -1 : dx >= chunkSize ? 1 : 0
          const oy = dy < 0 ? -1 : dy >= chunkSize ? 1 : 0
          const neighborKey = `${this.x + ox},${this.y + oy}`
          const neighborData = neighborTiles.get(neighborKey)

          if (neighborData) {
            const ix = dx < 0 ? chunkSize + dx : dx >= chunkSize ? dx - chunkSize : dx
            const iy = dy < 0 ? chunkSize + dy : dy >= chunkSize ? dy - chunkSize : dy
            neighborType = neighborData[iy * chunkSize + ix] ?? -1
          }
        }

        tr[rx + 1]![ry + 1] = neighborType
      }
    }

    if (tr[0]![0]! >= tr[1]![0]!) tr[0]![0] = -1
    if (tr[0]![0]! >= tr[0]![1]!) tr[0]![0] = -1
    if (tr[2]![0]! >= tr[1]![0]!) tr[2]![0] = -1
    if (tr[2]![0]! >= tr[2]![1]!) tr[2]![0] = -1
    if (tr[0]![2]! >= tr[0]![1]!) tr[0]![2] = -1
    if (tr[0]![2]! >= tr[1]![2]!) tr[0]![2] = -1
    if (tr[2]![2]! >= tr[2]![1]!) tr[2]![2] = -1
    if (tr[2]![2]! >= tr[1]![2]!) tr[2]![2] = -1

    const globalX = this.x * chunkSize + x
    const globalY = this.y * chunkSize + y

    for (let i = currentTileType - 1; i >= 0; i--) {
      const tileSet = getTileSet(i)
      if (!tileSet) continue

      let borderMask = 0
      let cornerMask = 0

      for (let o = 0; o < 4; o++) {
        if (tr[BX[o]!]![BY[o]!] === i) borderMask |= 1 << o
        if (tr[CX[o]!]![CY[o]!] === i) cornerMask |= 1 << o
      }

      if (borderMask !== 0) {
        const textureName = tileSet.getBorderTexture(borderMask, globalX, globalY)
        if (textureName) {
          const texture = spritesheet.textures[textureName]
          if (texture) {
            vertexBuffer.addVertex(sx, sy, TEXTURE_WIDTH, TEXTURE_HEIGHT, texture)
            hadBordersOrCorners = true
          }
        }
      }

      if (cornerMask !== 0) {
        const textureName = tileSet.getCornerTexture(cornerMask, globalX, globalY)
        if (textureName) {
          const texture = spritesheet.textures[textureName]
          if (texture) {
            vertexBuffer.addVertex(sx, sy, TEXTURE_WIDTH, TEXTURE_HEIGHT, texture)
            hadBordersOrCorners = true
          }
        }
      }
    }

    return hadBordersOrCorners
  }

  private destroySubchunks(): void {
    for (const subchunk of this.subchunks) {
      subchunk.destroy({ children: true })
    }
    this.subchunks = []
    this.subchunkDataList = []
  }

  /**
   * Get all subchunk data for culling registration.
   */
  getSubchunkDataList(): SubchunkData[] {
    return this.subchunkDataList
  }

  set visible(v: boolean) {
    if (this._visible !== v) {
      this._visible = v
      this.container.visible = v
    }
  }

  get visible(): boolean {
    return this._visible
  }

  destroy(): void {
    this.destroySubchunks()
    this.container.destroy({ children: true })
    this.tiles = null
  }
}
