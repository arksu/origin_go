<script setup lang="ts">
import { computed } from 'vue'
import type { CustomWindowState } from '@/stores/gameStore'
import GameWindow from './GameWindow.vue'

const props = defineProps<Pick<CustomWindowState, 'title' | 'fields'>>()
const emit = defineEmits<{ close: [] }>()
const innerHeight = computed(() => Math.min(240, Math.max(56, props.fields.length * 40)))
</script>

<template>
  <GameWindow :id="7104" :title="title" :inner-width="260" :inner-height="innerHeight" @close="emit('close')">
    <dl class="custom-window-fields">
      <div v-for="(field, index) in fields" :key="index" class="custom-window-field">
        <dt>{{ field.label }}</dt>
        <dd>{{ field.value }}</dd>
      </div>
    </dl>
  </GameWindow>
</template>

<style scoped lang="scss">
.custom-window-fields {
  height: 100%;
  margin: 0;
  overflow-y: auto;
  color: #e6dcc8;
  font-size: 14px;
  line-height: 1.5;
}

.custom-window-field {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
  gap: 12px;
  padding-bottom: 8px;
  overflow-wrap: anywhere;

  dt { color: #b8ad98; }
  dd { margin: 0; white-space: pre-wrap; }
}
</style>
