import { Sprite, type Texture } from 'pixi.js'

export interface Particle {
  sprite: Sprite
  ageMs: number
  lifetimeMs: number
  velocityX: number
  velocityY: number
  windResponse: number
  phase: number
  spin: number
}

export class ParticlePool {
  private readonly available: Particle[] = []
  private readonly active = new Set<Particle>()

  constructor(private readonly texture: Texture) {}

  acquire(): Particle {
    const particle = this.available.pop() ?? {
      sprite: new Sprite(this.texture), ageMs: 0, lifetimeMs: 0,
      velocityX: 0, velocityY: 0, windResponse: 0, phase: 0, spin: 0,
    }
    particle.sprite.anchor.set(0.5)
    particle.sprite.visible = true
    this.active.add(particle)
    return particle
  }

  release(particle: Particle): void {
    if (!this.active.delete(particle)) throw new Error('Particle was already released')
    particle.sprite.removeFromParent()
    particle.sprite.visible = false
    this.available.push(particle)
  }

  destroy(): void {
    for (const particle of this.active) this.release(particle)
    for (const particle of this.available) particle.sprite.destroy()
    this.available.length = 0
  }
}
