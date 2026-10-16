import {
  BufferImageSource, Container, GlProgram, Mesh, MeshGeometry, RenderTexture,
  Shader, Sprite, State, Texture, UniformGroup, type RenderOptions, type WebGLRenderer,
} from 'pixi.js'
import {
  createRadialLightPixels, getLightMapLayout, getNightOpacity, LIGHT_RADIAL_SIZE, LIGHT_FLICKER_RADIUS_RATIO,
  lightFlickerSeed, lightFlickerAmount, lightSwayOffset, NIGHT_FLOOR, resolveLightDefinition,
  shouldRefreshLightMap, type LightDefinition, type ResolvedLightDefinition,
} from './model'

export type { LightDefinition } from './model'

export interface LightingMetrics {
  sources: number
  intersecting: number
  mapWidth: number
  mapHeight: number
  mapBytes: number
  redrawCount: number
}

interface LightSource {
  readonly owner: Container
  readonly definition: ResolvedLightDefinition
  readonly sprite: Sprite
  readonly seed: number
  /** Unmodulated screen geometry; light animation must not dirty it every frame. */
  x: number
  y: number
  radiusX: number
  radiusY: number
}

const VERTEX_SHADER = `precision highp float;
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
uniform sampler2D uLightMap;
uniform vec3 uNightFloor;
uniform float uNightOpacity;
uniform float uHasLights;
void main() {
  vec3 light = vec3(0.0);
  if (uHasLights > 0.5) light = clamp(texture2D(uLightMap, vUV).rgb, 0.0, 1.0);
  vec3 multiplier = vec3(1.0) - uNightOpacity * (vec3(1.0) - uNightFloor) * (vec3(1.0) - light);
  gl_FragColor = vec4(multiplier, 1.0);
}`

/** One low-resolution additive light pass followed by a multiply quad, never a world capture. */
export class LightingManager {
  private readonly sources = new Map<number, LightSource>()
  private readonly lightContainer = new Container()
  private readonly renderOptions: RenderOptions = {
    container: this.lightContainer, clear: true, clearColor: [0, 0, 0, 0],
  }
  private readonly uniforms = new UniformGroup({
    uNightFloor: { value: new Float32Array(NIGHT_FLOOR), type: 'vec3<f32>' },
    uNightOpacity: { value: 0, type: 'f32' },
    uHasLights: { value: 0, type: 'f32' },
  })
  private readonly geometry: MeshGeometry
  private readonly shader: Shader
  private readonly mesh: Mesh<MeshGeometry, Shader>
  private readonly metrics: LightingMetrics = {
    sources: 0, intersecting: 0, mapWidth: 0, mapHeight: 0, mapBytes: 0, redrawCount: 0,
  }
  private radialTexture: Texture | null = null
  private lightMap: RenderTexture | null = null
  private width = 0
  private height = 0
  private wantedMapWidth = 0
  private wantedMapHeight = 0
  private mapValid = false
  private geometryDirty = true
  private lastRefreshMs = Number.NEGATIVE_INFINITY
  private contextLost = false
  private destroyed = false

  constructor(private readonly renderer: WebGLRenderer) {
    this.geometry = new MeshGeometry({
      positions: new Float32Array([0, 0, 1, 0, 1, 1, 0, 1]),
      uvs: new Float32Array([0, 0, 1, 0, 1, 1, 0, 1]),
      indices: new Uint32Array([0, 1, 2, 0, 2, 3]),
    })
    this.shader = new Shader({
      glProgram: GlProgram.from({ vertex: VERTEX_SHADER, fragment: FRAGMENT_SHADER, name: 'origin-night-lighting' }),
      resources: { uLightMap: Texture.WHITE.source, lighting: this.uniforms },
    })
    this.mesh = new Mesh({ geometry: this.geometry, shader: this.shader, state: State.for2d() })
    // MeshPipe takes its blend mode from the display object, not just from State.
    this.mesh.blendMode = 'multiply'
    this.mesh.eventMode = 'none'
    this.mesh.visible = false
    this.lightContainer.eventMode = 'none'
    this.lightContainer.interactiveChildren = false
    renderer.canvas.addEventListener('webglcontextlost', this.onContextLost)
    renderer.canvas.addEventListener('webglcontextrestored', this.onContextRestored)
  }

