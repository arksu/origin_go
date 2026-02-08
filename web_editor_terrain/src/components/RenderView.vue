<template>
  <div class="render-view" ref="containerRef">
    <canvas ref="canvasRef" tabindex="0" @keydown="onKeyDown"></canvas>
    <div class="hint" v-if="store.selectedLayerIndex >= 0">
      Layer [{{ store.selectedLayerIndex }}] — Arrow keys or drag to move
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useTerrainStore } from '@/stores/terrainStore'
import { TerrainRenderer } from '@/engine/TerrainRenderer'

const store = useTerrainStore()
const canvasRef = ref<HTMLCanvasElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const renderer = new TerrainRenderer()

let resizeObserver: ResizeObserver | null = null

function rerender(): void {
  const variant = store.selectedVariant
  if (!variant) return

  renderer.renderVariant(
    variant,
    (idx) => store.isLayerVisible(store.selectedVariantIndex, idx),
    (idx) => store.getLayerOffset(store.selectedVariantIndex, idx),
    store.selectedLayerIndex,
  )
}

function onKeyDown(e: KeyboardEvent): void {
  if (store.selectedLayerIndex < 0) return

  let dx = 0
  let dy = 0
  switch (e.key) {
    case 'ArrowUp':
      dy = -1
      break
    case 'ArrowDown':
      dy = 1
      break
    case 'ArrowLeft':
      dx = -1
      break
    case 'ArrowRight':
      dx = 1
      break
    default:
      return
  }
  e.preventDefault()
  store.moveLayer(store.selectedVariantIndex, store.selectedLayerIndex, dx, dy)
}

onMounted(async () => {
  const canvas = canvasRef.value
  const container = containerRef.value
  if (!canvas || !container) return

  canvas.width = container.clientWidth
  canvas.height = container.clientHeight

  try {
    await renderer.init(canvas)
    await renderer.loadSpritesheet('/assets/game/tiles.json')
  } catch (e) {
    console.error('[TerrainEditor] Failed to init renderer:', e)
    return
  }

  renderer.setOnLayerClick((layerIndex) => {
    store.selectLayer(layerIndex)
  })

  renderer.setOnDragMove((layerIndex, dx, dy) => {
    store.moveLayer(store.selectedVariantIndex, layerIndex, dx, dy)
  })

  rerender()

  resizeObserver = new ResizeObserver(() => {
    if (!canvas || !container) return
    canvas.width = container.clientWidth
    canvas.height = container.clientHeight
    renderer.resize(container.clientWidth, container.clientHeight)
    rerender()
  })
  resizeObserver.observe(container)
})

onUnmounted(() => {
  resizeObserver?.disconnect()
  renderer.destroy()
})

watch(
  () => [store.selectedFileIndex, store.selectedVariantIndex],
  () => nextTick(rerender),
)

watch(
  () => store.selectedLayerIndex,
  () => {
    renderer.updateSelection(store.selectedLayerIndex)
  },
)

watch(
  () => store.renderVersion,
  () => nextTick(rerender),
)
</script>

<style scoped>
.render-view {
  position: relative;
  width: 100%;
  height: 100%;
  background: #2a2a2a;
  overflow: hidden;
}

canvas {
  display: block;
  outline: none;
}

.hint {
  position: absolute;
  bottom: 8px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(0, 0, 0, 0.7);
  color: #4ade80;
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 12px;
  pointer-events: none;
}
</style>
