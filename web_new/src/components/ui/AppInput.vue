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
$inactiveTextColor: #bebebe;
$textColor: #d5eeed;

.app-input {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  width: 100%;
  margin-bottom: 25px;

  &__label {
    font-size: 0.875rem;
    color: $inactiveTextColor;
  }

  &__field {
    padding: 8px 10px;
    width: 100%;
    outline: none;
    background: transparent;
    border: 0;
    border-bottom: 1px solid $inactiveTextColor;
    border-radius: 4px;
    font-size: 19px;
    letter-spacing: 1.6px;
    color: $textColor;
    transition: border-color 0.2s;

    &::placeholder {
      color: $inactiveTextColor;
    }

    &:focus {
      outline: none;
      border-bottom-color: #7ed0fc;
    }

    &:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }
  }

  &--error &__field {
    border-bottom-color: #e74c3c;
  }

  &__error {
    font-size: 0.75rem;
    color: #e74c3c;
  }
}
</style>
