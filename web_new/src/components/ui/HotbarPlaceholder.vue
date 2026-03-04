<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  getActionShortLabel,
  type ActionId,
  type HotbarState,
} from '@/game/hud/actionCatalog'

const props = withDefaults(defineProps<{
  assignments: HotbarState
  draggingActionId?: ActionId | null
  touchHoverSlot?: number | null
}>(), {
  draggingActionId: null,
  touchHoverSlot: null,
})

const emit = defineEmits<{
  drop: [slotIndex: number, actionId: ActionId]
  clear: [slotIndex: number]
  activate: [slotIndex: number]
}>()

const longPressTimer = ref<number | null>(null)
const longPressTriggered = ref(false)

const leftGroupSlots = computed(() => [0, 1, 2, 3, 4])
const rightGroupSlots = computed(() => [5, 6, 7, 8, 9])

function parseActionIdFromDataTransfer(event: DragEvent): ActionId | null {
  const actionRaw = event.dataTransfer?.getData('application/x-origin-action-id')
    || event.dataTransfer?.getData('text/plain')
    || ''
  if (
    actionRaw !== 'settings' &&
    actionRaw !== 'actions' &&
    actionRaw !== 'craft' &&
    actionRaw !== 'build' &&
    actionRaw !== 'stats' &&
    actionRaw !== 'inventory'
  ) {
    return null
  }
  return actionRaw
}

function onDrop(event: DragEvent, slotIndex: number): void {
  event.preventDefault()
  const actionId = parseActionIdFromDataTransfer(event)
  if (!actionId) {
    return
  }
  emit('drop', slotIndex, actionId)
}

function onActivate(slotIndex: number): void {
  if (longPressTriggered.value) {
    longPressTriggered.value = false
    return
  }
  emit('activate', slotIndex)
}

function clearLongPressTimer(): void {
  if (longPressTimer.value != null) {
    window.clearTimeout(longPressTimer.value)
    longPressTimer.value = null
  }
}

function onSlotPointerDown(event: PointerEvent, slotIndex: number): void {
  if (event.pointerType !== 'touch' || !props.assignments[slotIndex]) {
    return
  }
  longPressTriggered.value = false
  clearLongPressTimer()
  longPressTimer.value = window.setTimeout(() => {
    longPressTriggered.value = true
    emit('clear', slotIndex)
  }, 650)
}

function onSlotPointerUp(): void {
  clearLongPressTimer()
}

function isDropTarget(slotIndex: number): boolean {
  return props.draggingActionId != null && props.touchHoverSlot === slotIndex
}
</script>

<template>
  <div class="hotbar" aria-label="Hotbar">
    <div class="hotbar__group">
      <button
        v-for="slotIndex in leftGroupSlots"
        :key="`slot-${slotIndex}`"
        class="hotbar__slot"
        :class="{ 'hotbar__slot--drop-target': isDropTarget(slotIndex) }"
        type="button"
        :data-hotbar-slot="slotIndex"
        :aria-label="`Hotbar slot ${slotIndex + 1}`"
        @dragover.prevent
        @drop="onDrop($event, slotIndex)"
        @click="onActivate(slotIndex)"
        @contextmenu.prevent="emit('clear', slotIndex)"
        @pointerdown="onSlotPointerDown($event, slotIndex)"
        @pointerup="onSlotPointerUp"
        @pointercancel="onSlotPointerUp"
      >
        <span class="hotbar__slot-index">{{ slotIndex + 1 }}</span>
        <span v-if="assignments[slotIndex]" class="hotbar__slot-label">{{ getActionShortLabel(assignments[slotIndex]!) }}</span>
      </button>
    </div>

    <div class="hotbar__gap" />

    <div class="hotbar__group">
      <button
        v-for="slotIndex in rightGroupSlots"
        :key="`slot-${slotIndex}`"
        class="hotbar__slot"
        :class="{ 'hotbar__slot--drop-target': isDropTarget(slotIndex) }"
        type="button"
        :data-hotbar-slot="slotIndex"
        :aria-label="`Hotbar slot ${slotIndex + 1}`"
        @dragover.prevent
        @drop="onDrop($event, slotIndex)"
        @click="onActivate(slotIndex)"
        @contextmenu.prevent="emit('clear', slotIndex)"
        @pointerdown="onSlotPointerDown($event, slotIndex)"
        @pointerup="onSlotPointerUp"
        @pointercancel="onSlotPointerUp"
      >
        <span class="hotbar__slot-index">{{ slotIndex + 1 }}</span>
        <span v-if="assignments[slotIndex]" class="hotbar__slot-label">{{ getActionShortLabel(assignments[slotIndex]!) }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped lang="scss">
.hotbar {
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: auto;
}

.hotbar__group {
  display: flex;
  gap: 8px;
}

.hotbar__gap {
  width: 24px;
}

.hotbar__slot {
  position: relative;
  width: 46px;
  height: 46px;
  border: 1px solid rgba(212, 188, 136, 0.85);
  border-radius: 10px;
  background: rgba(26, 34, 40, 0.88);
  color: #eef3f8;
  cursor: pointer;
  user-select: none;
}

.hotbar__slot--drop-target {
  border-color: #55c4ff;
  box-shadow: 0 0 0 2px rgba(85, 196, 255, 0.25) inset;
}

.hotbar__slot-index {
  position: absolute;
  left: 5px;
  top: 4px;
  font-size: 10px;
  color: #c8d3de;
}

.hotbar__slot-label {
  display: inline-block;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

@media (max-width: 900px) {
  .hotbar__group {
    gap: 6px;
  }

  .hotbar__gap {
    width: 16px;
  }

  .hotbar__slot {
    width: 40px;
    height: 40px;
    border-radius: 9px;
  }

  .hotbar__slot-index,
  .hotbar__slot-label {
    font-size: 9px;
  }
}

@media (orientation: landscape) and (pointer: coarse) {
  .hotbar__group {
    gap: 4px;
  }

  .hotbar__gap {
    width: 10px;
  }

  .hotbar__slot {
    width: 34px;
    height: 34px;
    border-radius: 8px;
  }

  .hotbar__slot-index {
    left: 4px;
    top: 3px;
    font-size: 8px;
  }

  .hotbar__slot-label {
    font-size: 8px;
    letter-spacing: 0.02em;
  }
}
</style>
