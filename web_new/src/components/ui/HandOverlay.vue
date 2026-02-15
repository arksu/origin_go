<script setup lang="ts">
import { computed } from 'vue'
import { useGameStore } from '@/stores/gameStore'

const gameStore = useGameStore()

const hand = computed(() => gameStore.handState)
const mouse = computed(() => gameStore.mousePos)

const offsetX = computed(() => hand.value?.handPos?.mouseOffsetX ?? 15)
const offsetY = computed(() => hand.value?.handPos?.mouseOffsetY ?? 15)

const itemResource = computed(() => hand.value?.item?.resource ?? '')
const qualityText = computed(() => String(hand.value?.item?.quality ?? 0))
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
    <div v-if="itemResource" class="hand-item">
      <img
        :src="`/assets/game/${itemResource}`"
        alt="hand"
        class="hand-image"
      />
      <span class="item-quality">{{ qualityText }}</span>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.hand-overlay {
  position: fixed;
  pointer-events: none;
  z-index: 9999;
}

.hand-item {
  position: relative;
  display: inline-block;
}

.hand-image {
  display: block;
}

.item-quality {
  position: absolute;
  right: 2px;
  bottom: 1px;
  max-width: calc(100% - 4px);
  overflow: hidden;
  white-space: nowrap;
  text-overflow: clip;
  color: #ffffff;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  text-shadow:
    -1px 0 0 #000000,
    1px 0 0 #000000,
    0 -1px 0 #000000,
    0 1px 0 #000000,
    -1px -1px 0 #000000,
    1px -1px 0 #000000,
    -1px 1px 0 #000000,
    1px 1px 0 #000000;
}
</style>
