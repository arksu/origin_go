<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useGameStore } from '@/stores/gameStore'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppAlert from '@/components/ui/AppAlert.vue'
import ChatContainer from '@/components/ui/ChatContainer.vue'
import InventoryWindow from '@/components/ui/InventoryWindow.vue'
import EquipmentWindow from '@/components/ui/EquipmentWindow.vue'
import NestedInventoryWindow from '@/components/ui/NestedInventoryWindow.vue'
import CharacterSheetWindow from '@/components/ui/CharacterSheetWindow.vue'
import PlayerStatsWindow from '@/components/ui/PlayerStatsWindow.vue'
import CraftWindow from '@/components/ui/CraftWindow.vue'
import BuildWindow from '@/components/ui/BuildWindow.vue'
import BuildStateWindow from '@/components/ui/BuildStateWindow.vue'
import HandOverlay from '@/components/ui/HandOverlay.vue'
import ContextMenu from '@/components/ui/ContextMenu.vue'
import ActionHourGlass from '@/components/ui/ActionHourGlass.vue'
import PlayerStatsBars from '@/components/ui/PlayerStatsBars.vue'
import MovementModePanel from '@/components/ui/MovementModePanel.vue'
import { sendChatMessage, sendOpenWindow, sendCloseWindow, sendStartBuild, sendBuildProgress } from '@/network'
import { useHotkeys } from '@/composables/useHotkeys'
import { DEFAULT_HOTKEYS, type HotkeyConfig } from '@/constants/hotkeys'
import { proto } from '@/network/proto/packets.js'

const router = useRouter()
const gameStore = useGameStore()

const gameCanvas = ref<HTMLCanvasElement | null>(null)
const chatContainerRef = ref<InstanceType<typeof ChatContainer>>()
const canvasInitialized = ref(false)
let gameFacade: any = null
let connectToGame: any = null
let disconnectFromGame: any = null
const navigatingAway = ref(false)
const showLoadingOverlay = ref(false)
const loadingOverlayFading = ref(false)
const showLoadingSlowHint = ref(false)
let overlayHideTimer: ReturnType<typeof setTimeout> | null = null
let loadingSlowHintTimer: ReturnType<typeof setTimeout> | null = null

const connectionState = computed(() => gameStore.connectionState)
const connectionError = computed(() => gameStore.connectionError)
const isConnecting = computed(() => 
  connectionState.value === 'connecting' || connectionState.value === 'authenticating'
)
const isConnected = computed(() => connectionState.value === 'connected')
const hasError = computed(() => connectionState.value === 'error')
const worldBootstrapState = computed(() => gameStore.worldBootstrapState)
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
const miniAlerts = computed(() => gameStore.miniAlerts)
const showCharacterSheet = computed(() => gameStore.characterSheetVisible)
const showPlayerStatsWindow = computed(() => gameStore.playerStatsWindowVisible)
const showCraftWindow = computed(() => gameStore.craftWindowVisible)
const showBuildWindow = computed(() => gameStore.buildWindowVisible)
const showBuildStateWindow = computed(() => gameStore.buildStateWindowVisible)
const currentBuildStateEntityId = computed(() => gameStore.buildStateEntityId)
const currentBuildStateList = computed(() => gameStore.buildStateList)
const playerEquipment = computed(() => gameStore.getPlayerEquipment())
const showEquipment = computed(() => {
  const visible = gameStore.playerEquipmentVisible
  const hasEquipment = !!playerEquipment.value?.equipment
  return visible && hasEquipment
})

function findBuildRecipeByKey(buildKey: string): proto.IBuildRecipeEntry | null {
  const normalized = buildKey.trim()
  if (!normalized) return null
  return gameStore.buildRecipes.find((recipe) => (recipe.buildKey || '') === normalized) || null
}

function syncBuildGhostFromStore(): void {
  if (!gameFacade || !canvasInitialized.value || !gameFacade.isInitialized?.()) {
    return
  }

  const armedBuildKey = (gameStore.armedBuildKey || '').trim()
  if (!armedBuildKey) {
    gameFacade.cancelBuildGhost?.()
    return
  }

  const build = findBuildRecipeByKey(armedBuildKey)
  if (!build) {
    gameStore.clearBuildPlacement()
    gameFacade.cancelBuildGhost?.()
    return
  }

  gameFacade.armBuildGhost?.({
    buildKey: armedBuildKey,
    objectKey: build.objectKey || '',
    objectResourcePath: build.objectResourcePath || '',
  })
}

