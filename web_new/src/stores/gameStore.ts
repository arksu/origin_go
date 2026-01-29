import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useGameStore = defineStore('game', () => {
  const wsToken = ref('')
  const characterId = ref<number | null>(null)

  function setGameSession(token: string, charId: number) {
    wsToken.value = token
    characterId.value = charId
  }

  function clearGameSession() {
    wsToken.value = ''
    characterId.value = null
  }

  return {
    wsToken,
    characterId,
    setGameSession,
    clearGameSession,
  }
})
