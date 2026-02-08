<script setup lang="ts">
import { ref } from 'vue'

const MAX_HISTORY = 30

const message = ref('')
const inputRef = ref<HTMLInputElement>()
const history = ref<string[]>([])
let historyIndex = -1
let savedInput = ''
const emit = defineEmits<{
  send: [text: string]
}>()

function handleSend() {
  const text = message.value.trim()
  if (text.length === 0) return
  
  emit('send', text)
  history.value.push(text)
  if (history.value.length > MAX_HISTORY) {
    history.value.shift()
  }
  historyIndex = -1
  savedInput = ''
  message.value = ''
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    handleSend()
    return
  }

  const length = history.value.length
  if (event.key === 'ArrowUp') {
    if (length > 0 && historyIndex < length - 1) {
      if (historyIndex === -1) {
        savedInput = message.value
      }
      historyIndex++
      message.value = history.value[length - historyIndex - 1]
    }
    event.preventDefault()
  } else if (event.key === 'ArrowDown') {
    if (historyIndex > 0) {
      historyIndex--
      message.value = history.value[length - historyIndex - 1]
    } else if (historyIndex === 0) {
      historyIndex = -1
      message.value = savedInput
    }
    event.preventDefault()
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

defineExpose({
  focus,
  blur,
  focusWithSlash
})
</script>

<template>
  <form action="#" class="chat-form" @submit.prevent="handleSend">
    <div class="input-container">
      <input
        ref="inputRef"
        v-model="message"
        autocomplete="off"
        class="text-input"
        placeholder="Chat here"
        type="text"
        @keydown="handleKeydown"
      >
      <span class="submit-logo" @click="handleSend">
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
</span>
    </div>
  </form>
</template>

<style scoped lang="scss">
.chat-form {
  pointer-events: none;
}

.input-container {
  border: 2px solid #103c2ab5;
  border-radius: 6px;
  background-color: #7b917eb3;
  padding: 0.1em 0.4em;
  pointer-events: auto;
  white-space: nowrap;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.text-input {
  background: transparent;
  outline: none;
  border: none;
  width: 100%;
  font-size: 16px;
  pointer-events: auto;
  color: #17241d;
  font-family: inherit;
  letter-spacing: normal;
  margin-bottom: 0;

  &::placeholder {
    color: #446755;
  }
}

.submit-logo {
  color: #264a44;
  cursor: pointer;
  pointer-events: auto;
  display: flex;
  align-items: center;
  
  svg {
    width: 24px;
    height: 24px;
  }
}
</style>
