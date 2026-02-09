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

export const useGameStore = defineStore('game', () => {
  // Session
  const wsToken = ref('')
  const characterId = ref<number | null>(null)

  // Connection
  const connectionState = ref<ConnectionState>('disconnected')
  const connectionError = ref<ConnectionError | null>(null)

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
  const openNestedInventories = ref(new Map<string, proto.IInventoryState>())
  const openedRootContainerRefs = ref(new Set<string>())
  let nextOpId = 1
  const mousePos = ref({ x: 0, y: 0 })

  // Computed
  const isConnected = computed(() => connectionState.value === 'connected')
  const isInGame = computed(() => isConnected.value && playerEntityId.value !== null)

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

  function togglePlayerInventory() {
    console.log('[gameStore] togglePlayerInventory called, current:', playerInventoryVisible.value)
    playerInventoryVisible.value = !playerInventoryVisible.value
    console.log('[gameStore] togglePlayerInventory new value:', playerInventoryVisible.value)
    console.log('[gameStore] Player inventory data:', getPlayerInventory())
  }

  function setPlayerInventoryVisible(visible: boolean) {
    playerInventoryVisible.value = visible
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

    // Cleanup chat
    if (cleanupTimer) {
      clearInterval(cleanupTimer)
      cleanupTimer = null
    }
    chatMessages.value = []
  }

  return {
    // State
    wsToken,
    characterId,
    connectionState,
    connectionError,
    playerEntityId,
    playerName,
    playerPosition,
    worldParams,
    chunks,
    entities,
    chatMessages,
    inventories,
    playerInventoryVisible,
    openNestedInventories,
    mousePos,

    // Computed
    isConnected,
    isInGame,
    handState,
    handInventoryState,

    // Actions
    setGameSession,
    clearGameSession,
    setConnectionState,
    setPlayerEnterWorld,
    setPlayerLeaveWorld,
    updatePlayerPosition,
    loadChunk,
    unloadChunk,
    spawnEntity,
    despawnEntity,
    updateEntityMovement,
    addChatMessage,
    cleanupExpiredMessages,
    updateInventory,
    removeInventory,
    getPlayerInventory,
    togglePlayerInventory,
    setPlayerInventoryVisible,
    onContainerOpened,
    onContainerClosed,
    closeNestedInventory,
    closeAllNestedInventories,
    getNestedInventoryData,
    getPlayerHandRef,
    allocOpId,
    updateMousePos,
    reset,
  }
})
