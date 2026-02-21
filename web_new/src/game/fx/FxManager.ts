import { Container, Sprite, Ticker } from 'pixi.js'
import { ResourceLoader } from '../ResourceLoader'
import { soundManager } from '../index'

export interface FloatingAnimationOptions {
  container: Container
  x: number
  y: number
  durationMs: number
  onComplete?: () => void
}

export class FxManager {
  private static instance: FxManager
  private ticker: Ticker
  private activeFx: Set<{ update: (delta: number) => boolean }> = new Set()

  private constructor() {
    this.ticker = Ticker.shared
    this.ticker.add(this.update.bind(this))
  }

  static getInstance(): FxManager {
    if (!FxManager.instance) {
      FxManager.instance = new FxManager()
    }
    return FxManager.instance
  }

  private update(ticker: Ticker): void {
    const delta = ticker.deltaMS
    const toRemove: any[] = []

    for (const fx of this.activeFx) {
      if (!fx.update(delta)) {
        toRemove.push(fx)
      }
    }

    for (const fx of toRemove) {
      this.activeFx.delete(fx)
    }
  }

  async playLpGainAnimation(options: FloatingAnimationOptions): Promise<void> {
    const stickTex = await ResourceLoader.loadTexture('fx/stick.svg')
    const sparkTex = await ResourceLoader.loadTexture('fx/spark.svg')

    const stick1 = new Sprite(stickTex)
    const stick2 = new Sprite(stickTex)
    const spark = new Sprite(sparkTex)

    // Setup initial state
    stick1.anchor.set(0.5, 0.5)
    stick2.anchor.set(0.5, 0.5)
    spark.anchor.set(0.5, 0.5)

    // Make everything 3x bigger
    stick1.scale.set(0.9)
    stick2.scale.set(0.9)
    spark.scale.set(0.6)

    // Set extremely high z-index to stay above all objects and terrain
    // (Objects use their Y coordinate for z-sorting, which can be up to 100,000)
    stick1.zIndex = 999999
    stick2.zIndex = 999999
    spark.zIndex = 999999

    // Adjust positions for larger scale
    stick1.position.set(options.x - 30, options.y)
    stick2.position.set(options.x + 30, options.y)
    spark.position.set(options.x, options.y - 30)

    stick1.rotation = -Math.PI / 4
    stick2.rotation = Math.PI / 4

    spark.alpha = 0
    spark.visible = false

    // Make spark extra bright by additive blending
    spark.blendMode = 'add'

    options.container.addChild(stick1, stick2, spark)

    let elapsed = 0
    const totalDuration = options.durationMs
    const rubbingDuration = totalDuration * 0.7
    const sparkDuration = totalDuration * 0.3

    const fx = {
      update: (delta: number): boolean => {
        elapsed += delta

        if (elapsed >= totalDuration) {
          options.container.removeChild(stick1, stick2, spark)
          stick1.destroy()
          stick2.destroy()
          spark.destroy()
          options.onComplete?.()
          return false
        }

        // Float up slowly (increased distance for bigger scale)
        const floatProgress = elapsed / totalDuration
        const yOffset = -45 * floatProgress
        stick1.y = options.y + yOffset
        stick2.y = options.y + yOffset
        spark.y = options.y - 30 + yOffset

        if (elapsed < rubbingDuration) {
          // Rubbing sticks (increased movement for bigger scale)
          const rubProgress = elapsed / rubbingDuration
          const rubSpeed = 20
          const rubOffset = Math.sin(rubProgress * Math.PI * rubSpeed) * 15

          stick1.x = options.x - 30 + rubOffset
          stick2.x = options.x + 30 - rubOffset
        } else {
          // Spark
          const sparkProgress = (elapsed - rubbingDuration) / sparkDuration
          if (!spark.visible) {
            spark.visible = true
            stick1.alpha = 0.5
            stick2.alpha = 0.5

            // Play the spark sound
            soundManager.play('exp_gain')
          }

          spark.alpha = 1 - sparkProgress
          // Scale from 1.0 to 3.0 (much larger)
          spark.scale.set(1.0 + sparkProgress * 2.0)
          stick1.alpha = Math.max(0, 0.5 - sparkProgress * 0.5)
          stick2.alpha = Math.max(0, 0.5 - sparkProgress * 0.5)
        }

        return true
      }
    }

    this.activeFx.add(fx)
  }
}

export const fxManager = FxManager.getInstance()
