<script setup lang="ts">
interface Props {
  variant?: 'primary' | 'secondary' | 'danger'
  size?: 'sm' | 'md' | 'lg'
  loading?: boolean
  disabled?: boolean
  type?: 'button' | 'submit' | 'reset'
}

withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  loading: false,
  disabled: false,
  type: 'button',
})
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="['app-button', `app-button--${variant}`, `app-button--${size}`]"
  >
    <span v-if="loading" class="app-button__spinner"></span>
    <slot v-else />
  </button>
</template>

<style scoped lang="scss">
.app-button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  font-weight: 500;
  border-radius: 6px;
  border: none;
  cursor: pointer;
  text-transform: uppercase;
  letter-spacing: 1.2px;
  transition-duration: 0.9s;

  &:disabled {
    transition-duration: 0.1s;
    background: rgb(86, 86, 91);
    color: #838383;
    box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);
    animation: none;
    cursor: not-allowed;
  }

  &--primary {
    background: #1a4f72;
    color: #fff;

    &:hover:not(:disabled) {
      transition-duration: 0.6s;
      background: rgb(34, 140, 190);
      box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);
      animation: btn-glow 0.6s ease-in-out infinite alternate;
    }
  }

  &--secondary {
    background: #105858aa;
    color: #d5eeed;
    border: 1px solid rgba(30, 67, 91, 0.6);

    &:hover:not(:disabled) {
      transition-duration: 0.6s;
      background: #228cbeff;
      box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);
    }
  }

  &--danger {
    background-color: rgba(174, 35, 39, 0.6);
    color: #fff;

    &:hover:not(:disabled) {
      background: rgba(220, 41, 48, 0.6);
    }
  }

  &--sm {
    padding: 0.375rem 0.75rem;
    font-size: 0.875rem;
  }

  &--md {
    padding: 10px 20px;
    font-size: 19px;
  }

  &--lg {
    padding: 0.75rem 1.5rem;
    font-size: 1.125rem;
  }

  &__spinner {
    position: absolute;
    width: 20px;
    height: 20px;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    margin: auto;
    border: 4px solid;
    border-radius: 80%;
    border-color: #fff transparent #fff transparent;
    animation: button-loading-spinner 1.2s ease infinite;
  }
}

@keyframes btn-glow {
  from {
    box-shadow: 0 0 10px rgba(52, 78, 88, 0.71);
  }
  to {
    box-shadow: 0 0 20px rgba(23, 180, 200, 0.71);
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
