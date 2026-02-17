<script setup lang="ts">
import { computed } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { sendMovementMode } from '@/network'
import { useGameStore } from '@/stores/gameStore'

interface ModeButton {
  id: proto.MovementMode
  label: string
}

const gameStore = useGameStore()
const activeMode = computed(() => gameStore.playerMoveMode)

const buttons: ModeButton[] = [
  { id: proto.MovementMode.MOVE_MODE_CRAWL, label: 'Crawl' },
  { id: proto.MovementMode.MOVE_MODE_WALK, label: 'Walk' },
  { id: proto.MovementMode.MOVE_MODE_RUN, label: 'Run' },
  { id: proto.MovementMode.MOVE_MODE_FAST_RUN, label: 'Fast' },
]

function onModeClick(mode: proto.MovementMode): void {
  sendMovementMode(mode)
}
</script>

<template>
  <div class="movement-mode-panel">
    <button
      v-for="button in buttons"
      :key="button.id"
      type="button"
      class="mode-btn"
      :class="{ 'is-active': activeMode === button.id }"
      @click="onModeClick(button.id)"
    >
      {{ button.label }}
    </button>
  </div>
</template>

<style scoped lang="scss">
.movement-mode-panel {
  display: flex;
  gap: 6px;
  pointer-events: auto;
}

.mode-btn {
  min-width: 56px;
  height: 26px;
  padding: 0 8px;
  border: 1px solid #7f7f7f;
  border-radius: 4px;
  background: #5d5d5d;
  color: #d0d0d0;
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
  filter: grayscale(1);
}

.mode-btn:hover {
  border-color: #a9a9a9;
  color: #efefef;
}

.mode-btn.is-active {
  border-color: #f7d86f;
  background: #c29a2f;
  color: #fff4c8;
  filter: none;
}
</style>
