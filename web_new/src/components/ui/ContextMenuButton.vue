<script setup lang="ts">
import { computed } from 'vue'
import type { ContextMenuActionItem } from '@/stores/gameStore'

const props = defineProps<{
  index: number
  item: ContextMenuActionItem
  total: number
  selected?: boolean
}>()
const emit = defineEmits<{
  select: [actionId: string]
}>()

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
  emit('select', props.item.actionId)
}
</script>

<template>
  <div
    :style="styleVars"
    class="context-menu-button"
    :class="{ 'is-selected': !!props.selected }"
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
  white-space: nowrap;
  width: max-content;
}

.context-menu-button p {
  margin: 0;
  white-space: inherit;
}

.is-selected {
  filter: brightness(1.08);
  animation: cm-selected-to-center 1s cubic-bezier(0.2, 0.85, 0.2, 1) forwards;
}

@keyframes cm-selected-to-center {
  0% {
    transform: translate(var(--x1), var(--y1)) translate(-50%, -50%);
    opacity: 1;
  }
  82% {
    transform: translate(-4px, -3px) translate(-50%, -50%);
    opacity: 1;
  }
  100% {
    transform: translate(0, 0) translate(-50%, -50%);
    opacity: 1;
  }
}
</style>
