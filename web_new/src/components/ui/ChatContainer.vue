<script setup lang="ts">
import { ref } from 'vue'
import ChatHistory from './ChatHistory.vue'
import ChatInput from './ChatInput.vue'

const emit = defineEmits<{
  send: [text: string]
}>()

const chatInputRef = ref<InstanceType<typeof ChatInput>>()

function handleSend(text: string) {
  emit('send', text)
}

function focusChat() {
  chatInputRef.value?.focus()
}

function unfocusChat() {
  chatInputRef.value?.blur()
}

function focusChatWithSlash() {
  chatInputRef.value?.focusWithSlash()
}

// Expose methods to parent
defineExpose({
  focusChat,
  unfocusChat,
  focusChatWithSlash
})
</script>

<template>
  <div class="chat-container">
    <ChatHistory />
    <ChatInput ref="chatInputRef" @send="handleSend" />
  </div>
</template>

<style scoped lang="scss">
.chat-container {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  pointer-events: auto; // Re-enable for chat interaction
}
</style>
