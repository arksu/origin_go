<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { gameFacade } from '@/game'
import { MINIMAP_DEFAULT_ZOOM, MINIMAP_SIZE, MINIMAP_ZOOM_LEVELS } from '@/constants/minimap'

const minimapCanvas = ref<HTMLCanvasElement | null>(null)
const zoomIndex = ref(MINIMAP_ZOOM_LEVELS.indexOf(MINIMAP_DEFAULT_ZOOM))
const zoom = computed(() => MINIMAP_ZOOM_LEVELS[zoomIndex.value] ?? MINIMAP_DEFAULT_ZOOM)

function changeZoom(direction: number): void {
  const nextIndex = Math.min(MINIMAP_ZOOM_LEVELS.length - 1, Math.max(0, zoomIndex.value + direction))
  if (nextIndex === zoomIndex.value) return
  zoomIndex.value = nextIndex
  gameFacade.setMinimapZoom(zoom.value)
}

function handleWheel(event: WheelEvent): void {
  if (event.deltaY !== 0) changeZoom(event.deltaY < 0 ? 1 : -1)
}

onMounted(() => {
  if (!minimapCanvas.value) return
  gameFacade.attachMinimap(minimapCanvas.value)
  gameFacade.setMinimapZoom(zoom.value)
})

onBeforeUnmount(() => {
  if (minimapCanvas.value) gameFacade.detachMinimap(minimapCanvas.value)
})
</script>

<template>
  <section
    class="minimap-window"
    aria-label="Minimap"
    @pointerdown.stop
    @pointermove.stop
    @pointerup.stop
    @pointercancel.stop
    @mousedown.stop
    @mouseup.stop
    @click.stop
    @dblclick.stop.prevent
    @auxclick.stop.prevent
    @contextmenu.stop.prevent
    @wheel.stop.prevent="handleWheel"
    @keydown.stop
  >
    <div class="minimap-window__header">
      <span class="minimap-window__title" aria-hidden="true">Map</span>
      <div class="minimap-window__controls" role="group" aria-label="Minimap zoom">
        <button
          type="button"
          class="minimap-window__button"
          aria-label="Zoom out minimap"
          title="Zoom out minimap"
          :disabled="zoomIndex === 0"
          @click="changeZoom(-1)"
        >−</button>
        <output class="minimap-window__zoom" aria-label="Minimap scale" aria-live="polite">{{ zoom }}×</output>
        <button
          type="button"
          class="minimap-window__button"
          aria-label="Zoom in minimap"
          title="Zoom in minimap"
          :disabled="zoomIndex === MINIMAP_ZOOM_LEVELS.length - 1"
          @click="changeZoom(1)"
        >+</button>
      </div>
    </div>
    <canvas
      ref="minimapCanvas"
      class="minimap-window__canvas"
      :width="MINIMAP_SIZE"
      :height="MINIMAP_SIZE"
      role="img"
      aria-label="Nearby terrain, with your direction arrow at the center. Fixed top-down orientation."
    ></canvas>
  </section>
</template>

<style scoped lang="scss">
.minimap-window {
  width: calc(var(--minimap-size, 200px) + 10px);
  padding: 4px;
  border: 1px solid rgba(160, 148, 107, 0.85);
  border-radius: 6px;
  background: #192321;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.5), inset 0 0 0 1px rgba(230, 216, 169, 0.1);
  color: #efe0b5;
  pointer-events: auto;
  touch-action: none;
  user-select: none;
}

.minimap-window__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 28px;
  margin-bottom: 4px;
}

.minimap-window__title {
  padding-left: 4px;
  font-size: 12px;
  letter-spacing: 0.04em;
}

.minimap-window__controls {
  display: flex;
  align-items: center;
  margin-left: auto;
}

.minimap-window__button {
  width: 28px;
  height: 28px;
  padding: 0;
  border: 1px solid rgba(160, 148, 107, 0.7);
  border-radius: 4px;
  background: #263631;
  color: #efe0b5;
  font: 19px/1 Arial, sans-serif;
  cursor: pointer;

  &:hover:not(:disabled) {
    background: #3a4b3d;
  }

  &:focus-visible {
    outline: 2px solid #efe0b5;
    outline-offset: 2px;
  }

  &:disabled {
    opacity: 0.35;
    cursor: default;
  }
}

.minimap-window__zoom {
  min-width: 34px;
  font: 11px/1 Arial, sans-serif;
  font-variant-numeric: tabular-nums;
  text-align: center;
}

.minimap-window__canvas {
  display: block;
  width: var(--minimap-size, 200px);
  height: var(--minimap-size, 200px);
  border-radius: 2px;
  background: #111915;
  image-rendering: pixelated;
}

@media (max-height: 440px) {
  .minimap-window__title {
    display: none;
  }
}
</style>
