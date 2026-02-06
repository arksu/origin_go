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
  display: inline-block;
  border: 4px solid;
  border-radius: 80%;
  border-color: #fff transparent #fff transparent;
  animation: button-loading-spinner 1.2s ease infinite;

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
  }
}

@keyframes button-loading-spinner {
  0% {
    transform: rotate(0);
    animation-timing-function: cubic-bezier(0.3, 0.055, 0.675, 0.19);
  }
  60% {
    transform: rotate(360deg);
    animation-timing-function: cubic-bezier(0.215, 0.61, 0.5, 1);
  }
  100% {
    transform: rotate(540deg);
  }
}
</style>
