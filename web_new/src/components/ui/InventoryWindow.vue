<script setup lang="ts">
import { computed } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import GameWindow from './GameWindow.vue'
import ItemSlot from './ItemSlot.vue'
import InventoryItem from './InventoryItem.vue'

interface Props {
  inventory: proto.IInventoryState
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()
const gridState = computed(() => props.inventory.grid)
const inventoryRef = computed(() => props.inventory.ref)

const onClose = () => {
  emit('close')
}

const onSlotClick = (_x: number, _y: number, _ox: number, _oy: number) => {
  console.log('Slot click - inventory operations not yet implemented')
}

const onItemClick = (_item: { x: number; y: number; instance: proto.IItemInstance }, _ox: number, _oy: number) => {
  console.log('Item click - inventory operations not yet implemented')
}

const onItemRightClick = (item: { x: number; y: number; instance: proto.IItemInstance }) => {
  console.log('Item right click:', item)
  console.log('Item nestedInventory data:', item.instance.nestedInventory)
  
  // Check if item has nested inventory
  if (item.instance.nestedInventory && item.instance.itemId) {
    console.log('Toggle nested inventory for item:', item.instance.itemId)
    
    // Create inventory key for nested inventory (assuming key 0 for nested inventory)
    const inventoryKey = 0
    const itemId = typeof item.instance.itemId === 'number' ? item.instance.itemId : Number(item.instance.itemId)
    const windowKey = gameStore.openNestedInventory(itemId, inventoryKey.toString(), item.instance.nestedInventory)
    
    // Check if window was closed (toggle behavior)
    const isClosed = !gameStore.openNestedInventories.has(windowKey)
    console.log('Nested inventory window', isClosed ? 'closed' : 'opened', 'with key:', windowKey)
  } else {
    console.log('Item right click - inventory operations not yet implemented')
  }
}

const getWindowId = () => {
  return inventoryRef.value?.ownerEntityId ? Number(inventoryRef.value.ownerEntityId) : 0
}
</script>

<template>
  <game-window
    v-if="gridState"
    :id="getWindowId()"
    :inner-height="gridState.height! * 31"
    :inner-width="gridState.width! * 31"
    title="Inventory"
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
        :inventory-ref="inventoryRef!"
        :item="{ x: gridItem.x!, y: gridItem.y!, instance: gridItem.item }"
        @item-click="onItemClick"
        @item-right-click="onItemRightClick"
      />
    </div>
  </game-window>
</template>
