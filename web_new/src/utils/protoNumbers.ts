/** Preserve signed int64 precision; unsafe JS numbers cannot be recovered. */
export function parseNonNegativeInt64(value: unknown): bigint | null {
  try {
    if (typeof value === 'number' && !Number.isSafeInteger(value)) return null
    if (value == null || Array.isArray(value) || !['number', 'bigint', 'string', 'object'].includes(typeof value)) return null
    const decimal = String(value)
    if (!/^(0|[1-9]\d*)$/.test(decimal)) return null
    const parsed = BigInt(decimal)
    return parsed <= (1n << 63n) - 1n ? parsed : null
  } catch { return null }
}

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

