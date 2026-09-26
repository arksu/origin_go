export interface ChunkEventIdentity {
  readonly streamEpoch: number
  readonly eventSeq: bigint
}

type WireSequence = number | { toString(): string } | null | undefined

function sequenceValue(value: WireSequence): bigint | null {
  if (typeof value === 'number' && !Number.isSafeInteger(value)) return null
  const decimal = value?.toString() ?? '0'
  if (!/^[1-9]\d{0,19}$/.test(decimal)) return null
  const sequence = BigInt(decimal)
  return sequence <= 18446744073709551615n ? sequence : null
}

/** Event order and tile history outlive renderer cache entries and AOI unloads. */
export class ChunkStreamGuard {
  private epoch = 0
  private lastEvents = new Map<string, bigint>()
  private versions = new Map<string, number>()

  reset(epoch: number): void {
    this.epoch = epoch
    this.lastEvents.clear()
    this.versions.clear()
  }

  accept(x: number, y: number, epoch: number | null | undefined, sequence: WireSequence, version?: number): ChunkEventIdentity | null {
    if (!this.epoch || epoch !== this.epoch) return null
    const eventSeq = sequenceValue(sequence)
    const key = `${x},${y}`
    if (eventSeq == null || eventSeq <= (this.lastEvents.get(key) ?? 0n)) return null
    if (version != null) {
      if (!Number.isInteger(version) || version < 0 || version > 0xffffffff || version < (this.versions.get(key) ?? 0)) return null
      this.versions.set(key, version)
    }
    this.lastEvents.set(key, eventSeq)
    return { streamEpoch: epoch, eventSeq }
  }
}
