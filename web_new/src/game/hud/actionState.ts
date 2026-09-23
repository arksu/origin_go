export function cancelActiveActionOnEscape(phase: string | null | undefined, cancel: () => void): boolean {
  if (!phase || phase === 'idle') return false
  cancel()
  return true
}
