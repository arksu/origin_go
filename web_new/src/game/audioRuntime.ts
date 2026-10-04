import { useGameStore } from '@/stores/gameStore'
import { timeSync } from '@/network/TimeSync'
import { soundManager } from './SoundManager'
import { LocalAudioController } from './LocalAudioController'
import { WorldAudioReceiver } from './WorldAudioReceiver'
import { DEFAULT_FOOTSTEP_SOUND_KEY, validateFootstepSoundProfiles } from './footstepConfig'

export const localAudioController = new LocalAudioController(soundManager)
export const worldAudioReceiver = new WorldAudioReceiver(soundManager,
  () => timeSync.estimateServerNowMs(), () => useGameStore().playerPosition)

export async function initializeAudio(): Promise<void> {
  const catalog = await soundManager.initialize()
  if (catalog.locomotionAudio?.some(binding => binding.contacts.some(contact => contact.sound_key === DEFAULT_FOOTSTEP_SOUND_KEY))) {
    validateFootstepSoundProfiles(catalog.sounds ?? {})
  }
  localAudioController.configure(catalog.locomotionAudio ?? [], catalog.actionAnimations)
}