  getContainer(): Container { return this.mesh }

  register(entityId: number, owner: Container, definition: LightDefinition): void {
    if (this.destroyed) return
    const resolved = resolveLightDefinition(definition)
    this.unregister(entityId)
    const sprite = new Sprite(this.radialTexture ?? Texture.EMPTY)
    sprite.anchor.set(0.5)
    sprite.tint = resolved.tint
    sprite.blendMode = 'add'
    sprite.eventMode = 'none'
    sprite.visible = false
    this.sources.set(entityId, {
      owner, definition: resolved, sprite, seed: lightFlickerSeed(entityId),
      x: Number.NaN, y: Number.NaN, radiusX: Number.NaN, radiusY: Number.NaN,
    })
    this.lightContainer.addChild(sprite)
    this.metrics.sources = this.sources.size
    this.invalidate()
  }

  unregister(entityId: number): void {
    const source = this.sources.get(entityId)
    if (!source) return
    source.sprite.destroy()
    this.sources.delete(entityId)
    this.metrics.sources = this.sources.size
    this.invalidate()
  }

  update(nowMs: number, dayPhase: number | null, world: Container, width: number, height: number, active = true): void {
    if (this.destroyed) return
    const opacity = getNightOpacity(dayPhase)
    if (!active || this.contextLost || opacity <= 0 || !Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
      this.mesh.visible = false
      this.metrics.intersecting = 0
      this.invalidate()
      return
    }
    this.mesh.visible = true
    this.uniforms.uniforms.uNightOpacity = opacity
    if (width !== this.width || height !== this.height) {
      this.width = width
      this.height = height
      const layout = getLightMapLayout(width, height)
      this.wantedMapWidth = layout.width
      this.wantedMapHeight = layout.height
      this.mesh.scale.set(width, height)
      this.geometryDirty = true
    }

    let intersecting = 0
    let hasAnimation = false
    const scaleX = world.scale.x
    const scaleY = world.scale.y
    for (const source of this.sources.values()) {
      const definition = source.definition
      const x = world.x + (source.owner.x + definition.offsetX) * scaleX
      const y = world.y + (source.owner.y + definition.offsetY) * scaleY
      const radiusX = definition.radius * Math.abs(scaleX)
      const radiusY = definition.radius * Math.abs(scaleY)
      const maxRadiusScale = 1 + definition.flicker * LIGHT_FLICKER_RADIUS_RATIO
      const extentX = radiusX * maxRadiusScale + definition.swayX * Math.abs(scaleX)
      const extentY = radiusY * maxRadiusScale + definition.swayY * Math.abs(scaleY)
      const visible = !source.owner.destroyed && definition.strength > 0 && definition.tint !== 0 &&
        radiusX > 0 && radiusY > 0 && x + extentX >= 0 && y + extentY >= 0 && x - extentX <= width && y - extentY <= height
      if (source.sprite.visible !== visible) {
        source.sprite.visible = visible
        this.geometryDirty = true
      }
      if (!visible) continue
      intersecting++
      hasAnimation ||= definition.flicker > 0 || definition.swayX > 0 || definition.swayY > 0
      if (source.x !== x || source.y !== y || source.radiusX !== radiusX || source.radiusY !== radiusY) {
        source.x = x
        source.y = y
        source.radiusX = radiusX
        source.radiusY = radiusY
        this.geometryDirty = true
      }
    }
    this.metrics.intersecting = intersecting
    if (intersecting === 0) {
      this.invalidate()
      return
    }
    if (!shouldRefreshLightMap(nowMs, this.lastRefreshMs, this.mapValid, this.geometryDirty, hasAnimation)) return

    this.prepareMap()
    for (const source of this.sources.values()) {
      if (source.sprite.visible) {
        const definition = source.definition
        const flicker = lightFlickerAmount(nowMs, definition.flicker, source.seed, definition.flickerHz, definition.flickerFastWeight)
        const radiusScale = 1 + flicker * LIGHT_FLICKER_RADIUS_RATIO
        source.sprite.position.set(
          source.x + lightSwayOffset(nowMs, definition.swayX, source.seed ^ 0x68bc21eb, definition.swayHz) * scaleX,
          source.y + lightSwayOffset(nowMs, definition.swayY, source.seed ^ 0x02e5be93, definition.swayHz) * scaleY,
        )
        source.sprite.alpha = Math.max(0, Math.min(1, definition.strength * (1 + flicker)))
        source.sprite.width = source.radiusX * 2 * radiusScale
        source.sprite.height = source.radiusY * 2 * radiusScale
      }
    }
    this.renderer.render(this.renderOptions)
    this.mapValid = true
    this.geometryDirty = false
    this.lastRefreshMs = nowMs
    this.uniforms.uniforms.uHasLights = 1
    this.metrics.redrawCount++
  }

