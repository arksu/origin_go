// Authored profiles are published in the immutable asset catalog.
export type { SoundProfile as SoundDef } from '../../types/soundDefs'
export type SoundRegistry = Readonly<Record<string, import('../../types/soundDefs').SoundProfile>>
