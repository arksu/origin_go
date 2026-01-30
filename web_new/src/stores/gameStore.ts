import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
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
  }

  function setPlayerLeaveWorld() {
    playerEntityId.value = null
    playerName.value = ''
    worldParams.value = null
    chunks.value.clear()
    entities.value.clear()
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

  // Reset
  function reset() {
    clearGameSession()
    setConnectionState('disconnected')
    setPlayerLeaveWorld()
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
    reset,
  }
})
