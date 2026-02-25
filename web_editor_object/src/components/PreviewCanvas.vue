<template>
  <div class="preview-shell" ref="containerRef">
    <canvas ref="canvasRef" tabindex="0" @keydown="onKeyDown"></canvas>
    <div class="hud">
      <span v-if="store.selectedObjectPath">Path: {{ store.selectedObjectPath }}</span>
      <span v-if="store.selectedLayerIndex >= 0">Layer: {{ store.selectedLayerIndex }}</span>
      <button class="hud-btn" @click="renderer.resetView()">Reset View</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useObjectEditorStore } from '@/stores/objectEditorStore'
import { ObjectPreviewRenderer } from '@/engine/ObjectPreviewRenderer'

const store = useObjectEditorStore()
const canvasRef = ref<HTMLCanvasElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const renderer = new ObjectPreviewRenderer()
let resizeObserver: ResizeObserver | null = null

function renderNow(): void {
  void renderer.renderResource(store.selectedResource, {
    selectedLayerIndex: store.selectedLayerIndex,
    getFrameIndex: (layerIndex) => {
      if (layerIndex !== store.selectedLayerIndex) return 0
      return store.selectedLayerFrameIndex
    },
    getImageOverride: (layerIndex) =>
      layerIndex === store.selectedLayerIndex ? store.selectedLayerPreviewOverride : undefined,
  })
}

function onKeyDown(event: KeyboardEvent): void {
  const step = event.shiftKey ? 10 : 1
  switch (event.key) {
    case 'ArrowUp':
      store.nudgeSelectedLayer(0, -step)
      break
    case 'ArrowDown':
      store.nudgeSelectedLayer(0, step)
      break
    case 'ArrowLeft':
      store.nudgeSelectedLayer(-step, 0)
      break
    case 'ArrowRight':
      store.nudgeSelectedLayer(step, 0)
      break
    default:
      return
  }
  event.preventDefault()
}

onMounted(async () => {
  const canvas = canvasRef.value
  const container = containerRef.value
  if (!canvas || !container) return

  canvas.width = container.clientWidth
  canvas.height = container.clientHeight
  await renderer.init(canvas)
  renderer.setOnLayerDrag((idx, dx, dy) => {
    if (store.selectedLayerIndex !== idx) return
    store.nudgeSelectedLayer(dx, dy)
  })
  renderNow()

  resizeObserver = new ResizeObserver(() => {
    if (!container) return
    renderer.resize(container.clientWidth, container.clientHeight)
    renderNow()
  })
  resizeObserver.observe(container)
})

onUnmounted(() => {
  resizeObserver?.disconnect()
  renderer.destroy()
})

watch(
  () => [
    store.renderVersion,
    store.selectedFileIndex,
    store.selectedObjectPath,
    store.selectedLayerIndex,
    store.selectedLayerFrameIndex,
    store.selectedLayerPreviewOverride,
  ],
  () => nextTick(renderNow),
)
</script>

<style scoped>
.preview-shell {
  position: relative;
  width: 100%;
  height: 100%;
  background: #2a2a2a;
  overflow: hidden;
}

canvas {
  display: block;
  width: 100%;
  height: 100%;
  outline: none;
  image-rendering: pixelated;
  image-rendering: crisp-edges;
}

.hud {
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: 8px;
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  pointer-events: none;
}

.hud span {
  background: rgba(0, 0, 0, 0.65);
  color: #d1d5db;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 11px;
}

.hud-btn {
  pointer-events: auto;
  border: 1px solid #555;
  background: rgba(30, 30, 30, 0.9);
  color: #ddd;
  border-radius: 4px;
  padding: 4px 8px;
  font-size: 11px;
  cursor: pointer;
}
</style>
