<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useGameStore } from '@/stores/gameStore'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppAlert from '@/components/ui/AppAlert.vue'
import ChatContainer from '@/components/ui/ChatContainer.vue'
import InventoryWindow from '@/components/ui/InventoryWindow.vue'
import NestedInventoryWindow from '@/components/ui/NestedInventoryWindow.vue'
import { sendChatMessage } from '@/network'
import { useHotkeys } from '@/composables/useHotkeys'
import { DEFAULT_HOTKEYS, type HotkeyConfig } from '@/constants/hotkeys'

const router = useRouter()
const gameStore = useGameStore()

const gameCanvas = ref<HTMLCanvasElement | null>(null)
const chatContainerRef = ref<InstanceType<typeof ChatContainer>>()
const canvasInitialized = ref(false)
let gameFacade: any = null
let connectToGame: any = null
let disconnectFromGame: any = null

const connectionState = computed(() => gameStore.connectionState)
const connectionError = computed(() => gameStore.connectionError)
const isConnecting = computed(() => 
  connectionState.value === 'connecting' || connectionState.value === 'authenticating'
)
const isConnected = computed(() => connectionState.value === 'connected')
const hasError = computed(() => connectionState.value === 'error')
const playerInventory = computed(() => {
  const inv = gameStore.getPlayerInventory()
  console.log('[GameView] playerInventory computed:', inv)
  return inv
})
const showInventory = computed(() => {
  const visible = gameStore.playerInventoryVisible
  const hasInventory = !!playerInventory.value
  const hasGrid = !!playerInventory.value?.grid
  console.log('[GameView] showInventory computed:', { visible, hasInventory, hasGrid, inventory: playerInventory.value })
  return visible && hasInventory && hasGrid
})

const openNestedInventoryWindows = computed(() => {
  const openWindows = Array.from(gameStore.openNestedInventories.keys()).filter(windowKey => 
    gameStore.openNestedInventories.get(windowKey)
  )
  console.log('[GameView] openNestedInventoryWindows computed:', {
    openWindows,
    totalOpenInventories: gameStore.openNestedInventories.size,
    allKeys: Array.from(gameStore.openNestedInventories.keys()),
    values: Array.from(gameStore.openNestedInventories.values())
  })
  return openWindows
})

async function initCanvas() {
  if (!gameCanvas.value || canvasInitialized.value) return

  try {
    await gameFacade.init(gameCanvas.value)
    canvasInitialized.value = true

    gameFacade.onPlayerClick((screenX: number, screenY: number) => {
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

onMounted(async () => {
  if (!gameStore.wsToken) {
    router.push('/characters')
    return
  }

  const [gameModule, networkModule] = await Promise.all([
    import('@/game'),
    import('@/network')
  ])

  gameFacade = gameModule.gameFacade
  connectToGame = networkModule.connectToGame
  disconnectFromGame = networkModule.disconnectFromGame

  connectToGame(gameStore.wsToken)
})

onUnmounted(() => {
  if (gameFacade) {
    gameFacade.destroy()
  }
  canvasInitialized.value = false
  if (disconnectFromGame) {
    disconnectFromGame()
  }
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

function handleChatSend(text: string) {
  sendChatMessage(text)
}

function handleInventoryClose() {
  gameStore.setPlayerInventoryVisible(false)
}

function handleNestedInventoryClose(windowKey: string) {
  gameStore.closeNestedInventory(windowKey)
}

// Setup hotkeys
const hotkeys: HotkeyConfig[] = DEFAULT_HOTKEYS.map(config => ({
  ...config,
  action: () => {
    switch (config.key) {
      case 'Enter':
        chatContainerRef.value?.focusChat()
        break
      case 'Escape':
        chatContainerRef.value?.unfocusChat()
        gameStore.setPlayerInventoryVisible(false)
        break
      case '/':
        if (config.modifiers?.includes('shift')) {
          chatContainerRef.value?.focusChatWithSlash()
        }
        break
      case 'tab':
      case 'i':
        console.log('[GameView] Toggling inventory, current state:', gameStore.playerInventoryVisible)
        gameStore.togglePlayerInventory()
        console.log('[GameView] New inventory state:', gameStore.playerInventoryVisible)
        break
      case '`':
        if (gameFacade && gameFacade.isInitialized()) {
          gameFacade.toggleDebugOverlay()
        }
        break
      default:
        config.action()
    }
  }
}))

useHotkeys(hotkeys)
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
      <div class="game-chat">
        <ChatContainer ref="chatContainerRef" @send="handleChatSend" />
      </div>
      <div v-if="showInventory" class="game-inventory">
        <InventoryWindow :inventory="playerInventory!" @close="handleInventoryClose" />
      </div>
      
      <!-- Nested inventory windows -->
      <div v-for="windowKey in openNestedInventoryWindows" :key="windowKey" class="game-nested-inventory">
        <NestedInventoryWindow 
          v-if="gameStore.getNestedInventoryData(windowKey)"
          :window-key="windowKey" 
          :nested-inventory-data="gameStore.getNestedInventoryData(windowKey)!.data"
          :item-id="gameStore.getNestedInventoryData(windowKey)!.itemId"
          @close="() => handleNestedInventoryClose(windowKey)" 
        />
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

.game-chat {
  position: absolute;
  bottom: 1rem;
  left: 1rem;
  z-index: 100;
}

.game-inventory {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 200;
}

.game-nested-inventory {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 300;
}

.game-disconnected {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  color: #a0a0a0;
}
</style>