async function initCanvas() {
  if (!gameCanvas.value || canvasInitialized.value) return

  try {
    await gameFacade.init(gameCanvas.value)
    canvasInitialized.value = true

    gameFacade.onPlayerClick(({ screenX, screenY, worldX, worldY, button }: { screenX: number; screenY: number; worldX: number; worldY: number; button: number }) => {
      console.debug('[GameView] Click:', screenX, screenY, 'button=', button)

      if (button !== 0) {
        return false
      }

      const armedBuildKey = (gameStore.armedBuildKey || '').trim()
      if (!armedBuildKey) {
        return false
      }

      const ghostPos = gameFacade?.getBuildGhostWorldPosition?.()
      const targetPos = ghostPos || { x: worldX, y: worldY }

      sendStartBuild(armedBuildKey, {
        x: targetPos.x,
        y: targetPos.y,
      })
      gameStore.clearBuildPlacement()
      gameFacade?.cancelBuildGhost?.()
      return true
    })

    syncBuildGhostFromStore()
  } catch (err) {
    console.error('[GameView] Failed to init canvas:', err)
  }
}

watch(() => gameStore.armedBuildKey, () => {
  syncBuildGhostFromStore()
})

watch(() => gameStore.buildRecipes, () => {
  syncBuildGhostFromStore()
})

watch(isConnected, async (connected) => {
  if (connected) {
    await nextTick()
    await initCanvas()
  }
})

watch([isConnected, worldBootstrapState], ([connected, bootstrap]) => {
  if (!connected) {
    if (overlayHideTimer) {
      clearTimeout(overlayHideTimer)
      overlayHideTimer = null
    }
    if (loadingSlowHintTimer) {
      clearTimeout(loadingSlowHintTimer)
      loadingSlowHintTimer = null
    }
    showLoadingOverlay.value = false
    loadingOverlayFading.value = false
    showLoadingSlowHint.value = false
    return
  }

  if (bootstrap !== 'ready') {
    if (overlayHideTimer) {
      clearTimeout(overlayHideTimer)
      overlayHideTimer = null
    }
    showLoadingOverlay.value = true
    loadingOverlayFading.value = false
    showLoadingSlowHint.value = false
    if (loadingSlowHintTimer) {
      clearTimeout(loadingSlowHintTimer)
    }
    loadingSlowHintTimer = setTimeout(() => {
      showLoadingSlowHint.value = true
    }, 4000)
    return
  }

  if (loadingSlowHintTimer) {
    clearTimeout(loadingSlowHintTimer)
    loadingSlowHintTimer = null
  }
  showLoadingSlowHint.value = false
  if (!showLoadingOverlay.value) return
  loadingOverlayFading.value = true
  overlayHideTimer = setTimeout(() => {
    showLoadingOverlay.value = false
    loadingOverlayFading.value = false
    overlayHideTimer = null
  }, 250)
})

watch(connectionState, (state) => {
  if (navigatingAway.value) return
  if (state !== 'error' && state !== 'disconnected') return

  const disconnectReason =
    gameStore.lastServerErrorMessage ||
    connectionError.value?.message ||
    'Disconnected from server'
  navigatingAway.value = true
  router.replace({ path: '/characters', query: { disconnectReason } })
})

function onMouseMove(e: MouseEvent) {
  gameStore.updateMousePos(e.clientX, e.clientY)
}

onMounted(async () => {
  window.addEventListener('mousemove', onMouseMove)

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

  gameStore.clearLastServerErrorMessage()
  gameStore.startWorldBootstrap()
  connectToGame(gameStore.wsToken)
})

onUnmounted(() => {
  window.removeEventListener('mousemove', onMouseMove)

  if (gameFacade) {
    gameFacade.destroy()
  }
  canvasInitialized.value = false
  if (disconnectFromGame) {
    disconnectFromGame()
  }
  if (overlayHideTimer) {
    clearTimeout(overlayHideTimer)
    overlayHideTimer = null
  }
  if (loadingSlowHintTimer) {
    clearTimeout(loadingSlowHintTimer)
    loadingSlowHintTimer = null
  }
  gameStore.reset()
})

function handleBack() {
  navigatingAway.value = true
  router.push('/characters')
}

