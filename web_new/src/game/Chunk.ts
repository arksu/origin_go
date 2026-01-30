import { Container, Mesh, MeshGeometry, Shader, GlProgram, State, Spritesheet } from 'pixi.js'
import { VertexBuffer } from './utils/VertexBuffer'
import { getRandomByCoord } from './utils/random'
import {
  TEXTURE_WIDTH,
  TEXTURE_HEIGHT,
  TILE_WIDTH_HALF,
  TILE_HEIGHT_HALF,
  getChunkSize,
} from './Tile'

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

export interface ChunkTileData {
  tiles: Uint8Array
}

export class Chunk {
  readonly x: number
  readonly y: number
  readonly key: string

  private container: Container
  private mesh: Mesh<MeshGeometry, Shader> | null = null
  private _visible: boolean = true

  constructor(x: number, y: number) {
    this.x = x
    this.y = y
    this.key = `${x},${y}`

    this.container = new Container()
    this.container.sortableChildren = true

    const chunkSize = getChunkSize()
    this.container.x =
      x * TILE_WIDTH_HALF * chunkSize - y * TILE_WIDTH_HALF * chunkSize - TILE_WIDTH_HALF
    this.container.y =
      x * TILE_HEIGHT_HALF * chunkSize + y * TILE_HEIGHT_HALF * chunkSize
  }

  getContainer(): Container {
    return this.container
  }

  buildTiles(tiles: Uint8Array, spritesheet: Spritesheet): void {
    if (this.mesh) {
      this.container.removeChild(this.mesh)
      this.mesh.destroy()
      this.mesh = null
    }

    const chunkSize = getChunkSize()
    const elements = chunkSize * chunkSize * 2
    const vertexBuffer = new VertexBuffer(elements)

    for (let tx = 0; tx < chunkSize; tx++) {
      for (let ty = 0; ty < chunkSize; ty++) {
        const idx = ty * chunkSize + tx
        const tileType = tiles[idx]
        if (tileType === undefined) continue

        const textureName = this.getGroundTexture(tileType, tx, ty)
        if (!textureName) continue

        const texture = spritesheet.textures[textureName]
        if (!texture) continue

        const sx = tx * TILE_WIDTH_HALF - ty * TILE_WIDTH_HALF
        const sy = tx * TILE_HEIGHT_HALF + ty * TILE_HEIGHT_HALF

        vertexBuffer.addVertex(sx, sy, TEXTURE_WIDTH, TEXTURE_HEIGHT, texture)
      }
    }

    if (vertexBuffer.count === 0) return

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

    this.mesh = new Mesh({
      geometry,
      shader,
      state: State.for2d(),
    })

    this.container.addChild(this.mesh)
  }

  private getGroundTexture(tileType: number, x: number, y: number): string | null {
    const seed = getRandomByCoord(x + this.x * getChunkSize(), y + this.y * getChunkSize())
    const variant = seed % 4
    return `ground_${tileType}_${variant}`
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
    if (this.mesh) {
      this.mesh.destroy()
      this.mesh = null
    }
    this.container.destroy({ children: true })
  }
}
