import sounds, { type SoundDef, type SoundRegistry } from './sounds'
import { Howl, Howler } from 'howler'
import { useAudioSettingsStore } from '@/stores/audioSettingsStore'

function clamp01(value: number): number {
  if (value < 0) return 0
  if (value > 1) return 1
  return value
}

export class SoundManager {
  private readonly soundDefs: SoundRegistry = sounds
  private readonly missingSoundKeys = new Set<string>()
  private readonly howlBySoundAndFile = new Map<string, Howl>()
  private readonly roundRobinStateBySoundKey = new Map<string, RoundRobinState>()

  play(soundKey: string): void {
    const normalizedKey = soundKey.trim()
    if (!normalizedKey) return

    console.log('[SoundManager] play requested', {
      soundKey: normalizedKey,
      howlerVolume: Howler.volume(),
      audioState: Howler.ctx?.state,
    })

    const soundDef = this.soundDefs[normalizedKey]
    if (!soundDef) {
      if (!this.missingSoundKeys.has(normalizedKey)) {
        this.missingSoundKeys.add(normalizedKey)
        console.warn(`[SoundManager] Sound key not found: ${normalizedKey}`)
      }
      return
    }

    const files = this.validFiles(soundDef)
    if (files.length === 0) {
      console.warn(`[SoundManager] No files for sound key: ${normalizedKey}`)
      return
    }

    const audioSettings = useAudioSettingsStore()
    if (!audioSettings.enabled) {
      return
    }

    const selected = this.nextRoundRobinFile(normalizedKey, files)
    const selectedFile = selected.file
    if (!selectedFile) return

    const howl = this.getHowl(normalizedKey, selectedFile)
    const volume = clamp01(audioSettings.masterVolume * audioSettings.sfxVolume * this.defVolume(soundDef))
    console.log('[SoundManager] play resolved', {
      soundKey: normalizedKey,
      selectedFile,
      selectedIndex: selected.fileIndex,
      selectedRound: selected.round,
      volume,
      enabled: audioSettings.enabled,
      masterVolume: audioSettings.masterVolume,
      sfxVolume: audioSettings.sfxVolume,
    })

    try {
      howl.volume(volume)
      if (howl.state() === 'unloaded') {
        howl.load()
      }
      const soundID = howl.play()
      console.log('[SoundManager] howl.play called', {
        soundKey: normalizedKey,
        soundID,
        howlState: howl.state(),
      })
    } catch (err: unknown) {
      console.warn(`[SoundManager] Failed to play sound: ${normalizedKey}`, err)
    }
  }

  private validFiles(soundDef: SoundDef): string[] {
    if (!Array.isArray(soundDef.files)) {
      return []
    }

    return soundDef.files
      .map((file) => file.trim())
      .filter((file) => file.length > 0)
  }

  private defVolume(soundDef: SoundDef): number {
    if (typeof soundDef.volume !== 'number' || Number.isNaN(soundDef.volume)) {
      return 1
    }
    return clamp01(soundDef.volume)
  }

  private resolveAssetPath(filePath: string): string {
    if (filePath.startsWith('/') || filePath.startsWith('http://') || filePath.startsWith('https://')) {
      return filePath
    }
    return `/assets/game/${filePath}`
  }

  private getHowl(soundKey: string, filePath: string): Howl {
    const cacheKey = `${soundKey}:${filePath}`
    const cached = this.howlBySoundAndFile.get(cacheKey)
    if (cached) {
      return cached
    }

    const howl = new Howl({
      src: [this.resolveAssetPath(filePath)],
      preload: true,
      html5: true,
      onload: () => {
        console.log('[SoundManager] howl loaded', { soundKey, filePath })
      },
      onplay: (soundID: number) => {
        console.log('[SoundManager] howl playing', { soundKey, filePath, soundID })
      },
      onloaderror: (_, error) => {
        console.warn('[SoundManager] howl load error', { soundKey, filePath, error })
      },
      onplayerror: (soundID, error) => {
        console.warn('[SoundManager] howl play error', { soundKey, filePath, soundID, error })
      },
      onend: (soundID) => {
        console.log('[SoundManager] howl ended', { soundKey, filePath, soundID })
      },
    })
    this.howlBySoundAndFile.set(cacheKey, howl)
    return howl
  }

  private nextRoundRobinFile(soundKey: string, files: string[]): { file: string; fileIndex: number; round: number } {
    let state = this.roundRobinStateBySoundKey.get(soundKey)
    if (!state || state.order.length !== files.length) {
      state = {
        round: 0,
        cursor: 0,
        order: this.buildDeterministicOrder(files.length, soundKey, 0),
      }
      this.roundRobinStateBySoundKey.set(soundKey, state)
    }

    if (state.cursor >= state.order.length) {
      state.round++
      state.cursor = 0
      state.order = this.buildDeterministicOrder(files.length, soundKey, state.round)
    }

    const fileIndex = state.order[state.cursor] ?? 0
    state.cursor++
    return {
      file: files[fileIndex] ?? '',
      fileIndex,
      round: state.round,
    }
  }

  private buildDeterministicOrder(length: number, soundKey: string, round: number): number[] {
    const order = Array.from({ length }, (_, index) => index)
    let seed = this.seedFromString(`${soundKey}:${round}`)

    for (let index = length - 1; index > 0; index--) {
      seed = this.nextPseudoRandom(seed)
      const swapIndex = seed % (index + 1)
      const current = order[index] ?? 0
      order[index] = order[swapIndex] ?? 0
      order[swapIndex] = current
    }

    return order
  }

  private seedFromString(input: string): number {
    let hash = 2166136261
    for (let index = 0; index < input.length; index++) {
      hash ^= input.charCodeAt(index)
      hash = Math.imul(hash, 16777619)
    }
    return hash >>> 0
  }

  private nextPseudoRandom(seed: number): number {
    let value = seed || 1
    value ^= value << 13
    value ^= value >>> 17
    value ^= value << 5
    return value >>> 0
  }
}

export const soundManager = new SoundManager()

interface RoundRobinState {
  round: number
  cursor: number
  order: number[]
}
