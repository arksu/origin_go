<script setup lang="ts">
import { INVENTORY_GRID } from '@/constants/inventoryLayout'
interface Props {
  left: number
  top: number
  x: number
  y: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  slotClick: [x: number, y: number, ox: number, oy: number]
}>()

const onClick = (e: MouseEvent) => {
  emit('slotClick', props.x, props.y, e.offsetX, e.offsetY)
}
</script>

<template>
  <div :style="{ left: `${left}px`, top: `${top}px`, width: `${INVENTORY_GRID.slotSize}px`, height: `${INVENTORY_GRID.slotSize}px` }"
       class="item-back"
       @click.prevent="onClick">
  </div>
</template>

<style lang="scss" scoped>
.item-back {
  position: absolute;
  box-sizing: border-box;
  background-image: url('/assets/img/inventory_slot.png');
  background-repeat: no-repeat;
  padding-top: 2px;
}
</style>
