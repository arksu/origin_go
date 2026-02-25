<template>
  <div class="panel-wrap">
    <h3>Shadow Paint</h3>

    <div v-if="!store.canPaintSelectedShadow" class="empty">
      Select a `shadow: true` layer to paint. Use “Add Shadow Layer” in the layer panel if needed.
    </div>

    <template v-else>
      <div class="controls">
        <label>
          Alpha
          <input v-model.number="alpha" type="range" min="0" max="255" step="1" />
          <span>{{ alpha }}</span>
        </label>
        <label>
          Brush
          <input v-model.number="brushSize" type="range" min="1" max="64" step="1" />
          <span>{{ brushSize }}</span>
        </label>
      </div>

      <div class="size-controls">
        <label>
          W
          <input v-model.number="resizeWidth" type="number" min="1" step="1" />
        </label>
        <label>
          H
          <input v-model.number="resizeHeight" type="number" min="1" step="1" />
        </label>
        <button class="small-btn" @click="applyResize">Resize Canvas</button>
      </div>

      <div class="target-row">
        <label>Target PNG</label>
        <input v-model="targetRelPath" type="text" @change="commitTargetRelPath" />
      </div>

      <div
        class="canvas-wrap"
        :class="{ panning: isPanningCanvas }"
        @wheel.prevent="onWheelZoom"
        @pointerdown="onWrapPointerDown"
        @pointermove="onWrapPointerMove"
        @pointerup="onWrapPointerUp"
        @pointerleave="onWrapPointerUp"
      >
        <canvas
          ref="canvasRef"
          class="paint-canvas"
          :style="canvasStyle"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointerleave="onPointerUp"
        />
      </div>

      <div class="actions">
        <button class="small-btn" @click="reloadFromOriginal">Reset To Original</button>
        <button class="small-btn warn" @click="clearToTransparent">Clear (Scratch)</button>
        <button class="small-btn" @click="refreshSuggestion">Suggest Filename</button>
        <button class="small-btn" @click="resetZoom">Zoom {{ zoom.toFixed(2) }}x</button>
      </div>

      <div class="hint">
        Paint sets pixel alpha directly (no opacity stacking on repeated movement in the same stroke).
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useObjectEditorStore } from '@/stores/objectEditorStore'
import { paintStrokeConstantAlpha } from '@/utils/shadowPaint'

const store = useObjectEditorStore()
const canvasRef = ref<HTMLCanvasElement | null>(null)
const alpha = ref(90)
const brushSize = ref(8)
const targetRelPath = ref('')
const zoom = ref(4)
const canvasPixelWidth = ref(0)
const canvasPixelHeight = ref(0)
const resizeWidth = ref(0)
const resizeHeight = ref(0)
const canvasOffsetX = ref(0)
const canvasOffsetY = ref(0)
const isPanningCanvas = ref(false)

let ctx: CanvasRenderingContext2D | null = null
let originalPixels: Uint8ClampedArray | null = null
let originalWidth = 0
let originalHeight = 0
let workingPixels: Uint8ClampedArray | null = null
let imageWidth = 0
let imageHeight = 0
let isPainting = false
let lastPoint: { x: number; y: number } | null = null
let visitedPixels = new Set<number>()
let panStartClientX = 0
let panStartClientY = 0
let panStartOffsetX = 0
let panStartOffsetY = 0

const sourcePath = computed(() => store.getSelectedShadowLayerSourcePath())
const fallbackSourcePath = computed(() => store.getSelectedShadowFallbackSourcePath())
const canvasStyle = computed(() => ({
  width: `${Math.max(1, Math.round(canvasPixelWidth.value * zoom.value))}px`,
  height: `${Math.max(1, Math.round(canvasPixelHeight.value * zoom.value))}px`,
  transform: `translate(${canvasOffsetX.value}px, ${canvasOffsetY.value}px)`,
}))

watch(
  () => [store.selectedFileName, store.selectedObjectPath, store.selectedLayerIndex, sourcePath.value] as const,
  async () => {
    await loadCanvasForSelection()
  },
  { immediate: true },
)

watch(
  () => store.selectedShadowDraft,
  (draft) => {
    if (!draft) return
    alpha.value = draft.alpha
    brushSize.value = draft.brushSize
    targetRelPath.value = draft.targetRelPath
  },
)

