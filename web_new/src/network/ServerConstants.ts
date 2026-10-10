import { shallowRef } from 'vue'

/** Immutable parameters owned by one authenticated server connection. */
export interface ServerConstantsSnapshot {
  readonly coordPerTile: number
  readonly chunkSize: number
  readonly tickRate: number
  readonly directionalMovementSupported: boolean
  readonly realSecondsPerGameDay: number
  readonly hoursPerDay: number
  readonly daysPerMonth: number
  readonly monthsPerYear: number
}

const numericFields = [
  'coordPerTile', 'chunkSize', 'tickRate', 'realSecondsPerGameDay',
  'hoursPerDay', 'daysPerMonth', 'monthsPerYear',
] as const

export class ServerConstants {
  private readonly snapshot = shallowRef<ServerConstantsSnapshot | null>(null)

  getSnapshot(): ServerConstantsSnapshot | null { return this.snapshot.value }
  isReady(): boolean { return this.snapshot.value !== null }
  requireSnapshot(): ServerConstantsSnapshot {
    if (!this.snapshot.value) throw new Error('Server constants are unavailable')
    return this.snapshot.value
  }

  accept(value: unknown): 'accepted' | 'duplicate' | 'invalid' | 'changed' {
    if (!value || typeof value !== 'object') return 'invalid'
    const fields = value as Record<string, unknown>
    for (const key of numericFields) {
      if (typeof fields[key] !== 'number' || !Number.isInteger(fields[key]) || fields[key] <= 0 || fields[key] > 0xffffffff) return 'invalid'
    }
    if (typeof fields.directionalMovementSupported !== 'boolean') return 'invalid'
    const next = Object.freeze({
      coordPerTile: fields.coordPerTile as number,
      chunkSize: fields.chunkSize as number,
      tickRate: fields.tickRate as number,
      directionalMovementSupported: fields.directionalMovementSupported,
      realSecondsPerGameDay: fields.realSecondsPerGameDay as number,
      hoursPerDay: fields.hoursPerDay as number,
      daysPerMonth: fields.daysPerMonth as number,
      monthsPerYear: fields.monthsPerYear as number,
    })
    if (this.snapshot.value) {
      return numericFields.every(key => this.snapshot.value![key] === next[key]) &&
        this.snapshot.value.directionalMovementSupported === next.directionalMovementSupported ? 'duplicate' : 'changed'
    }
    this.snapshot.value = next
    return 'accepted'
  }

  reset(): void { this.snapshot.value = null }
}

export const serverConstants = new ServerConstants()
