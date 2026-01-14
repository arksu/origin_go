import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useGameStore = defineStore('game', () => {
  const wsToken = ref('')
  const characterId = ref(null)
  const connectionState = ref('disconnected') // disconnected, connecting, connected, error
  const playerState = ref(null)
  const lastError = ref('')
  const worldReady = ref(false)
  const chunks = ref(new Map())
  const playerPosition = ref({ x: 0, y: 0, heading: 0 })
  const gameObjects = ref(new Map())

  const isConnected = computed(() => connectionState.value === 'connected')

  function setWsToken(token, charId) {
    wsToken.value = token
    characterId.value = charId
  }

  function setConnectionState(state) {
    connectionState.value = state
  }

  function setPlayerState(state) {
    playerState.value = state
  }

  function setWorldReady(ready) {
    worldReady.value = ready
  }

  function setPlayerPosition(pos) {
    playerPosition.value = { ...pos }
  }

  function addChunk(coord, data) {
    const key = `${coord.x},${coord.y}`
    chunks.value.set(key, { coord, data })
  }

  function removeChunk(coord) {
    const key = `${coord.x},${coord.y}`
    chunks.value.delete(key)
  }

  function addGameObject(entityId, gameObject) {
    gameObjects.value.set(entityId, gameObject)
  }

  function removeGameObject(entityId) {
    gameObjects.value.delete(entityId)
  }

  function updateGameObject(entityId, gameObject) {
    gameObjects.value.set(entityId, gameObject)
  }

  function setError(error) {
    lastError.value = error
    connectionState.value = 'error'
  }

  function clearError() {
    lastError.value = ''
  }

  function reset() {
    wsToken.value = ''
    characterId.value = null
    connectionState.value = 'disconnected'
    playerState.value = null
    worldReady.value = false
    chunks.value.clear()
    playerPosition.value = { x: 0, y: 0, heading: 0 }
    gameObjects.value.clear()
  }

  return {
    wsToken,
    characterId,
    connectionState,
    playerState,
    lastError,
    worldReady,
    chunks,
    playerPosition,
    gameObjects,
    isConnected,
    setWsToken,
    setConnectionState,
    setPlayerState,
    setWorldReady,
    setPlayerPosition,
    addChunk,
    removeChunk,
    addGameObject,
    removeGameObject,
    updateGameObject,
    setError,
    clearError,
    reset
  }
})
