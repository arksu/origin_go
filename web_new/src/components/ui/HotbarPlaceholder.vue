<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import ActionIcon from './ActionIcon.vue'
import ActionTooltip from './ActionTooltip.vue'
import { useActionPresentation } from '@/composables/useActionPresentation'
import {
  isHotbarActionId,
  type HotbarActionId,
  type HotbarState,
} from '@/game/hud/actionCatalog'

const props = withDefaults(defineProps<{
  assignments: HotbarState
  draggingActionId?: HotbarActionId | null
  touchHoverSlot?: number | null
}>(), {
  draggingActionId: null,
  touchHoverSlot: null,
})

const emit = defineEmits<{
  drop: [slotIndex: number, actionId: HotbarActionId]
  clear: [slotIndex: number]
  activate: [slotIndex: number]
}>()

const longPressTimer = ref<number | null>(null)
const longPressTriggered = ref(false)
const tooltipSlot = ref<number | null>(null)
const tooltipX = ref(0)
const tooltipY = ref(0)

const leftGroupSlots = computed(() => [0, 1, 2, 3, 4])
const rightGroupSlots = computed(() => [5, 6, 7, 8, 9])

function parseActionIdFromDataTransfer(event: DragEvent): HotbarActionId | null {
  const actionRaw = event.dataTransfer?.getData('application/x-origin-action-id')
    || event.dataTransfer?.getData('text/plain')
    || ''
  return isHotbarActionId(actionRaw) ? actionRaw : null
}

const { presentation } = useActionPresentation()

const tooltipText = computed(() => {
  const slotIndex = tooltipSlot.value
  return slotIndex != null && slotVisible(props.assignments[slotIndex]) ? slotTooltip(slotIndex) : ''
})

watch(() => props.draggingActionId, id => {
  if (id) hideTooltip()
})
onBeforeUnmount(clearLongPressTimer)

function slotVisible(id: HotbarActionId | null | undefined): boolean {
  return !!id && presentation(id).available
}

function slotCoolingDown(slotIndex: number): boolean {
  const id = props.assignments[slotIndex]
  return !!id && presentation(id).coolingDown
}

function onDrop(event: DragEvent, slotIndex: number): void {
  hideTooltip()
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
  const id = props.assignments[slotIndex]
  if (slotVisible(id)) emit('activate', slotIndex)
}

function clearLongPressTimer(): void {
  if (longPressTimer.value != null) {
    window.clearTimeout(longPressTimer.value)
    longPressTimer.value = null
  }
}

function showTooltip(slotIndex: number, clientX: number, clientY: number): void {
  if (props.draggingActionId) return
  tooltipSlot.value = slotIndex
  tooltipX.value = clientX + 10
  tooltipY.value = clientY + 10
}

function hideTooltip(): void {
  tooltipSlot.value = null
}

function onSlotPointerDown(event: PointerEvent, slotIndex: number): void {
  const actionId = props.assignments[slotIndex]
  if (!slotVisible(actionId)) {
    hideTooltip()
    return
  }
  showTooltip(slotIndex, event.clientX, event.clientY)
  if (event.pointerType !== 'touch') {
    return
  }
  longPressTriggered.value = false
  clearLongPressTimer()
  longPressTimer.value = window.setTimeout(() => {
    longPressTriggered.value = true
    hideTooltip()
    emit('clear', slotIndex)
  }, 650)
}

function onSlotPointerUp(): void {
  clearLongPressTimer()
  hideTooltip()
}

function isDropTarget(slotIndex: number): boolean {
  return props.draggingActionId != null && props.touchHoverSlot === slotIndex
}

function slotTooltip(slotIndex: number): string {
  const actionId = props.assignments[slotIndex]
  if (!actionId) {
    return `Hotbar slot ${slotIndex + 1} (empty)`
  }
  return `Hotbar slot ${slotIndex + 1}: ${presentation(actionId).tooltip}`
}

function onSlotPointerEnter(event: PointerEvent, slotIndex: number): void {
  if (!slotVisible(props.assignments[slotIndex])) {
    hideTooltip()
    return
  }
  if (event.pointerType === 'touch') {
    return
  }
  showTooltip(slotIndex, event.clientX, event.clientY)
}

function onSlotPointerMove(event: PointerEvent, slotIndex: number): void {
  if (!slotVisible(props.assignments[slotIndex])) {
    hideTooltip()
    return
  }
  if (tooltipSlot.value == null) {
    return
  }
  showTooltip(slotIndex, event.clientX, event.clientY)
}

