export function cancelActiveActionOnEscape(phase: string | null | undefined, cancel: () => void, cancelLocal?: () => boolean): boolean {
  if (cancelLocal?.()) return true
  if (!phase || phase === 'idle') return false
  cancel()
  return true
}
