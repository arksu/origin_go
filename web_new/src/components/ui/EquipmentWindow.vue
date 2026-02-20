<script setup lang="ts">
import { computed } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { useGameStore } from '@/stores/gameStore'
import { useInventoryOps } from '@/composables/useInventoryOps'
import GameWindow from './GameWindow.vue'
import ItemSlot from './ItemSlot.vue'
import InventoryItem from './InventoryItem.vue'

interface Props {
  inventory: proto.IInventoryState
}

interface SlotLayout {
  slot: proto.EquipSlot
  x: number
  y: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
}>()

const gameStore = useGameStore()
const {
  pickUpItem,
  placeItemToEquipmentSlot,
  placeOrSwapItemInEquipmentSlot,
  findInventoryState,
} = useInventoryOps()

const inventoryRef = computed(() => props.inventory.ref)
const equipmentState = computed(() => props.inventory.equipment)

const slotLayouts: SlotLayout[] = [
  { slot: proto.EquipSlot.EQUIP_SLOT_HEAD, x: 0, y: 0 },
  { slot: proto.EquipSlot.EQUIP_SLOT_CHEST, x: 0, y: 1 },
  { slot: proto.EquipSlot.EQUIP_SLOT_LEFT_HAND, x: 0, y: 2 },
  { slot: proto.EquipSlot.EQUIP_SLOT_RING_1, x: 0, y: 3 },
  { slot: proto.EquipSlot.EQUIP_SLOT_LEGS, x: 0, y: 4 },

  { slot: proto.EquipSlot.EQUIP_SLOT_BACK, x: 5, y: 0 },
  { slot: proto.EquipSlot.EQUIP_SLOT_NECK, x: 5, y: 1 },
  { slot: proto.EquipSlot.EQUIP_SLOT_RIGHT_HAND, x: 5, y: 2 },
  { slot: proto.EquipSlot.EQUIP_SLOT_RING_2, x: 5, y: 3 },
  { slot: proto.EquipSlot.EQUIP_SLOT_FEET, x: 5, y: 4 },
]

const slotItemMap = computed(() => {
  const map = new Map<proto.EquipSlot, proto.IEquipmentItem>()
  for (const entry of equipmentState.value?.items ?? []) {
    const slot = entry.slot ?? proto.EquipSlot.EQUIP_SLOT_NONE
    if (slot !== proto.EquipSlot.EQUIP_SLOT_NONE && entry.item) {
      map.set(slot, entry)
    }
  }
  return map
})

const EQUIPMENT_PANEL_WIDTH = 218
const EQUIPMENT_PANEL_HEIGHT = 268

function onClose() {
  emit('close')
}

function onSlotClick(slot: proto.EquipSlot) {
  if (!inventoryRef.value) return
  const hand = gameStore.handState
  if (!hand?.item) return

  placeItemToEquipmentSlot(inventoryRef.value, findInventoryState(inventoryRef.value), slot)
}

function onItemClick(slot: proto.EquipSlot, item: proto.IItemInstance, ox: number, oy: number) {
  if (!inventoryRef.value) return

  const hand = gameStore.handState
  if (hand?.item) {
    placeOrSwapItemInEquipmentSlot(inventoryRef.value, findInventoryState(inventoryRef.value), slot)
    return
  }

  const itemId = Number(item.itemId ?? 0)
  pickUpItem(inventoryRef.value, findInventoryState(inventoryRef.value), itemId, ox, oy)
}

function getWindowId() {
  const ownerId = Number(inventoryRef.value?.ownerId ?? 0)
  return ownerId + 2_000_000
}

function slotLeft(slot: SlotLayout): number {
  return 16 + slot.x * 31
}

function slotTop(slot: SlotLayout): number {
  return 22 + slot.y * 31
}
</script>

<template>
  <game-window
    v-if="equipmentState && inventoryRef"
    :id="getWindowId()"
    :inner-height="EQUIPMENT_PANEL_HEIGHT"
    :inner-width="EQUIPMENT_PANEL_WIDTH"
    title="Equipment"
    @close="onClose"
  >
    <div class="equipment-panel">
      <div v-for="slotLayout in slotLayouts" :key="slotLayout.slot">
        <item-slot
          :left="slotLeft(slotLayout)"
          :top="slotTop(slotLayout)"
          :x="slotLayout.x"
          :y="slotLayout.y"
          @slot-click="() => onSlotClick(slotLayout.slot)"
        />

        <inventory-item
          v-if="slotItemMap.get(slotLayout.slot)?.item"
          :inventory-ref="inventoryRef"
          :item="{ x: slotLayout.x, y: slotLayout.y, instance: slotItemMap.get(slotLayout.slot)!.item! }"
          @item-click="(_, ox, oy) => onItemClick(slotLayout.slot, slotItemMap.get(slotLayout.slot)!.item!, ox, oy)"
        />
      </div>
    </div>
  </game-window>
</template>

<style lang="scss" scoped>
.equipment-panel {
  position: relative;
  width: 218px;
  height: 268px;
  background-image: url('/assets/img/equip_back.png');
  background-repeat: no-repeat;
  background-position: center;
  background-size: cover;
}
</style>
