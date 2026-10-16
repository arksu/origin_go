<script setup lang="ts">
import { computed, ref } from 'vue'
import ActionIcon from './ActionIcon.vue'
import ActionTooltip from './ActionTooltip.vue'
import { useActionPresentation } from '@/composables/useActionPresentation'
import type { proto } from '@/network/proto/packets.js'
import { gameActionHotbarId, type HotbarActionId } from '@/game/hud/actionCatalog'

const props = defineProps<{
  actions: proto.IActionDefinition[]
  activeActionId: string
  activePhase: string
}>()

const emit = defineEmits<{
  activate: [actionId: string]
  dragStart: [actionId: HotbarActionId]
  dragEnd: []
  touchDragStart: [payload: { actionId: HotbarActionId; pointerId: number; clientX: number; clientY: number }]
  touchDragMove: [payload: { pointerId: number; clientX: number; clientY: number }]
  touchDragEnd: [payload: { pointerId: number; clientX: number; clientY: number }]
}>()

const { presentation } = useActionPresentation()

const touchPointerId = ref<number | null>(null)
const touchActionId = ref<HotbarActionId | null>(null)
const touchStartX = ref(0)
const touchStartY = ref(0)
const touchDragging = ref(false)
const suppressClick = ref(false)
const tooltipActionId = ref<string | null>(null)
const tooltipX = ref(0)
const tooltipY = ref(0)
const tooltipAction = computed(() => props.actions.find(action => action.id === tooltipActionId.value))
const tooltipText = computed(() => {
  const id = tooltipAction.value?.id
  if (!id) return ''
  const action = presentation(gameActionHotbarId(id))
  return action.available ? action.tooltip : ''
})

function showTooltip(action: proto.IActionDefinition, clientX: number, clientY: number): void {
  tooltipActionId.value = action.id || null
  tooltipX.value = clientX + 10
  tooltipY.value = clientY + 10
}

function hideTooltip(): void {
  tooltipActionId.value = null
}

function onPointerEnter(event: PointerEvent, action: proto.IActionDefinition): void {
  if (event.pointerType !== 'touch') showTooltip(action, event.clientX, event.clientY)
}

function onFocus(event: FocusEvent, action: proto.IActionDefinition): void {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  showTooltip(action, rect.left, rect.bottom)
}

function onClick(action: proto.IActionDefinition): void {
  if (suppressClick.value) {
    suppressClick.value = false
    return
  }
  if (action.id && presentation(gameActionHotbarId(action.id)).available) emit('activate', action.id)
}

function onDragStart(event: DragEvent, action: proto.IActionDefinition): void {
  hideTooltip()
  const id = gameActionHotbarId(action.id || '')
  event.dataTransfer?.setData('application/x-origin-action-id', id)
  event.dataTransfer?.setData('text/plain', id)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copy'
  emit('dragStart', id)
}

function onPointerDown(event: PointerEvent, action: proto.IActionDefinition): void {
  if (event.pointerType !== 'touch') return
  showTooltip(action, event.clientX, event.clientY)
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  touchPointerId.value = event.pointerId
  touchActionId.value = gameActionHotbarId(action.id || '')
  touchStartX.value = event.clientX
  touchStartY.value = event.clientY
  touchDragging.value = false
}

function onPointerMove(event: PointerEvent): void {
  if (touchPointerId.value !== event.pointerId || !touchActionId.value) {
    if (tooltipActionId.value && event.pointerType !== 'touch') {
      tooltipX.value = event.clientX + 10
      tooltipY.value = event.clientY + 10
    }
    return
  }
  if (!touchDragging.value && Math.hypot(event.clientX - touchStartX.value, event.clientY - touchStartY.value) > 8) {
    touchDragging.value = true
    hideTooltip()
    emit('dragStart', touchActionId.value)
    emit('touchDragStart', { actionId: touchActionId.value, pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY })
  }
  if (touchDragging.value) emit('touchDragMove', { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY })
}

function onPointerUp(event: PointerEvent): void {
  if (touchPointerId.value !== event.pointerId) return
  hideTooltip()
  if (touchDragging.value) {
    suppressClick.value = true
    setTimeout(() => { suppressClick.value = false }, 0)
    emit('touchDragEnd', { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY })
    emit('dragEnd')
  }
  touchPointerId.value = null
  touchActionId.value = null
  touchDragging.value = false
}
</script>

<template>
  <div class="actions-menu" data-actions-menu role="group" aria-label="Game actions">
    <button
      v-for="action in actions"
      :key="action.id || ''"
      type="button"
      class="actions-menu__action"
      :class="{ 'actions-menu__action--active': action.id === activeActionId && activePhase !== 'idle' }"
      :aria-label="presentation(gameActionHotbarId(action.id || '')).label"
      :aria-disabled="presentation(gameActionHotbarId(action.id || '')).coolingDown || undefined"
      :aria-pressed="action.id === activeActionId && activePhase !== 'idle'"
      draggable="true"
      @click="onClick(action)"
      @dragstart="onDragStart($event, action)"
      @dragend="emit('dragEnd')"
      @pointerenter="onPointerEnter($event, action)"
      @pointerleave="hideTooltip"
      @focus="onFocus($event, action)"
      @blur="hideTooltip"
      @pointerdown="onPointerDown($event, action)"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
    >
      <ActionIcon :action-id="gameActionHotbarId(action.id || '')" />
      <span>{{ presentation(gameActionHotbarId(action.id || '')).label }}</span>
    </button>
  </div>
  <ActionTooltip v-if="tooltipText" :text="tooltipText" :x="tooltipX" :y="tooltipY" />
</template>

<style scoped lang="scss">
.actions-menu {
  position: absolute;
  left: calc(100% + 8px);
  top: 0;
  z-index: 50;
  display: flex;
  flex-wrap: nowrap;
  gap: 6px;
  padding: 6px;
  max-width: calc(100vw - 75px);
  overflow-x: auto;
  border: 1px solid rgba(217, 199, 155, 0.8);
  border-radius: 6px;
  background: rgba(21, 30, 31, 0.96);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.actions-menu__action {
  flex: 0 0 88px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 3px;
  border: 1px solid rgba(110, 113, 92, 0.8);
  border-radius: 4px;
  background: #202b2b;
  color: #eef3f8;
  cursor: pointer;
  font: 10px/1.2 Arial, sans-serif;
  touch-action: none;
}


.actions-menu__action--active {
  border-color: #55c4ff;
  box-shadow: inset 0 0 0 1px #55c4ff;
}
</style>
