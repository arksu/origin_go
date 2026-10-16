<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  text: string
  x: number
  y: number
}>()

const tooltipElement = ref<HTMLElement | null>(null)
const left = ref(props.x)
const top = ref(props.y)

function updatePosition(): void {
  if (typeof window === 'undefined' || !tooltipElement.value) return
  const bounds = tooltipElement.value.getBoundingClientRect()
  left.value = Math.max(4, Math.min(props.x, window.innerWidth - bounds.width - 4))
  top.value = Math.max(4, Math.min(props.y, window.innerHeight - bounds.height - 4))
}

watch([() => props.text, () => props.x, () => props.y], updatePosition, { flush: 'post' })
onMounted(() => {
  updatePosition()
  if (typeof window !== 'undefined') window.addEventListener('resize', updatePosition)
})
onBeforeUnmount(() => {
  if (typeof window !== 'undefined') window.removeEventListener('resize', updatePosition)
})
</script>

<template>
  <Teleport to="body">
    <div
      ref="tooltipElement"
      class="action-tooltip"
      role="tooltip"
      :style="{ left: `${left}px`, top: `${top}px` }"
    >
      {{ text }}
    </div>
  </Teleport>
</template>

<style scoped>
.action-tooltip {
  position: fixed;
  box-sizing: border-box;
  width: max-content;
  max-width: calc(100vw - 8px);
  padding: 4px 8px;
  border: 1px solid rgba(217, 199, 155, 0.8);
  border-radius: 6px;
  background: rgba(11, 16, 22, 0.94);
  color: #e8ecf1;
  font-size: 11px;
  line-height: 1.2;
  white-space: pre-line;
  pointer-events: none;
  z-index: 1000;
}
</style>
