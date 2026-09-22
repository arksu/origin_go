import { Container, type Texture } from 'pixi.js'
import { FX_WIND } from '../../constants/fx'
import { ParticlePool, type Particle } from './ParticlePool'

export type Range = readonly [number, number]
type OverLife = { start: number; end: number }
export interface ParticleConfig {
  texture: string
  zIndex: number
  linger: boolean
  position: { x: number; y: number }
  spread: { x: number; y: number }
  rate: number
  maxParticles: number
  lifetimeMs: Range
  speed: Range
  angle: Range
  buoyancy: number
  drag: number
  windResponse: Range
  scale: OverLife
  alpha: OverLife & { peak: number; fadeInMs: number; fadeAwayMs: number }
  tint: number | readonly number[]
  rotation: Range
  spin: Range
  wander: { speed: number; frequency: number }
}

const randomRange = ([min, max]: Range): number => min + Math.random() * (max - min)
const interpolate = (start: number, end: number, progress: number): number => start + (end - start) * progress

export class ParticleEmitter {
  readonly container = new Container({ label: 'particle-emitter', eventMode: 'none' })
  private readonly pool: ParticlePool
  private readonly particles: Particle[] = []
  private spawnPhase = Math.random()
  private emitting = true
  private destroyed = false
  private owner: Container | null
  private scene: Container | null
  private readonly rememberScene = (parent: Container): void => { this.scene = parent }

  constructor(readonly config: ParticleConfig, texture: Texture, owner: Container, zIndex = config.zIndex) {
    if (!Number.isFinite(config.rate) || config.rate <= 0 ||
        !Number.isInteger(config.maxParticles) || config.maxParticles < 1) {
      throw new Error('Particle rate and maximum count must be positive')
    }
    this.pool = new ParticlePool(texture)
    this.owner = owner
    this.scene = owner.parent
    owner.on('added', this.rememberScene)
    this.container.zIndex = zIndex
    owner.addChild(this.container)
  }

  get particleCount(): number { return this.particles.length }

  stop(): void { this.emitting = false }

  /** Release the view while retaining its local transform in its former scene. */
  detach(linger = false): void {
    this.stop()
    const owner = this.owner
    const scene = owner?.parent ?? this.scene
    if (!linger || !owner || owner.destroyed || !scene || scene.destroyed || !this.particles.length) {
      this.destroy()
      return
    }
    // ObjectManager removes the owner from its parent before destroying the view.
    owner.updateLocalTransform()
    this.container.updateLocalTransform()
    const transform = this.container.localTransform.clone().prepend(owner.localTransform)
    scene.addChild(this.container)
    this.container.setFromMatrix(transform)
    this.container.zIndex = owner.zIndex
    this.container.alpha *= owner.alpha
    owner.off('added', this.rememberScene)
    this.owner = null
    this.scene = null
  }

  update(deltaMs: number, wind = FX_WIND): boolean {
    if (this.destroyed) return false
    if (this.container.destroyed || this.owner?.destroyed) {
      this.destroy()
      return false
    }
    if (!this.emitting && this.particles.length === 0) {
      this.destroy()
      return false
    }
    if (this.owner?.visible === false) return true
    if (!Number.isFinite(deltaMs) || deltaMs < 0) throw new Error('Particle delta must be finite and non-negative')
    // Bound catch-up work after a suspended tab; culling never accumulates time.
    const elapsedMs = Math.min(deltaMs, 100)
    const seconds = elapsedMs / 1000
    for (let index = this.particles.length - 1; index >= 0; index--) {
      const particle = this.particles[index]!
      particle.ageMs += elapsedMs
      if (particle.ageMs >= particle.lifetimeMs) {
        this.pool.release(particle)
        this.particles.splice(index, 1)
        continue
      }
      const drag = Math.exp(-this.config.drag * seconds)
      particle.velocityX *= drag
      particle.velocityY = (particle.velocityY - this.config.buoyancy * seconds) * drag
      const wander = Math.sin(particle.phase + particle.ageMs / 1000 * this.config.wander.frequency) * this.config.wander.speed
      particle.sprite.x += (particle.velocityX + wind.x * particle.windResponse * drag + wander) * seconds
      particle.sprite.y += (particle.velocityY + wind.y * particle.windResponse * drag) * seconds
      particle.sprite.rotation += particle.spin * seconds
      this.applyOverLife(particle)
    }
    if (this.emitting) {
      this.spawnPhase += seconds * this.config.rate
      const count = Math.min(Math.floor(this.spawnPhase), this.config.maxParticles - this.particles.length)
      this.spawnPhase %= 1
      for (let index = 0; index < count; index++) this.spawn()
    }
    if (!this.emitting && this.particles.length === 0) {
      this.destroy()
      return false
    }
    return true
  }

  private spawn(): void {
    const config = this.config
    const particle = this.pool.acquire()
    particle.ageMs = 0
    particle.lifetimeMs = randomRange(config.lifetimeMs)
    const speed = randomRange(config.speed)
    const angle = randomRange(config.angle)
    particle.velocityX = Math.cos(angle) * speed
    particle.velocityY = Math.sin(angle) * speed
    particle.windResponse = randomRange(config.windResponse)
    particle.phase = Math.random() * Math.PI * 2
    particle.spin = randomRange(config.spin)
    particle.sprite.position.set(
      config.position.x + randomRange([-config.spread.x, config.spread.x]),
      config.position.y + randomRange([-config.spread.y, config.spread.y]),
    )
    particle.sprite.rotation = randomRange(config.rotation)
    particle.sprite.tint = typeof config.tint === 'number' ? config.tint : config.tint[Math.floor(Math.random() * config.tint.length)]!
    this.applyOverLife(particle)
    this.container.addChild(particle.sprite)
    this.particles.push(particle)
  }

  private applyOverLife(particle: Particle): void {
    const { scale, alpha } = this.config
    const progress = particle.ageMs / particle.lifetimeMs
    particle.sprite.scale.set(interpolate(scale.start, scale.end, progress))
    const remainingMs = particle.lifetimeMs - particle.ageMs
    particle.sprite.alpha = particle.ageMs < alpha.fadeInMs
      ? interpolate(alpha.start, alpha.peak, particle.ageMs / alpha.fadeInMs)
      : remainingMs < alpha.fadeAwayMs
        ? interpolate(alpha.end, alpha.peak, remainingMs / alpha.fadeAwayMs)
        : alpha.peak
  }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    this.stop()
    this.owner?.off('added', this.rememberScene)
    this.owner = null
    this.scene = null
    this.pool.destroy()
    this.particles.length = 0
    if (!this.container.destroyed) this.container.destroy({ children: true })
  }
}
