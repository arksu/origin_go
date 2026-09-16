import { ref } from 'vue'
import { DEFAULT_ACTOR_RENDER_SETTINGS, type ActorRenderSettings } from '@/game/actors/config'

export type ActorRenderMode = ActorRenderSettings['mode']

export const ACTOR_RENDER_MODE_STORAGE_KEY = 'origin_actor_render_mode_v1'

type ActorRenderModeStorage = Pick<Storage, 'getItem' | 'setItem'>

function isActorRenderMode(value: unknown): value is ActorRenderMode {
  return value === 'hybrid3d' || value === 'baked8'
}

function getBrowserStorage(): ActorRenderModeStorage | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export function loadActorRenderMode(storage: ActorRenderModeStorage | null = getBrowserStorage()): ActorRenderMode {
  try {
    const savedMode = storage?.getItem(ACTOR_RENDER_MODE_STORAGE_KEY) ?? null
    return isActorRenderMode(savedMode) ? savedMode : DEFAULT_ACTOR_RENDER_SETTINGS.mode
  } catch {
    return DEFAULT_ACTOR_RENDER_SETTINGS.mode
  }
}

export function persistActorRenderMode(mode: ActorRenderMode, storage: ActorRenderModeStorage | null = getBrowserStorage()): void {
  try {
    storage?.setItem(ACTOR_RENDER_MODE_STORAGE_KEY, mode)
  } catch {
    // Rendering must remain usable when browser storage is disabled or full.
  }
}

export function useActorRenderSettings() {
  const mode = ref<ActorRenderMode>(loadActorRenderMode())

  function setMode(nextMode: ActorRenderMode): void {
    mode.value = nextMode
    persistActorRenderMode(nextMode)
  }

  return { mode, setMode }
}
