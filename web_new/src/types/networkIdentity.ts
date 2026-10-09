import { util } from 'protobufjs/minimal'

/** Canonical nonzero protocol identity, rejecting unsafe numbers and fabricated Longs. */
export function decodeNonzeroUint64(value: unknown): string | null {
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value) || value <= 0) return null
  } else if (typeof value !== 'string') {
    if (typeof util.Long !== 'function' || !(value instanceof util.Long) || !Number.isInteger(value.low) || !Number.isInteger(value.high) ||
        value.low < -2147483648 || value.low > 2147483647 || value.high < -2147483648 || value.high > 2147483647 ||
        typeof value.unsigned !== 'boolean') return null
  }
  const decimal = String(value)
  if (!/^[1-9]\d{0,19}$/.test(decimal) || (decimal.length === 20 && decimal > '18446744073709551615')) return null
  return decimal
}

export function decodeGeneration(value: unknown): string {
  if (typeof value !== 'string' || !/^-?\d+:\d+$/.test(value) || value.length > 64) throw new Error('Invalid character generation')
  return value
}

export function decodeUint64(value: unknown): string {
  if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new Error('Unsafe character revision')
  const revision = String(value ?? 0)
  if (!/^(0|[1-9]\d{0,19})$/.test(revision) || (revision.length === 20 && revision > '18446744073709551615')) throw new Error('Invalid character revision')
  return revision
}

export function compareUint64(first: string, second: string): number {
  return first.length !== second.length ? Math.sign(first.length - second.length) : first === second ? 0 : first > second ? 1 : -1
}
