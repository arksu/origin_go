<script setup lang="ts">
import { computed } from 'vue'
import type { proto } from '@/network/proto/packets.js'
import { toNonNegativeProtoInt } from '@/utils/protoNumbers'
import GameWindow from './GameWindow.vue'
import AppButton from './AppButton.vue'

interface Props {
  entityId: number | null
  list: proto.IBuildStateItem[]
}

const props = defineProps<Props>()

const emit = defineEmits<{
  close: []
  progress: [entityId: number]
}>()

const rows = computed(() => props.list || [])
const canBuild = computed(() => Number.isFinite(props.entityId ?? NaN) && (props.entityId ?? 0) > 0)

function onClose() {
  emit('close')
}

function onBuild() {
  const entityId = Math.trunc(Number(props.entityId ?? 0))
  if (!Number.isFinite(entityId) || entityId <= 0) return
  emit('progress', entityId)
}

function prettifyKey(value: string | null | undefined): string {
  const normalized = (value || '').trim()
  if (!normalized) return '-'
  return normalized
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function buildRowLabel(row: proto.IBuildStateItem): string {
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

function rowIconStyle(row: proto.IBuildStateItem): { backgroundImage: string } {
  const url = rowIconUrl(row)
  return { backgroundImage: url ? `url(${url})` : 'none' }
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
</script>

<template>
  <GameWindow
    :id="7103"
    :inner-width="420"
    :inner-height="330"
    title="Build"
    @close="onClose"
  >
    <div class="build-state-window">
      <div class="build-state-window__list">
        <div
          v-for="(row, idx) in rows"
          :key="`${entityId || 0}-row-${idx}`"
          class="build-state-window__row"
        >
          <span class="build-state-window__icon-slot">
            <span
              class="build-state-window__icon"
              :style="rowIconStyle(row)"
            />
          </span>
          <span class="build-state-window__label">{{ buildRowLabel(row) }}</span>
          <span class="build-state-window__counts">
            {{ leftCount(row) }}/{{ asCount(row.putCount) }}/{{ asCount(row.buildCount) }}
          </span>
        </div>

        <div v-if="rows.length === 0" class="build-state-window__empty">
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
  grid-template-columns: 26px 1fr auto;
  align-items: center;
  gap: 8px;
  min-height: 30px;
  padding: 4px 6px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
}

.build-state-window__icon-slot {
  width: 24px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.18);
}

.build-state-window__icon {
  width: 20px;
  height: 20px;
  background-position: center;
  background-repeat: no-repeat;
  background-size: contain;
}

.build-state-window__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #dfe8ea;
  font-size: 13px;
}

.build-state-window__counts {
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
  justify-content: center;
  align-items: center;
}
</style>
