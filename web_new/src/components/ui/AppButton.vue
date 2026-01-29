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
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  font-weight: 500;
  border-radius: 6px;
  transition: all 0.2s;
  cursor: pointer;

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  &--primary {
    background-color: #42b883;
    color: #fff;

    &:hover:not(:disabled) {
      background-color: #3aa876;
    }
  }

  &--secondary {
    background-color: #3a3a5c;
    color: #e0e0e0;

    &:hover:not(:disabled) {
      background-color: #4a4a6c;
    }
  }

  &--danger {
    background-color: #e74c3c;
    color: #fff;

    &:hover:not(:disabled) {
      background-color: #c0392b;
    }
  }

  &--sm {
    padding: 0.375rem 0.75rem;
    font-size: 0.875rem;
  }

  &--md {
    padding: 0.5rem 1rem;
    font-size: 1rem;
  }

  &--lg {
    padding: 0.75rem 1.5rem;
    font-size: 1.125rem;
  }

  &__spinner {
    width: 1em;
    height: 1em;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.75s linear infinite;
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
