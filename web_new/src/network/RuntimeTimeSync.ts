import { shallowRef } from 'vue'
import { parseNonNegativeInt64 } from '@/utils/protoNumbers'

const MAX_RUNTIME_MS = ((1n << 63n) - 1n) * 1000n

/** Presentation-only runtime estimate. The calendar keeps its own received snapshot. */
export class RuntimeTimeSync {
  private anchor: { runtimeMs: bigint; monotonicMs: number } | null = null
  private presentationFloorMs: bigint | null = null
  private lastSample: { runtime: bigint; wallMs: number } | null = null
  private readonly nowMs = shallowRef<bigint | null>(null)
  private consumers = 0
  private frame: number | null = null

  /** Delivery age is half a matched Ping's monotonic RTT, independent of wall-clock corrections. */
  acceptSample(runtimeValue: unknown, wallValue: unknown, deliveryAgeMs: number): boolean {
    const runtime = parseNonNegativeInt64(runtimeValue)
    const wall = parseNonNegativeInt64(wallValue)
    if (runtime === null || wall === null || wall > BigInt(Number.MAX_SAFE_INTEGER) ||
      !Number.isFinite(deliveryAgeMs) || deliveryAgeMs < 0) return false
    const wallMs = Number(wall)
    if (this.lastSample && (runtime < this.lastSample.runtime ||
      (runtime === this.lastSample.runtime && wallMs === this.lastSample.wallMs))) return false

    const ageMs = Math.floor(deliveryAgeMs)
    if (!Number.isSafeInteger(ageMs)) return false
    const monotonicMs = performance.now()
    const previousMs = this.estimateRuntimeMs(monotonicMs)
    let runtimeMs = runtime * 1000n + BigInt(ageMs)
    if (runtimeMs > MAX_RUNTIME_MS) runtimeMs = MAX_RUNTIME_MS
    // Keep the fresh anchor correctable. A fixed display floor holds progress
    // until a lower estimate catches up instead of retaining the old lead.
    this.anchor = { runtimeMs, monotonicMs }
    this.presentationFloorMs = previousMs
    this.lastSample = { runtime, wallMs }
    this.nowMs.value = this.estimateRuntimeMs(monotonicMs)
    this.startFrame()
    return true
  }

  getRuntimeMs(): bigint | null { return this.nowMs.value }

  estimateRuntimeMs(monotonicMs = performance.now()): bigint | null {
    if (!this.anchor) return null
    const elapsedMs = Math.floor(Math.max(0, monotonicMs - this.anchor.monotonicMs))
    const runtimeMs = this.anchor.runtimeMs + (Number.isSafeInteger(elapsedMs) ? BigInt(elapsedMs) : 0n)
    const cappedMs = runtimeMs > MAX_RUNTIME_MS ? MAX_RUNTIME_MS : runtimeMs
    return this.presentationFloorMs !== null && cappedMs < this.presentationFloorMs ? this.presentationFloorMs : cappedMs
  }

  /** Mounted active item displays share a frame; paused/finished items need none. */
  subscribe(): () => void {
    this.consumers++
    this.nowMs.value = this.estimateRuntimeMs()
    this.startFrame()
    let released = false
    return () => {
      if (released) return
      released = true
      this.consumers--
      if (this.consumers === 0) this.stopFrame()
    }
  }

  reset(): void {
    this.stopFrame()
    this.anchor = null
    this.presentationFloorMs = null
    this.lastSample = null
    this.nowMs.value = null
  }

  private readonly updateFrame = (): void => {
    this.frame = null
    this.nowMs.value = this.estimateRuntimeMs()
    this.startFrame()
  }

  private startFrame(): void {
    if (this.frame === null && this.anchor && this.consumers > 0) this.frame = requestAnimationFrame(this.updateFrame)
  }

  private stopFrame(): void {
    if (this.frame !== null) cancelAnimationFrame(this.frame)
    this.frame = null
  }
}

export const runtimeTimeSync = new RuntimeTimeSync()
