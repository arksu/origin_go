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
          </li>
        </ul>
      </div>

      <div v-if="store.selectedLayer" class="section">
        <h4>Selected Layer</h4>
        <div class="row">
          <label>Kind</label>
          <span class="readonly">{{ layerKind(store.selectedLayer) }}</span>
        </div>

        <template v-if="!Array.isArray(store.selectedLayer.frames)">
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
            <span class="readonly">{{ store.selectedLayer.fps ?? 'n/a' }} (read-only)</span>
          </div>
          <div class="row">
            <label>Loop</label>
            <span class="readonly">{{ store.selectedLayer.loop ?? true }} (read-only)</span>
          </div>
          <div class="row">
            <label>Preview Frame</label>
            <input
              type="number"
              min="0"
              :max="Math.max(0, store.selectedLayer.frames.length - 1)"
              :value="store.selectedLayerFrameIndex"
              @input="onFrameIndexInput"
            />
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

const rootOffset = computed(() => store.getSelectedRootOffset())
const layerOffset = computed(() => store.getSelectedLayerOffset())
const previewSrc = computed(() => {
  const override = store.selectedLayerPreviewOverride
  if (override) return override
  const img = store.selectedLayer?.img
  return img ? `/assets/game/${img}` : ''
})

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

function onFrameIndexInput(event: Event): void {
  const value = parseInputNumber(event)
  if (value == null) return
  store.setSelectedLayerFrameIndex(Math.max(0, Math.floor(value)))
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
