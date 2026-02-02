<script setup lang="ts">
import { computed, watch, nextTick, ref, onMounted, onUnmounted } from 'vue'
import { useGameStore } from '@/stores/gameStore'

const gameStore = useGameStore()
const chatHistoryRef = ref<HTMLElement>()

// Constants
const CHAT_MESSAGE_LIFETIME_MS = 5000 // 5 seconds full visibility
const CHAT_FADEOUT_DURATION_MS = 1000  // 1 second fade out

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
  gap: 0.25rem;
  max-height: 300px; // Increased to fit 10 messages
  overflow-y: auto;
  overflow-x: hidden;
  pointer-events: none; // Allow clicks to pass through to game
  margin-bottom: 0.5rem; // Space between history and input
  
  // Hide scrollbar but keep functionality
  scrollbar-width: none; // Firefox
  -ms-overflow-style: none; // IE/Edge
  
  &::-webkit-scrollbar {
    display: none; // Chrome/Safari/Opera
  }
}

.chat-message {
  display: flex;
  gap: 0.5rem;
  align-items: flex-start;
  font-size: 0.875rem;
  line-height: 1.25;
  transition: opacity 0.3s ease-out; // Smooth transition
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.8); // Improve readability
  word-wrap: break-word;
  max-width: 400px;

  &__name {
    color: #42b883; // Green for names
    font-weight: 500;
    flex-shrink: 0;
  }

  &__text {
    color: #e0e0e0; // Light gray for text
    flex: 1;
  }
}
</style>
