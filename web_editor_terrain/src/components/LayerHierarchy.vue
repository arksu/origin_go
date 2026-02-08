<template>
  <div class="layer-hierarchy">
    <h3>Layers</h3>
    <div v-if="!variant" class="empty">No variant selected</div>
    <ul v-else>
      <li
        v-for="(layer, idx) in variant.layers"
        :key="idx"
        :class="{ active: idx === store.selectedLayerIndex }"
        @click="store.selectLayer(idx)"
      >
        <div class="layer-row">
          <button
            class="vis-btn"
            :class="{ hidden: !store.isLayerVisible(store.selectedVariantIndex, idx) }"
            :title="store.isLayerVisible(store.selectedVariantIndex, idx) ? 'Hide layer' : 'Show layer'"
            @click.stop="store.toggleLayerVisibility(store.selectedVariantIndex, idx)"
          >
            {{ store.isLayerVisible(store.selectedVariantIndex, idx) ? '👁' : '—' }}
          </button>
          <div class="layer-info">
            <div class="layer-name">{{ layerLabel(layer, idx) }}</div>
            <div class="layer-meta">
              offset: [{{ layer.offset[0] }}, {{ layer.offset[1] }}]
              <span v-if="hasEditorOffset(idx)" class="editor-offset">
                + editor [{{ editorOffset(idx).dx }}, {{ editorOffset(idx).dy }}]
              </span>
            </div>
            <div class="layer-meta">p: {{ store.getLayerP(store.selectedVariantIndex, idx) }}{{ layer.z != null ? `, z: ${layer.z}` : '' }}</div>
          </div>
        </div>
      </li>
    </ul>

    <div v-if="store.selectedLayerIndex >= 0 && variant" class="edit-section">
      <label class="edit-label">
        Layer P
        <input
          type="number"
          class="edit-input"
          :value="store.getLayerP(store.selectedVariantIndex, store.selectedLayerIndex)"
          min="0"
          @input="onLayerPInput"
        />
      </label>
    </div>

    <div class="save-section">
      <button
        class="save-btn"
        :disabled="!store.hasUnsavedChanges || saving"
        @click="onSave"
      >
        {{ saving ? 'Saving...' : 'Save' }}
      </button>
      <div v-if="saveStatus" :class="['save-status', saveStatus.ok ? 'ok' : 'err']">
        {{ saveStatus.msg }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useTerrainStore } from '@/stores/terrainStore'
import type { TerrainLayer } from '@/types/terrain'

const store = useTerrainStore()
const variant = computed(() => store.selectedVariant)
const saving = ref(false)
const saveStatus = ref<{ ok: boolean; msg: string } | null>(null)

async function onSave(): Promise<void> {
  saving.value = true
  saveStatus.value = null
  try {
    await store.saveCurrentFile()
    saveStatus.value = { ok: true, msg: 'Saved successfully' }
  } catch (e) {
    saveStatus.value = { ok: false, msg: String(e) }
  } finally {
    saving.value = false
  }
}

function layerLabel(layer: TerrainLayer, idx: number): string {
  const parts = layer.img.split('/')
  return `[${idx}] ${parts[parts.length - 1]}`
}

function editorOffset(idx: number): { dx: number; dy: number } {
  return store.getLayerOffset(store.selectedVariantIndex, idx)
}

function hasEditorOffset(idx: number): boolean {
  const o = editorOffset(idx)
  return o.dx !== 0 || o.dy !== 0
}

function onLayerPInput(e: Event): void {
  const val = parseInt((e.target as HTMLInputElement).value, 10)
  if (!isNaN(val) && val >= 0) {
    store.setLayerP(store.selectedVariantIndex, store.selectedLayerIndex, val)
  }
}
</script>

<style scoped>
.layer-hierarchy {
  padding: 8px;
  overflow-y: auto;
  height: 100%;
}

h3 {
  margin: 0 0 8px;
  font-size: 13px;
  color: #aaa;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.empty {
  color: #666;
  font-size: 13px;
  padding: 8px;
}

ul {
  list-style: none;
  margin: 0;
  padding: 0;
}

li {
  padding: 6px 8px;
  cursor: pointer;
  border-radius: 4px;
  border: 1px solid transparent;
  transition: background 0.15s;
}

li:hover {
  background: #3a3a3a;
}

li.active {
  background: #1e3a2e;
  border-color: #00ff00;
}

.layer-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.vis-btn {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  border: 1px solid #555;
  background: #333;
  color: #ccc;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}

.vis-btn:hover {
  background: #444;
}

.vis-btn.hidden {
  opacity: 0.4;
}

.layer-info {
  flex: 1;
  min-width: 0;
}

.layer-name {
  font-size: 12px;
  color: #ddd;
  word-break: break-all;
}

.layer-meta {
  font-size: 11px;
  color: #777;
  margin-top: 2px;
}

.editor-offset {
  color: #4ade80;
}

.edit-section {
  margin-top: 12px;
  padding: 8px;
  background: #2a2a2a;
  border-radius: 4px;
}

.edit-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: #aaa;
}

.edit-input {
  width: 70px;
  padding: 4px 6px;
  background: #1e1e1e;
  border: 1px solid #555;
  border-radius: 3px;
  color: #ddd;
  font-size: 12px;
  text-align: right;
}

.edit-input:focus {
  outline: none;
  border-color: #2563eb;
}

.save-section {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid #333;
}

.save-btn {
  width: 100%;
  padding: 8px 16px;
  border: 1px solid #555;
  background: #2563eb;
  color: #fff;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  transition: background 0.15s;
}

.save-btn:hover:not(:disabled) {
  background: #1d4ed8;
}

.save-btn:disabled {
  background: #333;
  color: #666;
  cursor: not-allowed;
}

.save-status {
  margin-top: 6px;
  font-size: 11px;
  padding: 4px 8px;
  border-radius: 3px;
}

.save-status.ok {
  color: #4ade80;
  background: #1e3a2e;
}

.save-status.err {
  color: #f87171;
  background: #3a1e1e;
}
</style>
