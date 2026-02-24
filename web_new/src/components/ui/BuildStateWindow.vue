<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import type { proto } from '@/network/proto/packets.js'
import { toNonNegativeProtoInt } from '@/utils/protoNumbers'
import GameWindow from './GameWindow.vue'
import AppButton from './AppButton.vue'

interface Props {
  entityId: number | null
  title?: string
  list: proto.IBuildStateItem[]
  handHasItem?: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  close: []
  progress: [entityId: number]
  putFromHand: [entityId: number]
  takeBack: [entityId: number, slot: number]
}>()

const WINDOW_INNER_WIDTH = 210
const ROW_HEIGHT_PX = 40
const ROW_GAP_PX = 6
const LIST_VERTICAL_CHROME_PX = 10 // padding + border
const PANEL_GAP_PX = 10
const ACTIONS_HEIGHT_PX = 34

const rows = computed(() => props.list || [])
const displayRows = computed(() => rows.value.map((row, idx) => ({
  row,
  idx,
  iconUrl: rowIconUrl(row),
})))
const canBuild = computed(() => Number.isFinite(props.entityId ?? NaN) && (props.entityId ?? 0) > 0)
const windowTitle = computed(() => {
  const normalized = (props.title || '').trim()
  return normalized || 'Build'
})
const windowInnerHeight = computed(() => {
  const rowCount = Math.max(rows.value.length, 1)
  const listHeight = LIST_VERTICAL_CHROME_PX + (rowCount * ROW_HEIGHT_PX) + (Math.max(rowCount - 1, 0) * ROW_GAP_PX)
  return listHeight + PANEL_GAP_PX + ACTIONS_HEIGHT_PX
})

const tooltipVisible = ref(false)
const tooltipX = ref(0)
const tooltipY = ref(0)
let tooltipElement: HTMLDivElement | null = null

function onClose() {
  emit('close')
}

function onBuild() {
  const entityId = Math.trunc(Number(props.entityId ?? 0))
  if (!Number.isFinite(entityId) || entityId <= 0) return
  emit('progress', entityId)
}

function emitPutFromHand() {
  const entityId = Math.trunc(Number(props.entityId ?? 0))
  if (!Number.isFinite(entityId) || entityId <= 0) return
  emit('putFromHand', entityId)
}

function onListClick() {
  if (!props.handHasItem) return
  emitPutFromHand()
}

function onRowClick(slot: number) {
  const entityId = Math.trunc(Number(props.entityId ?? 0))
  if (!Number.isFinite(entityId) || entityId <= 0) return

  if (props.handHasItem) {
    emit('putFromHand', entityId)
    return
  }

  const slotIndex = Math.trunc(slot)
  if (!Number.isFinite(slotIndex) || slotIndex < 0) return
  emit('takeBack', entityId, slotIndex)
}

