<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import GameWindow from './GameWindow.vue'
import AppButton from './AppButton.vue'
import { useGameStore } from '@/stores/gameStore'
import { useActionCooldownStore } from '@/stores/actionCooldownStore'
import { sendStandUp } from '@/network'

const game = useGameStore()
const clock = useActionCooldownStore()
const sampledTime = ref(clock.serverNow())
const timer = setInterval(() => { sampledTime.value = clock.serverNow() }, 100)
onUnmounted(() => clearInterval(timer))
const secondsLeft = computed(() => sampledTime.value > 0
  ? Math.max(0, Math.ceil((game.playerStats.koUntilMs - sampledTime.value) / 1000))
  : null)
</script>

<template>
  <GameWindow :id="-60" title="Knockout" :inner-width="280" :inner-height="104"
              :closable="false" :center-on-open="true" :persist-position="false">
    <template v-if="game.playerStats.isKnockedOut">
      <p v-if="secondsLeft === null">Synchronizing time…</p>
      <p v-else-if="secondsLeft > 0">Knockout: {{ secondsLeft }} s</p>
      <p v-else>Waiting for server confirmation…</p>
    </template>
    <template v-else>
      <p>You are lying down.</p>
      <AppButton :disabled="!game.playerStats.canStandUp" @click="sendStandUp">Stand Up</AppButton>
    </template>
  </GameWindow>
</template>

<style scoped>
p { margin: 8px 0 14px; }
</style>