function onSlotFocus(event: FocusEvent, slotIndex: number): void {
  if (!slotVisible(props.assignments[slotIndex])) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  showTooltip(slotIndex, rect.left, rect.bottom)
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
        :aria-label="slotTooltip(slotIndex)"
        :aria-disabled="slotCoolingDown(slotIndex) || undefined"
        @dragover.prevent="hideTooltip"
        @dragstart="hideTooltip"
        @drop="onDrop($event, slotIndex)"
        @click="onActivate(slotIndex)"
        @contextmenu.prevent="emit('clear', slotIndex)"
        @pointerenter="onSlotPointerEnter($event, slotIndex)"
        @pointermove="onSlotPointerMove($event, slotIndex)"
        @pointerleave="hideTooltip"
        @focus="onSlotFocus($event, slotIndex)"
        @blur="hideTooltip"
        @pointerdown="onSlotPointerDown($event, slotIndex)"
        @pointerup="onSlotPointerUp"
        @pointercancel="onSlotPointerUp"
      >
        <span class="hotbar__slot-index">{{ slotIndex + 1 }}</span>
        <template v-if="slotVisible(assignments[slotIndex])">
          <ActionIcon class="hotbar__slot-icon" :action-id="assignments[slotIndex]!" />
          <span class="hotbar__slot-label">{{ presentation(assignments[slotIndex]!).shortLabel }}</span>
        </template>
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
        :aria-label="slotTooltip(slotIndex)"
        :aria-disabled="slotCoolingDown(slotIndex) || undefined"
        @dragover.prevent="hideTooltip"
        @dragstart="hideTooltip"
        @drop="onDrop($event, slotIndex)"
        @click="onActivate(slotIndex)"
        @contextmenu.prevent="emit('clear', slotIndex)"
        @pointerenter="onSlotPointerEnter($event, slotIndex)"
        @pointermove="onSlotPointerMove($event, slotIndex)"
        @pointerleave="hideTooltip"
        @focus="onSlotFocus($event, slotIndex)"
        @blur="hideTooltip"
        @pointerdown="onSlotPointerDown($event, slotIndex)"
        @pointerup="onSlotPointerUp"
        @pointercancel="onSlotPointerUp"
      >
        <span class="hotbar__slot-index">{{ slotIndex === 9 ? '0' : slotIndex + 1 }}</span>
        <template v-if="slotVisible(assignments[slotIndex])">
          <ActionIcon class="hotbar__slot-icon" :action-id="assignments[slotIndex]!" />
          <span class="hotbar__slot-label">{{ presentation(assignments[slotIndex]!).shortLabel }}</span>
        </template>
      </button>
    </div>
  </div>
  <ActionTooltip v-if="tooltipText" :text="tooltipText" :x="tooltipX" :y="tooltipY" />
</template>

<style scoped lang="scss">
.hotbar {
  position: relative;
  isolation: isolate;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: auto;
}

.hotbar::before {
  content: '';
  position: absolute;
  inset: -6px -10px;
  z-index: -1;
  background: url('/assets/img/hud/hotbar-background.svg') center / 100% 100% no-repeat;
  pointer-events: none;
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
  padding: 0;
  border: 1px solid rgba(110, 113, 92, 0.8);
  border-radius: 5px;
  background: #202b2b;
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
  left: 1px;
  top: 1px;
  z-index: 1;
  min-width: 11px;
  border-radius: 2px;
  background: rgba(22, 33, 34, 0.94);
  font: 10px/13px Arial, sans-serif;
  text-align: center;
  color: #efe0b5;
  pointer-events: none;
}

.hotbar__slot-label {
  display: none;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.hotbar__slot-icon {
  position: absolute;
  inset: 2px;
  width: 40px;
  height: 40px;
  display: block;
  margin: 0;
  pointer-events: none;
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
    border-radius: 5px;
  }

  .hotbar__slot-index,
  .hotbar__slot-label {
    font-size: 9px;
  }

  .hotbar__slot-icon {
    width: 34px;
    height: 34px;
  }
}

@media (orientation: landscape) and (pointer: coarse) {
  .hotbar::before {
    inset: -4px -6px;
  }

  .hotbar__group {
    gap: 4px;
  }

  .hotbar__gap {
    width: 10px;
  }

  .hotbar__slot {
    width: 34px;
    height: 34px;
    border-radius: 4px;
  }

  .hotbar__slot-index {
    font-size: 8px;
  }

  .hotbar__slot-label {
    font-size: 8px;
    letter-spacing: 0.02em;
  }

  .hotbar__slot-icon {
    width: 28px;
    height: 28px;
  }
}
</style>