function handleRetry() {
  if (gameStore.wsToken) {
    gameStore.startWorldBootstrap()
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

function handleCharacterSheetClose() {
  gameStore.setCharacterSheetVisible(false)
}

function handlePlayerStatsClose() {
  gameStore.setPlayerStatsWindowVisible(false)
}

function handleEquipmentClose() {
  gameStore.setPlayerEquipmentVisible(false)
}

function openCraftWindow() {
  if (gameStore.craftWindowVisible) return
  gameStore.setCraftWindowVisible(true)
  sendOpenWindow('craft')
}

function closeCraftWindow() {
  if (!gameStore.craftWindowVisible) return
  gameStore.setCraftWindowVisible(false)
  sendCloseWindow('craft')
}

function openBuildWindow() {
  if (gameStore.buildWindowVisible) return
  gameStore.setBuildWindowVisible(true)
  sendOpenWindow('build')
}

function closeBuildWindow() {
  if (!gameStore.buildWindowVisible) return
  gameStore.setBuildWindowVisible(false)
  gameFacade?.cancelBuildGhost?.()
  sendCloseWindow('build')
}

function handleBuildStateWindowClose() {
  gameStore.closeBuildStateWindow()
}

function handleBuildStateProgress(entityId: number) {
  sendBuildProgress(entityId)
}

function toggleBuildWindow() {
  if (gameStore.buildWindowVisible) {
    closeBuildWindow()
    return
  }
  openBuildWindow()
}

function toggleCraftWindow() {
  if (gameStore.craftWindowVisible) {
    closeCraftWindow()
    return
  }
  openCraftWindow()
}

function alertTypeForSeverity(severity: proto.AlertSeverity): 'error' | 'warning' | 'info' {
  switch (severity) {
    case proto.AlertSeverity.ALERT_SEVERITY_ERROR:
      return 'error'
    case proto.AlertSeverity.ALERT_SEVERITY_WARNING:
      return 'warning'
    default:
      return 'info'
  }
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
        if ((gameStore.armedBuildKey || '').trim()) {
          gameStore.clearBuildPlacement()
          gameFacade?.cancelBuildGhost?.()
          break
        }
        chatContainerRef.value?.unfocusChat()
        gameStore.setPlayerInventoryVisible(false)
        gameStore.setPlayerEquipmentVisible(false)
        gameStore.setCharacterSheetVisible(false)
        gameStore.setPlayerStatsWindowVisible(false)
        closeCraftWindow()
        closeBuildWindow()
        gameStore.closeBuildStateWindow()
        gameStore.closeContextMenu()
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
      case 'c':
        gameStore.toggleCharacterSheet()
        break
      case 'e':
        gameStore.togglePlayerEquipment()
        break
      case 'p':
        gameStore.togglePlayerStatsWindow()
        break
      case 'o':
        toggleCraftWindow()
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
        {{ connectionState === 'authenticating' ? 'Authenticating...' : 'Connecting...' }}
      </p>
    </div>

    <!-- Error state -->
    <div v-else-if="hasError" class="game-error">
      <AppAlert type="error">
        {{ connectionError?.message || 'Connection error' }}
      </AppAlert>
      <div class="game-error__actions">
        <AppButton @click="handleRetry">Retry</AppButton>
        <AppButton variant="secondary" @click="handleBack">Back</AppButton>
      </div>
    </div>

    <!-- Connected state -->
    <div v-else-if="isConnected" class="game-canvas-wrapper">
      <canvas ref="gameCanvas" class="game-canvas"></canvas>
      <div
        v-if="showLoadingOverlay"
        class="game-loading-overlay"
        :class="{ 'is-fading': loadingOverlayFading }"
      >
        <img src="/assets/img/origin_logo3.webp" alt="Origin logo" class="game-loading-overlay__logo">
        <AppSpinner size="lg" />
        <p class="game-loading-overlay__title">Loading world...</p>
        <p v-if="showLoadingSlowHint" class="game-loading-overlay__hint">Still loading, please wait...</p>
      </div>
      <div class="game-ui">
        <AppButton variant="secondary" size="sm" @click="toggleCraftWindow">
          {{ showCraftWindow ? 'Close Craft' : 'Craft' }}
        </AppButton>
        <AppButton variant="secondary" size="sm" @click="toggleBuildWindow">
          {{ showBuildWindow ? 'Close Build' : 'Build' }}
        </AppButton>
        <AppButton variant="secondary" size="sm" @click="handleBack">Exit</AppButton>
      </div>
      <div v-if="miniAlerts.length > 0" class="game-mini-alerts">
        <AppAlert
          v-for="alert in miniAlerts"
          :key="alert.id"
          :type="alertTypeForSeverity(alert.severity)"
          class="game-mini-alert"
        >
          {{ alert.message }}
        </AppAlert>
      </div>
      <ActionHourGlass />
      <div class="game-player-stats">
        <PlayerStatsBars />
      </div>
      <div class="game-movement-modes">
        <MovementModePanel />
      </div>
      <ContextMenu />
      <div class="game-chat">
        <ChatContainer ref="chatContainerRef" @send="handleChatSend" />
      </div>
      <div v-if="showInventory" class="game-inventory">
        <InventoryWindow :inventory="playerInventory!" @close="handleInventoryClose" />
      </div>

      <div v-if="showEquipment" class="game-equipment">
        <EquipmentWindow :inventory="playerEquipment!" @close="handleEquipmentClose" />
      </div>

      <div v-if="showCharacterSheet" class="game-character-sheet">
        <CharacterSheetWindow @close="handleCharacterSheetClose" />
      </div>

      <div v-if="showPlayerStatsWindow" class="game-player-stats-window">
        <PlayerStatsWindow @close="handlePlayerStatsClose" />
      </div>

      <div v-if="showCraftWindow" class="game-craft-window">
        <CraftWindow @close="closeCraftWindow" />
      </div>

      <div v-if="showBuildWindow" class="game-build-window">
        <BuildWindow @close="closeBuildWindow" />
      </div>

      <div v-if="showBuildStateWindow" class="game-build-state-window">
        <BuildStateWindow
          :entity-id="currentBuildStateEntityId"
          :list="currentBuildStateList"
          @close="handleBuildStateWindowClose"
          @progress="handleBuildStateProgress"
        />
      </div>
      
      <!-- Nested inventory windows -->
      <div v-for="windowKey in openNestedInventoryWindows" :key="windowKey" class="game-nested-inventory">
        <NestedInventoryWindow 
          v-if="gameStore.getNestedInventoryData(windowKey)"
          :window-key="windowKey" 
          :inventory-state="gameStore.getNestedInventoryData(windowKey)!"
          @close="() => handleNestedInventoryClose(windowKey)" 
        />
      </div>

      <!-- Hand overlay (item following cursor) -->
      <HandOverlay />
    </div>

    <!-- Disconnected state -->
    <div v-else class="game-disconnected">
      <p>Disconnected from server</p>
      <AppButton @click="handleBack">Back to characters</AppButton>
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
  padding: 30px;
  border-radius: 18px;
  background: rgba(16, 96, 109, 0.65);
  box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);

  &__text {
    color: #d5eeed;
    font-size: 1.125rem;
    font-weight: 500;
    letter-spacing: 0.3px;
    text-shadow: 0 1px 4px rgba(0, 0, 0, 0.35);
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

.game-loading-overlay {
  position: absolute;
  inset: 0;
  z-index: 400;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: rgba(12, 20, 25, 0.68);
  backdrop-filter: blur(2px);
  transition: opacity 0.25s ease;
  opacity: 1;
  pointer-events: all;

  &.is-fading {
    opacity: 0;
    pointer-events: none;
  }

  &__logo {
    width: min(380px, 68vw);
    filter: drop-shadow(2px 11px 8px rgba(0, 0, 0, 0.8));
  }

  &__title {
    color: #d5eeed;
    font-size: 22px;
    letter-spacing: 0.5px;
  }

  &__hint {
    color: #b8cdd6;
    font-size: 15px;
  }
}

.game-ui {
  position: absolute;
  top: 1rem;
  right: 1rem;
  z-index: 100;
  display: flex;
  gap: 8px;
}

.game-chat {
  position: absolute;
  left: 12px;
  bottom: 12px;
  width: calc(100% - 70px);
  max-width: 340px;
  z-index: 100;
}

.game-mini-alerts {
  position: absolute;
  top: 18px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 180;
  pointer-events: none;
  width: min(90vw, 420px);
}

.game-mini-alert {
  text-align: center;
}

.game-player-stats {
  position: absolute;
  top: 8px;
  left: 100px;
  z-index: 120;
}

.game-movement-modes {
  position: absolute;
  top: 62px;
  left: 100px;
  z-index: 120;
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

.game-character-sheet {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 250;
}

.game-player-stats-window {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 255;
}

.game-equipment {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 225;
}

.game-craft-window {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 260;
}

.game-build-window {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 261;
}

.game-build-state-window {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 262;
}

.game-disconnected {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
  color: #a0a0a0;
}
</style>
