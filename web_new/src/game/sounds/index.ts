import actions from './actions.json'

export interface SoundDef {
  files: string[]
  volume?: number
}

export type SoundRegistry = Record<string, SoundDef>

const sounds: SoundRegistry = {
  ...actions,
}

export default sounds