async function loadCanvasForSelection(): Promise<void> {
  isPainting = false
  isPanningCanvas.value = false
  lastPoint = null
  visitedPixels = new Set()
  originalWidth = 0
  originalHeight = 0
  resizeWidth.value = 0
  resizeHeight.value = 0
  canvasOffsetX.value = 0
  canvasOffsetY.value = 0

  if (!store.canPaintSelectedShadow || !sourcePath.value) {
    clearCanvas()
    return
  }

  const draft = store.getSelectedShadowDraft()
  alpha.value = draft?.alpha ?? 90
  brushSize.value = draft?.brushSize ?? 8
  targetRelPath.value = draft?.targetRelPath ?? store.suggestShadowRelPathForSelected() ?? ''

  try {
    await initializeShadowCanvasPixels(draft?.dataUrl)

    await nextTick()
    setupCanvas()
    drawWorkingPixels()
  } catch (error) {
    console.warn('[ShadowPaintPanel] Failed to prepare shadow paint canvas', error)
    clearCanvas()
  }
}

async function initializeShadowCanvasPixels(draftDataUrl?: string): Promise<void> {
  const sourceRel = sourcePath.value
  const fallbackRel = fallbackSourcePath.value

  if (draftDataUrl) {
    const visible = await decodeImageToPixels(draftDataUrl)
    imageWidth = visible.width
    imageHeight = visible.height
    workingPixels = new Uint8ClampedArray(visible.pixels)
    resizeWidth.value = imageWidth
    resizeHeight.value = imageHeight

    try {
      if (sourceRel) {
        const original = await decodeImageToPixels(`/assets/game/${sourceRel}`)
        if (original.width === imageWidth && original.height === imageHeight) {
          originalPixels = new Uint8ClampedArray(original.pixels)
          originalWidth = original.width
          originalHeight = original.height
          return
        }
      }
    } catch {
      // fall back to transparent original with draft dimensions
    }
    originalPixels = createTransparentPixels(imageWidth, imageHeight)
    originalWidth = imageWidth
    originalHeight = imageHeight
    return
  }

  if (sourceRel) {
    try {
      const original = await decodeImageToPixels(`/assets/game/${sourceRel}`)
      imageWidth = original.width
      imageHeight = original.height
      originalPixels = new Uint8ClampedArray(original.pixels)
      workingPixels = new Uint8ClampedArray(original.pixels)
      originalWidth = imageWidth
      originalHeight = imageHeight
      resizeWidth.value = imageWidth
      resizeHeight.value = imageHeight
      return
    } catch {
      // Missing target shadow image is allowed: use fallback size and start transparent
    }
  }

  if (fallbackRel) {
    const fallback = await decodeImageToPixels(`/assets/game/${fallbackRel}`)
    imageWidth = fallback.width
    imageHeight = fallback.height
    originalPixels = createTransparentPixels(imageWidth, imageHeight)
    workingPixels = createTransparentPixels(imageWidth, imageHeight)
    originalWidth = imageWidth
    originalHeight = imageHeight
    resizeWidth.value = imageWidth
    resizeHeight.value = imageHeight
    return
  }

  throw new Error('No source or fallback image available to determine shadow canvas size')
}

function createTransparentPixels(width: number, height: number): Uint8ClampedArray {
  return new Uint8ClampedArray(width * height * 4)
}

async function decodeImageToPixels(url: string): Promise<{ width: number; height: number; pixels: Uint8ClampedArray }> {
  const img = new Image()
  img.crossOrigin = 'anonymous'
  img.src = url
  await new Promise<void>((resolve, reject) => {
    img.onload = () => resolve()
    img.onerror = () => reject(new Error(`Failed to load image: ${url}`))
  })
  const canvas = document.createElement('canvas')
  canvas.width = img.naturalWidth || img.width
  canvas.height = img.naturalHeight || img.height
  const localCtx = canvas.getContext('2d')
  if (!localCtx) throw new Error('2D context not available')
  localCtx.clearRect(0, 0, canvas.width, canvas.height)
  localCtx.drawImage(img, 0, 0)
  const data = localCtx.getImageData(0, 0, canvas.width, canvas.height)
  return { width: canvas.width, height: canvas.height, pixels: new Uint8ClampedArray(data.data) }
}

function setupCanvas(): void {
  const canvas = canvasRef.value
  if (!canvas || imageWidth <= 0 || imageHeight <= 0) return
  canvas.width = imageWidth
  canvas.height = imageHeight
  canvasPixelWidth.value = imageWidth
  canvasPixelHeight.value = imageHeight
  ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('Canvas 2D context unavailable')
  ctx.imageSmoothingEnabled = false
}

function drawWorkingPixels(): void {
  if (!ctx || !workingPixels) return
  const imageData = new ImageData(new Uint8ClampedArray(workingPixels), imageWidth, imageHeight)
  ctx.putImageData(imageData, 0, 0)
}

