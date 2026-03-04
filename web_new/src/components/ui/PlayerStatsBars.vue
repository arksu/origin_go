<script setup lang="ts">
import { computed } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import StatBar from '@/components/ui/StatBar.vue'
import HealthBar from '@/components/ui/HealthBar.vue'

const gameStore = useGameStore()
const playerStats = computed(() => gameStore.playerStats)
const energyLayers = [
  { min: 0, max: 499, color: '#600000' }, // Starving
  { min: 500, max: 799, color: '#ff4000' }, // Very Hungry
  { min: 800, max: 899, color: '#ffc000' }, // Hungry
  { min: 900, max: 1000, color: '#00ff00' }, // Full
  { min: 1001, max: 1100, color: '#e27c21' }, // Overstuffed
]
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
      :layers="energyLayers"
    />

    <HealthBar
      :mhp="playerStats.hhp.max"
      :hhp="playerStats.hhp.current"
      :shp="playerStats.shp.current"
      frame-color="#bfa57a"
      bar-back-color="#1a0707"
      hhp-color="#7a1f1f"
      shp-color="#d94a3a"
    />

    <div v-if="playerStats.isKnockedOut" class="ko-badge">
      Knocked out
    </div>
  </div>
</template>

<style scoped lang="scss">
.stats-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: auto;
}

.ko-badge {
  color: #ffd7d7;
  background: #5b1f1f;
  border: 1px solid #9b4545;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  padding: 4px 8px;
  width: fit-content;
}
</style>