  invalidate(): void {
    this.mapValid = false
    this.geometryDirty = true
    this.uniforms.uniforms.uHasLights = 0
  }

  clear(): void {
    for (const source of this.sources.values()) source.sprite.destroy()
    this.sources.clear()
    this.metrics.sources = 0
    this.metrics.intersecting = 0
    this.metrics.redrawCount = 0
    this.lastRefreshMs = Number.NEGATIVE_INFINITY
    this.mesh.visible = false
    this.invalidate()
  }

  /** Read-only live counters; no per-frame snapshot allocation. */
  getMetrics(): Readonly<LightingMetrics> { return this.metrics }

  destroy(): void {
    if (this.destroyed) return
    this.clear()
    this.destroyed = true
    this.renderer.canvas.removeEventListener('webglcontextlost', this.onContextLost)
    this.renderer.canvas.removeEventListener('webglcontextrestored', this.onContextRestored)
    this.mesh.destroy()
    this.geometry.destroy(true)
    this.shader.destroy()
    this.lightContainer.destroy()
    this.lightMap?.destroy(true)
    this.lightMap = null
    this.renderOptions.target = undefined
    this.radialTexture?.destroy(true)
    this.radialTexture = null
    this.metrics.mapWidth = this.metrics.mapHeight = this.metrics.mapBytes = 0
  }

  private prepareMap(): void {
    if (!this.radialTexture) {
      this.radialTexture = new Texture({
        source: new BufferImageSource({
          width: LIGHT_RADIAL_SIZE, height: LIGHT_RADIAL_SIZE,
          resource: createRadialLightPixels(), format: 'rgba8unorm',
          alphaMode: 'premultiplied-alpha', scaleMode: 'linear',
        }),
      })
      for (const source of this.sources.values()) source.sprite.texture = this.radialTexture
    }
    if (!this.lightMap) {
      this.lightMap = RenderTexture.create({
        width: this.wantedMapWidth, height: this.wantedMapHeight, resolution: 1,
        dynamic: true, format: 'rgba8unorm', scaleMode: 'linear',
      })
      this.shader.resources.uLightMap = this.lightMap.source
      this.renderOptions.target = this.lightMap
    } else if (this.wantedMapWidth !== this.metrics.mapWidth || this.wantedMapHeight !== this.metrics.mapHeight) {
      this.lightMap.resize(this.wantedMapWidth, this.wantedMapHeight, 1)
    }
    this.lightContainer.scale.set(this.wantedMapWidth / this.width, this.wantedMapHeight / this.height)
    // Pixi retains this matrix in reusable RenderOptions after the first draw.
    // Detached render-group roots need an explicit refresh when their scale changes.
    this.lightContainer.updateLocalTransform()
    this.metrics.mapWidth = this.wantedMapWidth
    this.metrics.mapHeight = this.wantedMapHeight
    this.metrics.mapBytes = this.wantedMapWidth * this.wantedMapHeight * 4
  }

  private readonly onContextLost = (): void => {
    this.contextLost = true
    this.mesh.visible = false
    this.invalidate()
  }

  private readonly onContextRestored = (): void => {
    this.contextLost = false
    this.invalidate()
  }
}
