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
import ActionsRail from '@/components/ui/ActionsRail.vue'
import HotbarPlaceholder from '@/components/ui/HotbarPlaceholder.vue'
import PortraitWarningBanner from '@/components/ui/PortraitWarningBanner.vue'
import { sendChatMessage, sendOpenWindow, sendCloseWindow, sendStartBuild, sendBuildProgress, sendBuildTakeBack, sendLiftPutDown } from '@/network'
import { useInventoryOps } from '@/composables/useInventoryOps'
import { useHotkeys } from '@/composables/useHotkeys'
import { useHotbarAssignments } from '@/composables/useHotbarAssignments'
import { DEFAULT_HOTKEYS, type HotkeyConfig } from '@/constants/hotkeys'
import { proto } from '@/network/proto/packets.js'
import { useAuthStore } from '@/stores/authStore'
import { getActionLabel, type ActionId } from '@/game/hud/actionCatalog'

const router = useRouter()
const gameStore = useGameStore()
const authStore = useAuthStore()
const { placeItemIntoBuild } = useInventoryOps()

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
const currentBuildStateName = computed(() => gameStore.buildStateName)
const currentBuildStateList = computed(() => gameStore.buildStateList)
const buildStateHandHasItem = computed(() => !!gameStore.handState?.item)
const liftCarryActive = computed(() => gameStore.liftCarryActive)
const liftCarriedEntityId = computed(() => gameStore.liftCarriedEntityId)
const liftCarriedResourcePath = computed(() => {
  const entityId = gameStore.liftCarriedEntityId
  if (!entityId) return ''
  return gameStore.entities.get(entityId)?.resourcePath || ''
})
const liftPutDownModeActive = ref(false)
const playerEquipment = computed(() => gameStore.getPlayerEquipment())
const showEquipment = computed(() => {
  const visible = gameStore.playerEquipmentVisible
  const hasEquipment = !!playerEquipment.value?.equipment
  return visible && hasEquipment
})
const deathDialog = computed(() => gameStore.deathDialog)
const draggingActionId = ref<ActionId | null>(null)
const touchDraggingActionId = ref<ActionId | null>(null)
const touchDragX = ref(0)
const touchDragY = ref(0)
const touchHoverSlot = ref<number | null>(null)
const isPortrait = ref(false)
const isMobileDevice = ref(false)
const portraitWarningDismissed = ref(false)

const PORTRAIT_WARNING_DISMISSED_KEY = 'hud_portrait_warning_dismissed_v1'

const accountId = computed(() => {
  const token = authStore.token || ''
  if (!token) {
    return ''
  }

  const payloadPart = token.split('.')[1]
  if (!payloadPart) {
    return ''
  }

  try {
    const base64 = payloadPart.replace(/-/g, '+').replace(/_/g, '/')
    const decoded = JSON.parse(atob(base64))
    if (typeof decoded?.sub === 'string' && decoded.sub.length > 0) {
      return decoded.sub
    }
  } catch {
    // no-op: fallback to anonymous storage key in composable
  }
  return ''
})

const { assignments: hotbarAssignments, assign: assignHotbarSlot, clear: clearHotbarSlot, get: getHotbarSlotAction } =
  useHotbarAssignments(accountId, computed(() => gameStore.characterId))

const showPortraitWarning = computed(() => isMobileDevice.value && isPortrait.value && !portraitWarningDismissed.value)
const touchDragLabel = computed(() => (touchDraggingActionId.value ? getActionLabel(touchDraggingActionId.value) : ''))

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

function syncLiftGhostFromStore(): void {
  if (!gameFacade || !canvasInitialized.value || !gameFacade.isInitialized?.()) {
    return
  }

  if (!liftPutDownModeActive.value || !gameStore.liftCarryActive || !gameStore.liftCarriedEntityId) {
    gameFacade.cancelLiftGhost?.()
    return
  }

  gameFacade.armLiftGhost?.({
    entityId: gameStore.liftCarriedEntityId,
    resourcePath: gameStore.entities.get(gameStore.liftCarriedEntityId)?.resourcePath || '',
  })
}

function cancelLiftPutDownMode(): void {
  if (!liftPutDownModeActive.value) return
  liftPutDownModeActive.value = false
  gameFacade?.cancelLiftGhost?.()
}

