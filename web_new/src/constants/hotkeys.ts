// Game hotkeys configuration
export interface HotkeyConfig {
  key: string
  modifiers?: ('ctrl' | 'shift' | 'alt')[]
  description: string
  action: () => void
}

// Default hotkey configurations
export const DEFAULT_HOTKEYS: HotkeyConfig[] = [
  {
    key: 'Enter',
    description: 'Focus chat input',
    action: () => {
      // Will be injected by GameView
    }
  },
  {
    key: 'Escape',
    description: 'Unfocus chat input / clear selection',
    action: () => {
      // Will be injected by GameView
    }
  },
  {
    key: '/',
    modifiers: ['shift'],
    description: 'Open chat with slash command',
    action: () => {
      // Will be injected by GameView
    }
  },
  {
    key: 'm',
    description: 'Toggle map (if implemented)',
    action: () => {
      console.log('[Hotkeys] Map toggle - not implemented yet')
    }
  },
  {
    key: 'tab',
    description: 'Toggle inventory',
    action: () => {
      console.log('[Hotkeys] Inventory toggle - not implemented yet')
    }
  },
  {
    key: 'i',
    description: 'Toggle inventory (alternative)',
    action: () => {
      console.log('[Hotkeys] Inventory toggle - not implemented yet')
    }
  },
  {
    key: 'c',
    description: 'Toggle character sheet',
    action: () => {
      console.log('[Hotkeys] Character sheet toggle - not implemented yet')
    }
  },
  {
    key: '`',
    description: 'Toggle debug overlay',
    action: () => {
      console.log('[Hotkeys] Debug overlay toggle - not implemented yet')
    }
  },
  {
    key: 'f',
    description: 'Toggle fullscreen',
    action: () => {
      if (!document.fullscreenElement) {
        document.documentElement.requestFullscreen()
      } else {
        document.exitFullscreen()
      }
    }
  }
]
