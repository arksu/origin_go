import { ref } from 'vue'

export const ECONOMY_RENDERING_STORAGE_KEY = 'origin_economy_rendering_v1'

type EconomyRenderingStorage = Pick<Storage, 'getItem' | 'setItem'>

function getBrowserStorage(): EconomyRenderingStorage | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export function loadEconomyRendering(
  storage: EconomyRenderingStorage | null = getBrowserStorage(),
): boolean {
  try {
    return storage?.getItem(ECONOMY_RENDERING_STORAGE_KEY) === 'true'
  } catch {
    return false
  }
}

export function persistEconomyRendering(
  enabled: boolean,
  storage: EconomyRenderingStorage | null = getBrowserStorage(),
): void {
  try {
    storage?.setItem(ECONOMY_RENDERING_STORAGE_KEY, String(enabled))
  } catch {
    // Rendering must remain usable when browser storage is disabled or full.
  }
}

export function useEconomyRendering() {
  const enabled = ref(loadEconomyRendering())

  function setEnabled(nextEnabled: boolean): void {
    enabled.value = nextEnabled
    persistEconomyRendering(nextEnabled)
  }

  return { enabled, setEnabled }
}
