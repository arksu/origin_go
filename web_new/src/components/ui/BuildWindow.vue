<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import type { proto } from '@/network/proto/packets.js'
import objectVisualDefs from '@/game/objects'
import GameWindow from './GameWindow.vue'
import AppButton from './AppButton.vue'

const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()
const searchQuery = ref('')

const builds = computed(() => gameStore.buildRecipes)
const selectedBuildKey = computed(() => gameStore.selectedBuildKey)
const armedBuildKey = computed(() => gameStore.armedBuildKey)

const TILE_NAMES: Record<number, string> = {
  1: 'Water Deep',
  3: 'Water',
  10: 'Stone',
  11: 'Plowed',
  13: 'Pine Forest',
  15: 'Leaf Forest',
  17: 'Grass',
  23: 'Swamp',
  29: 'Clay',
  30: 'Dirt',
  32: 'Sand',
  42: 'Cave',
}

const filteredBuilds = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return builds.value
  return builds.value.filter((build) => {
    const name = (build.name || '').toLowerCase()
    const key = (build.buildKey || '').toLowerCase()
    const objectKey = (build.objectKey || '').toLowerCase()
    return name.includes(query) || key.includes(query) || objectKey.includes(query)
  })
})

const selectedBuild = computed<proto.IBuildRecipeEntry | null>(() => {
  const selected = filteredBuilds.value.find((build) => (build.buildKey || '') === selectedBuildKey.value)
  if (selected) return selected
  return filteredBuilds.value[0] || null
})

const tooltipVisible = ref(false)
const tooltipX = ref(0)
const tooltipY = ref(0)
let tooltipElement: HTMLDivElement | null = null

watch(
  selectedBuild,
  (build) => {
    const key = build?.buildKey || ''
    if (key !== gameStore.selectedBuildKey) {
      gameStore.selectBuildRecipe(key)
    }
  },
  { immediate: true },
)

const isSelectedArmed = computed(() => {
  const selectedKey = (selectedBuild.value?.buildKey || '').trim()
  return !!selectedKey && selectedKey === armedBuildKey.value
})

function onClose() {
  emit('close')
}

function selectBuild(buildKey: string) {
  gameStore.selectBuildRecipe(buildKey)
}

