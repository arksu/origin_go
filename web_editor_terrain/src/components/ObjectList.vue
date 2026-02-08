<template>
  <div class="object-list">
    <h3>Terrain Files</h3>
    <ul>
      <li
        v-for="(file, idx) in store.files"
        :key="file.fileName"
        :class="{ active: idx === store.selectedFileIndex }"
        @click="store.selectFile(idx)"
      >
        {{ file.fileName }}
      </li>
    </ul>

    <template v-if="selectedConfig">
      <h3>Variants</h3>
      <ul>
        <li
          v-for="(variant, idx) in selectedConfig"
          :key="idx"
          :class="{ active: idx === store.selectedVariantIndex }"
          @click="store.selectVariant(idx)"
        >
          Variant {{ idx }} <span class="chance">chance: 1/{{ store.getVariantChance(idx) }}</span>
        </li>
      </ul>

      <div v-if="store.selectedVariantIndex >= 0" class="edit-section">
        <label class="edit-label">
          Variant Chance
          <input
            type="number"
            class="edit-input"
            :value="store.getVariantChance(store.selectedVariantIndex)"
            min="1"
            @input="onChanceInput"
          />
        </label>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useTerrainStore } from '@/stores/terrainStore'

const store = useTerrainStore()
const selectedConfig = computed(() => store.selectedConfig)

function onChanceInput(e: Event): void {
  const val = parseInt((e.target as HTMLInputElement).value, 10)
  if (!isNaN(val) && val >= 0) {
    store.setVariantChance(store.selectedVariantIndex, val)
  }
}
</script>

<style scoped>
.object-list {
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

ul {
  list-style: none;
  margin: 0 0 16px;
  padding: 0;
}

li {
  padding: 6px 10px;
  cursor: pointer;
  border-radius: 4px;
  font-size: 13px;
  color: #ccc;
  transition: background 0.15s;
}

li:hover {
  background: #3a3a3a;
}

li.active {
  background: #2563eb;
  color: #fff;
}

.chance {
  font-size: 11px;
  color: #888;
  margin-left: 6px;
}

li.active .chance {
  color: #bfdbfe;
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
</style>
