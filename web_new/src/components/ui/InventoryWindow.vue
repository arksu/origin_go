<script setup lang="ts">
import { computed } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import { sendCloseContainer, sendOpenContainer } from '@/network'
import { useInventoryOps } from '@/composables/useInventoryOps'
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
const { pickUpItem, placeItem, placeOrSwapItem, findInventoryState, getPlacePositionFromSlotClick } = useInventoryOps()
const gridState = computed(() => props.inventory.grid)
const inventoryRef = computed(() => props.inventory.ref)

const isPlayerRootInventoryRef = (ref: proto.IInventoryRef): boolean => {
  const playerID = gameStore.playerEntityId ?? 0
  return (
    (ref.kind ?? 0) === proto.InventoryKind.INVENTORY_KIND_GRID &&
    Number(ref.ownerId ?? 0) === playerID &&
    (ref.inventoryKey ?? 0) === 0
  )
}

const onClose = () => {
  emit('close')
}

const onSlotClick = (x: number, y: number, _ox: number, _oy: number) => {
  if (!inventoryRef.value) return
  const hand = gameStore.handState
  if (hand?.item) {
    const placePos = getPlacePositionFromSlotClick(x, y)
    placeItem(inventoryRef.value, findInventoryState(inventoryRef.value), placePos.x, placePos.y)
  }
}

const onItemClick = (item: { x: number; y: number; instance: proto.IItemInstance }, ox: number, oy: number) => {
  if (!inventoryRef.value) return
  const hand = gameStore.handState
  if (hand?.item) {
    placeOrSwapItem(inventoryRef.value, findInventoryState(inventoryRef.value), item.x, item.y)
  } else {
    const itemId = Number(item.instance.itemId ?? 0)
    pickUpItem(inventoryRef.value, findInventoryState(inventoryRef.value), itemId, ox, oy)
  }
}

const onItemRightClick = (item: { x: number; y: number; instance: proto.IItemInstance }) => {
  console.log('Item right click:', item)
  
  // Check if item has nested container ref
  if (item.instance.nestedRef) {
    const ref = item.instance.nestedRef
    const windowKey = `${ref.kind ?? 0}_${ref.ownerId ?? 0}_${ref.inventoryKey ?? 0}`
    
    // Toggle: if already open, close it
    if (gameStore.openNestedInventories.has(windowKey)) {
      console.log('Closing nested inventory:', windowKey)
      if (!isPlayerRootInventoryRef(ref)) {
        sendCloseContainer(ref)
      }
      gameStore.closeNestedInventory(windowKey)
    } else {
      console.log('Requesting open container:', ref)
      sendOpenContainer(ref)
    }
  } else {
    console.log('Item right click - no nested container')
  }
}

const getWindowId = () => {
  return inventoryRef.value?.ownerId ? Number(inventoryRef.value.ownerId) : 0
}

const windowTitle = computed(() => {
  const title = (props.inventory.title || '').trim()
  if (!title) {
    throw new Error('InventoryWindow: missing title in InventoryState')
  }
  return title
})
</script>

<template>
  <game-window
    v-if="gridState"
    :id="getWindowId()"
    :inner-height="gridState.height! * 31"
    :inner-width="gridState.width! * 31"
    :title="windowTitle"
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
