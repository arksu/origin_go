<script setup lang="ts">
import { computed } from 'vue'
import { useGameStore } from '@/stores/gameStore'

const gameStore = useGameStore()

const hand = computed(() => gameStore.handState)
const mouse = computed(() => gameStore.mousePos)

const offsetX = computed(() => hand.value?.handPos?.mouseOffsetX ?? 15)
const offsetY = computed(() => hand.value?.handPos?.mouseOffsetY ?? 15)

const itemResource = computed(() => hand.value?.item?.resource ?? '')
</script>

<template>
  <div
    v-if="hand"
    class="hand-overlay"
    :style="{
      left: (mouse.x - offsetX) + 'px',
      top: (mouse.y - offsetY) + 'px',
    }"
  >
    <img
      v-if="itemResource"
      :src="`/assets/game/${itemResource}`"
      alt="hand"
      class="hand-image"
    />
  </div>
</template>

<style lang="scss" scoped>
.hand-overlay {
  position: fixed;
  pointer-events: none;
  z-index: 9999;
}

.hand-image {
  display: block;
}
</style>
