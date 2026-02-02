import { ref, onMounted, onUnmounted } from 'vue'
import type { HotkeyConfig } from '@/constants/hotkeys'

export function useHotkeys(hotkeys: HotkeyConfig[]) {
  const isEnabled = ref(true)

  // Normalize key event to match our config format
  function normalizeKeyEvent(event: KeyboardEvent): { key: string; modifiers: string[] } {
    const modifiers: string[] = []
    if (event.ctrlKey) modifiers.push('ctrl')
    if (event.shiftKey) modifiers.push('shift')
    if (event.altKey) modifiers.push('alt')
    
    return {
      key: event.key.toLowerCase(),
      modifiers
    }
  }

  // Check if event matches hotkey config
  function matchesHotkey(event: KeyboardEvent, config: HotkeyConfig): boolean {
    if (!isEnabled.value) return false
    
    const normalized = normalizeKeyEvent(event)
    
    // Check key match
    if (normalized.key !== config.key.toLowerCase()) return false
    
    // Check modifiers match
    const configModifiers = config.modifiers || []
    const normalizedModifiers = normalized.modifiers
    
    // Same number of modifiers
    if (configModifiers.length !== normalizedModifiers.length) return false
    
    // All modifiers match
    return configModifiers.every(mod => normalizedModifiers.includes(mod))
  }

  // Handle keyboard events
  function handleKeyDown(event: KeyboardEvent) {
    // Don't trigger hotkeys when typing in input fields (except Enter for chat focus)
    const isInputElement = event.target instanceof HTMLInputElement || 
                          event.target instanceof HTMLTextAreaElement
    
    if (isInputElement && event.key !== 'Enter') return

    // Find matching hotkey
    const matchingHotkey = hotkeys.find(config => matchesHotkey(event, config))
    
    if (matchingHotkey) {
      event.preventDefault()
      event.stopPropagation()
      matchingHotkey.action()
    }
  }

  // Enable/disable hotkeys
  function enable() {
    isEnabled.value = true
  }

  function disable() {
    isEnabled.value = false
  }

  // Setup event listeners
  onMounted(() => {
    document.addEventListener('keydown', handleKeyDown)
  })

  onUnmounted(() => {
    document.removeEventListener('keydown', handleKeyDown)
  })

  return {
    enable,
    disable,
    isEnabled
  }
}