function onBuild() {
  const buildKey = (selectedBuild.value?.buildKey || '').trim()
  if (!buildKey) return
  gameStore.armBuildPlacement(buildKey)
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

function buildInputTooltipLabel(entry: { itemKey?: string | null; itemTag?: string | null }): string {
  const itemKey = (entry.itemKey || '').trim()
  if (itemKey) return prettifyKey(itemKey)
  const itemTag = (entry.itemTag || '').trim()
  return `Any ${prettifyKey(itemTag)}`
}

function buildInputCountLabel(entry: { count?: number | null }): string {
  const count = Math.max(1, Number(entry.count || 0))
  return `x${count}`
}

function itemResourceUrl(resource: string | null | undefined): string {
  const normalized = (resource || '').trim()
  if (!normalized) return ''
  return `/assets/game/${normalized}`
}

function tileLabel(tileID: number | null | undefined): string {
  const value = Number(tileID ?? 0)
  return TILE_NAMES[value] || `Tile ${value}`
}

type LayerDefLike = { img?: string; shadow?: boolean }
type VariantDefLike = { layers?: LayerDefLike[] }
type ObjectVisualDefsLike = Record<string, Record<string, VariantDefLike>>

function objectIconUrl(objectKey: string | null | undefined): string {
  const key = (objectKey || '').trim()
  if (!key) return ''
  const allDefs = objectVisualDefs as unknown as ObjectVisualDefsLike
  const objDef = allDefs[key]
  if (!objDef || typeof objDef !== 'object') return ''

  for (const variant of Object.values(objDef)) {
    if (!variant || !Array.isArray(variant.layers)) continue
    const preferred = variant.layers.find((layer) => layer?.img && !layer.shadow) || variant.layers.find((layer) => layer?.img)
    if (preferred?.img) {
      return `/assets/game/${preferred.img}`
    }
  }
  return ''
}

function firstResultIconForBuild(build: proto.IBuildRecipeEntry | null | undefined): string {
  return objectIconUrl(build?.objectKey)
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

function onInputMouseEnter(input: proto.IBuildInputDef, event: MouseEvent) {
  tooltipVisible.value = true
  tooltipX.value = event.clientX + 10
  tooltipY.value = event.clientY + 10
  createTooltip(buildInputTooltipLabel(input))
  updateTooltipPosition()
}

function onInputMouseMove(event: MouseEvent) {
  if (!tooltipVisible.value) return
  tooltipX.value = event.clientX + 10
  tooltipY.value = event.clientY + 10
  updateTooltipPosition()
}

function onInputMouseLeave() {
  tooltipVisible.value = false
  removeTooltip()
}

onUnmounted(() => {
  removeTooltip()
})
</script>

<template>
  <GameWindow
    :id="7102"
    :inner-width="490"
    :inner-height="360"
    title="Build"
    @close="onClose"
  >
    <div class="build-window">
      <div class="build-window__search-row">
        <input
          v-model="searchQuery"
          type="text"
          class="build-window__search"
          placeholder="Search builds..."
          autocomplete="off"
        >
      </div>

      <div class="build-window__body">
        <section class="build-window__panel build-window__recipes">
          <div class="build-window__panel-title">Builds</div>
          <div class="build-window__recipe-list">
            <button
              v-for="(build, idx) in filteredBuilds"
              :key="build.buildKey || build.name || idx"
              type="button"
              class="build-window__recipe-row"
              :class="{ 'is-selected': (build.buildKey || '') === (selectedBuild?.buildKey || '') }"
              @click="selectBuild(build.buildKey || '')"
            >
              <span class="build-window__icon-slot build-window__icon-slot--recipe">
                <img
                  v-if="firstResultIconForBuild(build)"
                  class="build-window__icon"
                  :src="firstResultIconForBuild(build)"
                  alt=""
                  draggable="false"
                >
              </span>
              <span class="build-window__recipe-name">{{ build.name || prettifyKey(build.buildKey) }}</span>
            </button>

            <div v-if="filteredBuilds.length === 0" class="build-window__empty">
              No builds
            </div>
          </div>
        </section>

        <section class="build-window__panel build-window__details">
          <template v-if="selectedBuild">
            <div class="build-window__panel-title">{{ selectedBuild.name || prettifyKey(selectedBuild.buildKey) }}</div>

            <div class="build-window__section">
              <div class="build-window__section-title">Inputs</div>
              <div class="build-window__chips">
                <div
                  v-for="(input, idx) in selectedBuild.inputs || []"
                  :key="`${selectedBuild.buildKey || 'build'}-in-${idx}`"
                  class="build-window__chip"
                  @mouseenter="onInputMouseEnter(input, $event)"
                  @mousemove="onInputMouseMove"
                  @mouseleave="onInputMouseLeave"
                >
                  <span class="build-window__icon-slot">
                    <img
                      v-if="itemResourceUrl(input.resource)"
                      class="build-window__icon"
                      :src="itemResourceUrl(input.resource)"
                      alt=""
                      draggable="false"
                    >
                  </span>
                  {{ buildInputCountLabel(input) }}
                </div>
              </div>
            </div>

            <div class="build-window__section">
              <div class="build-window__section-title">Placement Rules</div>
              <div class="build-window__chips">
                <template v-if="(selectedBuild.allowedTiles?.length || 0) > 0">
                  <div
                    v-for="(tileID, idx) in selectedBuild.allowedTiles || []"
                    :key="`${selectedBuild.buildKey || 'build'}-allow-${idx}`"
                    class="build-window__chip build-window__chip--rule-allow"
                  >
                    {{ tileLabel(tileID as number) }}
                  </div>
                </template>
                <template v-else-if="(selectedBuild.disallowedTiles?.length || 0) > 0">
                  <div
                    v-for="(tileID, idx) in selectedBuild.disallowedTiles || []"
                    :key="`${selectedBuild.buildKey || 'build'}-deny-${idx}`"
                    class="build-window__chip build-window__chip--rule-deny"
                  >
                    {{ tileLabel(tileID as number) }}
                  </div>
                </template>
                <div v-else class="build-window__chip build-window__chip--rule-any">
                  Any tile
                </div>
              </div>
            </div>

            <div class="build-window__section">
              <div class="build-window__section-title">Result</div>
              <div class="build-window__chips">
                <div class="build-window__chip build-window__chip--result">
                  <span class="build-window__icon-slot">
                    <img
                      v-if="objectIconUrl(selectedBuild.objectKey)"
                      class="build-window__icon"
                      :src="objectIconUrl(selectedBuild.objectKey)"
                      alt=""
                      draggable="false"
                    >
                  </span>
                  {{ prettifyKey(selectedBuild.objectKey) }}
                </div>
              </div>
            </div>
          </template>

          <div v-else class="build-window__empty build-window__empty--details">
            Select a build
          </div>
        </section>
      </div>

      <div class="build-window__actions">
        <AppButton
          size="sm"
          :variant="isSelectedArmed ? 'primary' : 'secondary'"
          :disabled="!selectedBuild"
          @click="onBuild"
        >
          Build
        </AppButton>
      </div>
    </div>
  </GameWindow>
</template>

<style scoped lang="scss">
.build-window {
  width: 100%;
  height: 100%;
  display: grid;
  grid-template-rows: auto 1fr auto;
  gap: 10px;
  color: #dbe5ea;
  text-align: left;
}

.build-window__search-row {
  display: flex;
}

.build-window__search {
  width: 100%;
  height: 30px;
  padding: 6px 10px;
  border-radius: 7px;
  border: 1px solid rgba(183, 204, 216, 0.28);
  background: rgba(247, 250, 252, 0.05);
  color: #e7f0f5;
  font-size: 13px;
  outline: none;

  &::placeholder {
    color: rgba(215, 228, 235, 0.65);
  }

  &:focus {
    border-color: rgba(107, 188, 220, 0.65);
    box-shadow: 0 0 0 1px rgba(77, 160, 194, 0.18);
  }
}

.build-window__body {
  min-height: 0;
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 10px;
}

.build-window__panel {
  min-height: 0;
  border: 1px solid rgba(196, 214, 224, 0.16);
  border-radius: 8px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.035), rgba(0, 0, 0, 0.05)),
    rgba(18, 33, 41, 0.42);
  padding: 10px;
}

