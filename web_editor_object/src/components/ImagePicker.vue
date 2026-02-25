<template>
  <div v-if="open" class="picker-backdrop" @click.self="$emit('close')">
    <div class="picker-panel">
      <div class="picker-head">
        <input v-model="query" class="search" placeholder="Search obj/*.png" />
        <button class="close-btn" @click="$emit('close')">Close</button>
      </div>
      <div class="list">
        <button
          v-for="img in filteredImages"
          :key="img.relPath"
          class="image-row"
          @click="$emit('select', img.relPath)"
        >
          <img :src="`/assets/game/${img.relPath}`" :alt="img.relPath" loading="lazy" @error="onImgError" />
          <span>{{ img.relPath }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ImageAssetEntry } from '@/types/objectEditor'

const props = defineProps<{
  open: boolean
  images: ImageAssetEntry[]
  initialQuery?: string
}>()

defineEmits<{
  close: []
  select: [relPath: string]
}>()

const query = ref('')

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) query.value = props.initialQuery ?? ''
  },
  { immediate: true },
)

const filteredImages = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.images.slice(0, 500)
  return props.images.filter((img) => img.relPath.toLowerCase().includes(q)).slice(0, 500)
})

function onImgError(e: Event): void {
  const el = e.target as HTMLImageElement
  el.style.visibility = 'hidden'
}
</script>

<style scoped>
.picker-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}

.picker-panel {
  width: min(900px, 92vw);
  height: min(700px, 90vh);
  background: #222;
  border: 1px solid #444;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
}

.picker-head {
  display: flex;
  gap: 8px;
  padding: 10px;
  border-bottom: 1px solid #333;
}

.search {
  flex: 1;
  background: #1a1a1a;
  color: #ddd;
  border: 1px solid #555;
  border-radius: 4px;
  padding: 6px 8px;
}

.close-btn {
  border: 1px solid #555;
  background: #333;
  color: #ddd;
  border-radius: 4px;
  padding: 6px 10px;
  cursor: pointer;
}

.list {
  overflow: auto;
  padding: 10px;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 8px;
}

.image-row {
  border: 1px solid #3a3a3a;
  background: #2a2a2a;
  color: #ddd;
  border-radius: 6px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  cursor: pointer;
  text-align: left;
}

.image-row:hover {
  border-color: #4f8ef7;
}

.image-row img {
  width: 100%;
  height: 90px;
  object-fit: contain;
  background:
    linear-gradient(45deg, #303030 25%, transparent 25%),
    linear-gradient(-45deg, #303030 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, #303030 75%),
    linear-gradient(-45deg, transparent 75%, #303030 75%);
  background-size: 12px 12px;
  background-position: 0 0, 0 6px, 6px -6px, -6px 0;
  border-radius: 4px;
}

.image-row span {
  font-size: 11px;
  word-break: break-all;
}
</style>
