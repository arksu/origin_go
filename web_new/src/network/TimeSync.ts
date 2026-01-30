/**
 * TimeSync - estimates server time on client based on Ping/Pong RTT measurements.
 * Uses EWMA (Exponential Weighted Moving Average) to smooth offset estimation.
 */

// Constants
const EWMA_ALPHA = 0.2 // smoothing factor for offset
const JITTER_EWMA_ALPHA = 0.1 // smoothing factor for jitter
const MAX_SAMPLES = 20 // max samples to keep for median calculation
const BASE_INTERPOLATION_DELAY_MS = 120
const MIN_INTERPOLATION_DELAY_MS = 80
const MAX_INTERPOLATION_DELAY_MS = 250
const JITTER_MULTIPLIER = 2.5 // interpolationDelay = base + jitter * multiplier

interface PingSample {
  clientSendMs: number
  clientReceiveMs: number
  serverTimeMs: number
  rttMs: number
  offsetMs: number
}

class TimeSync {
  private samples: PingSample[] = []
  private smoothedOffsetMs = 0
  private smoothedRttMs = 0
  private smoothedJitterMs = 0
  private lastRttMs = 0
  private initialized = false

  /**
   * Record a pong response from server.
   * @param clientSendMs - client timestamp when ping was sent
   * @param serverTimeMs - server timestamp from pong
   */
  onPong(clientSendMs: number, serverTimeMs: number): void {
    const clientReceiveMs = Date.now()
    const rttMs = clientReceiveMs - clientSendMs
    
    // Estimate offset: server_now ≈ client_now + offset
    // At send time: serverTime ≈ clientSend + offset + rtt/2
    // So: offset ≈ serverTime - clientSend - rtt/2
    const offsetMs = serverTimeMs - clientSendMs - rttMs / 2

    const sample: PingSample = {
      clientSendMs,
      clientReceiveMs,
      serverTimeMs,
      rttMs,
      offsetMs,
    }

    this.samples.push(sample)
    if (this.samples.length > MAX_SAMPLES) {
      this.samples.shift()
    }

    // Calculate jitter as deviation from smoothed RTT
    const jitter = Math.abs(rttMs - this.smoothedRttMs)

    if (!this.initialized) {
      // First sample - initialize directly
      this.smoothedOffsetMs = offsetMs
      this.smoothedRttMs = rttMs
      this.smoothedJitterMs = jitter
      this.initialized = true
    } else {
      // EWMA smoothing
      this.smoothedOffsetMs = EWMA_ALPHA * offsetMs + (1 - EWMA_ALPHA) * this.smoothedOffsetMs
      this.smoothedRttMs = EWMA_ALPHA * rttMs + (1 - EWMA_ALPHA) * this.smoothedRttMs
      this.smoothedJitterMs = JITTER_EWMA_ALPHA * jitter + (1 - JITTER_EWMA_ALPHA) * this.smoothedJitterMs
    }

    this.lastRttMs = rttMs
  }

  /**
   * Estimate current server time based on client time.
   */
  estimateServerNowMs(clientNowMs: number = Date.now()): number {
    return clientNowMs + this.smoothedOffsetMs
  }

  /**
   * Get the recommended interpolation delay.
   * Higher jitter = higher delay for smoother interpolation.
   */
  getInterpolationDelayMs(): number {
    const delay = BASE_INTERPOLATION_DELAY_MS + this.smoothedJitterMs * JITTER_MULTIPLIER
    return Math.max(MIN_INTERPOLATION_DELAY_MS, Math.min(MAX_INTERPOLATION_DELAY_MS, delay))
  }

  /**
   * Get current RTT estimate.
   */
  getRttMs(): number {
    return this.smoothedRttMs
  }

  /**
   * Get last measured RTT.
   */
  getLastRttMs(): number {
    return this.lastRttMs
  }

  /**
   * Get current jitter estimate.
   */
  getJitterMs(): number {
    return this.smoothedJitterMs
  }

  /**
   * Get current time offset estimate.
   */
  getOffsetMs(): number {
    return this.smoothedOffsetMs
  }

  /**
   * Check if TimeSync has been initialized with at least one sample.
   */
  isInitialized(): boolean {
    return this.initialized
  }

  /**
   * Get debug metrics.
   */
  getDebugMetrics(): {
    rttMs: number
    jitterMs: number
    offsetMs: number
    interpolationDelayMs: number
    sampleCount: number
  } {
    return {
      rttMs: Math.round(this.smoothedRttMs),
      jitterMs: Math.round(this.smoothedJitterMs),
      offsetMs: Math.round(this.smoothedOffsetMs),
      interpolationDelayMs: Math.round(this.getInterpolationDelayMs()),
      sampleCount: this.samples.length,
    }
  }

  /**
   * Reset all state.
   */
  reset(): void {
    this.samples = []
    this.smoothedOffsetMs = 0
    this.smoothedRttMs = 0
    this.smoothedJitterMs = 0
    this.lastRttMs = 0
    this.initialized = false
  }
}

export const timeSync = new TimeSync()
