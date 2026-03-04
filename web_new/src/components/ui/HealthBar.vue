<script setup lang="ts">
import { computed, ref, onUnmounted } from 'vue'

interface Props {
  mhp: number
  hhp: number
  shp: number
  frameColor?: string
  barBackColor?: string
  hhpColor?: string
  shpColor?: string
}

const props = withDefaults(defineProps<Props>(), {
  frameColor: '#bfa57a',
  barBackColor: '#1a0707',
  hhpColor: '#7a1f1f',
  shpColor: '#d94a3a',
})

const normalizedMhp = computed(() => {
  const value = Number(props.mhp)
  if (!Number.isFinite(value) || value <= 0) return 0
  return Math.floor(value)
})

function clampToMhp(value: number): number {
  if (normalizedMhp.value <= 0) return 0
  const normalized = Number(value)
  if (!Number.isFinite(normalized) || normalized <= 0) return 0
  return Math.max(0, Math.min(Math.floor(normalized), normalizedMhp.value))
}

const normalizedHhp = computed(() => clampToMhp(props.hhp))
const normalizedShp = computed(() => clampToMhp(props.shp))

const hhpPercent = computed(() => {
  if (normalizedMhp.value <= 0) return 0
  return (normalizedHhp.value / normalizedMhp.value) * 100
})

const shpPercent = computed(() => {
  if (normalizedMhp.value <= 0) return 0
  return (normalizedShp.value / normalizedMhp.value) * 100
})

const tooltipText = computed(() =>
  `HP: SHP ${normalizedShp.value} / HHP ${normalizedHhp.value} / MHP ${normalizedMhp.value}`
)

const tooltipVisible = ref(false)
const tooltipX = ref(0)
const tooltipY = ref(0)
let tooltipElement: HTMLDivElement | null = null

function createTooltip() {
  if (tooltipElement) return

  tooltipElement = document.createElement('div')
  tooltipElement.className = 'health-tooltip-global'
  tooltipElement.innerHTML = `<pre>${tooltipText.value}</pre>`
  document.body.appendChild(tooltipElement)
}

function removeTooltip() {
  if (!tooltipElement) return
  document.body.removeChild(tooltipElement)
  tooltipElement = null
}

function updateTooltipPosition() {
  if (!tooltipElement) return
  tooltipElement.innerHTML = `<pre>${tooltipText.value}</pre>`
  tooltipElement.style.left = `${tooltipX.value}px`
  tooltipElement.style.top = `${tooltipY.value}px`
}

function handleMouseEnter(event: MouseEvent) {
  tooltipVisible.value = true
  tooltipX.value = event.clientX + 10
  tooltipY.value = event.clientY - 10
  createTooltip()
  updateTooltipPosition()
}

function handleMouseMove(event: MouseEvent) {
  if (!tooltipVisible.value) return
  tooltipX.value = event.clientX + 10
  tooltipY.value = event.clientY - 10
  updateTooltipPosition()
}

function handleMouseLeave() {
  tooltipVisible.value = false
  removeTooltip()
}

onUnmounted(() => {
  removeTooltip()
})
</script>

<template>
  <div
    class="health-frame"
    :style="{ borderColor: frameColor }"
    @mouseenter="handleMouseEnter"
    @mouseleave="handleMouseLeave"
    @mousemove="handleMouseMove"
  >
    <div class="health-back" :style="{ backgroundColor: barBackColor }"></div>
    <div class="health-fill health-fill--hhp" :style="{ width: `${hhpPercent}%`, backgroundColor: hhpColor }"></div>
    <div class="health-fill health-fill--shp" :style="{ width: `${shpPercent}%`, backgroundColor: shpColor }"></div>
  </div>
</template>

<style scoped lang="scss">
.health-frame {
  box-sizing: content-box;
  position: relative;
  height: 5px;
  width: 120px;
  border: 2px solid;
  border-radius: 10px;
  pointer-events: auto;
}

.health-back {
  position: absolute;
  width: 100%;
  height: 5px;
  border-radius: 10px;
}

.health-fill {
  position: absolute;
  height: 5px;
  border-radius: 10px;
}

.health-fill--hhp {
  z-index: 1;
}

.health-fill--shp {
  z-index: 2;
}
</style>

<style lang="scss">
.health-tooltip-global {
  position: fixed;
  transform: translateY(-100%);
  background: rgba(0, 0, 0, 0.75);
  color: #ffffff;
  border: 2px solid #555;
  border-radius: 8px;
  padding: 4px 8px;
  font-size: 12px;
  white-space: pre-wrap;
  z-index: 999999;
  pointer-events: none;
  max-width: 260px;
  word-wrap: break-word;
}
</style>
