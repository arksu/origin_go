<script setup lang="ts">
import { proto } from '@/network/proto/packets.js'

interface Props {
  item: {
    x: number
    y: number
    instance: proto.IItemInstance
  }
  inventoryRef: proto.IInventoryRef
}

const props = defineProps<Props>()
const emit = defineEmits<{
  itemClick: [item: Props['item'], ox: number, oy: number]
  itemRightClick: [item: Props['item']]
}>()

const onClick = (e: MouseEvent) => {
  emit('itemClick', props.item, e.offsetX, e.offsetY)
}

const onContextmenu = () => {
  emit('itemRightClick', props.item)
}
</script>

<template>
  <img
    :src="`/assets/game/${item.instance.resource}`"
    :style="`left: ${17 + item.x * 31}px; top: ${23 + item.y * 31}px;`"
    alt="item"
    class="item-image"
    @click.prevent="onClick"
    @contextmenu.prevent.stop="onContextmenu"
  >
</template>

<style lang="scss" scoped>
.item-image {
  position: absolute;
  cursor: pointer;
}
</style>
