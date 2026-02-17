<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { proto } from '@/network/proto/packets.js'
import { sendMovementMode } from '@/network'
import { useGameStore } from '@/stores/gameStore'
import { MOVEMENT_MODE_ACTIVE_COLOR } from '@/constants/game'

interface ModeZone {
  id: proto.MovementMode
  title: string
  x: number
  width: number
}

const gameStore = useGameStore()
const activeMode = computed(() => gameStore.playerMoveMode)
const panelCanvas = ref<HTMLCanvasElement | null>(null)
const panelWidth = ref(168)
const panelHeight = ref(49)

let spriteImage: HTMLImageElement | null = null

// Tune these values manually for precise clickable zones over the sprite.
// Coordinates are in source image pixels.
const zones: ModeZone[] = [
  { id: proto.MovementMode.MOVE_MODE_CRAWL, title: 'Crawl', x: 0, width: 38 },
  { id: proto.MovementMode.MOVE_MODE_WALK, title: 'Walk', x: 42, width: 30 },
  { id: proto.MovementMode.MOVE_MODE_RUN, title: 'Run', x: 70, width: 42 },
  { id: proto.MovementMode.MOVE_MODE_FAST_RUN, title: 'Fast Run', x: 110, width: 45 },
]

function onModeClick(mode: proto.MovementMode): void {
  sendMovementMode(mode)
}

function parseHexColor(hex: string): [number, number, number] {
  const normalized = hex.replace('#', '')
  const value = normalized.length === 3
    ? normalized.split('').map((c) => c + c).join('')
    : normalized
  const num = Number.parseInt(value, 16)
  if (!Number.isFinite(num)) {
    return [247, 216, 111]
  }
  return [(num >> 16) & 255, (num >> 8) & 255, num & 255]
}

function activeModeToZone(mode: number | null): ModeZone | null {
  if (mode == null) return null
  return zones.find((zone) => zone.id === mode) || null
}

function tintZoneByPixels(ctx: CanvasRenderingContext2D, zone: ModeZone | null): void {
  if (!zone) return

  const width = ctx.canvas.width
  const height = ctx.canvas.height
  if (width <= 0 || height <= 0) return

  const zoneX = Math.max(0, Math.floor(zone.x))
  const zoneWidth = Math.max(1, Math.floor(zone.width))
  if (zoneX >= width) return
  const clampedWidth = Math.min(zoneWidth, width - zoneX)
  const [targetR, targetG, targetB] = parseHexColor(MOVEMENT_MODE_ACTIVE_COLOR)

  const imageData = ctx.getImageData(zoneX, 0, clampedWidth, height)
  const data = imageData.data
  for (let i = 0; i < data.length; i += 4) {
    const alpha = data[i + 3]
    if (alpha === 0) continue

    const r = data[i] ?? 0
    const g = data[i + 1] ?? 0
    const b = data[i + 2] ?? 0
    const luminance = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255
    const shade = 0.35 + (luminance * 0.65)

    data[i] = Math.round(targetR * shade)
    data[i + 1] = Math.round(targetG * shade)
    data[i + 2] = Math.round(targetB * shade)
  }
  ctx.putImageData(imageData, zoneX, 0)
}

function redraw(): void {
  if (!panelCanvas.value || !spriteImage) return
  const ctx = panelCanvas.value.getContext('2d', { willReadFrequently: true })
  if (!ctx) return

  ctx.clearRect(0, 0, panelCanvas.value.width, panelCanvas.value.height)
  ctx.drawImage(spriteImage, 0, 0, panelCanvas.value.width, panelCanvas.value.height)
  tintZoneByPixels(ctx, activeModeToZone(activeMode.value))
}

onMounted(() => {
  spriteImage = new Image()
  spriteImage.src = '/assets/img/movement_modes.png'
  spriteImage.onload = () => {
    if (!spriteImage) return
    panelWidth.value = spriteImage.naturalWidth || panelWidth.value
    panelHeight.value = spriteImage.naturalHeight || panelHeight.value
    if (panelCanvas.value) {
      panelCanvas.value.width = panelWidth.value
      panelCanvas.value.height = panelHeight.value
    }
    redraw()
  }
})

watch(activeMode, () => {
  redraw()
})

onBeforeUnmount(() => {
  if (spriteImage) {
    spriteImage.onload = null
  }
  spriteImage = null
})
</script>

<template>
  <div class="movement-mode-panel" aria-label="Movement mode" :style="{ width: `${panelWidth}px`, height: `${panelHeight}px` }">
    <canvas ref="panelCanvas" class="movement-mode-panel__canvas" aria-hidden="true" />
    <button
      v-for="zone in zones"
      :key="zone.id"
      type="button"
      class="mode-zone"
      :title="zone.title"
      :style="{ left: `${zone.x}px`, width: `${zone.width}px` }"
      @click="onModeClick(zone.id)"
    />
  </div>
</template>

<style scoped lang="scss">
.movement-mode-panel {
  position: relative;
  pointer-events: auto;
}

.movement-mode-panel__canvas {
  display: block;
  width: 100%;
  height: 100%;
  user-select: none;
  pointer-events: none;
}

.mode-zone {
  position: absolute;
  top: 0;
  bottom: 0;
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
  padding: 0;
}

.mode-zone:hover {
  border-color: rgba(238, 227, 188, 0.75);
  background: rgba(246, 232, 185, 0.2);
}
</style>
