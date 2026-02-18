<script setup lang="ts">
import { computed, ref, onUnmounted } from 'vue'

interface Props {
  label: string
  current: number
  max: number
  frameColor: string
  barBackColor: string
  bar1Color: string
  layers?: StatLayer[]
}

const props = defineProps<Props>()

interface StatLayer {
  min: number
  max: number
  color: string
}

interface LayerFill {
  key: string
  width: string
  color: string
  zIndex: number
}

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

const normalizedLayers = computed<StatLayer[]>(() => {
  if (!props.layers || props.layers.length === 0) return []

  return props.layers
    .map((layer) => {
      const min = Number(layer.min)
      const max = Number(layer.max)
      return {
        min: Number.isFinite(min) ? Math.floor(min) : 0,
        max: Number.isFinite(max) ? Math.floor(max) : 0,
        color: layer.color,
      }
    })
    .filter((layer) => layer.max > layer.min)
})

const layerFills = computed<LayerFill[]>(() => {
  if (normalizedLayers.value.length === 0) return []

  return normalizedLayers.value.map((layer, index) => {
    const span = layer.max - layer.min
    const progress = span <= 0 ? 0 : (currentValue.value - layer.min) / span
    const clampedPercent = Math.max(0, Math.min(100, progress * 100))

    return {
      key: `${layer.min}-${layer.max}-${index}`,
      width: `${clampedPercent}%`,
      color: layer.color,
      zIndex: index + 1,
    }
  })
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
    <template v-if="layerFills.length > 0">
      <div
        v-for="layer in layerFills"
        :key="layer.key"
        class="stat-bar-fill"
        :style="{ width: layer.width, backgroundColor: layer.color, zIndex: layer.zIndex }"
      ></div>
    </template>
    <div
      v-else
      class="stat-bar-fill"
      :style="{ width: `${percent}%`, backgroundColor: bar1Color }"
    ></div>
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