function clearCanvas(): void {
  const canvas = canvasRef.value
  if (canvas) {
    const localCtx = canvas.getContext('2d')
    localCtx?.clearRect(0, 0, canvas.width, canvas.height)
  }
  ctx = null
  originalPixels = null
  originalWidth = 0
  originalHeight = 0
  workingPixels = null
  imageWidth = 0
  imageHeight = 0
  canvasPixelWidth.value = 0
  canvasPixelHeight.value = 0
  canvasOffsetX.value = 0
  canvasOffsetY.value = 0
}

function resizePixelBuffer(
  source: Uint8ClampedArray,
  sourceWidth: number,
  sourceHeight: number,
  targetWidth: number,
  targetHeight: number,
): Uint8ClampedArray {
  const out = createTransparentPixels(targetWidth, targetHeight)
  const copyWidth = Math.min(sourceWidth, targetWidth)
  const copyHeight = Math.min(sourceHeight, targetHeight)

  for (let y = 0; y < copyHeight; y++) {
    for (let x = 0; x < copyWidth; x++) {
      const srcIndex = (y * sourceWidth + x) * 4
      const dstIndex = (y * targetWidth + x) * 4
      out[dstIndex] = source[srcIndex] ?? 0
      out[dstIndex + 1] = source[srcIndex + 1] ?? 0
      out[dstIndex + 2] = source[srcIndex + 2] ?? 0
      out[dstIndex + 3] = source[srcIndex + 3] ?? 0
    }
  }

  return out
}

function pointerToPixel(event: PointerEvent): { x: number; y: number } | null {
  const canvas = canvasRef.value
  if (!canvas || imageWidth <= 0 || imageHeight <= 0) return null
  const rect = canvas.getBoundingClientRect()
  if (rect.width === 0 || rect.height === 0) return null
  const x = ((event.clientX - rect.left) / rect.width) * imageWidth
  const y = ((event.clientY - rect.top) / rect.height) * imageHeight
  return { x, y }
}

function onPointerDown(event: PointerEvent): void {
  if (!store.canPaintSelectedShadow || !workingPixels) return
  const point = pointerToPixel(event)
  if (!point) return
  isPainting = true
  lastPoint = point
  visitedPixels = new Set()
  paintAt(point, point)
  ;(event.target as HTMLElement).setPointerCapture?.(event.pointerId)
}

function onPointerMove(event: PointerEvent): void {
  if (!isPainting || !workingPixels || !lastPoint) return
  const point = pointerToPixel(event)
  if (!point) return
  paintAt(lastPoint, point)
  lastPoint = point
}

function onPointerUp(): void {
  isPainting = false
  lastPoint = null
  visitedPixels = new Set()
}

function onWrapPointerDown(event: PointerEvent): void {
  const canvas = canvasRef.value
  if (!canvas) return
  if (event.target === canvas) return
  isPanningCanvas.value = true
  panStartClientX = event.clientX
  panStartClientY = event.clientY
  panStartOffsetX = canvasOffsetX.value
  panStartOffsetY = canvasOffsetY.value
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  event.preventDefault()
}

function onWrapPointerMove(event: PointerEvent): void {
  if (!isPanningCanvas.value) return
  canvasOffsetX.value = panStartOffsetX + (event.clientX - panStartClientX)
  canvasOffsetY.value = panStartOffsetY + (event.clientY - panStartClientY)
}

function onWrapPointerUp(): void {
  isPanningCanvas.value = false
}

function paintAt(from: { x: number; y: number }, to: { x: number; y: number }): void {
  if (!workingPixels) return
  paintStrokeConstantAlpha(workingPixels, {
    width: imageWidth,
    height: imageHeight,
    radius: Math.max(1, brushSize.value),
    alpha: Math.max(0, Math.min(255, alpha.value)),
    from,
    to,
    visited: visitedPixels,
  })
  drawWorkingPixels()
  commitDraftToStore()
}

function commitDraftToStore(): void {
  const canvas = canvasRef.value
  if (!canvas || !workingPixels || imageWidth <= 0 || imageHeight <= 0) return
  store.upsertSelectedShadowDraft({
    dataUrl: canvas.toDataURL('image/png'),
    width: imageWidth,
    height: imageHeight,
    targetRelPath: targetRelPath.value.trim() || undefined,
    alpha: alpha.value,
    brushSize: brushSize.value,
  })
}

function reloadFromOriginal(): void {
  if (!originalPixels) return
  imageWidth = originalWidth || imageWidth
  imageHeight = originalHeight || imageHeight
  resizeWidth.value = imageWidth
  resizeHeight.value = imageHeight
  workingPixels = new Uint8ClampedArray(originalPixels)
  setupCanvas()
  drawWorkingPixels()
  store.resetSelectedShadowDraft()
  targetRelPath.value = store.suggestShadowRelPathForSelected() ?? targetRelPath.value
}

