import { ref } from 'vue'

export function useActionsPanel() {
  const isOpen = ref(false)

  function toggle(): void {
    isOpen.value = !isOpen.value
  }

  function close(): void {
    isOpen.value = false
  }

  function closeOnOutsidePointer(target: EventTarget | null): void {
    if (!isOpen.value) return
    const element = target as Element | null
    if (!element?.closest?.('[data-actions-menu], [data-actions-toggle]')) close()
  }

  return { isOpen, toggle, close, closeOnOutsidePointer }
}
