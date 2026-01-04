import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useGameStore = defineStore('game', () => {
  const wsToken = ref('')
  const characterId = ref(null)
  const connectionState = ref('disconnected') // disconnected, connecting, connected, error
  const playerState = ref(null)
  const lastError = ref('')

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

  function setError(error) {
    lastError.value = error
    connectionState.value = 'error'
  }

  function reset() {
    wsToken.value = ''
    characterId.value = null
    connectionState.value = 'disconnected'
    playerState.value = null
    lastError.value = ''
  }

  return {
    wsToken,
    characterId,
    connectionState,
    playerState,
    lastError,
    isConnected,
    setWsToken,
    setConnectionState,
    setPlayerState,
    setError,
    reset
  }
})
