import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { proto } from '@/network/proto/packets.js'
import { runtimeTimeSync } from '@/network/RuntimeTimeSync'
import { parseItemProgress, presentItemProgress } from '@/utils/itemProgress'

export function useItemProgress(source: () => proto.IItemProgress | null | undefined) {
  const mounted = ref(false)
  const progress = computed(() => parseItemProgress(source()))
  const presentation = computed(() => presentItemProgress(progress.value,
    progress.value && !progress.value.paused ? runtimeTimeSync.getRuntimeMs() : null))
  let releaseClock: (() => void) | null = null

  watch(() => mounted.value && progress.value !== null && !progress.value.paused &&
    (presentation.value === null || presentation.value.remainingMs > 0n), active => {
    if (active && !releaseClock) releaseClock = runtimeTimeSync.subscribe()
    else if (!active && releaseClock) { releaseClock(); releaseClock = null }
  })

  onMounted(() => { mounted.value = true })
  onUnmounted(() => { releaseClock?.(); releaseClock = null })
  return presentation
}
