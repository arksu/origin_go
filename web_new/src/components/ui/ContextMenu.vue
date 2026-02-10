<script setup lang="ts">
import { computed } from 'vue'
import ContextMenuButton from '@/components/ui/ContextMenuButton.vue'
import { useGameStore } from '@/stores/gameStore'

const gameStore = useGameStore()

const menu = computed(() => gameStore.contextMenu)
const list = computed(() => menu.value?.actions || [])
const posX = computed(() => menu.value?.anchorX || 0)
const posY = computed(() => menu.value?.anchorY || 0)
const entityId = computed(() => menu.value?.entityId || 0)
</script>

<template>
  <transition-group
    :style="{ left: `${posX}px`, top: `${posY}px` }"
    class="context-menu-container"
    name="spiral"
    tag="div"
  >
    <ContextMenuButton
      v-for="(item, idx) in list"
      :key="item.actionId"
      :item="item"
      :index="idx"
      :total="list.length"
      :entity-id="entityId"
      class="action-button"
    />
  </transition-group>
</template>

<style scoped lang="scss">
.context-menu-container {
  pointer-events: auto;
  opacity: 1;
  position: absolute;
  z-index: 220;
}

.action-button {
  position: absolute;
  transform: translate(var(--x1), var(--y1));
  animation-duration: 0.5s;
  animation-direction: alternate;
  animation-timing-function: linear;
}

.spiral-enter-active {
  animation-name: cm-move;
}

.spiral-leave-active {
  animation-duration: 0.2s;
  animation-name: cm-move-hide;
}

.context-menu-container:empty {
  pointer-events: none;
}

@keyframes cm-move {
  0% {
    transform: translate(0, 0);
    opacity: 0;
  }
  33% {
    transform: translate(var(--x3), var(--y3));
    opacity: 0.2;
  }
  66% {
    transform: translate(var(--x2), var(--y2));
    opacity: 0.66;
  }
  100% {
    transform: translate(var(--x1), var(--y1));
    opacity: 1;
  }
}

@keyframes cm-move-hide {
  100% {
    transform: translate(var(--x1), var(--y1));
    opacity: 0;
  }
  0% {
    transform: translate(var(--x1), var(--y1));
    opacity: 1;
  }
}
</style>
