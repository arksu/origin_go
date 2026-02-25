export function prettyJson(value: unknown): string {
  return JSON.stringify(value, null, 2) + '\n'
}

function splitLines(text: string): string[] {
  const lines = text.split('\n')
  if (lines.length > 0 && lines[lines.length - 1] === '') {
    lines.pop()
  }
  return lines
}

export function generateUnifiedDiff(
  beforeText: string,
  afterText: string,
  beforeName = 'before',
  afterName = 'after',
): string {
  if (beforeText === afterText) return ''

  const a = splitLines(beforeText)
  const b = splitLines(afterText)

  let prefix = 0
  while (prefix < a.length && prefix < b.length && a[prefix] === b[prefix]) {
    prefix++
  }

  let suffix = 0
  while (
    suffix < a.length - prefix &&
    suffix < b.length - prefix &&
    a[a.length - 1 - suffix] === b[b.length - 1 - suffix]
  ) {
    suffix++
  }

  const context = 3
  const aChangeStart = prefix
  const bChangeStart = prefix
  const aChangeEnd = a.length - suffix
  const bChangeEnd = b.length - suffix

  const aHunkStartIdx = Math.max(0, aChangeStart - context)
  const bHunkStartIdx = Math.max(0, bChangeStart - context)
  const aHunkEndIdx = Math.min(a.length, aChangeEnd + context)
  const bHunkEndIdx = Math.min(b.length, bChangeEnd + context)

  const out: string[] = []
  out.push(`--- ${beforeName}`)
  out.push(`+++ ${afterName}`)

  const aStartLine = aHunkStartIdx + 1
  const bStartLine = bHunkStartIdx + 1
  const aCount = aHunkEndIdx - aHunkStartIdx
  const bCount = bHunkEndIdx - bHunkStartIdx
  out.push(`@@ -${aStartLine},${aCount} +${bStartLine},${bCount} @@`)

  const sharedPrefixStart = aHunkStartIdx
  const sharedPrefixEnd = aChangeStart
  for (let i = sharedPrefixStart; i < sharedPrefixEnd; i++) {
    out.push(` ${a[i]}`)
  }
  for (let i = aChangeStart; i < aChangeEnd; i++) {
    out.push(`-${a[i]}`)
  }
  for (let i = bChangeStart; i < bChangeEnd; i++) {
    out.push(`+${b[i]}`)
  }
  const sharedSuffixStart = aChangeEnd
  const sharedSuffixEnd = aHunkEndIdx
  for (let i = sharedSuffixStart; i < sharedSuffixEnd; i++) {
    out.push(` ${a[i]}`)
  }

  return out.join('\n') + '\n'
}
