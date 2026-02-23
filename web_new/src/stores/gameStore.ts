import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { CHAT_MESSAGE_LIFETIME_MS, CHAT_FADEOUT_DURATION_MS, CHAT_CLEANUP_INTERVAL_MS, CHAT_MAX_MESSAGES } from '@/constants/chat'
import type { ConnectionState, ConnectionError } from '@/network/types'

export interface Position {
  x: number
  y: number
  heading: number
}

export interface EntityMovement {
  position: Position
  velocity: { x: number; y: number }
  moveMode: number
  targetPosition?: { x: number; y: number }
  isMoving: boolean
}

export interface GameObjectData {
  entityId: number
  typeId: number
  resourcePath: string
  position: { x: number; y: number }
  size: { x: number; y: number }
  movement?: EntityMovement
}

export interface ChunkData {
  x: number
  y: number
  tiles: Uint8Array
  version: number
}

export interface WorldParams {
  coordPerTile: number
  chunkSize: number
  streamEpoch: number
}

export interface ChatMessage {
  id: string
  fromName: string
  text: string
  timestamp: number
  channel: proto.ChatChannel
}

export interface ContextMenuActionItem {
  actionId: string
  title: string
}

export interface ContextMenuState {
  entityId: number
  actions: ContextMenuActionItem[]
  anchorX: number
  anchorY: number
}

export interface MiniAlertInput {
  reasonCode: string
  message?: string
  severity: proto.AlertSeverity
  ttlMs: number
}

export interface MiniAlertItem {
  id: string
  debounceKey: string
  reasonCode: string
  message: string
  severity: proto.AlertSeverity
  createdAt: number
  expiresAt: number
}

export interface ActionProgress {
  total: number
  current: number
}

export interface PlayerResourceStat {
  current: number
  max: number
}

export interface PlayerStatsState {
  stamina: PlayerResourceStat
  energy: PlayerResourceStat
}

export interface CharacterAttributeViewItem {
  key: proto.CharacterAttributeKey
  label: string
  value: number
  icon: string
}

export interface CharacterExperienceState {
  lp: number
  nature: number
  industry: number
  combat: number
}

export type CraftRecipeState = proto.ICraftRecipeEntry
export type BuildRecipeState = proto.IBuildRecipeEntry

export type WorldBootstrapState =
  | 'idle'
  | 'waiting_enter_world'
  | 'waiting_first_chunk'
  | 'waiting_player_spawn'
  | 'ready'

