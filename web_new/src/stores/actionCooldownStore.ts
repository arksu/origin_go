import { defineStore } from 'pinia'
import { onScopeDispose, ref } from 'vue'
import type { proto } from '@/network/proto/packets.js'
import { timeSync } from '@/network/TimeSync'
import { toNonNegativeProtoInt } from '@/utils/protoNumbers'

export interface ActionCooldown {
  startedAtMs: number
  expiresAtMs: number
}

export const useActionCooldownStore = defineStore('actionCooldowns', () => {
  const cooldowns = ref(new Map<string, ActionCooldown>())
  const nowMs = ref(0)
  let frame: number | null = null
  let packetServerMs = 0
  let packetReceivedMs = 0

  function serverNow(): number {
    if (timeSync.isInitialized()) return timeSync.estimateServerNowMs()
    return packetServerMs + performance.now() - packetReceivedMs
  }

  function stopClock(): void {
    if (frame !== null) cancelAnimationFrame(frame)
    frame = null
  }

  function updateClock(): void {
    frame = null
    nowMs.value = serverNow()
    if ([...cooldowns.value.values()].some(cooldown => cooldown.expiresAtMs > nowMs.value)) {
      frame = requestAnimationFrame(updateClock)
    }
  }

  function setSnapshot(snapshot: proto.IS2C_ActionStateChanged): void {
    stopClock()
    packetServerMs = toNonNegativeProtoInt(snapshot.serverTimeMs)
    packetReceivedMs = performance.now()
    const next = new Map<string, ActionCooldown>()
    for (const cooldown of snapshot.cooldowns || []) {
      const startedAtMs = toNonNegativeProtoInt(cooldown.startedAtMs)
      const expiresAtMs = toNonNegativeProtoInt(cooldown.expiresAtMs)
      if (!cooldown.actionId || !Number.isSafeInteger(startedAtMs) || !Number.isSafeInteger(expiresAtMs) || expiresAtMs <= startedAtMs) continue
      next.set(cooldown.actionId, { startedAtMs, expiresAtMs })
    }
    cooldowns.value = next
    updateClock()
  }

  // Input checks sample the clock directly even when a background tab paused RAF.
  function isCoolingDown(actionId: string): boolean {
    return (cooldowns.value.get(actionId)?.expiresAtMs || 0) > serverNow()
  }

  function progress(actionId: string): number {
    const cooldown = cooldowns.value.get(actionId)
    if (!cooldown) return 1
    return Math.max(0, Math.min(1, (nowMs.value - cooldown.startedAtMs) / (cooldown.expiresAtMs - cooldown.startedAtMs)))
  }

  function reset(): void {
    stopClock()
    cooldowns.value = new Map()
    nowMs.value = 0
    packetServerMs = 0
    packetReceivedMs = 0
  }

  onScopeDispose(reset)
  return { cooldowns, nowMs, setSnapshot, isCoolingDown, progress, reset }
})
