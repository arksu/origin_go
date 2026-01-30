import { Container, Mesh, MeshGeometry, Shader, GlProgram, State, Spritesheet } from 'pixi.js'
import { VertexBuffer } from './utils/VertexBuffer'
import {
  TEXTURE_WIDTH,
  TEXTURE_HEIGHT,
  TILE_WIDTH_HALF,
  TILE_HEIGHT_HALF,
  getChunkSize,
} from './Tile'
import { getGroundTextureName, getTileSet } from './TileSet'

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

export class Chunk {
  readonly x: number
  readonly y: number
  readonly key: string

  private container: Container
  private subchunks: Container[] = []
  private _visible: boolean = true
  private tiles: Uint8Array | null = null

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

  buildTiles(tiles: Uint8Array, spritesheet: Spritesheet, neighborTiles?: Map<string, Uint8Array>): void {
    console.log(`[Chunk ${this.key}] buildTiles called, tiles.length=${tiles.length}`)
    this.destroySubchunks()
    this.tiles = tiles

    const chunkSize = getChunkSize()
    const subchunkSize = chunkSize / DIVIDER
    console.log(`[Chunk ${this.key}] chunkSize=${chunkSize}, subchunkSize=${subchunkSize}, DIVIDER=${DIVIDER}`)

    let subchunksCreated = 0
    for (let cx = 0; cx < DIVIDER; cx++) {
      for (let cy = 0; cy < DIVIDER; cy++) {
        const subchunk = this.buildSubchunk(cx, cy, subchunkSize, tiles, spritesheet, neighborTiles)
        if (subchunk) {
          this.subchunks.push(subchunk)
          this.container.addChild(subchunk)
          subchunksCreated++
        }
      }
    }
    console.log(`[Chunk ${this.key}] Created ${subchunksCreated} subchunks`)
  }

  private buildSubchunk(
    cx: number,
    cy: number,
    subchunkSize: number,
    tiles: Uint8Array,
    spritesheet: Spritesheet,
    neighborTiles?: Map<string, Uint8Array>,
  ): Container | null {
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

        this.addBordersAndCorners(
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
      }
    }

    if (cx === 0 && cy === 0) {
      console.log(`[Chunk ${this.key}] Subchunk(0,0) stats: tiles=${tilesProcessed}, texturesFound=${texturesFound}, missing=${texturesMissing}, firstMissing="${firstMissingTexture}"`)
    }

    if (vertexBuffer.count === 0) {
      console.log(`[Chunk ${this.key}] Subchunk(${cx},${cy}) has 0 vertices, skipping`)
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
    return subchunkContainer
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
  ): void {
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
          }
        }
      }

      if (cornerMask !== 0) {
        const textureName = tileSet.getCornerTexture(cornerMask, globalX, globalY)
        if (textureName) {
          const texture = spritesheet.textures[textureName]
          if (texture) {
            vertexBuffer.addVertex(sx, sy, TEXTURE_WIDTH, TEXTURE_HEIGHT, texture)
          }
        }
      }
    }
  }

  private destroySubchunks(): void {
    for (const subchunk of this.subchunks) {
      subchunk.destroy({ children: true })
    }
    this.subchunks = []
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