function prettifyKey(value: string | null | undefined): string {
  const normalized = (value || '').trim()
  if (!normalized) return 'Unknown'
  return normalized
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function rowTooltipText(row: proto.IBuildStateItem): string {
  const itemKey = (row.itemKey || '').trim()
  if (itemKey) return prettifyKey(itemKey)
  const itemTag = (row.itemTag || '').trim()
  if (itemTag) return `Any ${prettifyKey(itemTag)}`
  return 'Unknown'
}

function primaryResourceUrl(resource: string | null | undefined): string {
  const normalized = (resource || '').trim()
  if (!normalized) return ''
  return `/assets/game/${normalized}`
}

function fallbackIconName(row: proto.IBuildStateItem): string {
  return (row.itemKey || row.itemTag || '').trim()
}

function fallbackIconUrl(iconName: string): string {
  const normalized = iconName.trim()
  if (!normalized) return ''
  return `/assets/game/items/${normalized}.png`
}

function rowIconUrl(row: proto.IBuildStateItem): string {
  const primary = primaryResourceUrl(row.resource)
  if (primary) return primary
  return fallbackIconUrl(fallbackIconName(row))
}

function asCount(value: unknown): number {
  return toNonNegativeProtoInt(value)
}

function leftCount(row: proto.IBuildStateItem): number {
  const required = asCount(row.requiredCount)
  const put = asCount(row.putCount)
  const built = asCount(row.buildCount)
  return Math.max(required - put - built, 0)
}

function createTooltip(text: string) {
  if (tooltipElement) return
  tooltipElement = document.createElement('div')
  tooltipElement.className = 'build-item-tooltip-global'
  tooltipElement.innerHTML = `<pre>${text}</pre>`
  document.body.appendChild(tooltipElement)
}

function removeTooltip() {
  if (!tooltipElement) return
  document.body.removeChild(tooltipElement)
  tooltipElement = null
}

function updateTooltipPosition() {
  if (!tooltipElement) return
  tooltipElement.style.left = `${tooltipX.value}px`
  tooltipElement.style.top = `${tooltipY.value}px`
}

function onRowMouseEnter(row: proto.IBuildStateItem, event: MouseEvent) {
  tooltipVisible.value = true
  tooltipX.value = event.clientX + 10
  tooltipY.value = event.clientY + 10
  createTooltip(rowTooltipText(row))
  updateTooltipPosition()
}

function onRowMouseMove(event: MouseEvent) {
  if (!tooltipVisible.value) return
  tooltipX.value = event.clientX + 10
  tooltipY.value = event.clientY + 10
  updateTooltipPosition()
}

function onRowMouseLeave() {
  tooltipVisible.value = false
  removeTooltip()
}

onUnmounted(() => {
  removeTooltip()
})
</script>

<template>
  <GameWindow
    :id="7103"
    :inner-width="WINDOW_INNER_WIDTH"
    :inner-height="windowInnerHeight"
    :title="windowTitle"
    @close="onClose"
  >
    <div class="build-state-window">
      <div class="build-state-window__list" @click="onListClick">
        <div
          v-for="entry in displayRows"
          :key="`${entityId || 0}-row-${entry.idx}`"
          class="build-state-window__row"
          @click.stop="onRowClick(entry.idx)"
          @mouseenter="onRowMouseEnter(entry.row, $event)"
          @mousemove="onRowMouseMove"
          @mouseleave="onRowMouseLeave"
        >
          <span class="build-state-window__icon-slot">
            <img
              v-if="entry.iconUrl"
              class="build-state-window__icon"
              :src="entry.iconUrl"
              alt=""
              draggable="false"
            />
          </span>
          <span class="build-state-window__counts">
            {{ leftCount(entry.row) }}/{{ asCount(entry.row.putCount) }}/{{ asCount(entry.row.buildCount) }}
          </span>
        </div>

        <div v-if="displayRows.length === 0" class="build-state-window__empty">
          No requirements
        </div>
      </div>

      <div class="build-state-window__actions">
        <AppButton size="sm" :disabled="!canBuild" @click="onBuild">
          Build
        </AppButton>
      </div>
    </div>
  </GameWindow>
</template>

<style scoped lang="scss">
.build-state-window {
  width: 100%;
  height: 100%;
  display: grid;
  grid-template-rows: 1fr auto;
  gap: 10px;
  color: #dbe5ea;
}

.build-state-window__list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 4px;
  border: 1px solid rgba(180, 210, 220, 0.18);
  background: rgba(8, 20, 24, 0.35);
  border-radius: 8px;
  overflow: auto;
}

.build-state-window__row {
  display: grid;
  grid-template-columns: 36px 1fr;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 4px 6px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
}

.build-state-window__icon-slot {
  width: 36px;
  height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.18);
}

.build-state-window__icon {
  display: block;
  width: auto;
  height: auto;
  max-width: 32px;
  max-height: 32px;
  object-fit: contain;
  image-rendering: pixelated;
}

.build-state-window__counts {
  justify-self: center;
  text-align: center;
  color: #b8d7de;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.3px;
}

.build-state-window__empty {
  margin: auto 0;
  text-align: center;
  color: rgba(219, 229, 234, 0.65);
  font-size: 13px;
}

.build-state-window__actions {
  display: flex;
  min-height: 34px;
  justify-content: center;
  align-items: center;
}
</style>

<style lang="scss">
.build-item-tooltip-global {
  position: fixed;
  background: rgba(0, 0, 0, 0.7);
  color: #ffffff;
  border: 2px solid #555;
  border-radius: 8px;
  padding: 3px 6px;
  font-size: 12px;
  white-space: pre-wrap;
  z-index: 999999;
  pointer-events: none;
  max-width: 300px;
  word-wrap: break-word;
}
</style>
