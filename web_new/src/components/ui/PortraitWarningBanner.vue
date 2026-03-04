<script setup lang="ts">
const emit = defineEmits<{
  close: []
}>()
</script>

<template>
  <div class="portrait-warning" role="status" aria-live="polite">
    <span class="portrait-warning__text">Please use landscape mode</span>
    <button class="portrait-warning__close" type="button" aria-label="Close warning" @click="emit('close')">
      Close
    </button>
  </div>
</template>

<style scoped lang="scss">
.portrait-warning {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border: 1px solid rgba(231, 183, 86, 0.95);
  border-radius: 10px;
  background: rgba(32, 25, 10, 0.92);
  color: #ffe7a9;
  pointer-events: auto;
}

.portrait-warning__text {
  font-size: 13px;
  font-weight: 600;
}

.portrait-warning__close {
  border: 1px solid rgba(231, 183, 86, 0.9);
  border-radius: 8px;
  background: rgba(66, 51, 18, 0.95);
  color: #ffe7a9;
  font-size: 12px;
  font-weight: 600;
  padding: 4px 8px;
  cursor: pointer;
}
</style>
