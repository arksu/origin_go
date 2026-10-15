<script setup lang="ts">
import { useInventoryDrag } from '@/composables/useInventoryDrag'

const { activeDrag } = useInventoryDrag()
</script>

<template>
  <div
    v-if="activeDrag"
    class="inventory-drag-overlay"
    aria-hidden="true"
    :style="{ left: `${activeDrag.clientX - activeDrag.offsetX}px`, top: `${activeDrag.clientY - activeDrag.offsetY}px` }"
  >
    <img :src="activeDrag.imageUrl" alt="" draggable="false" />
    <span v-if="activeDrag.qualityText !== undefined" class="inventory-drag-quality">{{ activeDrag.qualityText }}</span>
  </div>
</template>

<style scoped lang="scss">
.inventory-drag-overlay {
  position: fixed;
  z-index: 999999;
  pointer-events: none;
  user-select: none;

  img {
    display: block;
    image-rendering: pixelated;
  }
}

.inventory-drag-quality {
  position: absolute;
  right: 2px;
  bottom: 1px;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
  text-shadow: -1px 0 #000, 1px 0 #000, 0 -1px #000, 0 1px #000;
}
</style>
