<script setup lang="ts">
import { ref, computed } from 'vue'
import AppInput from './AppInput.vue'
import AppButton from './AppButton.vue'

const message = ref('')
const inputRef = ref<InstanceType<typeof AppInput>>()
const emit = defineEmits<{
  send: [text: string]
}>()

const canSend = computed(() => message.value.trim().length > 0)

function handleSend() {
  const text = message.value.trim()
  if (text.length === 0) return
  
  emit('send', text)
  message.value = ''
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    handleSend()
  }
}

function focus() {
  inputRef.value?.focus()
}

function blur() {
  inputRef.value?.blur()
}

function focusWithSlash() {
  message.value = '/'
  focus()
}

// Expose methods to parent
defineExpose({
  focus,
  blur,
  focusWithSlash
})
</script>

<template>
  <div class="chat-input">
    <AppInput
      ref="inputRef"
      v-model="message"
      placeholder="Enter message..."
      @keydown="handleKeydown"
    />
    <AppButton
      variant="secondary"
      size="sm"
      :disabled="!canSend"
      @click="handleSend"
    >
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <line x1="22" y1="2" x2="11" y2="13"></line>
        <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
      </svg>
    </AppButton>
  </div>
</template>

<style scoped lang="scss">
.chat-input {
  display: flex;
  gap: 0.5rem;
  align-items: stretch;
  min-width: 280px;
  max-width: 400px;

  :deep(.app-input) {
    flex: 1;
    
    .app-input__field {
      height: 2.25rem; // Match button height
      padding: 0.5rem 0.75rem;
    }
  }

  .app-button {
    flex-shrink: 0;
    
    svg {
      flex-shrink: 0;
    }
  }
}
</style>
