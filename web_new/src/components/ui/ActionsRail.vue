<script setup lang="ts">
import { ref } from 'vue'
import { ACTION_CATALOG, getActionLabel, type ActionId } from '@/game/hud/actionCatalog'

const emit = defineEmits<{
  activate: [actionId: ActionId]
  dragStart: [actionId: ActionId]
  dragEnd: []
  touchDragStart: [payload: { actionId: ActionId; pointerId: number; clientX: number; clientY: number }]
  touchDragMove: [payload: { pointerId: number; clientX: number; clientY: number }]
  touchDragEnd: [payload: { pointerId: number; clientX: number; clientY: number }]
}>()

const touchActionId = ref<ActionId | null>(null)
const touchPointerId = ref<number | null>(null)
const touchDragActive = ref(false)
const touchStartX = ref(0)
const touchStartY = ref(0)
const tooltipText = ref('')
const tooltipX = ref(0)
const tooltipY = ref(0)
const tooltipVisible = ref(false)
const touchTooltipTimer = ref<number | null>(null)

function clearTouchTooltipTimer(): void {
  if (touchTooltipTimer.value != null) {
    window.clearTimeout(touchTooltipTimer.value)
    touchTooltipTimer.value = null
  }
}

function showTooltip(text: string, clientX: number, clientY: number): void {
  tooltipText.value = text
  tooltipX.value = clientX + 10
  tooltipY.value = clientY - 10
  tooltipVisible.value = true
}

function hideTooltip(): void {
  tooltipVisible.value = false
}

function onButtonClick(actionId: ActionId): void {
  if (touchDragActive.value) {
    return
  }
  emit('activate', actionId)
}

function onDragStart(event: DragEvent, actionId: ActionId): void {
  event.dataTransfer?.setData('text/plain', actionId)
  event.dataTransfer?.setData('application/x-origin-action-id', actionId)
  event.dataTransfer!.effectAllowed = 'copy'
  hideTooltip()
  emit('dragStart', actionId)
}

function onDragEnd(): void {
  emit('dragEnd')
}

function onPointerDown(event: PointerEvent, actionId: ActionId): void {
  if (event.pointerType !== 'touch') {
    return
  }
  const target = event.currentTarget as HTMLElement | null
  target?.setPointerCapture(event.pointerId)
  touchActionId.value = actionId
  touchPointerId.value = event.pointerId
  touchDragActive.value = false
  touchStartX.value = event.clientX
  touchStartY.value = event.clientY
  clearTouchTooltipTimer()
  touchTooltipTimer.value = window.setTimeout(() => {
    showTooltip(getActionLabel(actionId), event.clientX, event.clientY)
  }, 350)
}

function onPointerMove(event: PointerEvent, actionLabel: string): void {
  if (event.pointerType !== 'touch') {
    if (tooltipVisible.value) {
      showTooltip(actionLabel, event.clientX, event.clientY)
    }
    return
  }

  if (touchPointerId.value == null || touchPointerId.value !== event.pointerId || !touchActionId.value) {
    return
  }

  const deltaX = event.clientX - touchStartX.value
  const deltaY = event.clientY - touchStartY.value
  const movedEnough = Math.abs(deltaX) > 8 || Math.abs(deltaY) > 8

  if (!touchDragActive.value && movedEnough) {
    clearTouchTooltipTimer()
    hideTooltip()
    touchDragActive.value = true
    emit('dragStart', touchActionId.value)
    emit('touchDragStart', {
      actionId: touchActionId.value,
      pointerId: event.pointerId,
      clientX: event.clientX,
      clientY: event.clientY,
    })
  }

  if (touchDragActive.value) {
    emit('touchDragMove', {
      pointerId: event.pointerId,
      clientX: event.clientX,
      clientY: event.clientY,
    })
  }
}

