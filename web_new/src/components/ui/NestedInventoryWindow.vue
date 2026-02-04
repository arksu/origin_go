<script setup lang="ts">
import { computed } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import GameWindow from './GameWindow.vue'
import ItemSlot from './ItemSlot.vue'
import InventoryItem from './InventoryItem.vue'

interface Props {
  windowKey: string
  nestedInventoryData: proto.IInventoryGridState
  itemId: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()

// Use the nested inventory data directly from props instead of store
const gridState = computed(() => props.nestedInventoryData)

const onClose = () => {
  gameStore.closeNestedInventory(props.windowKey)
  emit('close')
}

const onSlotClick = (_x: number, _y: number, _ox: number, _oy: number) => {
  console.log('Nested slot click - inventory operations not yet implemented')
}

const onItemClick = (_item: { x: number; y: number; instance: proto.IItemInstance }, _ox: number, _oy: number) => {
  console.log('Nested item click - inventory operations not yet implemented')
}

const onItemRightClick = (item: { x: number; y: number; instance: proto.IItemInstance }) => {
  console.log('Nested item right click:', item)
  
  // Check if this nested item also has a nested inventory (recursive support)
  if (item.instance.nestedInventory && item.instance.itemId) {
    console.log('Toggle deeper nested inventory for item:', item.instance.itemId)
    
    // Create inventory key for nested inventory (assuming key 0 for nested inventory)
    const inventoryKey = 0
    const itemId = typeof item.instance.itemId === 'number' ? item.instance.itemId : Number(item.instance.itemId)
    const windowKey = gameStore.openNestedInventory(itemId, inventoryKey.toString(), item.instance.nestedInventory)
    
    // Check if window was closed (toggle behavior)
    const isClosed = !gameStore.openNestedInventories.has(windowKey)
    console.log('Deeper nested inventory window', isClosed ? 'closed' : 'opened', 'with key:', windowKey)
  } else {
    console.log('Nested item right click - inventory operations not yet implemented')
  }
}

const getWindowId = () => {
  return props.itemId
}

const getWindowTitle = () => {
  return `Container ${props.itemId}`
}
</script>

<template>
  <game-window
    v-if="gridState"
    :id="getWindowId()"
    :inner-height="gridState.height! * 31"
    :inner-width="gridState.width! * 31"
    :title="getWindowTitle()"
    @close="onClose"
  >
    <div v-for="y in gridState.height" :key="y">
      <div v-for="x in gridState.width" :key="x">
        <item-slot 
          :left="16 + (x-1) * 31" 
          :top="22 + (y-1) * 31" 
          :x="x-1" 
          :y="y-1"
          @slot-click="onSlotClick"
        />
      </div>
    </div>

    <div v-for="(gridItem, idx) in gridState.items" :key="idx">
      <inventory-item 
        v-if="gridItem.item"
        :inventory-ref="{ ownerItemId: props.itemId, inventoryKey: 0 }"
        :item="{ x: gridItem.x!, y: gridItem.y!, instance: gridItem.item }"
        @item-click="onItemClick"
        @item-right-click="onItemRightClick"
      />
    </div>
  </game-window>
</template>
