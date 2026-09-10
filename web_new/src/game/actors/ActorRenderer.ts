import { Sprite, Texture, TextureSource, WebGLRenderer as PixiRenderer } from 'pixi.js'
import { OrthographicCamera, Scene, WebGLRenderer } from 'three'
import { ActorAssetCache } from './ActorAssetCache'
import { ActorInstance } from './ActorInstance'
import { PixelActorPass } from './PixelActorPass'
import { ACTOR_RENDER } from './config'

export interface ActorHandle {
  readonly actor: ActorInstance
  readonly sprite: Sprite
  lastRenderMs: number
  renderedRevision: number
  priority: boolean
  released: boolean
}

/** All character rendering shares the game's GL context; images stay on the GPU. */
export class ActorRenderer {
  readonly cache = new ActorAssetCache()
  private readonly renderer: WebGLRenderer
  private readonly pass = new PixelActorPass()
  private readonly scene = new Scene()
  private readonly camera: OrthographicCamera
  private readonly actors = new Set<ActorHandle>()
  private readonly outputs: Texture[] = []
  private readonly readPixel = new Uint8Array(4)
  private readFramebuffer: WebGLFramebuffer
  private lost = false
  private destroyed = false
  private renderedThisFrame = 0
  private renderMs = 0

  constructor(private readonly pixi: PixiRenderer) {
    if (pixi.context.webGLVersion !== 2) throw new Error('Hybrid characters require WebGL2')
    // Three initializes array textures immediately; WebGL forbids those uploads
    // while Pixi's UNPACK_PREMULTIPLY_ALPHA_WEBGL flag is still enabled.
    pixi.resetState()
    pixi.canvas.addEventListener('webglcontextrestored', this.beforeThreeContextRestore)
    try {
      this.renderer = new WebGLRenderer({ canvas: pixi.canvas as HTMLCanvasElement, context: pixi.gl as WebGL2RenderingContext, antialias: false, stencil: true, alpha: true })
    } catch (error) {
      pixi.canvas.removeEventListener('webglcontextrestored', this.beforeThreeContextRestore)
      this.pass.destroy()
      this.cache.destroy()
      throw error
    }
    this.renderer.setClearColor(0, 0)
    this.renderer.info.autoReset = false
    const half = ACTOR_RENDER.orthoHeight / 2
    this.camera = new OrthographicCamera(-half, half, half, -half, .1, 20)
    this.camera.position.set(0, ACTOR_RENDER.cameraHeight + 3, Math.sqrt(27))
    this.camera.lookAt(0, ACTOR_RENDER.cameraHeight, 0)
    const framebuffer = this.pixi.gl.createFramebuffer()
    if (!framebuffer) throw new Error('Unable to allocate actor picking framebuffer')
    this.readFramebuffer = framebuffer
    pixi.canvas.addEventListener('webglcontextlost', this.onContextLost)
    pixi.canvas.addEventListener('webglcontextrestored', this.onContextRestored)
    pixi.resetState()
  }

  create(): ActorHandle {
    if (this.destroyed) throw new Error('Actor renderer is disposed')
    const actor = new ActorInstance(this.cache)
    const sprite = new Sprite(Texture.EMPTY)
    sprite.position.set(-ACTOR_RENDER.anchorX, -ACTOR_RENDER.anchorY)
    sprite.roundPixels = true
    const handle: ActorHandle = { actor, sprite, lastRenderMs: -Infinity, renderedRevision: -1, priority: false, released: false }
    this.actors.add(handle)
    return handle
  }

  private allocateOutput(): Texture | null {
    const pooled = this.outputs.pop()
    if (pooled) return pooled
    const allocated = [...this.actors].filter((handle) => handle.sprite.texture !== Texture.EMPTY).length
    if (allocated >= ACTOR_RENDER.maxOutputSlots) return null
    return new Texture({ source: new TextureSource({ width: ACTOR_RENDER.cellSize, height: ACTOR_RENDER.cellSize,
      scaleMode: 'nearest', alphaMode: 'premultiplied-alpha', autoGenerateMipmaps: false, autoGarbageCollect: false }) })
  }