export const useGameStore = defineStore('game', () => {
  // Session
  const wsToken = ref('')
  const characterId = ref<number | null>(null)

  // Connection
  const connectionState = ref<ConnectionState>('disconnected')
  const connectionError = ref<ConnectionError | null>(null)
  const lastServerErrorMessage = ref('')
  const worldBootstrapState = ref<WorldBootstrapState>('idle')

  // Player
  const playerEntityId = ref<number | null>(null)
  const playerName = ref('')
  const playerPosition = ref<Position>({ x: 0, y: 0, heading: 0 })

  // World
  const worldParams = ref<WorldParams | null>(null)
  const chunks = ref(new Map<string, ChunkData>())
  const entities = ref(new Map<number, GameObjectData>())

  // Chat
  const chatMessages = ref<ChatMessage[]>([])
  let cleanupTimer: ReturnType<typeof setInterval> | null = null

  // Inventory
  const inventories = ref(new Map<string, proto.IInventoryState>())
  const playerInventoryVisible = ref(false)
  const playerEquipmentVisible = ref(false)
  const openNestedInventories = ref(new Map<string, proto.IInventoryState>())
  const openedRootContainerRefs = ref(new Set<string>())
  let nextOpId = 1
  const mousePos = ref({ x: 0, y: 0 })
  const contextMenu = ref<ContextMenuState | null>(null)
  const miniAlerts = ref<MiniAlertItem[]>([])
  const miniAlertDebounceUntil = new Map<string, number>()
  let miniAlertTimer: ReturnType<typeof setInterval> | null = null
  const actionProgress = ref<ActionProgress>({
    total: 0,
    current: 0,
  })
  const characterSheetVisible = ref(false)
  const playerStatsWindowVisible = ref(false)
  const craftWindowVisible = ref(false)
  const craftRecipes = ref<CraftRecipeState[]>([])
  const selectedCraftKey = ref<string>('')
  const buildWindowVisible = ref(false)
  const buildRecipes = ref<BuildRecipeState[]>([])
  const selectedBuildKey = ref<string>('')
  const armedBuildKey = ref<string>('')
  const characterAttributes = ref<CharacterAttributeViewItem[]>(defaultCharacterAttributes())
  const characterExperience = ref<CharacterExperienceState>(defaultCharacterExperience())
  const playerStats = ref<PlayerStatsState>(defaultPlayerStats())
  const playerMoveMode = ref<number | null>(null)
  const hasBootstrapFirstChunk = ref(false)
  const hasBootstrapPlayerSpawn = ref(false)

  // Computed
  const isConnected = computed(() => connectionState.value === 'connected')
  const isInGame = computed(() => isConnected.value && playerEntityId.value !== null)
  const actionFrame = computed(() => {
    if (actionProgress.value.total <= 0) return 0
    return Math.round((actionProgress.value.current / actionProgress.value.total) * 21)
  })

  const handState = computed((): proto.IInventoryHandState | null => {
    if (!playerEntityId.value) return null
    // HAND = kind 1, inventoryKey 0
    const key = `1_${playerEntityId.value}_0`
    const inv = inventories.value.get(key)
    if (!inv?.hand?.item) return null
    return inv.hand
  })

  const handInventoryState = computed((): proto.IInventoryState | undefined => {
    if (!playerEntityId.value) return undefined
    const key = `1_${playerEntityId.value}_0`
    return inventories.value.get(key)
  })

  // Session actions
  function setGameSession(token: string, charId: number) {
    wsToken.value = token
    characterId.value = charId
  }

  function clearGameSession() {
    wsToken.value = ''
    characterId.value = null
  }

  // Connection actions
  function setConnectionState(state: ConnectionState, error?: ConnectionError) {
    connectionState.value = state
    connectionError.value = error ?? null
  }

  function syncWorldBootstrapState() {
    if (worldBootstrapState.value === 'idle' || worldBootstrapState.value === 'waiting_enter_world') {
      return
    }
    if (hasBootstrapFirstChunk.value && hasBootstrapPlayerSpawn.value) {
      worldBootstrapState.value = 'ready'
      return
    }
    if (hasBootstrapFirstChunk.value) {
      worldBootstrapState.value = 'waiting_player_spawn'
      return
    }
    worldBootstrapState.value = 'waiting_first_chunk'
  }

  function startWorldBootstrap() {
    hasBootstrapFirstChunk.value = false
    hasBootstrapPlayerSpawn.value = false
    worldBootstrapState.value = 'waiting_enter_world'
  }

  function markPlayerEnterWorldBootstrap() {
    hasBootstrapFirstChunk.value = false
    hasBootstrapPlayerSpawn.value = false
    worldBootstrapState.value = 'waiting_first_chunk'
  }

  function markBootstrapFirstChunkLoaded() {
    if (worldBootstrapState.value === 'idle') return
    hasBootstrapFirstChunk.value = true
    syncWorldBootstrapState()
  }

  function markBootstrapPlayerSpawned() {
    if (worldBootstrapState.value === 'idle') return
    hasBootstrapPlayerSpawn.value = true
    syncWorldBootstrapState()
  }

  function resetWorldBootstrap() {
    hasBootstrapFirstChunk.value = false
    hasBootstrapPlayerSpawn.value = false
    worldBootstrapState.value = 'idle'
  }

  function setLastServerErrorMessage(message: string) {
    lastServerErrorMessage.value = message.trim()
  }

  function clearLastServerErrorMessage() {
    lastServerErrorMessage.value = ''
  }

  // Player actions
  function setPlayerEnterWorld(
    entityId: number,
    name: string,
    coordPerTile: number,
    chunkSize: number,
    streamEpoch: number,
  ) {
    playerEntityId.value = entityId
    playerName.value = name
    worldParams.value = { coordPerTile, chunkSize, streamEpoch }

    // Clear inventories when entering new world
    console.log('[gameStore] Clearing inventories on world enter')
    inventories.value.clear()
    openNestedInventories.value.clear()  // Clear nested inventories
    openedRootContainerRefs.value.clear()
    playerInventoryVisible.value = false
    playerEquipmentVisible.value = false
    characterSheetVisible.value = false
    playerStatsWindowVisible.value = false
    craftWindowVisible.value = false
    craftRecipes.value = []
    selectedCraftKey.value = ''
    buildWindowVisible.value = false
    buildRecipes.value = []
    selectedBuildKey.value = ''
    armedBuildKey.value = ''
    characterAttributes.value = defaultCharacterAttributes()
    characterExperience.value = defaultCharacterExperience()
    playerStats.value = defaultPlayerStats()
    playerMoveMode.value = null
    contextMenu.value = null
    miniAlerts.value = []
    miniAlertDebounceUntil.clear()
    clearLastServerErrorMessage()
    resetWorldBootstrap()
    if (miniAlertTimer) {
      clearInterval(miniAlertTimer)
      miniAlertTimer = null
    }
    clearActionProgress()
  }

  function setPlayerLeaveWorld() {
    playerEntityId.value = null
    playerName.value = ''
    worldParams.value = null
    chunks.value.clear()
    entities.value.clear()

    // Clear inventories when leaving world
    console.log('[gameStore] Clearing inventories on world leave')
    inventories.value.clear()
    openNestedInventories.value.clear()  // Clear nested inventories
    openedRootContainerRefs.value.clear()
    playerInventoryVisible.value = false
    playerEquipmentVisible.value = false
    characterSheetVisible.value = false
    playerStatsWindowVisible.value = false
    craftWindowVisible.value = false
    craftRecipes.value = []
    selectedCraftKey.value = ''
    buildWindowVisible.value = false
    buildRecipes.value = []
    selectedBuildKey.value = ''
    armedBuildKey.value = ''
    characterAttributes.value = defaultCharacterAttributes()
    characterExperience.value = defaultCharacterExperience()
    playerStats.value = defaultPlayerStats()
    playerMoveMode.value = null
    contextMenu.value = null
    miniAlerts.value = []
    miniAlertDebounceUntil.clear()
    resetWorldBootstrap()
    if (miniAlertTimer) {
      clearInterval(miniAlertTimer)
      miniAlertTimer = null
    }
    clearActionProgress()
  }

  function updatePlayerPosition(position: Position) {
    playerPosition.value = position
  }

  // Chunk actions
  function chunkKey(x: number, y: number): string {
    return `${x},${y}`
  }

  function loadChunk(x: number, y: number, tiles: Uint8Array, version: number) {
    chunks.value.set(chunkKey(x, y), { x, y, tiles, version })
  }

  function unloadChunk(x: number, y: number) {
    chunks.value.delete(chunkKey(x, y))
  }

  // Entity actions
  function spawnEntity(data: GameObjectData) {
    entities.value.set(data.entityId, data)
  }

  function despawnEntity(entityId: number) {
    entities.value.delete(entityId)
  }

  function updateEntityMovement(entityId: number, movement: EntityMovement) {
    const entity = entities.value.get(entityId)
    if (entity) {
      entity.movement = movement
    }
  }

  function setPlayerMoveMode(entityId: number, moveMode: number) {
    if (entityId !== playerEntityId.value) {
      return
    }
    playerMoveMode.value = moveMode
  }

  // Inventory actions
  function refKey(r: proto.IInventoryRef): string {
    return `${r.kind ?? 0}_${r.ownerId ?? 0}_${r.inventoryKey ?? 0}`
  }

  function inventoryKey(state: proto.IInventoryState): string {
    if (!state.ref) return 'no_ref'
    return refKey(state.ref)
  }

  function updateInventory(state: proto.IInventoryState) {
    if (state.ref) {
      const key = inventoryKey(state)
      inventories.value.set(key, state)

      // Also update openNestedInventories if this container is currently open
      if (openNestedInventories.value.has(key)) {
        openNestedInventories.value.set(key, state)
      }
    }

    validateOpenNestedInventories()
  }

  function collectNestedRefsFromGrids(): Set<string> {
    const refs = new Set<string>()
    for (const inv of inventories.value.values()) {
      if (!inv.grid?.items) continue
      for (const gridItem of inv.grid.items) {
        if (gridItem.item?.nestedRef) {
          refs.add(refKey(gridItem.item.nestedRef))
        }
      }
    }

    for (const inv of openNestedInventories.value.values()) {
      if (!inv.grid?.items) continue
      for (const gridItem of inv.grid.items) {
        if (gridItem.item?.nestedRef) {
          refs.add(refKey(gridItem.item.nestedRef))
        }
      }
    }
    return refs
  }

  function validateOpenNestedInventories() {
    if (openNestedInventories.value.size === 0) return

    const validRefs = collectNestedRefsFromGrids()
    const toClose: string[] = []

    for (const windowKey of openNestedInventories.value.keys()) {
      // Root world-object containers are opened explicitly by the server and
      // should only close via S2C_ContainerClosed/unlink flow.
      if (openedRootContainerRefs.value.has(windowKey)) {
        continue
      }

      if (!validRefs.has(windowKey)) {
        toClose.push(windowKey)
      }
    }

    for (const key of toClose) {
      console.log('[gameStore] Auto-closing nested inventory (item removed from grid):', key)
      openNestedInventories.value.delete(key)
    }
  }

  function removeInventory(state: proto.IInventoryState) {
    inventories.value.delete(inventoryKey(state))
  }

  function getPlayerInventory(): proto.IInventoryState | undefined {
    if (!playerEntityId.value) return undefined

    // key format: kind_ownerId_inventoryKey
    // INVENTORY_KIND_GRID=0
    const gridKey = `0_${playerEntityId.value}_0`
    const gridInv = inventories.value.get(gridKey)

    if (gridInv && gridInv.grid) {
      return gridInv
    }

    // Fallback: search all inventories for one with grid belonging to player
    for (const [, inv] of inventories.value.entries()) {
      if (inv.grid && inv.ref && Number(inv.ref.ownerId) === playerEntityId.value) {
        return inv
      }
    }

    return undefined
  }

  function getPlayerEquipment(): proto.IInventoryState | undefined {
    if (!playerEntityId.value) return undefined

    // key format: kind_ownerId_inventoryKey
    // INVENTORY_KIND_EQUIPMENT=2
    const equipmentKey = `2_${playerEntityId.value}_0`
    const equipmentInv = inventories.value.get(equipmentKey)
    if (equipmentInv && equipmentInv.equipment) {
      return equipmentInv
    }

    // Fallback: search all inventories for one with equipment belonging to player.
    for (const [, inv] of inventories.value.entries()) {
      if (inv.equipment && inv.ref && Number(inv.ref.ownerId) === playerEntityId.value) {
        return inv
      }
    }

    return undefined
  }

  function getPlayerHandRef(): proto.IInventoryRef | null {
    if (!playerEntityId.value) return null
    return {
      kind: proto.InventoryKind.INVENTORY_KIND_HAND,
      ownerId: playerEntityId.value,
      inventoryKey: 0,
    }
  }

  function allocOpId(): number {
    return nextOpId++
  }

  function updateMousePos(x: number, y: number) {
    mousePos.value.x = x
    mousePos.value.y = y
  }

  function openContextMenu(entityId: number, actions: ContextMenuActionItem[]) {
    if (!actions.length) {
      contextMenu.value = null
      return
    }

    contextMenu.value = {
      entityId,
      actions,
      anchorX: mousePos.value.x,
      anchorY: mousePos.value.y,
    }
  }

  function closeContextMenu() {
    contextMenu.value = null
  }

  function setActionProgress(totalTicks: number, elapsedTicks: number) {
    actionProgress.value.total = totalTicks
    actionProgress.value.current = elapsedTicks
  }

  function clearActionProgress() {
    actionProgress.value.total = 0
    actionProgress.value.current = 0
  }

  function formatReasonCode(reasonCode: string): string {
    const normalized = reasonCode.trim().toLowerCase()
    if (!normalized) return 'unknown'
    const words = normalized.split('_').filter(Boolean)
    if (!words.length) return normalized
    return words
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  }

  function alertTtlMs(severity: proto.AlertSeverity, ttlMs: number): number {
    if (ttlMs > 0) return ttlMs
    switch (severity) {
      case proto.AlertSeverity.ALERT_SEVERITY_ERROR:
        return 2500
      case proto.AlertSeverity.ALERT_SEVERITY_WARNING:
        return 2000
      default:
        return 1500
    }
  }

  function startMiniAlertCleanupTimer() {
    if (miniAlertTimer) return
    miniAlertTimer = setInterval(() => {
      const now = Date.now()
      miniAlerts.value = miniAlerts.value.filter((alert) => alert.expiresAt > now)
      if (miniAlerts.value.length === 0 && miniAlertTimer) {
        clearInterval(miniAlertTimer)
        miniAlertTimer = null
      }
    }, 150)
  }

  function pushMiniAlert(input: MiniAlertInput) {
    const reasonCode = input.reasonCode.trim()
    if (!reasonCode) return

    const now = Date.now()
    const message = input.message?.trim() || formatReasonCode(reasonCode)
    const playerPart = playerEntityId.value ?? 0
    const debounceKey = `${playerPart}:${reasonCode}`
    const existingIndex = miniAlerts.value.findIndex((alert) => alert.debounceKey === debounceKey)
    const ttlMs = alertTtlMs(input.severity, input.ttlMs)

    if (existingIndex !== -1) {
      // Coalesce identical alerts: refresh the existing one instead of stacking.
      const existing = miniAlerts.value[existingIndex]!
      miniAlerts.value[existingIndex] = {
        id: existing.id,
        debounceKey: existing.debounceKey,
        severity: input.severity,
        reasonCode,
        message,
        createdAt: now,
        expiresAt: now + ttlMs,
      }
      miniAlertDebounceUntil.set(debounceKey, now + 250)
      startMiniAlertCleanupTimer()
      return
    }

    const debounceUntil = miniAlertDebounceUntil.get(debounceKey) ?? 0
    if (debounceUntil > now) {
      return
    }
    miniAlertDebounceUntil.set(debounceKey, now + 250)

    // Keep UI readable on mobile/desktop.
    // Product rule: max 3 alerts at once.
    if (miniAlerts.value.length >= 3) {
      miniAlerts.value.sort((a, b) => a.createdAt - b.createdAt)
      miniAlerts.value.shift()
    }

    miniAlerts.value.push({
      id: `${now}-${Math.random().toString(16).slice(2)}`,
      debounceKey,
      reasonCode,
      message,
      severity: input.severity,
      createdAt: now,
      expiresAt: now + ttlMs,
    })
    startMiniAlertCleanupTimer()
  }

  function togglePlayerInventory() {
    console.log('[gameStore] togglePlayerInventory called, current:', playerInventoryVisible.value)
    playerInventoryVisible.value = !playerInventoryVisible.value
    console.log('[gameStore] togglePlayerInventory new value:', playerInventoryVisible.value)
    console.log('[gameStore] Player inventory data:', getPlayerInventory())
  }

  function togglePlayerEquipment() {
    playerEquipmentVisible.value = !playerEquipmentVisible.value
  }

  function toggleCharacterSheet() {
    characterSheetVisible.value = !characterSheetVisible.value
  }

  function setCharacterSheetVisible(visible: boolean) {
    characterSheetVisible.value = visible
  }

  function togglePlayerStatsWindow() {
    playerStatsWindowVisible.value = !playerStatsWindowVisible.value
  }

  function setPlayerStatsWindowVisible(visible: boolean) {
    playerStatsWindowVisible.value = visible
  }

  function toggleCraftWindow() {
    craftWindowVisible.value = !craftWindowVisible.value
  }

  function setCraftWindowVisible(visible: boolean) {
    craftWindowVisible.value = visible
  }

  function toggleBuildWindow() {
    buildWindowVisible.value = !buildWindowVisible.value
  }

  function setBuildWindowVisible(visible: boolean) {
    buildWindowVisible.value = visible
    if (!visible) {
      armedBuildKey.value = ''
    }
  }

  function selectCraftRecipe(craftKey: string | null | undefined) {
    const key = (craftKey || '').trim()
    selectedCraftKey.value = key
  }

  function setCraftListSnapshot(snapshot: proto.IS2C_CraftList | null | undefined) {
    const nextRecipes = (snapshot?.recipes || []).slice()
    craftRecipes.value = nextRecipes

    if (nextRecipes.length === 0) {
      selectedCraftKey.value = ''
      return
    }

    if (!selectedCraftKey.value) {
      selectedCraftKey.value = nextRecipes[0]?.craftKey || ''
      return
    }

    const selectedStillExists = nextRecipes.some((recipe) => (recipe.craftKey || '') === selectedCraftKey.value)
    if (!selectedStillExists) {
      selectedCraftKey.value = nextRecipes[0]?.craftKey || ''
    }
  }

  function selectBuildRecipe(buildKey: string | null | undefined) {
    const key = (buildKey || '').trim()
    selectedBuildKey.value = key
  }

  function setBuildListSnapshot(snapshot: proto.IS2C_BuildList | null | undefined) {
    const nextRecipes = (snapshot?.builds || []).slice()
    buildRecipes.value = nextRecipes

    if (nextRecipes.length === 0) {
      selectedBuildKey.value = ''
      if (armedBuildKey.value) {
        armedBuildKey.value = ''
      }
      return
    }

    if (!selectedBuildKey.value) {
      selectedBuildKey.value = nextRecipes[0]?.buildKey || ''
    } else {
      const selectedStillExists = nextRecipes.some((recipe) => (recipe.buildKey || '') === selectedBuildKey.value)
      if (!selectedStillExists) {
        selectedBuildKey.value = nextRecipes[0]?.buildKey || ''
      }
    }

    if (armedBuildKey.value) {
      const armedStillExists = nextRecipes.some((recipe) => (recipe.buildKey || '') === armedBuildKey.value)
      if (!armedStillExists) {
        armedBuildKey.value = ''
      }
    }
  }

  function armBuildPlacement(buildKey: string | null | undefined) {
    const key = (buildKey || '').trim()
    if (!key) return
    armedBuildKey.value = key
  }

  function clearBuildPlacement() {
    armedBuildKey.value = ''
  }

  function consumeArmedBuildPlacement(): string {
    const key = armedBuildKey.value.trim()
    if (!key) return ''
    armedBuildKey.value = ''
    return key
  }

  function setCharacterProfileSnapshot(snapshot: proto.IS2C_CharacterProfile | null | undefined) {
    const entries = snapshot?.attributes
    const defaults = defaultCharacterAttributes()
    if (!entries || entries.length === 0) {
      characterAttributes.value = defaults
    } else {
      const byKey = new Map<proto.CharacterAttributeKey, number>()
      for (const entry of entries) {
        const key = entry.key ?? proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_UNSPECIFIED
        const rawValue = entry.value ?? 0
        byKey.set(key, Number.isFinite(rawValue) && rawValue >= 1 ? rawValue : 1)
      }

      characterAttributes.value = defaults.map((item) => ({
        ...item,
        value: byKey.get(item.key) ?? 1,
      }))
    }

    const exp = snapshot?.exp
    if (!exp) {
      characterExperience.value = defaultCharacterExperience()
      return
    }

    characterExperience.value = {
      lp: sanitizeNonNegativeInt64(exp.lp),
      nature: sanitizeNonNegativeInt64(exp.nature),
      industry: sanitizeNonNegativeInt64(exp.industry),
      combat: sanitizeNonNegativeInt64(exp.combat),
    }
  }

  function applyExpGained(snapshot: proto.IS2C_ExpGained | null | undefined) {
    if (!snapshot || playerEntityId.value == null) return

    const targetEntityId = sanitizeNonNegativeInt64(snapshot.entityId)
    if (targetEntityId !== playerEntityId.value) {
      return
    }

    characterExperience.value = {
      lp: clampNonNegative(characterExperience.value.lp + sanitizeSignedInt64(snapshot.lp)),
      nature: clampNonNegative(characterExperience.value.nature + sanitizeSignedInt64(snapshot.nature)),
      industry: clampNonNegative(characterExperience.value.industry + sanitizeSignedInt64(snapshot.industry)),
      combat: clampNonNegative(characterExperience.value.combat + sanitizeSignedInt64(snapshot.combat)),
    }
  }

  function setPlayerStats(snapshot: proto.IS2C_PlayerStats | null | undefined) {
    if (!snapshot) {
      playerStats.value = defaultPlayerStats()
      return
    }

    const staminaMax = sanitizeMaxStat(snapshot.staminaMax)
    const energyMax = sanitizeMaxStat(snapshot.energyMax)

    playerStats.value = {
      stamina: {
        current: clampStatCurrent(snapshot.stamina, staminaMax),
        max: staminaMax,
      },
      energy: {
        current: clampStatCurrent(snapshot.energy, energyMax),
        max: energyMax,
      },
    }
  }

  function setPlayerInventoryVisible(visible: boolean) {
    playerInventoryVisible.value = visible
  }

  function setPlayerEquipmentVisible(visible: boolean) {
    playerEquipmentVisible.value = visible
  }

  function onContainerOpened(state: proto.IInventoryState) {
    if (!state.ref) return
    const key = refKey(state.ref)
    console.log('[gameStore] onContainerOpened:', key)
    inventories.value.set(key, state)
    openNestedInventories.value.set(key, state)
    openedRootContainerRefs.value.add(key)
  }

  function onContainerClosed(r: proto.IInventoryRef) {
    const key = refKey(r)
    console.log('[gameStore] onContainerClosed:', key)
    openNestedInventories.value.delete(key)
    openedRootContainerRefs.value.delete(key)
  }

  function closeNestedInventory(windowKey: string) {
    openNestedInventories.value.delete(windowKey)
  }

  function closeAllNestedInventories() {
    openNestedInventories.value.clear()
    openedRootContainerRefs.value.clear()
  }

  function getNestedInventoryData(windowKey: string): proto.IInventoryState | undefined {
    return openNestedInventories.value.get(windowKey)
  }

  // Chat actions
  function addChatMessage(fromName: string, text: string, channel: proto.ChatChannel) {
    const message: ChatMessage = {
      id: `${Date.now()}-${Math.random()}`,
      fromName,
      text,
      timestamp: Date.now(),
      channel
    }

    chatMessages.value.push(message)

    // Limit message count to prevent memory leaks
    if (chatMessages.value.length > CHAT_MAX_MESSAGES) {
      chatMessages.value = chatMessages.value.slice(-CHAT_MAX_MESSAGES)
    }

    // Start cleanup timer if not running
    if (!cleanupTimer) {
      cleanupTimer = setInterval(cleanupExpiredMessages, CHAT_CLEANUP_INTERVAL_MS)
    }
  }

  function cleanupExpiredMessages() {
    const now = Date.now()
    const totalLifetime = CHAT_MESSAGE_LIFETIME_MS + CHAT_FADEOUT_DURATION_MS
    const cutoffTime = now - totalLifetime

    // Remove messages that have completely faded out
    chatMessages.value = chatMessages.value.filter(msg => msg.timestamp > cutoffTime)

    // Stop timer if no messages
    if (chatMessages.value.length === 0 && cleanupTimer) {
      clearInterval(cleanupTimer)
      cleanupTimer = null
    }
  }

  // Reset
  function reset() {
    clearGameSession()
    setConnectionState('disconnected')
    setPlayerLeaveWorld()

    // Clear nested inventories
    openNestedInventories.value.clear()
    openedRootContainerRefs.value.clear()
    playerEquipmentVisible.value = false
    characterSheetVisible.value = false
    playerStatsWindowVisible.value = false
    craftWindowVisible.value = false
    craftRecipes.value = []
    selectedCraftKey.value = ''
    buildWindowVisible.value = false
    buildRecipes.value = []
    selectedBuildKey.value = ''
    armedBuildKey.value = ''
    characterAttributes.value = defaultCharacterAttributes()
    characterExperience.value = defaultCharacterExperience()
    playerStats.value = defaultPlayerStats()
    playerMoveMode.value = null
    contextMenu.value = null
    miniAlerts.value = []
    miniAlertDebounceUntil.clear()
    clearLastServerErrorMessage()
    resetWorldBootstrap()
    clearActionProgress()

    // Cleanup chat
    if (cleanupTimer) {
      clearInterval(cleanupTimer)
      cleanupTimer = null
    }
    if (miniAlertTimer) {
      clearInterval(miniAlertTimer)
      miniAlertTimer = null
    }
    chatMessages.value = []
  }

  return {
    // State
    wsToken,
    characterId,
    connectionState,
    connectionError,
    lastServerErrorMessage,
    worldBootstrapState,
    playerEntityId,
    playerName,
    playerPosition,
    worldParams,
    chunks,
    entities,
    chatMessages,
    inventories,
    playerInventoryVisible,
    playerEquipmentVisible,
    characterSheetVisible,
    playerStatsWindowVisible,
    craftWindowVisible,
    craftRecipes,
    selectedCraftKey,
    buildWindowVisible,
    buildRecipes,
    selectedBuildKey,
    armedBuildKey,
    characterAttributes,
    characterExperience,
    playerStats,
    playerMoveMode,
    openNestedInventories,
    mousePos,
    contextMenu,
    miniAlerts,
    actionProgress,

    // Computed
    isConnected,
    isInGame,
    actionFrame,
    handState,
    handInventoryState,

    // Actions
    setGameSession,
    clearGameSession,
    setConnectionState,
    startWorldBootstrap,
    markPlayerEnterWorldBootstrap,
    markBootstrapFirstChunkLoaded,
    markBootstrapPlayerSpawned,
    resetWorldBootstrap,
    setLastServerErrorMessage,
    clearLastServerErrorMessage,
    setPlayerEnterWorld,
    setPlayerLeaveWorld,
    updatePlayerPosition,
    loadChunk,
    unloadChunk,
    spawnEntity,
    despawnEntity,
    updateEntityMovement,
    setPlayerMoveMode,
    addChatMessage,
    cleanupExpiredMessages,
    updateInventory,
    removeInventory,
    getPlayerInventory,
    getPlayerEquipment,
    togglePlayerInventory,
    togglePlayerEquipment,
    setPlayerInventoryVisible,
    setPlayerEquipmentVisible,
    toggleCharacterSheet,
    setCharacterSheetVisible,
    togglePlayerStatsWindow,
    setPlayerStatsWindowVisible,
    toggleCraftWindow,
    setCraftWindowVisible,
    selectCraftRecipe,
    setCraftListSnapshot,
    toggleBuildWindow,
    setBuildWindowVisible,
    selectBuildRecipe,
    setBuildListSnapshot,
    armBuildPlacement,
    clearBuildPlacement,
    consumeArmedBuildPlacement,
    setCharacterProfileSnapshot,
    applyExpGained,
    setPlayerStats,
    onContainerOpened,
    onContainerClosed,
    closeNestedInventory,
    closeAllNestedInventories,
    getNestedInventoryData,
    getPlayerHandRef,
    allocOpId,
    updateMousePos,
    openContextMenu,
    closeContextMenu,
    pushMiniAlert,
    setActionProgress,
    clearActionProgress,
    reset,
  }
})