function onPointerUp(event: PointerEvent, actionId: ActionId): void {
  if (touchPointerId.value == null || touchPointerId.value !== event.pointerId) {
    return
  }
  const target = event.currentTarget as HTMLElement | null
  if (target?.hasPointerCapture(event.pointerId)) {
    target.releasePointerCapture(event.pointerId)
  }

  if (touchDragActive.value) {
    emit('touchDragEnd', {
      pointerId: event.pointerId,
      clientX: event.clientX,
      clientY: event.clientY,
    })
    emit('dragEnd')
  } else {
    emit('activate', actionId)
  }

  touchActionId.value = null
  touchPointerId.value = null
  touchDragActive.value = false
  clearTouchTooltipTimer()
  hideTooltip()
}

function onPointerEnter(event: PointerEvent, actionLabel: string): void {
  if (event.pointerType === 'touch') {
    return
  }
  showTooltip(actionLabel, event.clientX, event.clientY)
}

function onPointerLeave(): void {
  clearTouchTooltipTimer()
  hideTooltip()
}
</script>

<template>
  <nav class="actions-rail" aria-label="Actions rail">
    <button
      v-for="entry in ACTION_CATALOG"
      :key="entry.id"
      class="actions-rail__button"
      type="button"
      draggable="true"
      :aria-label="entry.label"
      @click="onButtonClick(entry.id)"
      @dragstart="onDragStart($event, entry.id)"
      @dragend="onDragEnd"
      @pointerenter="onPointerEnter($event, entry.label)"
      @pointerleave="onPointerLeave"
      @pointerdown="onPointerDown($event, entry.id)"
      @pointermove="onPointerMove($event, entry.label)"
      @pointerup="onPointerUp($event, entry.id)"
      @pointercancel="onPointerUp($event, entry.id)"
    >
      <img class="actions-rail__icon" :src="entry.iconPath" :alt="entry.label" draggable="false">
      <span class="actions-rail__fallback">{{ entry.shortLabel }}</span>
    </button>
    <div
      v-if="tooltipVisible"
      class="actions-rail__tooltip"
      :style="{ left: `${tooltipX}px`, top: `${tooltipY}px` }"
    >
      {{ tooltipText }}
    </div>
  </nav>
</template>

<style scoped lang="scss">
.actions-rail {
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: auto;
}

.actions-rail__button {
  position: relative;
  width: 48px;
  height: 48px;
  border: 1px solid rgba(217, 199, 155, 0.72);
  border-radius: 10px;
  background: rgba(26, 35, 41, 0.84);
  color: #e8ecf1;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  cursor: pointer;
  user-select: none;
}

.actions-rail__icon {
  width: 22px;
  height: 22px;
  display: block;
  margin: 0 auto;
  pointer-events: none;
}

.actions-rail__fallback {
  position: absolute;
  inset: 0;
  display: none;
  align-items: center;
  justify-content: center;
}

.actions-rail__tooltip {
  position: fixed;
  transform: translateY(-100%);
  padding: 4px 8px;
  border: 1px solid rgba(217, 199, 155, 0.8);
  border-radius: 6px;
  background: rgba(11, 16, 22, 0.94);
  color: #e8ecf1;
  font-size: 11px;
  line-height: 1.2;
  white-space: nowrap;
  pointer-events: none;
  z-index: 1000;
}

.actions-rail__button:active {
  transform: translateY(1px);
}

@media (max-width: 900px) {
  .actions-rail {
    gap: 6px;
  }

  .actions-rail__button {
    width: 42px;
    height: 42px;
    border-radius: 9px;
    font-size: 8px;
  }

  .actions-rail__icon {
    width: 18px;
    height: 18px;
  }
}

@media (orientation: landscape) and (pointer: coarse) {
  .actions-rail {
    gap: 4px;
  }

  .actions-rail__button {
    width: 34px;
    height: 34px;
    border-radius: 8px;
    font-size: 7px;
    letter-spacing: 0.02em;
  }

  .actions-rail__icon {
    width: 15px;
    height: 15px;
  }
}
</style>