.build-window__panel-title {
  margin-bottom: 8px;
  color: #eef7fb;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.2px;
}

.build-window__recipes {
  display: flex;
  flex-direction: column;
}

.build-window__recipe-list {
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  overflow-y: auto;
  padding-right: 2px;
}

.build-window__recipe-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  text-align: left;
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: rgba(246, 249, 251, 0.03);
  color: #d9e4ea;
  cursor: pointer;
  transition: background-color 0.12s ease, border-color 0.12s ease;

  &:hover {
    background: rgba(255, 255, 255, 0.07);
  }

  &.is-selected {
    background: rgba(40, 122, 95, 0.22);
    border-color: rgba(69, 186, 145, 0.45);
    color: #f1fbf8;
  }
}

.build-window__recipe-name {
  font-size: 13px;
}

.build-window__details {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.build-window__section {
  margin-bottom: 12px;
}

.build-window__section:last-child {
  margin-bottom: 0;
}

.build-window__section-title {
  margin-bottom: 6px;
  color: #c5d5dd;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.8px;
}

.build-window__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.build-window__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 28px;
  padding: 4px 9px;
  border-radius: 6px;
  border: 1px solid rgba(190, 208, 218, 0.16);
  background: rgba(249, 252, 253, 0.04);
  color: #e4edf2;
  font-size: 12px;
  line-height: 1.2;
}

.build-window__chip--result {
  border-color: rgba(104, 176, 149, 0.24);
  background: rgba(48, 106, 88, 0.11);
}

.build-window__chip--rule-allow {
  border-color: rgba(104, 176, 149, 0.24);
  background: rgba(48, 106, 88, 0.11);
}

.build-window__chip--rule-deny {
  border-color: rgba(184, 116, 116, 0.24);
  background: rgba(118, 55, 55, 0.12);
}

.build-window__chip--rule-any {
  border-color: rgba(120, 162, 205, 0.26);
  background: rgba(45, 72, 100, 0.13);
}

.build-window__icon-slot {
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.build-window__icon-slot--recipe {
  width: 32px;
  height: 32px;
  flex-basis: 32px;
}

.build-window__icon {
  display: block;
  width: auto;
  height: auto;
  max-width: 31px;
  max-height: 31px;
  object-fit: contain;
  image-rendering: pixelated;
}

.build-window__empty {
  padding: 8px 4px;
  color: rgba(219, 229, 234, 0.62);
  font-size: 13px;
}

.build-window__empty--details {
  margin-top: 24px;
}

.build-window__actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 10px;
  padding-top: 4px;
}

.build-window__actions :deep(.app-button) {
  min-width: 130px;
}

@media (max-width: 900px) {
  .build-window__body {
    grid-template-columns: 1fr;
    grid-template-rows: 120px 1fr;
  }

  .build-window__recipe-list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    align-content: start;
  }
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
