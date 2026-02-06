<script setup lang="ts">
import { computed } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import { sendOpenContainer } from '@/network'
import GameWindow from './GameWindow.vue'
import ItemSlot from './ItemSlot.vue'
import InventoryItem from './InventoryItem.vue'

interface Props {
  windowKey: string
  inventoryState: proto.IInventoryState
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()

const gridState = computed(() => props.inventoryState.grid)
const inventoryRef = computed(() => props.inventoryState.ref)

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
  
  if (item.instance.nestedRef) {
    const ref = item.instance.nestedRef
    const windowKey = `${ref.kind ?? 0}_${ref.ownerId ?? 0}_${ref.inventoryKey ?? 0}`
    
    if (gameStore.openNestedInventories.has(windowKey)) {
      console.log('Closing deeper nested inventory:', windowKey)
      gameStore.closeNestedInventory(windowKey)
    } else {
      console.log('Requesting open deeper container:', ref)
      sendOpenContainer(ref)
    }
  } else {
    console.log('Nested item right click - no nested container')
  }
}

const getWindowId = () => {
  return inventoryRef.value?.ownerId ? Number(inventoryRef.value.ownerId) : 0
}

const getWindowTitle = () => {
  const ownerId = inventoryRef.value?.ownerId ? Number(inventoryRef.value.ownerId) : 0
  return `Container ${ownerId}`
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
        :inventory-ref="inventoryRef!"
        :item="{ x: gridItem.x!, y: gridItem.y!, instance: gridItem.item }"
        @item-click="onItemClick"
        @item-right-click="onItemRightClick"
      />
    </div>
  </game-window>
</template>
