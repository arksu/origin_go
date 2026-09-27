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
