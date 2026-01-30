<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useGameStore } from '@/stores/gameStore'
import { connectToGame, disconnectFromGame } from '@/network'
import { gameFacade } from '@/game'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppAlert from '@/components/ui/AppAlert.vue'

const router = useRouter()
const gameStore = useGameStore()

const gameCanvas = ref<HTMLCanvasElement | null>(null)
const canvasInitialized = ref(false)

const connectionState = computed(() => gameStore.connectionState)
const connectionError = computed(() => gameStore.connectionError)
const isConnecting = computed(() => 
  connectionState.value === 'connecting' || connectionState.value === 'authenticating'
)
const isConnected = computed(() => connectionState.value === 'connected')
const hasError = computed(() => connectionState.value === 'error')

async function initCanvas() {
  if (!gameCanvas.value || canvasInitialized.value) return

  try {
    await gameFacade.init(gameCanvas.value)
    canvasInitialized.value = true

    gameFacade.onPlayerClick((screenX, screenY) => {
      console.debug('[GameView] Click:', screenX, screenY)
    })
  } catch (err) {
    console.error('[GameView] Failed to init canvas:', err)
  }
}

watch(isConnected, async (connected) => {
  if (connected) {
    await nextTick()
    await initCanvas()
  }
})

onMounted(() => {
  if (!gameStore.wsToken) {
    router.push('/characters')
    return
  }

  connectToGame(gameStore.wsToken)
})

onUnmounted(() => {
  gameFacade.destroy()
  canvasInitialized.value = false
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
    <div v-else-if="isConnected" class="game-canvas-wrapper">
      <canvas ref="gameCanvas" class="game-canvas"></canvas>
      <div class="game-ui">
        <AppButton variant="secondary" size="sm" @click="handleBack">Выйти</AppButton>
      </div>
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

.game-canvas-wrapper {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: hidden;
}

.game-canvas {
  display: block;
  width: 100%;
  height: 100%;
}

.game-ui {
  position: absolute;
  top: 1rem;
  right: 1rem;
  z-index: 100;
}

.game-disconnected {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  color: #a0a0a0;
}
</style>
