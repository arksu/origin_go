<template>
  <div class="panel-wrap">
    <h3>Object / Layers</h3>

    <div v-if="!store.selectedResource" class="empty">
      Select a resource node (`layers`) in the left tree.
    </div>

    <template v-else>
      <div class="section">
        <div class="row">
          <label>Root Offset X</label>
          <input type="number" :value="rootOffset[0]" @input="onRootOffsetInput(0, $event)" />
        </div>
        <div class="row">
          <label>Root Offset Y</label>
          <input type="number" :value="rootOffset[1]" @input="onRootOffsetInput(1, $event)" />
        </div>
      </div>

      <div class="section">
        <div class="section-head">
          <span>Layers ({{ store.selectedResource.layers.length }})</span>
          <div class="buttons">
            <button
              class="small-btn"
              :class="{ accent: !store.selectedResourceHasShadowLayer }"
              @click="store.addShadowLayerToSelectedResource"
            >
              {{ store.selectedResourceHasShadowLayer ? 'Select Shadow' : 'Add Shadow Layer' }}
            </button>
            <button class="small-btn" @click="store.moveSelectedLayerUp">Up</button>
            <button class="small-btn" @click="store.moveSelectedLayerDown">Down</button>
          </div>
        </div>
        <ul class="layer-list">
          <li
            v-for="(layer, idx) in store.selectedResource.layers"
            :key="idx"
            :class="{ active: idx === store.selectedLayerIndex }"
            @click="store.selectLayer(idx)"
          >
            <div class="layer-title">
              <span>[{{ idx }}]</span>
              <span>{{ layerKind(layer) }}</span>
              <span v-if="layer.shadow" class="tag shadow">shadow</span>
            </div>
            <div class="layer-meta">{{ layerPathLabel(layer) }}</div>
            <div v-if="Array.isArray(layer.frames)" class="frame-tree" @click.stop>
              <button class="small-btn frame-toggle" @click="toggleFrames(idx)">
                {{ isFramesExpanded(idx) ? 'Collapse' : 'Expand' }}
              </button>
              <ul v-if="isFramesExpanded(idx)" class="frame-list">
                <li
                  v-for="(frame, frameIdx) in layer.frames"
                  :key="frameIdx"
                  class="frame-item"
                  :class="{ active: idx === store.selectedLayerIndex && frameIdx === store.selectedLayerFrameIndex }"
                  draggable="true"
                  @click="selectFrameLayer(idx, frameIdx)"
                  @dragstart="onFrameDragStart(frameIdx, $event)"
                  @dragover.prevent
                  @drop="onFrameDrop(idx, frameIdx)"
                  @dragend="draggedFrameIndex = null"
                >
                  <span class="drag-handle">☷</span>
                  <span>[{{ frameIdx }}]</span>
                  <img :src="`/assets/game/${frame.img}`" :alt="frame.img" />
                  <span class="frame-name">{{ frame.img }}</span>
                </li>
              </ul>
            </div>
          </li>
        </ul>
      </div>

      <div v-if="store.selectedLayer" class="section">
        <h4>Selected Layer</h4>
        <div class="row">
          <label>Kind</label>
          <span class="readonly">{{ layerKind(store.selectedLayer) }}</span>
        </div>

        <template v-if="!store.selectedLayer.spine">
          <div class="row">
            <label>Offset X</label>
            <input type="number" :value="layerOffset[0]" @input="onLayerOffsetInput(0, $event)" />
          </div>
          <div class="row">
            <label>Offset Y</label>
            <input type="number" :value="layerOffset[1]" @input="onLayerOffsetInput(1, $event)" />
          </div>
        </template>

        <div class="row" v-if="store.selectedLayer.z != null || store.isSelectedLayerEditableImage()">
          <label>Z</label>
          <input
            type="number"
            :value="store.selectedLayer.z ?? 0"
            @input="onZInput"
          />
        </div>

        <template v-if="typeof store.selectedLayer.img === 'string'">
          <div class="image-preview">
            <img :src="previewSrc" :alt="store.selectedLayer.img" />
            <code>{{ store.selectedLayer.img }}</code>
          </div>
          <button class="small-btn" @click="pickerOpen = true">Select Image</button>
        </template>

        <template v-if="Array.isArray(store.selectedLayer.frames)">
          <div class="row">
            <label>Frames</label>
            <span class="readonly">{{ store.selectedLayer.frames.length }} (read-only)</span>
          </div>
          <div class="row">
            <label>FPS</label>
            <input
              type="number"
              min="0"
              step="0.1"
              :value="store.selectedLayer.fps ?? 0"
              @input="onFpsInput"
            />
          </div>
          <div class="row">
            <label>Loop</label>
            <input
              class="checkbox"
              type="checkbox"
              :checked="store.selectedLayer.loop !== false"
              @change="onLoopInput"
            />
          </div>
          <div class="row">
            <label>Preview Frame</label>
            <span class="readonly">{{ store.selectedLayerFrameIndex + 1 }} / {{ store.selectedLayer.frames.length }}</span>
          </div>
          <div class="row">
            <label>Frame Offset X</label>
            <input type="number" :value="frameOffset[0]" @input="onFrameOffsetInput(0, $event)" />
          </div>
          <div class="row">
            <label>Frame Offset Y</label>
            <input type="number" :value="frameOffset[1]" @input="onFrameOffsetInput(1, $event)" />
          </div>
          <div class="image-preview" v-if="store.selectedLayer.frames[store.selectedLayerFrameIndex]">
            <img :src="`/assets/game/${store.selectedLayer.frames[store.selectedLayerFrameIndex]!.img}`" />
            <code>{{ store.selectedLayer.frames[store.selectedLayerFrameIndex]!.img }}</code>
          </div>
        </template>

        <template v-if="store.selectedLayer.spine">
          <div class="row">
            <label>Spine</label>
            <span class="readonly">{{ store.selectedLayer.spine.file }} (preview only)</span>
          </div>
        </template>
      </div>
    </template>

    <ImagePicker
      :open="pickerOpen"
      :images="store.images"
      @close="pickerOpen = false"
      @select="onSelectImage"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { LayerDefLike } from '@/types/objectEditor'
