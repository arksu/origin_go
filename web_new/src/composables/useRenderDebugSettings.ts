import { ref } from 'vue'

export const RENDER_DEBUG_STORAGE_KEY = 'origin_render_debug_v1'

type RenderDebugStorage = Pick<Storage, 'getItem' | 'setItem'>

function getBrowserStorage(): RenderDebugStorage | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export function loadRenderDebugEnabled(
  storage: RenderDebugStorage | null = getBrowserStorage(),
  defaultEnabled = false,
): boolean {
  try {
    const savedValue = storage?.getItem(RENDER_DEBUG_STORAGE_KEY) ?? null
    if (savedValue === 'true') return true
    if (savedValue === 'false') return false
  } catch {
    // Rendering must remain usable when browser storage is disabled or unavailable.
  }

  return defaultEnabled
}

export function persistRenderDebugEnabled(
  enabled: boolean,
  storage: RenderDebugStorage | null = getBrowserStorage(),
): void {
  try {
    storage?.setItem(RENDER_DEBUG_STORAGE_KEY, String(enabled))
  } catch {
    // Rendering must remain usable when browser storage is disabled or full.
  }
}

export function useRenderDebugSettings(defaultEnabled = false) {
  const enabled = ref(loadRenderDebugEnabled(getBrowserStorage(), defaultEnabled))

  function setEnabled(nextEnabled: boolean): void {
    enabled.value = nextEnabled
    persistRenderDebugEnabled(nextEnabled)
  }

  return { enabled, setEnabled }
}
