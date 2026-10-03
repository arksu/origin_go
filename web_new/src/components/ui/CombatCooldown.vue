<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useGameStore } from '@/stores/gameStore'
import { timeSync } from '@/network/TimeSync'
const props = defineProps<{ actionId: string }>()
const store = useGameStore(), now = ref(timeSync.estimateServerNowMs())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => { timer = setInterval(() => { now.value = timeSync.estimateServerNowMs() }, 100) })
onUnmounted(() => { if (timer) clearInterval(timer) })
const seconds = computed(() => Math.max(0, ((store.combat.cooldowns.get(props.actionId.replace(/^game:/, '')) ?? 0) - now.value) / 1000))
</script>
<template><span v-if="seconds > 0" class="combat-cooldown" aria-label="Cooldown">{{ seconds.toFixed(1) }}s</span></template>
<style scoped>
.combat-cooldown { position: absolute; right: 2px; top: 2px; padding: 1px 3px; border-radius: 3px; color: #ffe1a2; background: #17212be6; font-size: 11px; pointer-events: none; }
</style>
