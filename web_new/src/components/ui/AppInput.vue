<script setup lang="ts">
import { ref } from 'vue'

interface Props {
  modelValue: string
  type?: 'text' | 'password' | 'email'
  placeholder?: string
  label?: string
  error?: string
  disabled?: boolean
  autocomplete?: string
}

const inputRef = ref<HTMLInputElement>()

withDefaults(defineProps<Props>(), {
  type: 'text',
  placeholder: '',
  label: '',
  error: '',
  disabled: false,
  autocomplete: 'off',
})

defineEmits<{
  'update:modelValue': [value: string]
}>()

function focus() {
  inputRef.value?.focus()
}

function blur() {
  inputRef.value?.blur()
}

// Expose methods to parent
defineExpose({
  focus,
  blur
})
</script>

<template>
  <div class="app-input" :class="{ 'app-input--error': error }">
    <label v-if="label" class="app-input__label">{{ label }}</label>
    <input
      ref="inputRef"
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :autocomplete="autocomplete"
      class="app-input__field"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
    <span v-if="error" class="app-input__error">{{ error }}</span>
  </div>
</template>

<style scoped lang="scss">
.app-input {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  width: 100%;

  &__label {
    font-size: 0.875rem;
    color: #a0a0a0;
  }

  &__field {
    padding: 0.625rem 0.875rem;
    font-size: 1rem;
    border: 1px solid #3a3a5c;
    border-radius: 6px;
    background-color: #16213e;
    color: #e0e0e0;
    transition: border-color 0.2s;

    &:focus {
      outline: none;
      border-color: #42b883;
    }

    &:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }
  }

  &--error &__field {
    border-color: #e74c3c;
  }

  &__error {
    font-size: 0.75rem;
    color: #e74c3c;
  }
}
</style>
