import type { proto } from '@/network/proto/packets.js'

export type ActionId = 'settings' | 'actions' | 'craft' | 'build' | 'stats' | 'inventory' | 'equip'
export type HotbarActionId = ActionId | `game:${string}`

export type HotbarAssignment = HotbarActionId | null
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
  iconPath: string
}

export const ACTION_CATALOG: readonly ActionCatalogEntry[] = [
  { id: 'settings', label: 'Settings', shortLabel: 'SET', iconPath: '/assets/img/hud/actions/settings.svg' },
  { id: 'actions', label: 'Actions', shortLabel: 'ACT', iconPath: '/assets/img/hud/actions/actions.svg' },
  { id: 'craft', label: 'Craft', shortLabel: 'CRF', iconPath: '/assets/img/hud/actions/craft.svg' },
  { id: 'build', label: 'Build', shortLabel: 'BLD', iconPath: '/assets/img/hud/actions/build.svg' },
  { id: 'stats', label: 'Stats', shortLabel: 'STS', iconPath: '/assets/img/hud/actions/stats.svg' },
  { id: 'inventory', label: 'Inventory', shortLabel: 'INV', iconPath: '/assets/img/hud/actions/inventory.svg' },
  { id: 'equip', label: 'Equipment', shortLabel: 'EQP', iconPath: '/assets/img/hud/actions/equip.svg' },
] as const

const ACTION_IDS = new Set<ActionId>(ACTION_CATALOG.map((entry) => entry.id))

export function isActionId(value: string): value is ActionId {
  return ACTION_IDS.has(value as ActionId)
}

export function isHotbarActionId(value: string): value is HotbarActionId {
  return isActionId(value) || /^game:[a-z][a-z0-9_]*$/.test(value)
}

export function gameActionHotbarId(actionId: string): `game:${string}` {
  return `game:${actionId}`
}

export function requestGameAction(actionId: string, actions: readonly proto.IActionDefinition[], listLoaded: boolean, send: (id: string) => void): boolean {
  if (!listLoaded || !actions.some(action => action.id === actionId)) return false
  send(actionId)
  return true
}

export function getActionLabel(actionId: ActionId): string {
  const entry = ACTION_CATALOG.find((item) => item.id === actionId)
  return entry?.label ?? actionId
}

export function getActionShortLabel(actionId: ActionId): string {
  const entry = ACTION_CATALOG.find((item) => item.id === actionId)
  return entry?.shortLabel ?? actionId.slice(0, 3).toUpperCase()
}

export function getActionIconPath(actionId: ActionId): string {
  const entry = ACTION_CATALOG.find((item) => item.id === actionId)
  return entry?.iconPath ?? ''
}

export function emptyHotbarState(): HotbarState {
  return [null, null, null, null, null, null, null, null, null, null]
}