function clearToTransparent(): void {
  if (imageWidth <= 0 || imageHeight <= 0) return
  workingPixels = createTransparentPixels(imageWidth, imageHeight)
  setupCanvas()
  drawWorkingPixels()
  commitDraftToStore()
}

function onWheelZoom(event: WheelEvent): void {
  if (canvasPixelWidth.value <= 0 || canvasPixelHeight.value <= 0) return
  const step = event.deltaY > 0 ? -0.25 : 0.25
  zoom.value = Math.min(32, Math.max(1, Number((zoom.value + step).toFixed(2))))
}

function resetZoom(): void {
  zoom.value = 4
}

function applyResize(): void {
  if (!workingPixels || imageWidth <= 0 || imageHeight <= 0) return

  const nextWidth = Math.max(1, Math.floor(Number(resizeWidth.value)))
  const nextHeight = Math.max(1, Math.floor(Number(resizeHeight.value)))
  if (!Number.isFinite(nextWidth) || !Number.isFinite(nextHeight)) return
  if (nextWidth === imageWidth && nextHeight === imageHeight) return

  workingPixels = resizePixelBuffer(workingPixels, imageWidth, imageHeight, nextWidth, nextHeight)
  imageWidth = nextWidth
  imageHeight = nextHeight
  resizeWidth.value = nextWidth
  resizeHeight.value = nextHeight
  setupCanvas()
  drawWorkingPixels()
  commitDraftToStore()
}

function refreshSuggestion(): void {
  const suggested = store.suggestShadowRelPathForSelected()
  if (suggested) {
    targetRelPath.value = suggested
    commitTargetRelPath()
  }
}

function commitTargetRelPath(): void {
  const next = targetRelPath.value.trim()
  if (!next) return
  store.setSelectedShadowTargetRelPath(next)
}

onBeforeUnmount(() => {
  clearCanvas()
})
</script>

<style scoped>
.panel-wrap {
  padding: 8px;
  border-top: 1px solid #333;
  background: #252525;
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

h3 {
  margin: 0 0 8px;
  font-size: 12px;
  color: #aaa;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.empty {
  color: #777;
  font-size: 12px;
  padding: 8px 0;
}

.controls {
  display: grid;
  grid-template-columns: 1fr;
  gap: 6px;
  margin-bottom: 8px;
}

.controls label {
  display: grid;
  grid-template-columns: 52px 1fr 44px;
  align-items: center;
  gap: 8px;
  color: #ccc;
  font-size: 12px;
}

.size-controls {
  display: grid;
  grid-template-columns: 92px 92px 1fr;
  gap: 8px;
  align-items: end;
  margin-bottom: 8px;
}

.size-controls label {
  display: grid;
  grid-template-columns: 14px 1fr;
  gap: 6px;
  align-items: center;
  color: #ccc;
  font-size: 12px;
}

.size-controls input {
  min-width: 0;
  background: #1d1d1d;
  border: 1px solid #555;
  color: #ddd;
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 12px;
}

.target-row {
  display: grid;
  grid-template-columns: 72px 1fr;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  font-size: 12px;
  color: #ccc;
}

.target-row input {
  min-width: 0;
  background: #1d1d1d;
  border: 1px solid #555;
  color: #ddd;
  border-radius: 4px;
  padding: 4px 6px;
  font-size: 12px;
}

.canvas-wrap {
  border: 1px solid #333;
  border-radius: 6px;
  background:
    linear-gradient(45deg, #2d2d2d 25%, transparent 25%),
    linear-gradient(-45deg, #2d2d2d 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, #2d2d2d 75%),
    linear-gradient(-45deg, transparent 75%, #2d2d2d 75%);
  background-size: 12px 12px;
  background-position: 0 0, 0 6px, 6px -6px, -6px 0;
  overflow: auto;
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding: 8px;
  cursor: grab;
}

.canvas-wrap.panning {
  cursor: grabbing;
}

.paint-canvas {
  image-rendering: pixelated;
  image-rendering: crisp-edges;
  cursor: crosshair;
  flex-shrink: 0;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: transparent;
  transform-origin: top left;
}

.actions {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}

.small-btn {
  border: 1px solid #555;
  background: #333;
  color: #ddd;
  border-radius: 4px;
  padding: 5px 8px;
  font-size: 12px;
  cursor: pointer;
}

.small-btn.warn {
  border-color: #7f1d1d;
  color: #fecaca;
  background: #351919;
}

.small-btn.warn:hover {
  background: #4a1f1f;
}

.hint {
  margin-top: 6px;
  color: #888;
  font-size: 11px;
}
</style>
