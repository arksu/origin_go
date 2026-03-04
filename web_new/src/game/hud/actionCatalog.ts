export type ActionId = 'settings' | 'actions' | 'craft' | 'build' | 'stats' | 'inventory'

export type HotbarAssignment = ActionId | null
export type HotbarState = [
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
  HotbarAssignment,
]

export interface ActionCatalogEntry {
  id: ActionId
  label: string
  shortLabel: string
}

export const ACTION_CATALOG: readonly ActionCatalogEntry[] = [
  { id: 'settings', label: 'Settings', shortLabel: 'SET' },
  { id: 'actions', label: 'Actions', shortLabel: 'ACT' },
  { id: 'craft', label: 'Craft', shortLabel: 'CRF' },
  { id: 'build', label: 'Build', shortLabel: 'BLD' },
  { id: 'stats', label: 'Stats', shortLabel: 'STS' },
  { id: 'inventory', label: 'Inventory', shortLabel: 'INV' },
] as const

const ACTION_IDS = new Set<ActionId>(ACTION_CATALOG.map((entry) => entry.id))

export function isActionId(value: string): value is ActionId {
  return ACTION_IDS.has(value as ActionId)
}

export function getActionLabel(actionId: ActionId): string {
  const entry = ACTION_CATALOG.find((item) => item.id === actionId)
  return entry?.label ?? actionId
}

export function getActionShortLabel(actionId: ActionId): string {
  const entry = ACTION_CATALOG.find((item) => item.id === actionId)
  return entry?.shortLabel ?? actionId.slice(0, 3).toUpperCase()
}

export function emptyHotbarState(): HotbarState {
  return [null, null, null, null, null, null, null, null, null, null]
}
