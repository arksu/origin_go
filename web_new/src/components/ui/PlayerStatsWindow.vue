<script setup lang="ts">
import { computed } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import GameWindow from './GameWindow.vue'

const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()
const experience = computed(() => gameStore.characterExperience)

function onClose() {
  emit('close')
}
</script>

<template>
  <GameWindow
    :id="7002"
    :inner-height="190"
    :inner-width="236"
    title="Player Stats"
    @close="onClose"
  >
    <div class="player-stats">
      <p class="section-title">Experience:</p>
      <ul class="stat-list">
        <li class="stat-row">
          <span class="stat-label">Learning Points:</span>
          <span class="stat-value">{{ experience.lp }}</span>
        </li>
        <li class="stat-row">
          <span class="stat-label">Nature:</span>
          <span class="stat-value">{{ experience.nature }}</span>
        </li>
        <li class="stat-row">
          <span class="stat-label">Industry:</span>
          <span class="stat-value">{{ experience.industry }}</span>
        </li>
        <li class="stat-row">
          <span class="stat-label">Combat:</span>
          <span class="stat-value">{{ experience.combat }}</span>
        </li>
      </ul>
    </div>
  </GameWindow>
</template>

<style scoped lang="scss">
.player-stats {
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

.stat-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.stat-row {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: center;
  column-gap: 8px;
  padding: 1px 2px;
}

.stat-label {
  font-size: 14px;
  color: #d2d7be;
}

.stat-value {
  min-width: 40px;
  text-align: right;
  font-size: 14px;
  color: #d8decd;
}
</style>
