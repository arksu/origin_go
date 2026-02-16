<script setup lang="ts">
import { computed } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import StatBar from '@/components/ui/StatBar.vue'

const gameStore = useGameStore()
const playerStats = computed(() => gameStore.playerStats)
</script>

<template>
  <div class="stats-container">
    <StatBar
      label="Stamina"
      :current="playerStats.stamina.current"
      :max="playerStats.stamina.max"
      frame-color="#4b81beba"
      bar-back-color="#08111c"
      bar1-color="#2243ee"
    />

    <StatBar
      label="Energy"
      :current="playerStats.energy.current"
      :max="playerStats.energy.max"
      frame-color="#e3bc56ba"
      bar-back-color="#1b1505"
      bar1-color="#e9b93d"
    />
  </div>
</template>

<style scoped lang="scss">
.stats-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: auto;
}
</style>
