<script setup lang="ts">
import { computed, ref, onUnmounted } from 'vue'
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

const tooltipVisible = ref(false)
const tooltipX = ref(0)
const tooltipY = ref(0)

const tooltipText = computed(() => {
  const name = props.item.instance.name || 'Unknown Item'
  const quality = props.item.instance.quality || 0
  let text = `${name}, quality: ${quality}`
  const hintExt = props.item.instance.hintExt
  if (hintExt) {
    // Split on \r\n and join with \n
    const lines = hintExt.split('\r\n')
    text += '\n' + lines.join('\n')
  }
  return text
})

const onMouseMove = (e: MouseEvent) => {
  if (tooltipVisible.value) {
    tooltipX.value = e.clientX + 10
    tooltipY.value = e.clientY + 10
    updateTooltipPosition()
  }
}

const onMouseEnter = (e: MouseEvent) => {
  tooltipVisible.value = true
  tooltipX.value = e.clientX + 10
  tooltipY.value = e.clientY + 10
  createTooltip()
  updateTooltipPosition()
}

const onMouseLeave = () => {
  tooltipVisible.value = false
  removeTooltip()
}

let tooltipElement: HTMLDivElement | null = null

// Cleanup tooltip when component is unmounted (item deleted)
onUnmounted(() => {
  removeTooltip()
})

const createTooltip = () => {
  if (tooltipElement) return
  
  tooltipElement = document.createElement('div')
  tooltipElement.className = 'item-tooltip-global'
  tooltipElement.innerHTML = `<pre>${tooltipText.value}</pre>`
  document.body.appendChild(tooltipElement)
}

const removeTooltip = () => {
  if (tooltipElement) {
    document.body.removeChild(tooltipElement)
    tooltipElement = null
  }
}

const updateTooltipPosition = () => {
  if (tooltipElement) {
    tooltipElement.style.left = `${tooltipX.value}px`
    tooltipElement.style.top = `${tooltipY.value}px`
  }
}

const onClick = (e: MouseEvent) => {
  // Hide tooltip when picking up item
  onMouseLeave()
  emit('itemClick', props.item, e.offsetX, e.offsetY)
}

const onContextmenu = () => {
  // Hide tooltip on right click
  onMouseLeave()
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
    @mouseenter="onMouseEnter"
    @mouseleave="onMouseLeave"
    @mousemove="onMouseMove"
  >
</template>

<style lang="scss" scoped>
.item-image {
  position: absolute;
  cursor: pointer;
}
</style>

<style lang="scss">
/* Global tooltip styles */
.item-tooltip-global {
  position: fixed;
  background: rgba(0, 0, 0, 0.7);
  color: #ffffff;
  border: 2px solid #555;
  border-radius: 8px;
  padding: 3px 6px;
  font-size: 12px;
  white-space: pre-wrap;
  z-index: 999999;
  pointer-events: none;
  max-width: 300px;
  word-wrap: break-word;
}
</style>
