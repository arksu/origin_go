import { defineStore } from 'pinia'
import { ref } from 'vue'

function clamp01(value: number): number {
  if (value < 0) return 0
  if (value > 1) return 1
  return value
}

export const useAudioSettingsStore = defineStore('audioSettings', () => {
  const enabled = ref(true)
  const masterVolume = ref(0.8)
  const sfxVolume = ref(1)

  function setEnabled(value: boolean) {
    enabled.value = value
  }

  function setMasterVolume(value: number) {
    masterVolume.value = clamp01(value)
  }

  function setSfxVolume(value: number) {
    sfxVolume.value = clamp01(value)
  }

  return {
    enabled,
    masterVolume,
    sfxVolume,
    setEnabled,
    setMasterVolume,
    setSfxVolume,
  }
})
