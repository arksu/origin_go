<script setup lang="ts">
import { ref } from 'vue'
import { ACTION_CATALOG, type ActionId } from '@/game/hud/actionCatalog'

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
}

function onPointerMove(event: PointerEvent): void {
  if (touchPointerId.value == null || touchPointerId.value !== event.pointerId || !touchActionId.value) {
    return
  }

  const deltaX = event.clientX - touchStartX.value
  const deltaY = event.clientY - touchStartY.value
  const movedEnough = Math.abs(deltaX) > 8 || Math.abs(deltaY) > 8

  if (!touchDragActive.value && movedEnough) {
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
      @pointerdown="onPointerDown($event, entry.id)"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp($event, entry.id)"
      @pointercancel="onPointerUp($event, entry.id)"
    >
      {{ entry.shortLabel }}
    </button>
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
    font-size: 9px;
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
    font-size: 8px;
    letter-spacing: 0.02em;
  }
}
</style>