function defaultCharacterAttributes(): CharacterAttributeViewItem[] {
  return [
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_STR, label: 'Strength', value: 1, icon: '⚒' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_AGI, label: 'Agility', value: 1, icon: '⚡' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_INT, label: 'Intelligence', value: 1, icon: '✦' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_CON, label: 'Constitution', value: 1, icon: '❤' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_PER, label: 'Perception', value: 1, icon: '◉' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_CHA, label: 'Charisma', value: 1, icon: '☯' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_DEX, label: 'Dexterity', value: 1, icon: '⚙' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_PSY, label: 'Psyche', value: 1, icon: '☁' },
    { key: proto.CharacterAttributeKey.CHARACTER_ATTRIBUTE_KEY_WIL, label: 'Will', value: 1, icon: '♜' },
  ]
}

function defaultPlayerStats(): PlayerStatsState {
  return {
    stamina: { current: 0, max: 0 },
    energy: { current: 0, max: 0 },
  }
}

function defaultCharacterExperience(): CharacterExperienceState {
  return {
    lp: 0,
    nature: 0,
    industry: 0,
    combat: 0,
  }
}

function sanitizeMaxStat(raw: number | null | undefined): number {
  const value = Number(raw ?? 0)
  if (!Number.isFinite(value) || value <= 0) return 0
  return Math.floor(value)
}

function sanitizeNonNegativeInt64(raw: number | Long | null | undefined): number {
  if (raw == null) return 0
  if (typeof raw === 'number') {
    if (!Number.isFinite(raw) || raw <= 0) return 0
    return Math.floor(raw)
  }

  if (typeof raw.toNumber === 'function') {
    const value = raw.toNumber()
    if (!Number.isFinite(value) || value <= 0) return 0
    return Math.floor(value)
  }

  return 0
}

function sanitizeSignedInt64(raw: number | Long | null | undefined): number {
  if (raw == null) return 0
  if (typeof raw === 'number') {
    if (!Number.isFinite(raw)) return 0
    return Math.trunc(raw)
  }

  if (typeof raw.toNumber === 'function') {
    const value = raw.toNumber()
    if (!Number.isFinite(value)) return 0
    return Math.trunc(value)
  }

  return 0
}

function clampNonNegative(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return 0
  return Math.trunc(value)
}

function clampStatCurrent(rawCurrent: number | null | undefined, max: number): number {
  const value = Number(rawCurrent ?? 0)
  if (!Number.isFinite(value) || value <= 0) return 0
  const normalized = Math.floor(value)
  if (max <= 0) return normalized
  return Math.min(normalized, max)
}
