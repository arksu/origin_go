<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import ContextMenuButton from '@/components/ui/ContextMenuButton.vue'
import { useGameStore } from '@/stores/gameStore'
import { playerCommandController } from '@/game'
import type { ContextMenuActionItem } from '@/stores/gameStore'

const gameStore = useGameStore()

const menu = computed(() => gameStore.contextMenu)
const displayActions = ref<ContextMenuActionItem[]>([])
const selectedActionId = ref<string | null>(null)
const selectedOriginIndex = ref<number>(0)
const selectedOriginTotal = ref<number>(1)

let selectedHoldTimer: ReturnType<typeof setTimeout> | null = null
let closeTimer: ReturnType<typeof setTimeout> | null = null

const posX = computed(() => menu.value?.anchorX || 0)
const posY = computed(() => menu.value?.anchorY || 0)

watch(menu, (nextMenu) => {
  clearTimers()
  selectedActionId.value = null
  selectedOriginIndex.value = 0
  selectedOriginTotal.value = Math.max(1, nextMenu?.actions?.length || 1)
  displayActions.value = nextMenu?.actions ? [...nextMenu.actions] : []
}, { immediate: true })

function clearTimers() {
  if (selectedHoldTimer) {
    clearTimeout(selectedHoldTimer)
    selectedHoldTimer = null
  }
  if (closeTimer) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
}

function onSelect(actionId: string) {
  const currentMenu = menu.value
  if (!currentMenu || !actionId || selectedActionId.value) {
    return
  }

  selectedActionId.value = actionId
  playerCommandController.sendSelectContextAction(currentMenu.entityId, actionId)

  // Preserve selected button original radial layout so it can move to center
  // from its own coordinates even after we collapse the list to one item.
  const originIdx = displayActions.value.findIndex((a) => a.actionId === actionId)
  selectedOriginIndex.value = Math.max(0, originIdx)
  selectedOriginTotal.value = Math.max(1, displayActions.value.length)

  // Phase 1: immediately hide non-selected actions (0.3s leave transition).
  const selected = displayActions.value.find((a) => a.actionId === actionId)
  displayActions.value = selected ? [selected] : []

  // Phase 2: keep selected visible for 1.0s, then hide it with 0.5s transition.
  selectedHoldTimer = setTimeout(() => {
    displayActions.value = []
  }, 1000)

  // After selected leave transition finishes, reset menu state.
  // A tiny buffer avoids end-frame flicker on slower devices.
  closeTimer = setTimeout(() => {
    gameStore.closeContextMenu()
    selectedActionId.value = null
  }, 1600)
}

onBeforeUnmount(() => {
  clearTimers()
})
</script>

<template>
  <transition-group
    :style="{ left: `${posX}px`, top: `${posY}px` }"
    class="context-menu-container"
    name="spiral"
    tag="div"
  >
    <ContextMenuButton
      v-for="(item, idx) in displayActions"
      :key="item.actionId"
      :item="item"
      :index="selectedActionId === item.actionId ? selectedOriginIndex : idx"
      :total="selectedActionId === item.actionId ? selectedOriginTotal : displayActions.length"
      :selected="selectedActionId === item.actionId"
      class="action-button"
      @select="onSelect"
    />
  </transition-group>
</template>

<style scoped lang="scss">
.context-menu-container {
  pointer-events: auto;
  opacity: 1;
  position: absolute;
  z-index: 220;
}

.action-button {
  position: absolute;
  transform: translate(var(--x1), var(--y1)) translate(-50%, -50%);
  animation-duration: 0.5s;
  animation-direction: alternate;
  animation-timing-function: linear;
}

.spiral-enter-active {
  animation-name: cm-move;
}

.spiral-leave-active {
  animation-duration: 0.3s;
  animation-name: cm-move-hide;
  animation-fill-mode: forwards;
}

.action-button.is-selected.spiral-leave-active {
  animation-duration: 0.5s;
  animation-name: cm-selected-hide;
  animation-fill-mode: forwards;
}

@keyframes cm-selected-hide {
  0% {
    transform: translate(0, 0) translate(-50%, -50%);
    opacity: 1;
  }
  100% {
    transform: translate(0, 0) translate(-50%, -50%);
    opacity: 0;
  }
}

.context-menu-container:empty {
  pointer-events: none;
}

@keyframes cm-move {
  0% {
    transform: translate(0, 0) translate(-50%, -50%);
    opacity: 0;
  }
  33% {
    transform: translate(var(--x3), var(--y3)) translate(-50%, -50%);
    opacity: 0.2;
  }
  66% {
    transform: translate(var(--x2), var(--y2)) translate(-50%, -50%);
    opacity: 0.66;
  }
  100% {
    transform: translate(var(--x1), var(--y1)) translate(-50%, -50%);
    opacity: 1;
  }
}

@keyframes cm-move-hide {
  100% {
    transform: translate(var(--x1), var(--y1)) translate(-50%, -50%);
    opacity: 0;
  }
  0% {
    transform: translate(var(--x1), var(--y1)) translate(-50%, -50%);
    opacity: 1;
  }
}
</style>
