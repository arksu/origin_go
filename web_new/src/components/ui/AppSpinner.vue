<script setup lang="ts">
interface Props {
  size?: 'sm' | 'md' | 'lg'
}

withDefaults(defineProps<Props>(), {
  size: 'md',
})
</script>

<template>
  <div :class="['app-spinner', `app-spinner--${size}`]"></div>
</template>

<style scoped lang="scss">
.app-spinner {
  border: 3px solid #3a3a5c;
  border-top-color: #42b883;
  border-radius: 50%;
  animation: spin 0.75s linear infinite;

  &--sm {
    width: 1rem;
    height: 1rem;
    border-width: 2px;
  }

  &--md {
    width: 2rem;
    height: 2rem;
  }

  &--lg {
    width: 3rem;
    height: 3rem;
    border-width: 4px;
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
