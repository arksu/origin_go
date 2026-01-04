<template>
  <div class="min-h-screen bg-gray-900 flex flex-col">
    <header class="bg-gray-800 border-b border-gray-700 px-6 py-4">
      <div class="flex justify-between items-center">
        <h1 class="text-xl font-bold text-white">Game</h1>
        <div class="flex items-center gap-4">
          <span
            class="px-3 py-1 rounded-full text-sm font-medium"
            :class="connectionStatusClass"
          >
            {{ connectionStatusText }}
          </span>
          <button
            @click="handleDisconnect"
            class="px-4 py-2 text-sm font-medium text-gray-300 hover:text-white hover:bg-gray-700 rounded-lg transition-colors"
          >
            Disconnect
          </button>
        </div>
      </div>
    </header>

    <main class="flex-1 p-6">
      <div v-if="gameStore.connectionState === 'connecting'" class="text-center text-gray-400 py-12">
        Connecting to game server...
      </div>

      <div v-else-if="gameStore.connectionState === 'error'" class="text-center py-12">
        <p class="text-red-400 mb-4">{{ gameStore.lastError }}</p>
        <button
          @click="handleRetry"
          class="px-6 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-lg"
        >
          Retry
        </button>
      </div>

      <div v-else-if="gameStore.isConnected" class="text-center text-gray-400 py-12">
        <p class="text-green-400 mb-4">Connected to game server!</p>
        <p v-if="gameStore.playerState" class="text-white">
          Player Entity ID: {{ gameStore.playerState.entityId }}
        </p>
        <p class="text-gray-500 mt-4 text-sm">Game canvas will be implemented here</p>
      </div>

      <div v-else class="text-center text-gray-400 py-12">
        Disconnected from server
      </div>
    </main>
  </div>
</template>

<script setup>
import { computed, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useGameStore } from '../stores/game'
import { gameConnection } from '../network/GameConnection'

const router = useRouter()
const gameStore = useGameStore()

watch(() => gameStore.connectionState, (newState) => {
  if (newState === 'disconnected' && gameStore.wsToken) {
    router.push('/characters')
  }
})

const connectionStatusClass = computed(() => {
  switch (gameStore.connectionState) {
    case 'connected':
      return 'bg-green-600 text-white'
    case 'connecting':
      return 'bg-yellow-600 text-white'
    case 'error':
      return 'bg-red-600 text-white'
    default:
      return 'bg-gray-600 text-white'
  }
})

const connectionStatusText = computed(() => {
  switch (gameStore.connectionState) {
    case 'connected':
      return 'Connected'
    case 'connecting':
      return 'Connecting...'
    case 'error':
      return 'Error'
    default:
      return 'Disconnected'
  }
})

function handleDisconnect() {
  gameConnection.disconnect()
  router.push('/characters')
}

function handleRetry() {
  if (gameStore.wsToken) {
    gameConnection.connect()
  } else {
    router.push('/characters')
  }
}

onUnmounted(() => {
  gameConnection.disconnect()
})
</script>
