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
  objectType: number
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

  // Computed
  const isConnected = computed(() => connectionState.value === 'connected')
  const isInGame = computed(() => isConnected.value && playerEntityId.value !== null)

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
  function inventoryKey(state: proto.IInventoryState): string {
    if (!state.ref) return 'no_ref'

    // Determine inventory type
    let type = 'unknown'
    if (state.grid) type = 'grid'
    else if (state.equipment) type = 'equipment'
    else if (state.hand) type = 'hand'

    const key = state.ref.ownerEntityId
      ? `entity_${state.ref.ownerEntityId}_${state.ref.inventoryKey}_${type}`
      : state.ref.ownerItemId
        ? `item_${state.ref.ownerItemId}_${state.ref.inventoryKey}_${type}`
        : `unknown_${state.ref.inventoryKey}_${type}`

    console.log('[gameStore] inventoryKey generated:', {
      ref: state.ref,
      type,
      key,
      ownerEntityId: state.ref.ownerEntityId,
      ownerItemId: state.ref.ownerItemId,
      inventoryKey: state.ref.inventoryKey,
      hasGrid: !!state.grid,
      hasEquipment: !!state.equipment,
      hasHand: !!state.hand
    })

    return key
  }

  function updateInventory(state: proto.IInventoryState) {
    if (state.ref) {
      inventories.value.set(inventoryKey(state), state)
    }
  }

  function removeInventory(state: proto.IInventoryState) {
    inventories.value.delete(inventoryKey(state))
  }

  function getPlayerInventory(): proto.IInventoryState | undefined {
    if (!playerEntityId.value) return undefined

    console.log('[gameStore] All inventories:', Array.from(inventories.value.entries()))

    // Try to find grid inventory with new key format
    const gridKey = `entity_${playerEntityId.value}_0_grid`
    const equipmentKey = `entity_${playerEntityId.value}_0_equipment`
    const handKey = `entity_${playerEntityId.value}_0_hand`

    const gridInv = inventories.value.get(gridKey)
    const equipmentInv = inventories.value.get(equipmentKey)
    const handInv = inventories.value.get(handKey)

    console.log('[gameStore] Looking for inventory:', {
      playerEntityId: playerEntityId.value,
      gridKey,
      equipmentKey,
      handKey,
      hasGrid: !!gridInv,
      hasEquipment: !!equipmentInv,
      hasHand: !!handInv,
      gridInv,
      equipmentInv,
      handInv
    })

    // Return grid inventory first
    if (gridInv && gridInv.grid) {
      console.log('[gameStore] Found grid inventory at key:', gridKey)
      return gridInv
    }

    // Search all inventories for one with grid
    for (const [key, inv] of inventories.value.entries()) {
      if (inv.grid) {
        console.log('[gameStore] Found grid inventory at key:', key)
        return inv
      }
    }

    console.log('[gameStore] No grid inventory found')
    return undefined
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

    // Computed
    isConnected,
    isInGame,

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
    reset,
  }
})
