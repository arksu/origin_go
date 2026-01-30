<script setup lang="ts">
import { onMounted, onUnmounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useGameStore } from '@/stores/gameStore'
import { connectToGame, disconnectFromGame } from '@/network'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppAlert from '@/components/ui/AppAlert.vue'

const router = useRouter()
const gameStore = useGameStore()

const connectionState = computed(() => gameStore.connectionState)
const connectionError = computed(() => gameStore.connectionError)
const isConnecting = computed(() => 
  connectionState.value === 'connecting' || connectionState.value === 'authenticating'
)
const isConnected = computed(() => connectionState.value === 'connected')
const hasError = computed(() => connectionState.value === 'error')

onMounted(() => {
  if (!gameStore.wsToken) {
    router.push('/characters')
    return
  }

  connectToGame(gameStore.wsToken)
})

onUnmounted(() => {
  disconnectFromGame()
  gameStore.reset()
})

function handleBack() {
  router.push('/characters')
}

function handleRetry() {
  if (gameStore.wsToken) {
    connectToGame(gameStore.wsToken)
  }
}
</script>

<template>
  <div class="game-view">
    <!-- Connecting state -->
    <div v-if="isConnecting" class="game-connecting">
      <AppSpinner size="lg" />
      <p class="game-connecting__text">
        {{ connectionState === 'authenticating' ? 'Авторизация...' : 'Подключение...' }}
      </p>
    </div>

    <!-- Error state -->
    <div v-else-if="hasError" class="game-error">
      <AppAlert type="error">
        {{ connectionError?.message || 'Ошибка подключения' }}
      </AppAlert>
      <div class="game-error__actions">
        <AppButton @click="handleRetry">Повторить</AppButton>
        <AppButton variant="secondary" @click="handleBack">Назад</AppButton>
      </div>
    </div>

    <!-- Connected state -->
    <div v-else-if="isConnected" class="game-canvas">
      <div class="game-debug">
        <p><strong>Entity ID:</strong> {{ gameStore.playerEntityId }}</p>
        <p><strong>Name:</strong> {{ gameStore.playerName }}</p>
        <p><strong>Position:</strong> {{ gameStore.playerPosition.x }}, {{ gameStore.playerPosition.y }}</p>
        <p><strong>Chunks:</strong> {{ gameStore.chunks.size }}</p>
        <p><strong>Entities:</strong> {{ gameStore.entities.size }}</p>
      </div>
      <p class="game-placeholder">Canvas будет добавлен на этапе 4</p>
      <AppButton variant="secondary" @click="handleBack">Выйти</AppButton>
    </div>

    <!-- Disconnected state -->
    <div v-else class="game-disconnected">
      <p>Отключено от сервера</p>
      <AppButton @click="handleBack">Назад к персонажам</AppButton>
    </div>
  </div>
</template>

<style scoped lang="scss">
.game-view {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 1rem;
}

.game-connecting {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;

  &__text {
    color: #a0a0a0;
    font-size: 1.125rem;
  }
}

.game-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  max-width: 400px;
  text-align: center;

  &__actions {
    display: flex;
    gap: 0.5rem;
  }
}

.game-canvas {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  width: 100%;
  height: 100%;
  padding: 1rem;
}

.game-debug {
  position: absolute;
  top: 1rem;
  left: 1rem;
  padding: 1rem;
  background: rgba(0, 0, 0, 0.7);
  border-radius: 8px;
  font-size: 0.875rem;
  color: #e0e0e0;

  p {
    margin: 0.25rem 0;
  }
}

.game-placeholder {
  color: #a0a0a0;
  font-size: 1.125rem;
}

.game-disconnected {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  color: #a0a0a0;
}
</style>
