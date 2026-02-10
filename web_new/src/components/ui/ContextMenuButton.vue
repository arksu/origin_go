<script setup lang="ts">
import { computed, ref } from 'vue'
import { useGameStore, type ContextMenuActionItem } from '@/stores/gameStore'
import { playerCommandController } from '@/game'

const props = defineProps<{
  index: number
  item: ContextMenuActionItem
  total: number
  entityId: number
}>()

const gameStore = useGameStore()
const selected = ref(false)

const styleVars = computed(() => {
  const len = Math.max(1, props.total)
  const radius1 = 50 + len * 10
  const radius2 = 55
  const radius3 = 40
  const offset = len * 0.3
  const angle1 = (props.index / len) * Math.PI - offset
  const angle2 = (props.index / len) * Math.PI - offset - 0.72
  const angle3 = (props.index / len) * Math.PI - offset - 1.9

  const xOffset = 50
  const yOffset = 30

  const x1 = Math.cos(angle1) * radius1 - xOffset
  const y1 = Math.sin(angle1) * radius1 - yOffset
  const x2 = Math.cos(angle2) * radius2 - xOffset
  const y2 = Math.sin(angle2) * radius2 - yOffset
  const x3 = Math.cos(angle3) * radius3 - xOffset
  const y3 = Math.sin(angle3) * radius3 - yOffset

  return {
    '--x1': `${x1}px`,
    '--y1': `${y1}px`,
    '--x2': `${x2}px`,
    '--y2': `${y2}px`,
    '--x3': `${x3}px`,
    '--y3': `${y3}px`,
  }
})

function onClick() {
  selected.value = true
  gameStore.closeContextMenu()
  playerCommandController.sendSelectContextAction(props.entityId, props.item.actionId)
}
</script>

<template>
  <div
    :style="styleVars"
    class="context-menu-button"
    :class="{ selected }"
    @click.prevent.stop="onClick"
  >
    <p>{{ item.title }}</p>
  </div>
</template>

<style scoped lang="scss">
.context-menu-button {
  padding: 5px;
  color: #a8b087;
  border: 2px solid #9f935d;
  border-radius: 5px;
  background-color: #363e19;
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
}

.context-menu-button p {
  margin: 0;
}

.selected {
  animation-duration: 0.5s !important;
  animation-name: cm-move-leave !important;
}

@keyframes cm-move-leave {
  100% {
    transform: translate(-40px, -30px);
    opacity: 0;
  }
  50% {
    transform: translate(-40px, -30px);
    opacity: 1;
  }
  0% {
    transform: translate(var(--x1), var(--y1));
    opacity: 1;
  }
}
</style>