import { useObjectEditorStore } from '@/stores/objectEditorStore'
import ImagePicker from '@/components/ImagePicker.vue'

const store = useObjectEditorStore()
const pickerOpen = ref(false)
const collapsedAnimationLayers = ref(new Set<number>())
const draggedFrameIndex = ref<number | null>(null)

const rootOffset = computed(() => store.getSelectedRootOffset())
const layerOffset = computed(() => store.getSelectedLayerOffset())
const frameOffset = computed(() => store.getSelectedFrameOffset())
const previewSrc = computed(() => {
  const override = store.selectedLayerPreviewOverride
  if (override) return override
  const img = store.selectedLayer?.img
  return img ? `/assets/game/${img}` : ''
})

function isFramesExpanded(layerIndex: number): boolean {
  return !collapsedAnimationLayers.value.has(layerIndex)
}

function toggleFrames(layerIndex: number): void {
  const next = new Set(collapsedAnimationLayers.value)
  if (next.has(layerIndex)) next.delete(layerIndex)
  else next.add(layerIndex)
  collapsedAnimationLayers.value = next
}

function selectFrameLayer(layerIndex: number, frameIndex: number): void {
  store.selectLayer(layerIndex)
  store.setSelectedLayerFrameIndex(frameIndex)
}

function onFrameDragStart(frameIndex: number, event: DragEvent): void {
  draggedFrameIndex.value = frameIndex
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(frameIndex))
  }
}

function onFrameDrop(layerIndex: number, toIndex: number): void {
  const fromIndex = draggedFrameIndex.value
  draggedFrameIndex.value = null
  if (store.selectedLayerIndex !== layerIndex || fromIndex == null) return
  store.reorderSelectedLayerFrame(fromIndex, toIndex)
}

function parseInputNumber(event: Event): number | null {
  const value = Number((event.target as HTMLInputElement).value)
  return Number.isFinite(value) ? value : null
}

function onRootOffsetInput(axis: 0 | 1, event: Event): void {
  const value = parseInputNumber(event)
  if (value == null) return
  store.setRootOffsetAxis(axis, value)
}

function onLayerOffsetInput(axis: 0 | 1, event: Event): void {
  const value = parseInputNumber(event)
  if (value == null) return
  store.setSelectedLayerOffsetAxis(axis, value)
}

function onZInput(event: Event): void {
  const value = parseInputNumber(event)
  if (value == null) return
  store.setSelectedLayerZ(value)
}

function onFrameOffsetInput(axis: 0 | 1, event: Event): void {
  const value = parseInputNumber(event)
  if (value == null) return
  store.setSelectedFrameOffsetAxis(axis, value)
}

function onFpsInput(event: Event): void {
  const value = parseInputNumber(event)
  if (value == null) return
  store.setSelectedLayerFps(Math.max(0, value))
}

