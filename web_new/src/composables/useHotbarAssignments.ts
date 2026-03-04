import { computed, ref, watch, type Ref } from 'vue'
import {
  emptyHotbarState,
  isActionId,
  type ActionId,
  type HotbarAssignment,
  type HotbarState,
} from '@/game/hud/actionCatalog'

const HOTBAR_STORAGE_PREFIX = 'hotbar_assignments_v1'

function toStorageKey(accountId: string, characterId: number | null): string {
  if (!accountId || characterId == null) {
    return `${HOTBAR_STORAGE_PREFIX}:anonymous`
  }
  return `${HOTBAR_STORAGE_PREFIX}:${accountId}:${characterId}`
}

function parseHotbarState(raw: string | null): HotbarState {
  if (!raw) {
    return emptyHotbarState()
  }

  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed) || parsed.length !== 10) {
      return emptyHotbarState()
    }

    const normalized = parsed.map((item: unknown): HotbarAssignment => {
      if (typeof item !== 'string') {
        return null
      }
      return isActionId(item) ? item : null
    })

    return [
      normalized[0] ?? null,
      normalized[1] ?? null,
      normalized[2] ?? null,
      normalized[3] ?? null,
      normalized[4] ?? null,
      normalized[5] ?? null,
      normalized[6] ?? null,
      normalized[7] ?? null,
      normalized[8] ?? null,
      normalized[9] ?? null,
    ]
  } catch {
    return emptyHotbarState()
  }
}

function serializeHotbarState(state: HotbarState): string {
  return JSON.stringify(state)
}

export function useHotbarAssignments(accountId: Ref<string>, characterId: Ref<number | null>) {
  const assignments = ref<HotbarState>(emptyHotbarState())

  const storageKey = computed(() => toStorageKey(accountId.value, characterId.value))

  function loadForKey(key: string): void {
    const parsed = parseHotbarState(localStorage.getItem(key))
    assignments.value = parsed
    localStorage.setItem(key, serializeHotbarState(parsed))
  }

  function save(): void {
    localStorage.setItem(storageKey.value, serializeHotbarState(assignments.value))
  }

  function assign(slotIndex: number, actionId: ActionId): void {
    if (slotIndex < 0 || slotIndex >= 10) {
      return
    }
    const next = [...assignments.value] as HotbarState
    next[slotIndex] = actionId
    assignments.value = next
    save()
  }

  function clear(slotIndex: number): void {
    if (slotIndex < 0 || slotIndex >= 10) {
      return
    }
    const next = [...assignments.value] as HotbarState
    next[slotIndex] = null
    assignments.value = next
    save()
  }

  function get(slotIndex: number): HotbarAssignment {
    if (slotIndex < 0 || slotIndex >= 10) {
      return null
    }
    return assignments.value[slotIndex] ?? null
  }

  watch(
    storageKey,
    (key) => {
      loadForKey(key)
    },
    { immediate: true },
  )

  return {
    assignments,
    assign,
    clear,
    get,
    storageKey,
  }
}