  render(now = performance.now()): void {
    if (this.destroyed || this.lost) return
    const started = performance.now()
    this.renderedThisFrame = 0
    this.renderer.info.reset()
    let touched = false
    let clearColor: Float32Array | null = null
    try {
      for (const handle of this.actors) {
        const visible = handle.sprite.parent?.visible && handle.sprite.parent?.renderable
        if (!visible) {
          if (handle.sprite.texture !== Texture.EMPTY) {
            this.outputs.push(handle.sprite.texture)
            handle.sprite.texture = Texture.EMPTY
            handle.renderedRevision = -1
          }
          continue
        }
        if (!handle.actor.isReady) continue
        const transform = handle.sprite.parent!.worldTransform
        handle.actor.setLowDetail(Math.hypot(transform.a, transform.b) < .8)
        handle.actor.updatePose()
        if (handle.renderedRevision === handle.actor.revision) continue
        if (!handle.priority && handle.renderedRevision >= 0 && now - handle.lastRenderMs < ACTOR_RENDER.secondaryUpdateMs) continue
        if (handle.sprite.texture === Texture.EMPTY) {
          const texture = this.allocateOutput()
          if (!texture) continue
          handle.sprite.texture = texture
        }
        if (!touched) {
          clearColor = this.pixi.gl.getParameter(this.pixi.gl.COLOR_CLEAR_VALUE) as Float32Array
          this.renderer.resetState()
          touched = true
        }
        this.scene.add(handle.actor.root)
        this.renderer.setRenderTarget(this.pass.source)
        this.renderer.render(this.scene, this.camera)
        this.pass.render(this.renderer, handle.actor.hovered)
        this.scene.remove(handle.actor.root)
        this.copyOutput(handle.sprite.texture)
        handle.lastRenderMs = now
        handle.renderedRevision = handle.actor.revision
        this.renderedThisFrame++
      }
    } finally {
      if (touched) {
        this.renderer.setRenderTarget(null)
        this.pixi.resetState()
        // Pixi 8.16 does not invalidate its clear-color cache in resetState.
        if (clearColor) this.pixi.gl.clearColor(clearColor[0]!, clearColor[1]!, clearColor[2]!, clearColor[3]!)
      }
      this.renderMs = performance.now() - started
    }
  }

  private copyOutput(texture: Texture): void {
    const gl = this.pixi.gl as WebGL2RenderingContext
    // Pixi owns the destination. copyTexSubImage2D copies entirely within GL,
    // without depending on Three's private texture/framebuffer bookkeeping.
    const destination = this.pixi.texture.getGlSource(texture.source)
    gl.activeTexture(gl.TEXTURE0)
    gl.bindTexture(gl.TEXTURE_2D, destination.texture)
    gl.copyTexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, 0, 0, ACTOR_RENDER.cellSize, ACTOR_RENDER.cellSize)
    this.renderer.resetState()
  }

  hitTest(handle: ActorHandle, localX: number, localY: number): boolean {
    const column = Math.floor(localX + ACTOR_RENDER.anchorX)
    const row = Math.floor(localY + ACTOR_RENDER.anchorY)
    if (column < 0 || row < 0 || column >= ACTOR_RENDER.cellSize || row >= ACTOR_RENDER.cellSize || handle.sprite.texture === Texture.EMPTY || this.lost) return false
    const gl = this.pixi.gl as WebGL2RenderingContext
    // A single pixel is read only on an interaction, never to transport frames.
    const texture = this.pixi.texture.getGlSource(handle.sprite.texture.source)
    gl.bindFramebuffer(gl.FRAMEBUFFER, this.readFramebuffer)
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, texture.texture, 0)
    gl.readPixels(column, row, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, this.readPixel)
    gl.bindFramebuffer(gl.FRAMEBUFFER, null)
    this.pixi.resetState()
    return this.readPixel[3]! > 0
  }

  release(handle: ActorHandle): void {
    if (handle.released) return
    handle.released = true
    this.actors.delete(handle)
    if (handle.sprite.texture !== Texture.EMPTY) this.outputs.push(handle.sprite.texture)
    handle.sprite.texture = Texture.EMPTY
    handle.sprite.destroy()
    handle.actor.destroy()
  }

  private readonly onContextLost = (event: Event): void => { event.preventDefault(); this.lost = true }
  private readonly beforeThreeContextRestore = (): void => { this.pixi.resetState() }
  private readonly onContextRestored = (): void => {
    const framebuffer = this.pixi.gl.createFramebuffer()
    if (!framebuffer) throw new Error('Unable to restore actor picking framebuffer')
    this.readFramebuffer = framebuffer
    this.lost = false
    for (const handle of this.actors) handle.renderedRevision = -1
  }

  get metrics() {
    return { actors: this.actors.size, updated: this.renderedThisFrame, cpuMs: this.renderMs,
      triangles: this.renderer.info.render.triangles, drawCalls: this.renderer.info.render.calls,
      assetBytes: this.cache.residentBytes, assets: this.cache.loadedCount,
      outputBytes: (this.outputs.length + [...this.actors].filter((handle) => handle.sprite.texture !== Texture.EMPTY).length) * ACTOR_RENDER.cellSize ** 2 * 4 }
  }

  get hasUpdatedPoses(): boolean { return this.renderedThisFrame > 0 }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    this.pixi.canvas.removeEventListener('webglcontextlost', this.onContextLost)
    this.pixi.canvas.removeEventListener('webglcontextrestored', this.beforeThreeContextRestore)
    this.pixi.canvas.removeEventListener('webglcontextrestored', this.onContextRestored)
    for (const handle of [...this.actors]) this.release(handle)
    for (const texture of this.outputs) texture.destroy(true)
    this.outputs.length = 0
    this.pixi.gl.deleteFramebuffer(this.readFramebuffer)
    this.pass.destroy()
    this.renderer.dispose()
    this.cache.destroy()
    this.pixi.resetState()
  }
}
