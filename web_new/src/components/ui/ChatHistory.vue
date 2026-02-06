<script setup lang="ts">
import { computed, watch, nextTick, ref, onMounted, onUnmounted } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import { CHAT_MESSAGE_LIFETIME_MS, CHAT_FADEOUT_DURATION_MS } from '@/constants/chat'

const gameStore = useGameStore()
const chatHistoryRef = ref<HTMLElement>()

// Reactive time for smooth animations
const currentTime = ref(Date.now())
let animationFrame: number | null = null

// Calculate opacity based on message age
function getMessageOpacity(timestamp: number): number {
  const age = currentTime.value - timestamp
  
  if (age < CHAT_MESSAGE_LIFETIME_MS) {
    // Full visibility during lifetime
    return 1
  } else {
    // Fade out over fadeout duration
    const fadeAge = age - CHAT_MESSAGE_LIFETIME_MS
    return Math.max(0, 1 - (fadeAge / CHAT_FADEOUT_DURATION_MS))
  }
}

// Update current time for smooth animations using requestAnimationFrame
function updateTime() {
  currentTime.value = Date.now()
  animationFrame = requestAnimationFrame(updateTime)
}

// Start animation when component mounts
onMounted(() => {
  animationFrame = requestAnimationFrame(updateTime)
})

// Cleanup animation when component unmounts
onUnmounted(() => {
  if (animationFrame) {
    cancelAnimationFrame(animationFrame)
    animationFrame = null
  }
})

// Filter and sort messages
const visibleMessages = computed(() => {
  return gameStore.chatMessages
    .slice(-10) // Take 10 most recent messages (newest at bottom)
    .filter(message => {
      const age = currentTime.value - message.timestamp
      const totalLifetime = CHAT_MESSAGE_LIFETIME_MS + CHAT_FADEOUT_DURATION_MS
      return age < totalLifetime // Only show messages that haven't completely faded out
    })
})

// Auto-scroll to bottom when new messages arrive
watch(visibleMessages, async () => {
  await nextTick()
  if (chatHistoryRef.value) {
    chatHistoryRef.value.scrollTop = chatHistoryRef.value.scrollHeight
  }
}, { flush: 'post' })
</script>

<template>
  <div ref="chatHistoryRef" class="chat-history">
    <div
      v-for="message in visibleMessages"
      :key="message.id"
      class="chat-message"
      :style="{ opacity: getMessageOpacity(message.timestamp) }"
    >
      <span class="chat-message__name">{{ message.fromName }}:</span>
      <span class="chat-message__text">{{ message.text }}</span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.chat-history {
  display: flex;
  flex-direction: column;
  pointer-events: none;
  overflow-y: auto;
  overflow-x: hidden;
  max-height: 300px;
  margin-bottom: 0.5rem;

  scrollbar-width: none;
  -ms-overflow-style: none;

  &::-webkit-scrollbar {
    display: none;
  }
}

.chat-message {
  font-size: 16px;
  line-height: 1.4;
  color: #daedfa;
  transition: opacity 0.3s ease-out;
  text-shadow:
    2px 2px 0 #000,
    -1px -1px 0 #000,
    1px -1px 0 #000,
    -1px 1px 0 #000,
    1px 1px 0 #000;
  word-wrap: break-word;

  &__name {
    font-weight: 500;
  }

  &__text {
    color: #daedfa;
  }
}
</style>
