<script setup lang="ts">
interface Props {
  type?: 'error' | 'success' | 'warning' | 'info'
}

withDefaults(defineProps<Props>(), {
  type: 'info',
})
</script>

<template>
  <div :class="['app-alert', `app-alert--${type}`]">
    <slot />
  </div>
</template>

<style scoped lang="scss">
.app-alert {
  padding: 0.75rem 1rem;
  border-radius: 6px;
  font-size: 0.875rem;

  &--error {
    background-color: rgba(252, 13, 13, 0.5);
    color: #cdd2e0;
    border: none;
  }

  &--success {
    background-color: rgba(46, 204, 113, 0.15);
    color: #2ecc71;
    border: 1px solid rgba(46, 204, 113, 0.3);
  }

  &--warning {
    background-color: rgba(241, 196, 15, 0.15);
    color: #f1c40f;
    border: 1px solid rgba(241, 196, 15, 0.3);
  }

  &--info {
    background-color: rgba(52, 152, 219, 0.4);
    color: #ffffff;
    border: 1px solid rgba(52, 152, 219, 0.6);
    text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
  }
}
</style>