function onLoopInput(event: Event): void {
  store.setSelectedLayerLoop((event.target as HTMLInputElement).checked)
}

function onSelectImage(relPath: string): void {
  pickerOpen.value = false
  try {
    store.setSelectedLayerImage(relPath)
  } catch (error) {
    console.warn(error)
  }
}

function layerKind(layer: LayerDefLike): string {
  if (layer.spine) return 'spine'
  if (Array.isArray(layer.frames)) return 'frames'
  if (layer.img) return 'img'
  return 'unknown'
}

function layerPathLabel(layer: LayerDefLike): string {
  if (typeof layer.img === 'string') return layer.img
  if (Array.isArray(layer.frames) && layer.frames[0]) return `${layer.frames.length} frames (${layer.frames[0].img})`
  if (layer.spine) return `spine/${layer.spine.file}`
  return '(no path)'
}
</script>

<style scoped>
.panel-wrap {
  padding: 8px;
  height: 100%;
  overflow: auto;
}

h3 {
  margin: 0 0 8px;
  font-size: 13px;
  color: #aaa;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

h4 {
  margin: 0 0 8px;
  font-size: 12px;
  color: #ddd;
}

.empty {
  color: #777;
  font-size: 12px;
  padding: 8px;
}

.section {
  margin-bottom: 12px;
  padding: 8px;
  background: #2a2a2a;
  border-radius: 6px;
  border: 1px solid #333;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 12px;
  color: #ddd;
}

.buttons {
  display: flex;
  gap: 4px;
}

.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 12px;
}

.row:last-child {
  margin-bottom: 0;
}

.row label {
  color: #aaa;
}

.row input {
  width: 96px;
  background: #1e1e1e;
  border: 1px solid #555;
  color: #ddd;
  border-radius: 4px;
  padding: 4px 6px;
  text-align: right;
}

.readonly {
  color: #ddd;
  font-size: 12px;
}

.layer-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.layer-list li {
  padding: 6px;
  border: 1px solid #333;
  border-radius: 4px;
  cursor: pointer;
  background: #242424;
}

.layer-list li:hover {
  background: #2d2d2d;
}

.layer-list li.active {
  border-color: #28c76f;
  background: #1d3128;
}

.layer-title {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: #ddd;
}

.layer-meta {
  font-size: 10px;
  color: #8b8b8b;
  margin-top: 2px;
  word-break: break-all;
}

.frame-tree {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px solid #333;
}

.frame-toggle {
  margin-bottom: 4px;
}

.frame-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.frame-item {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  padding: 3px 4px;
  border: 1px solid #3b3b3b;
  border-radius: 3px;
  background: #1f1f1f;
  color: #aaa;
  font-size: 11px;
  cursor: grab;
}

.frame-item:active {
  cursor: grabbing;
}

.frame-item.active {
  border-color: #28c76f;
  color: #ddd;
}

.frame-item img {
  width: 24px;
  height: 24px;
  object-fit: contain;
  image-rendering: pixelated;
  flex: 0 0 auto;
}

.drag-handle {
  color: #777;
}

.frame-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tag.shadow {
  font-size: 10px;
  border: 1px solid #6b7280;
  color: #d1d5db;
  border-radius: 999px;
  padding: 0 6px;
}

.small-btn {
  border: 1px solid #555;
  background: #333;
  color: #ddd;
  border-radius: 4px;
  padding: 5px 8px;
  font-size: 12px;
  cursor: pointer;
}

.small-btn:hover {
  background: #3a3a3a;
}

.small-btn.accent {
  border-color: #1f6b46;
  color: #a7f3d0;
  background: #1b2f24;
}

.small-btn.accent:hover {
  background: #234032;
}

.image-preview {
  border: 1px solid #333;
  background: #202020;
  border-radius: 4px;
  padding: 6px;
  margin-bottom: 6px;
}

.image-preview img {
  width: 100%;
  max-height: 120px;
  object-fit: contain;
  display: block;
  background:
    linear-gradient(45deg, #2c2c2c 25%, transparent 25%),
    linear-gradient(-45deg, #2c2c2c 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, #2c2c2c 75%),
    linear-gradient(-45deg, transparent 75%, #2c2c2c 75%);
  background-size: 12px 12px;
  background-position: 0 0, 0 6px, 6px -6px, -6px 0;
}

.image-preview code {
  display: block;
  margin-top: 6px;
  font-size: 10px;
  color: #bbb;
  word-break: break-all;
}
</style>
