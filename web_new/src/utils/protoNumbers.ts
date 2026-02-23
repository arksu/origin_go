export function toNonNegativeProtoInt(value: unknown): number {
  if (value == null) return 0
  if (typeof value === 'number') {
    if (!Number.isFinite(value) || value <= 0) return 0
    return Math.trunc(value)
  }
  if (typeof value === 'object' && 'toNumber' in value) {
    const toNumber = (value as { toNumber?: unknown }).toNumber
    if (typeof toNumber === 'function') {
      const parsed = toNumber.call(value)
      if (!Number.isFinite(parsed) || parsed <= 0) return 0
      return Math.trunc(parsed)
    }
  }
  return 0
}

