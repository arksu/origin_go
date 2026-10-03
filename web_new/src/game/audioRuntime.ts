import { useGameStore } from '@/stores/gameStore'
import { timeSync } from '@/network/TimeSync'
import { soundManager } from './SoundManager'
import { LocalAudioController } from './LocalAudioController'
import { WorldAudioReceiver } from './WorldAudioReceiver'

export const localAudioController = new LocalAudioController(soundManager)
export const worldAudioReceiver = new WorldAudioReceiver(soundManager,
  () => timeSync.estimateServerNowMs(), () => useGameStore().playerPosition)

export async function initializeAudio(): Promise<void> {
  const catalog = await soundManager.initialize()
  localAudioController.configure(catalog.locomotionAudio ?? [], catalog.actionAnimations)
}
