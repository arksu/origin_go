<script setup lang="ts">
import { computed, ref, onUnmounted } from 'vue'

interface Props {
  label: string
  current: number
  max: number
  frameColor: string
  barBackColor: string
  bar1Color: string
}

const props = defineProps<Props>()

const maxValue = computed(() => {
  const normalized = Number(props.max)
  if (!Number.isFinite(normalized) || normalized <= 0) return 0
  return normalized
})

const currentValue = computed(() => {
  const normalized = Number(props.current)
  if (!Number.isFinite(normalized) || normalized <= 0) return 0
  if (maxValue.value <= 0) return normalized
  return Math.min(normalized, maxValue.value)
})

const percent = computed(() => {
  if (maxValue.value <= 0) return 0
  return Math.max(0, Math.min(100, (currentValue.value / maxValue.value) * 100))
})

const tooltipText = computed(() => `${props.label}: ${currentValue.value}/${maxValue.value}`)

const tooltipVisible = ref(false)
const tooltipX = ref(0)
const tooltipY = ref(0)
let tooltipElement: HTMLDivElement | null = null

function createTooltip() {
  if (tooltipElement) return

  tooltipElement = document.createElement('div')
  tooltipElement.className = 'stat-tooltip-global'
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
  tooltipY.value = event.clientY + 10
  createTooltip()
  updateTooltipPosition()
}

function handleMouseMove(event: MouseEvent) {
  if (!tooltipVisible.value) return
  tooltipX.value = event.clientX + 10
  tooltipY.value = event.clientY + 10
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
    class="stat-frame"
    :style="{ borderColor: frameColor }"
    @mouseenter="handleMouseEnter"
    @mouseleave="handleMouseLeave"
    @mousemove="handleMouseMove"
  >
    <div class="stat-bar-back" :style="{ backgroundColor: barBackColor }"></div>
    <div class="stat-bar-fill" :style="{ width: `${percent}%`, backgroundColor: bar1Color }"></div>
  </div>
</template>

<style scoped lang="scss">
.stat-frame {
  box-sizing: content-box;
  position: relative;
  height: 5px;
  width: 120px;
  border: 2px solid;
  border-radius: 10px;
  pointer-events: auto;
}

.stat-bar-back {
  position: absolute;
  width: 100%;
  height: 5px;
  border-radius: 10px;
}

.stat-bar-fill {
  position: absolute;
  height: 5px;
  border-radius: 10px;
}
</style>

<style lang="scss">
.stat-tooltip-global {
  position: fixed;
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
