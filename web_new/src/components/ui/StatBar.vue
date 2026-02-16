<script setup lang="ts">
import { computed } from 'vue'

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
</script>

<template>
  <div
    class="stat-frame"
    :style="{ borderColor: frameColor }"
    :title="tooltipText"
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