function toggleLiftPutDownMode(): void {
  if (!gameStore.liftCarryActive || !gameStore.liftCarriedEntityId) {
    cancelLiftPutDownMode()
    return
  }
  liftPutDownModeActive.value = !liftPutDownModeActive.value
  syncLiftGhostFromStore()
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
        const carriedEntityId = gameStore.liftCarriedEntityId
        if (liftPutDownModeActive.value && gameStore.liftCarryActive && carriedEntityId) {
          const ghostPos = gameFacade?.getLiftGhostWorldPosition?.()
          const targetPos = ghostPos || { x: worldX, y: worldY }
          sendLiftPutDown(carriedEntityId, {
            x: targetPos.x,
            y: targetPos.y,
          })
          return true
        }
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
    syncLiftGhostFromStore()
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

watch(
  [liftCarryActive, liftCarriedEntityId, liftCarriedResourcePath, liftPutDownModeActive, () => gameStore.entities.size],
  () => {
    if (!gameStore.liftCarryActive) {
      liftPutDownModeActive.value = false
    }
    syncLiftGhostFromStore()
  }
)

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
  window.addEventListener('resize', onOrientationChange)
  window.matchMedia('(orientation: portrait)').addEventListener('change', onOrientationChange)
  portraitWarningDismissed.value = sessionStorage.getItem(PORTRAIT_WARNING_DISMISSED_KEY) === '1'
  onOrientationChange()

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
  window.removeEventListener('resize', onOrientationChange)
  window.matchMedia('(orientation: portrait)').removeEventListener('change', onOrientationChange)

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

function handleDeathDialogBack() {
  gameStore.clearDeathDialog()
  handleBack()
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

function handleBuildStatePutFromHand(entityId: number) {
  placeItemIntoBuild(entityId)
}

function handleBuildStateTakeBack(entityId: number, slot: number) {
  sendBuildTakeBack(entityId, slot)
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

function executeAction(actionId: ActionId): void {
  switch (actionId) {
    case 'craft':
      toggleCraftWindow()
      return
    case 'build':
      toggleBuildWindow()
      return
    case 'stats':
      gameStore.togglePlayerStatsWindow()
      return
    case 'inventory':
      gameStore.togglePlayerInventory()
      return
    case 'equip':
      gameStore.togglePlayerEquipment()
      return
    case 'settings':
      console.log('[HUD] Settings action selected (not implemented yet)')
      return
    case 'actions':
      console.log('[HUD] Actions action selected (not implemented yet)')
      return
  }
}

function onActionsRailActivate(actionId: ActionId): void {
  executeAction(actionId)
}

function onActionDragStart(actionId: ActionId): void {
  draggingActionId.value = actionId
}

function onActionDragEnd(): void {
  draggingActionId.value = null
}

function onHotbarDrop(slotIndex: number, actionId: ActionId): void {
  assignHotbarSlot(slotIndex, actionId)
  draggingActionId.value = null
  touchDraggingActionId.value = null
  touchHoverSlot.value = null
}

function onHotbarActivate(slotIndex: number): void {
  const actionId = getHotbarSlotAction(slotIndex)
  if (!actionId) {
    return
  }
  executeAction(actionId)
}

function onHotbarClear(slotIndex: number): void {
  clearHotbarSlot(slotIndex)
}

function updateTouchHoverSlot(clientX: number, clientY: number): void {
  const element = document.elementFromPoint(clientX, clientY) as HTMLElement | null
  if (!element) {
    touchHoverSlot.value = null
    return
  }
  const slotElement = element.closest('[data-hotbar-slot]') as HTMLElement | null
  if (!slotElement) {
    touchHoverSlot.value = null
    return
  }
  const raw = slotElement.getAttribute('data-hotbar-slot')
  const index = raw ? Number.parseInt(raw, 10) : Number.NaN
  if (Number.isNaN(index) || index < 0 || index > 9) {
    touchHoverSlot.value = null
    return
  }
  touchHoverSlot.value = index
}

function onTouchDragStart(payload: { actionId: ActionId; pointerId: number; clientX: number; clientY: number }): void {
  void payload.pointerId
  touchDraggingActionId.value = payload.actionId
  draggingActionId.value = payload.actionId
  touchDragX.value = payload.clientX
  touchDragY.value = payload.clientY
  updateTouchHoverSlot(payload.clientX, payload.clientY)
}

function onTouchDragMove(payload: { pointerId: number; clientX: number; clientY: number }): void {
  void payload.pointerId
  if (!touchDraggingActionId.value) {
    return
  }
  touchDragX.value = payload.clientX
  touchDragY.value = payload.clientY
  updateTouchHoverSlot(payload.clientX, payload.clientY)
}

function onTouchDragEnd(payload: { pointerId: number; clientX: number; clientY: number }): void {
  void payload.pointerId
  touchDragX.value = payload.clientX
  touchDragY.value = payload.clientY
  updateTouchHoverSlot(payload.clientX, payload.clientY)

  if (touchDraggingActionId.value != null && touchHoverSlot.value != null) {
    assignHotbarSlot(touchHoverSlot.value, touchDraggingActionId.value)
  }

  touchDraggingActionId.value = null
  draggingActionId.value = null
  touchHoverSlot.value = null
}

function dismissPortraitWarning(): void {
  portraitWarningDismissed.value = true
  sessionStorage.setItem(PORTRAIT_WARNING_DISMISSED_KEY, '1')
}

function onOrientationChange(): void {
  const supportsTouch = navigator.maxTouchPoints > 0 || 'ontouchstart' in window
  const coarsePointer = window.matchMedia('(pointer: coarse)').matches
  const mobileUa = /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent)

  isMobileDevice.value = supportsTouch && (coarsePointer || mobileUa)
  isPortrait.value = window.matchMedia('(orientation: portrait)').matches
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

const hotbarNumberHotkeys: HotkeyConfig[] = Array.from({ length: 10 }, (_, index) => {
  const key = index === 9 ? '0' : String(index + 1)
  const slotIndex = index
  return {
    key,
    description: `Activate hotbar slot ${slotIndex + 1}`,
    action: () => {
      onHotbarActivate(slotIndex)
    },
  }
})

// Setup hotkeys
const hotkeys: HotkeyConfig[] = [...DEFAULT_HOTKEYS, ...hotbarNumberHotkeys].map(config => ({
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
        if (liftPutDownModeActive.value) {
          cancelLiftPutDownMode()
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
        gameStore.togglePlayerInventory()
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
      case '1':
      case '2':
      case '3':
      case '4':
      case '5':
      case '6':
      case '7':
      case '8':
      case '9':
      case '0':
        config.action()
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
      <div v-if="deathDialog" class="game-death-dialog-backdrop">
        <div class="game-death-dialog">
          <h2 class="game-death-dialog__title">{{ deathDialog.title }}</h2>
          <p class="game-death-dialog__message">{{ deathDialog.message }}</p>
          <AppButton @click="handleDeathDialogBack">Back to characters</AppButton>
        </div>
      </div>
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
      <div class="hud-root">
        <div class="hud-top-hotbar">
          <HotbarPlaceholder
            :assignments="hotbarAssignments"
            :dragging-action-id="draggingActionId"
            :touch-hover-slot="touchHoverSlot"
            @drop="onHotbarDrop"
            @clear="onHotbarClear"
            @activate="onHotbarActivate"
          />
        </div>

        <div class="hud-left-rail">
          <ActionsRail
            @activate="onActionsRailActivate"
            @drag-start="onActionDragStart"
            @drag-end="onActionDragEnd"
            @touch-drag-start="onTouchDragStart"
            @touch-drag-move="onTouchDragMove"
            @touch-drag-end="onTouchDragEnd"
          />
          <AppButton
            v-if="liftCarryActive"
            variant="secondary"
            size="sm"
            class="hud-left-rail__lift-button"
            @click="toggleLiftPutDownMode"
          >
            {{ liftPutDownModeActive ? 'Cancel' : 'Lift down' }}
          </AppButton>
          <AppButton variant="secondary" size="sm" class="hud-left-rail__exit-button" @click="handleBack">Exit</AppButton>
        </div>

        <div class="hud-bottom-row">
          <div class="hud-bottom-left-stats">
            <MovementModePanel />
            <PlayerStatsBars />
          </div>

          <div class="hud-bottom-center-alerts">
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
          </div>

          <div class="hud-bottom-right-chat">
            <ChatContainer ref="chatContainerRef" @send="handleChatSend" />
          </div>
        </div>

        <div v-if="showPortraitWarning" class="hud-portrait-warning">
          <PortraitWarningBanner @close="dismissPortraitWarning" />
        </div>

        <div
          v-if="touchDraggingActionId"
          class="hud-drag-ghost"
          :style="{ left: `${touchDragX}px`, top: `${touchDragY}px` }"
        >
          {{ touchDragLabel }}
        </div>
      </div>

      <ActionHourGlass />
      <ContextMenu />
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
          :title="currentBuildStateName"
          :list="currentBuildStateList"
          :hand-has-item="buildStateHandHasItem"
          @close="handleBuildStateWindowClose"
          @progress="handleBuildStateProgress"
          @put-from-hand="handleBuildStatePutFromHand"
          @take-back="handleBuildStateTakeBack"
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

.hud-root {
  position: absolute;
  inset: 0;
  z-index: 120;
  pointer-events: none;
}

.hud-top-hotbar {
  position: absolute;
  top: calc(8px + env(safe-area-inset-top));
  left: 50%;
  transform: translateX(-50%);
}

.hud-left-rail {
  position: absolute;
  top: calc(8px + env(safe-area-inset-top));
  left: calc(8px + env(safe-area-inset-left));
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: auto;
}

.hud-left-rail__lift-button,
.hud-left-rail__exit-button {
  width: 64px;
}

.hud-bottom-row {
  position: absolute;
  left: calc(8px + env(safe-area-inset-left));
  right: calc(8px + env(safe-area-inset-right));
  bottom: calc(8px + env(safe-area-inset-bottom));
  display: grid;
  grid-template-columns: minmax(220px, 28vw) minmax(220px, 1fr) minmax(280px, 32vw);
  gap: 12px;
  align-items: end;
}

.hud-bottom-left-stats {
  pointer-events: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.hud-bottom-center-alerts {
  display: flex;
  justify-content: center;
  align-items: flex-end;
}

.hud-bottom-right-chat {
  pointer-events: auto;
  justify-self: end;
  width: min(360px, 100%);
}

.hud-portrait-warning {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  pointer-events: auto;
}

.hud-drag-ghost {
  position: fixed;
  transform: translate(-50%, -50%);
  padding: 6px 10px;
  border: 1px solid rgba(92, 183, 231, 0.95);
  border-radius: 8px;
  background: rgba(17, 35, 47, 0.92);
  color: #d8edf8;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.03em;
  pointer-events: none;
}

.game-death-dialog-backdrop {
  position: absolute;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(7, 10, 14, 0.72);
  backdrop-filter: blur(2px);
}

.game-death-dialog {
  width: min(420px, calc(100% - 32px));
  padding: 22px 20px;
  border-radius: 14px;
  border: 1px solid rgba(255, 255, 255, 0.2);
  background: rgba(20, 24, 30, 0.95);
  box-shadow: 0 16px 34px rgba(0, 0, 0, 0.5);
  color: #e8edf2;
  text-align: center;
}

.game-death-dialog__title {
  margin: 0 0 8px;
  font-size: 24px;
  font-weight: 700;
}

.game-death-dialog__message {
  margin: 0 0 16px;
  font-size: 16px;
}

.game-mini-alerts {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: min(420px, 100%);
}

.game-mini-alert {
  text-align: center;
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

@media (max-width: 900px) {
  .hud-bottom-row {
    grid-template-columns: minmax(180px, 32vw) minmax(180px, 1fr) minmax(220px, 38vw);
    gap: 8px;
  }

  .hud-left-rail__lift-button,
  .hud-left-rail__exit-button {
    width: 56px;
    min-height: 34px;
    font-size: 11px;
  }

  .hud-bottom-right-chat {
    width: min(300px, 100%);
  }
}

@media (orientation: landscape) and (pointer: coarse) {
  .hud-top-hotbar {
    top: max(4px, env(safe-area-inset-top));
  }

  .hud-left-rail {
    top: max(4px, env(safe-area-inset-top));
    left: max(4px, env(safe-area-inset-left));
    gap: 4px;
  }

  .hud-left-rail__lift-button,
  .hud-left-rail__exit-button {
    width: 46px;
    min-height: 26px;
    font-size: 10px;
    padding: 4px 6px;
  }

  .hud-bottom-row {
    left: env(safe-area-inset-left);
    right: 0;
    bottom: max(4px, env(safe-area-inset-bottom));
  }

  .hud-bottom-right-chat {
    justify-self: end;
    width: min(320px, 42vw);
  }
}
</style>
