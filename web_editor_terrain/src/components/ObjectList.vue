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
          Variant {{ idx }} <span class="chance">chance: 1/{{ variant.chance }}</span>
        </li>
      </ul>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useTerrainStore } from '@/stores/terrainStore'

const store = useTerrainStore()
const selectedConfig = computed(() => store.selectedConfig)
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
</style>
