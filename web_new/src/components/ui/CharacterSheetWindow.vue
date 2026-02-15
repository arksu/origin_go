<script setup lang="ts">
import { computed } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import GameWindow from './GameWindow.vue'

const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()
const attributes = computed(() => gameStore.characterAttributes)

function onClose() {
  emit('close')
}
</script>

<template>
  <GameWindow
    :id="7001"
    :inner-height="292"
    :inner-width="236"
    title="Character Sheet"
    @close="onClose"
  >
    <div class="character-sheet">
      <p class="section-title">Base Attributes:</p>
      <ul class="attribute-list">
        <li v-for="attribute in attributes" :key="attribute.key" class="attribute-row">
          <span class="attribute-icon" aria-hidden="true">{{ attribute.icon }}</span>
          <span class="attribute-label">{{ attribute.label }}:</span>
          <span class="attribute-value">{{ attribute.value }}</span>
        </li>
      </ul>
    </div>
  </GameWindow>
</template>

<style scoped lang="scss">
.character-sheet {
  width: 100%;
  height: 100%;
  text-align: left;
  color: #d7ddd3;
}

.section-title {
  margin: 2px 0 8px;
  font-size: 16px;
  line-height: 1.2;
  color: #d9dfc1;
  text-shadow: 0 1px 0 #1d201f;
}

.attribute-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.attribute-row {
  display: grid;
  grid-template-columns: 18px 1fr auto;
  align-items: center;
  column-gap: 8px;
  padding: 1px 2px;
}

.attribute-icon {
  width: 18px;
  text-align: center;
  font-size: 13px;
  color: #ccb874;
}

.attribute-label {
  font-size: 14px;
  color: #d2d7be;
}

.attribute-value {
  min-width: 24px;
  text-align: right;
  font-size: 14px;
  color: #d8decd;
}
</style>
