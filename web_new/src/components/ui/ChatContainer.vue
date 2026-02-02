<script setup lang="ts">
import ChatHistory from './ChatHistory.vue'
import ChatInput from './ChatInput.vue'

const emit = defineEmits<{
  send: [text: string]
}>()

function handleSend(text: string) {
  emit('send', text)
}
</script>

<template>
  <div class="chat-container">
    <ChatHistory />
    <ChatInput @send="handleSend" />
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
